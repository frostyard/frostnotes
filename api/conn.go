package api

import "context"

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
// schema/rpc lists the kinds it admits (ADR-0008).
type Caller string

const (
	// CallerLocal is a process of the same user on notesd's Unix socket
	// (the desktop app, notesctl, notesctl mcp), or the loopback
	// development gateway that stands in for the app's bridge.
	CallerLocal Caller = "local"
	// CallerSession is a signed-in browser on a hub.
	CallerSession Caller = "session"
	// CallerDevice is a paired device's notesd syncing with a hub.
	CallerDevice Caller = "device"
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
