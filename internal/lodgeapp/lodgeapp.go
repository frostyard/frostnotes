// Package lodgeapp serves notesd as lodge's notes app
// (docs/specs/lodge-app.md, ADR-0026) over lodge/kit, which owns the app
// socket, its peer check and the Lodge-* headers: /health, /api as caller
// session, /content/blobs/ for lodge-content, /mcp for agents with lodge's
// tokens, and the hub's device routes under /device/.
package lodgeapp

import (
	"context"
	"log/slog"
	"net"
	"net/http"
	"strings"

	kit "github.com/frostyard/lodge/kit/lodgeapp"
	"github.com/frostyard/lodge/kit/mcpsession"

	"github.com/frostyard/frostnotes/api"
	"github.com/frostyard/frostnotes/internal/blobhttp"
	"github.com/frostyard/frostnotes/internal/blobs"
	"github.com/frostyard/frostnotes/internal/hub"
	"github.com/frostyard/frostnotes/internal/mcpserver"
	"github.com/frostyard/frostnotes/internal/rpcserver"
)

// ID is notes' app id: its manifest's, its socket's name and its URLs'
// segment.
const ID = "notes"

// Limits of a blob answer (docs/specs/lodge-app.md#limits).
const (
	MaxRange = 8 << 20
	MaxWhole = 64 << 20
)

// Config is what the app socket serves.
type Config struct {
	RPC    *rpcserver.Server
	Router *api.Router
	Blobs  *blobs.Store
	// Hub serves the device routes; nil answers them 404.
	Hub *hub.Hub
	// Version is notesd's, for /health.
	Version string
	Log     *slog.Logger
}

// Server returns the kit's server for notes' routes.
func Server(cfg Config) *kit.Server {
	s := &kit.Server{
		Health: func(context.Context) kit.Health {
			return kit.Health{OK: true, Version: cfg.Version, Protocol: api.Protocol}
		},
		API: func(ctx context.Context, nc net.Conn, c kit.Caller) {
			cfg.RPC.ServeConn(ctx, nc, cfg.Router, Identity(c))
		},
		Content: blobhttp.Handler(cfg.Blobs, blobhttp.Options{Prefix: "/content/blobs/",
			MaxRange: MaxRange, MaxWhole: MaxWhole, Download: true}),
		MCP:    MCP(cfg),
		Logger: cfg.Log,
	}
	if cfg.Hub != nil {
		s.Device = Device(cfg.Hub.Handler())
	}
	return s
}

// Serve listens on notes' app socket (LODGE_ROOT and LODGE_UID for
// development, else /run/lodge/apps/notes.sock, or the socket systemd
// passed) and serves it until ctx ends.
func Serve(ctx context.Context, cfg Config) error {
	env, err := kit.LoadEnv()
	if err != nil {
		return err
	}
	ln, err := env.Listen(ID)
	if err != nil {
		return err
	}
	if cfg.Log != nil {
		cfg.Log.Info("lodge app socket", "path", env.SocketPath(ID), "lodge uid", env.LodgeUID)
	}
	return Server(cfg).Serve(ctx, ln)
}

// MCP serves /mcp (docs/specs/mcp.md#through-lodge): the kit binds each
// MCP session to its token's name and scope, keeps 8 per token and ends
// idle ones, and each session's tools reach notesd over an in-process
// connection of caller agent bound to that scope, which the engine
// enforces.
func MCP(cfg Config) *mcpsession.Sessions {
	h := mcpserver.Handler(mcpserver.HTTPOptions{
		Caller: func(r *http.Request) (string, string, bool) {
			c, ok := kit.CallerFrom(r.Context())
			return c.Token, c.Scope, ok && c.Kind == kit.Agent
		},
		Open: func(ctx context.Context, token string, scope api.Scope) (*api.Client, error) {
			return Agent(ctx, cfg, token, scope)
		},
		Version: cfg.Version,
		Log:     cfg.Log,
	})
	return mcpsession.New(h, mcpsession.Options{
		CheckScope: func(s string) error {
			_, err := api.ParseScope(s)
			return err
		},
		Logger: cfg.Log,
	})
}

// Agent opens an in-process connection to notesd as caller agent, bound to
// the token's name and scope, and completes rpc.hello. It lives until the
// client is closed.
func Agent(ctx context.Context, cfg Config, token string, scope api.Scope) (*api.Client, error) {
	server, client := net.Pipe()
	id := api.Identity{Caller: api.CallerAgent, Token: token, Scope: &scope}
	go cfg.RPC.ServeConn(context.WithoutCancel(ctx), server, cfg.Router, id)
	c := api.NewClient(client)
	if _, err := c.RPC().Hello(ctx, &api.RPCHelloParams{Protocol: api.Protocol, Client: "mcp " + token}); err != nil {
		_ = c.Close()
		return nil, err
	}
	return c, nil
}

// Identity is the connection identity of an /api session: caller session,
// named by lodge's headers, whose writes are "<node> (web)".
func Identity(c kit.Caller) api.Identity {
	return api.Identity{Caller: api.CallerSession, Login: c.Login, DisplayName: c.DisplayName, Node: c.NodeName}
}

// Device serves the hub's endpoints (sync-protocol.md's paths) under
// /device/, as lodge forwards /device/notes/…: the device token still
// authenticates each request, and lodge's peer headers only name the
// node beside a pending pairing.
func Device(h http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		rest, ok := strings.CutPrefix(r.URL.Path, "/device")
		if !ok || (rest != "/sync" && !strings.HasPrefix(rest, "/sync/")) {
			http.NotFound(w, r)
			return
		}
		ctx := hub.WithPeer(r.Context(), peer(r.Context()))
		r2 := r.Clone(ctx)
		r2.URL.Path, r2.URL.RawPath = rest, ""
		h.ServeHTTP(w, r2)
	})
}

// peer is the node lodge named for a device request, with its login when
// lodge sent one: "phone (ana@example.com)", "phone", or "".
func peer(ctx context.Context) string {
	c, ok := kit.CallerFrom(ctx)
	if !ok || c.NodeName == "" {
		return ""
	}
	if c.Login != "" {
		return c.NodeName + " (" + c.Login + ")"
	}
	return c.NodeName
}
