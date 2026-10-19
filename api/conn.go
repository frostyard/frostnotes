package api

import (
	"context"

	"github.com/frostyard/lodge/kit/jsonrpc"
)

// Conn is the client connection a request arrived on, for services that
// keep per-connection state, such as views.
type Conn interface {
	// Notify sends a transient event to this connection only.
	Notify(Event) error
	// Done is closed when the connection ends.
	Done() <-chan struct{}
	// Client is the client name the connection gave rpc.hello, such as
	// "notesctl": the default author of the writes it makes.
	Client() string
}

// ConnFrom returns the connection a request arrived on, or nil.
func ConnFrom(ctx context.Context) Conn {
	if c := jsonrpc.ConnFrom(ctx); c != nil {
		return rpcConn{c}
	}
	return nil
}

// rpcConn is the server's connection as a Conn.
type rpcConn struct{ *jsonrpc.Conn }

// Notify sends ev as a transient event.
func (c rpcConn) Notify(ev Event) error {
	env, err := NewEnvelope(0, ev)
	if err != nil {
		return err
	}
	return c.Conn.Notify(EventMethod, env)
}

// Caller is the kind of client a connection serves; each domain in
// schema/rpc lists the kinds it admits, and a method may narrow its
// domain's (ADR-0008, ADR-0026).
type Caller string

const (
	// CallerLocal is a process of the same user on notesd's Unix socket
	// (the desktop app, notesctl, notesctl mcp), or the loopback
	// development gateway that stands in for the app's bridge.
	CallerLocal Caller = "local"
	// CallerSession is a browser lodge signed in, on the lodge app
	// socket's /api.
	CallerSession Caller = "session"
	// CallerDevice is a paired device's notesd syncing with a hub.
	CallerDevice Caller = "device"
	// CallerAgent is an MCP session behind lodge's /mcp, in process,
	// bound to its token's scope.
	CallerAgent Caller = "agent"
)

type callerKey struct{}

// WithCaller returns ctx carrying the caller kind; the RPC server sets it
// for every request from what the connection authenticated as.
func WithCaller(ctx context.Context, c Caller) context.Context {
	return context.WithValue(ctx, callerKey{}, c)
}

// CallerFrom returns the caller kind in ctx, or "" when none was set.
func CallerFrom(ctx context.Context) Caller {
	c, _ := ctx.Value(callerKey{}).(Caller)
	return c
}

// Identity is who a connection's listener authenticated, fixed for the
// connection's lifetime: its caller kind, the names lodge gave a session
// or an agent, and an agent's scope.
type Identity struct {
	Caller Caller
	// Login, DisplayName and Node name a session's browser: the tailnet
	// login, its owner's display name and the node's short host name, from
	// Lodge-Login, Lodge-Display-Name and Lodge-Node-Name.
	Login, DisplayName, Node string
	// Token is an agent's token name, from Lodge-Token.
	Token string
	// ContentKey is a session's key for content URLs, unpadded base64url,
	// which rpc.hello hands it.
	ContentKey string
	// Scope binds an agent's calls. An agent without one reaches no vault
	// and writes nothing.
	Scope *Scope
}

// Author is who a connection's writes are by, when the connection fixes
// it (ok): a session writes as source editor and "<node> (web)", an agent
// as source agent and its token's name. Other connections name their
// writes in the params, and ok is false.
func (id Identity) Author() (source VersionSource, name string, ok bool) {
	switch id.Caller {
	case CallerSession:
		return VersionSourceEditor, id.Node + " (web)", true
	case CallerAgent:
		return VersionSourceAgent, id.Token, true
	}
	return "", "", false
}

type identityKey struct{}

// WithIdentity returns ctx carrying id and its caller kind; the RPC server
// sets it for every request.
func WithIdentity(ctx context.Context, id Identity) context.Context {
	return context.WithValue(WithCaller(ctx, id.Caller), identityKey{}, id)
}

// IdentityFrom returns the identity in ctx. Without one it is the caller
// kind alone.
func IdentityFrom(ctx context.Context) Identity {
	id, ok := ctx.Value(identityKey{}).(Identity)
	if !ok {
		return Identity{Caller: CallerFrom(ctx)}
	}
	return id
}

// scoped returns the scope that binds ctx's calls, and whether one does:
// every agent is bound, by its scope or, without one, to nothing.
func scoped(ctx context.Context) (*Scope, bool) {
	id := IdentityFrom(ctx)
	return id.Scope, id.Scope != nil || id.Caller == CallerAgent
}

// mayWrite reports whether ctx's scope, if any, lets it call method,
// which is marked writes: a write right does, and daily does for the
// daily note's own method (ADR-0028). The engine then judges each path.
func mayWrite(ctx context.Context, method string) bool {
	sc, bound := scoped(ctx)
	switch {
	case !bound:
		return true
	case sc == nil:
		return false
	}
	return sc.CanWrite() || sc.Daily && dailyMethod(method)
}

// dailyMethod reports whether method is one daily admits.
func dailyMethod(method string) bool { return method == "note.daily" }

// inScope answers a call that names a vault outside ctx's scope as one
// naming a vault that does not exist, so a scoped caller cannot learn
// what other vaults there are.
func inScope(ctx context.Context, vaultID int64) error {
	if sc, bound := scoped(ctx); bound && (sc == nil || sc.Vault != vaultID) {
		return NotFound("vault %d does not exist", vaultID)
	}
	return nil
}
