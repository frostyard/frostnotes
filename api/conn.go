package api

import (
	"context"
	"fmt"
	"strconv"
	"strings"
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

type connKey struct{}

// WithConn returns ctx carrying c; the RPC server sets it for every request.
func WithConn(ctx context.Context, c Conn) context.Context {
	return context.WithValue(ctx, connKey{}, c)
}

// ConnFrom returns the connection a request arrived on, or nil.
func ConnFrom(ctx context.Context) Conn {
	c, _ := ctx.Value(connKey{}).(Conn)
	return c
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

// Scope is what an agent's token allows (Lodge-Scope, ADR-0026): one
// vault, read only unless Write.
type Scope struct {
	Vault int64
	Write bool
}

// ParseScope parses a scope, "vault=<id>" or "vault=<id>;write", where
// <id> is a vault's ID on this notesd. Anything else is an error.
func ParseScope(s string) (Scope, error) {
	rest, ok := strings.CutPrefix(s, "vault=")
	if !ok {
		return Scope{}, fmt.Errorf("scope %q does not start with vault=", s)
	}
	id, write := rest, false
	if before, after, found := strings.Cut(rest, ";"); found {
		if after != "write" {
			return Scope{}, fmt.Errorf("scope %q: only ;write may follow the vault", s)
		}
		id, write = before, true
	}
	n, err := strconv.ParseInt(id, 10, 64)
	if err != nil || n < 1 || strconv.FormatInt(n, 10) != id {
		return Scope{}, fmt.Errorf("scope %q: the vault is a positive decimal ID", s)
	}
	return Scope{Vault: n, Write: write}, nil
}

// String is the scope's Lodge-Scope form.
func (s Scope) String() string {
	if s.Write {
		return fmt.Sprintf("vault=%d;write", s.Vault)
	}
	return fmt.Sprintf("vault=%d", s.Vault)
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

// mayWrite reports whether ctx's scope, if any, lets it call a method
// marked writes.
func mayWrite(ctx context.Context) bool {
	sc, bound := scoped(ctx)
	return !bound || sc != nil && sc.Write
}

// inScope answers a call that names a vault outside ctx's scope as one
// naming a vault that does not exist, so a scoped caller cannot learn
// what other vaults there are.
func inScope(ctx context.Context, vaultID int64) error {
	if sc, bound := scoped(ctx); bound && (sc == nil || sc.Vault != vaultID) {
		return NotFound("vault %d does not exist", vaultID)
	}
	return nil
}
