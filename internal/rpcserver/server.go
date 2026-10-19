// Package rpcserver serves the notesd API on lodge's kit
// (github.com/frostyard/lodge/kit/jsonrpc): rpc.hello, events.subscribe and
// a caller identity per connection. The kit does the framing, the gating
// on rpc.hello, concurrent dispatch, panics and oversized replies.
package rpcserver

import (
	"context"
	"errors"
	"log/slog"
	"net"
	"sync"

	"github.com/frostyard/lodge/kit/jsonrpc"

	"github.com/frostyard/frostnotes/api"
	"github.com/frostyard/frostnotes/internal/events"
)

// Options configures a Server.
type Options struct {
	// Name is reported by rpc.hello, e.g. "notesd v0.1.0".
	Name   string
	Broker *events.Broker
	Logger *slog.Logger
}

// Server implements api.RPCService and api.EventsService, which need the
// connection, and serves a Router built with them.
type Server struct {
	opts Options
	rpc  *jsonrpc.Server[api.ErrorCode]
}

// New returns a Server.
func New(opts Options) *Server {
	if opts.Logger == nil {
		opts.Logger = slog.New(slog.DiscardHandler)
	}
	return &Server{opts: opts, rpc: jsonrpc.NewServer[api.ErrorCode](jsonrpc.ServerOptions{
		Hello: "rpc.hello", Logger: opts.Logger})}
}

var (
	_ api.RPCService    = (*Server)(nil)
	_ api.EventsService = (*Server)(nil)
)

// Listen creates notesd's private socket (jsonrpc.ListenWith).
func (s *Server) Listen(path string) (net.Listener, error) {
	return jsonrpc.ListenWith(path, jsonrpc.ListenOptions{Name: "notesd", Logger: s.opts.Logger})
}

// Serve accepts connections on notesd's socket until ctx ends, then waits
// for them to finish. Each serves api.CallerLocal.
func (s *Server) Serve(ctx context.Context, ln net.Listener, r *api.Router) error {
	return s.rpc.Serve(ctx, ln, r, func(ctx context.Context) context.Context {
		return withState(ctx, api.Identity{Caller: api.CallerLocal})
	})
}

// ServeConn serves one connection that its listener has already
// authenticated as id (the development gateway, and lodge's sessions and
// agents) until it ends or ctx does.
func (s *Server) ServeConn(ctx context.Context, nc net.Conn, r *api.Router, id api.Identity) {
	s.rpc.ServeConn(withState(ctx, id), nc, r)
}

// state is a connection's own: whether it has subscribed.
type state struct {
	mu         sync.Mutex
	subscribed bool
}

type stateKey struct{}

func withState(ctx context.Context, id api.Identity) context.Context {
	return context.WithValue(api.WithIdentity(ctx, id), stateKey{}, &state{})
}

// Hello implements rpc.hello.
func (s *Server) Hello(ctx context.Context, p *api.RPCHelloParams) (*api.Hello, error) {
	if p.Protocol != api.Protocol {
		return nil, api.ProtocolMismatch("notesd speaks protocol %d, not %d", api.Protocol, p.Protocol)
	}
	id := api.IdentityFrom(ctx)
	if c := jsonrpc.ConnFrom(ctx); c != nil {
		c.SetReady(p.Client)
		s.opts.Logger.Debug("client connected", "client", p.Client, "caller", id.Caller)
	}
	h := &api.Hello{Protocol: api.Protocol, Server: s.opts.Name, Caller: string(id.Caller)}
	if id.Caller == api.CallerSession {
		h.Login, h.DisplayName, h.Node = optional(id.Login), optional(id.DisplayName), optional(id.Node)
		h.ContentKey = optional(id.ContentKey)
	}
	return h, nil
}

func optional(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}

// Ping implements rpc.ping.
func (s *Server) Ping(context.Context, *api.RPCPingParams) error { return nil }

// Subscribe implements events.subscribe; delivery starts after the reply.
func (s *Server) Subscribe(ctx context.Context, p *api.EventsSubscribeParams) (*api.Subscription, error) {
	c := jsonrpc.ConnFrom(ctx)
	st, _ := ctx.Value(stateKey{}).(*state)
	if c == nil || st == nil || s.opts.Broker == nil {
		return nil, api.Unavailable("events are not available on this connection")
	}
	st.mu.Lock()
	defer st.mu.Unlock()
	if st.subscribed {
		return nil, api.Conflict("this connection is already subscribed")
	}
	sub, info, err := s.opts.Broker.Subscribe(ctx, p.SinceSeq)
	if err != nil {
		return nil, err
	}
	st.subscribed = true
	c.OnClose(sub.Close)
	caller := api.IdentityFrom(ctx).Caller
	jsonrpc.AfterReply(ctx, func() {
		c.Go(func(ctx context.Context) {
			err := sub.Run(ctx, func(ev api.EventEnvelope) error {
				if !api.EventAdmits(ev.Event, caller) {
					return nil // another caller's event, as its domain says
				}
				return c.Notify(api.EventMethod, ev)
			})
			if errors.Is(err, events.ErrLagged) {
				s.opts.Logger.Warn("dropping lagging event subscriber")
				_ = c.Close() // the client resumes with sinceSeq
			}
		})
	})
	return &info, nil
}
