package mcpserver

import (
	"context"
	"log/slog"
	"net/http"
	"sync"

	"github.com/frostyard/frostnotes/api"
	sdk "github.com/modelcontextprotocol/go-sdk/mcp"
)

// The tools behind lodge's /mcp/notes (docs/specs/mcp.md#through-lodge):
// MCP's Streamable HTTP transport, one server per MCP session, built from
// the session's token and scope, whose tools reach notesd over a
// connection of caller agent bound to that scope.

// HTTPOptions configure Handler.
type HTTPOptions struct {
	// Caller is the request's token name and scope, as lodge named them
	// (Lodge-Token, Lodge-Scope); ok is false for a request that has none.
	Caller func(r *http.Request) (token, scope string, ok bool)
	// Open connects a new session to notesd as caller agent, bound to the
	// token's name and scope, and returns a client that completed
	// rpc.hello. Handler closes it when the session ends.
	Open func(ctx context.Context, token string, scope api.Scope) (*api.Client, error)
	// Version is reported to the MCP host as the server's.
	Version string
	// Log receives sessions opened and ended; nil discards them.
	Log *slog.Logger
}

// sessionHeader is MCP Streamable HTTP's session header.
const sessionHeader = "Mcp-Session-Id"

// Handler serves MCP's Streamable HTTP transport for lodge's /mcp. Each
// initialize gets a server for its token's scope: the scope's vault, and
// under a read scope only the read tools. Sessions end when the client
// deletes them; binding them to their token, bounding them and ending idle
// ones is the caller's (lodge/kit's mcpsession, which ends a session with a
// DELETE).
func Handler(o HTTPOptions) http.Handler {
	if o.Log == nil {
		o.Log = slog.New(slog.DiscardHandler)
	}
	h := &httpSessions{o: o, clients: map[string]*api.Client{}}
	h.next = sdk.NewStreamableHTTPHandler(h.server, &sdk.StreamableHTTPOptions{Logger: o.Log})
	return h
}

type httpSessions struct {
	o    HTTPOptions
	next http.Handler

	mu      sync.Mutex
	clients map[string]*api.Client // by MCP session ID
}

// opening carries a new session's notesd client from server back to
// ServeHTTP, which keeps it under the session's ID.
type opening struct{ client *api.Client }

type openingKey struct{}

func (h *httpSessions) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	id := r.Header.Get(sessionHeader)
	if id != "" {
		h.next.ServeHTTP(w, r)
		if r.Method == http.MethodDelete {
			h.forget(id)
		}
		return
	}
	op := &opening{}
	rec := &sessionRecorder{ResponseWriter: w}
	h.next.ServeHTTP(rec, r.WithContext(context.WithValue(r.Context(), openingKey{}, op)))
	if op.client == nil {
		return
	}
	if rec.id == "" {
		_ = op.client.Close()
		return
	}
	h.mu.Lock()
	if old := h.clients[rec.id]; old != nil {
		_ = old.Close()
	}
	h.clients[rec.id] = op.client
	h.mu.Unlock()
}

// forget closes an ended session's notesd client.
func (h *httpSessions) forget(id string) {
	h.mu.Lock()
	c := h.clients[id]
	delete(h.clients, id)
	h.mu.Unlock()
	if c != nil {
		_ = c.Close()
	}
}

// server builds a new session's server from its request's token and
// scope; nil answers 400. A scope that does not parse never reaches here
// (mcpsession refuses it), but is refused again.
func (h *httpSessions) server(r *http.Request) *sdk.Server {
	op, _ := r.Context().Value(openingKey{}).(*opening)
	token, raw, ok := h.o.Caller(r)
	if op == nil || !ok || token == "" {
		return nil
	}
	scope, err := api.ParseScope(raw)
	if err != nil {
		h.o.Log.Warn("an MCP session with a scope that does not parse", "token", token, "err", err)
		return nil
	}
	c, err := h.o.Open(r.Context(), token, scope)
	if err != nil {
		h.o.Log.Error("connect an MCP session to notesd", "token", token, "err", err)
		return nil
	}
	op.client = c
	h.o.Log.Info("MCP session", "token", token, "scope", scope.String())
	return newServer(Options{Client: c, VaultID: scope.Vault, ReadOnly: !scope.Write, Version: h.o.Version,
		Author: token, Log: h.o.Log})
}

// sessionRecorder notes the session ID a response issues, keeping
// streams streaming.
type sessionRecorder struct {
	http.ResponseWriter
	id    string
	wrote bool
}

func (r *sessionRecorder) WriteHeader(code int) {
	if !r.wrote {
		r.wrote = true
		if code/100 == 2 {
			r.id = r.Header().Get(sessionHeader)
		}
	}
	r.ResponseWriter.WriteHeader(code)
}

func (r *sessionRecorder) Write(p []byte) (int, error) {
	if !r.wrote {
		r.WriteHeader(http.StatusOK)
	}
	return r.ResponseWriter.Write(p)
}

// Flush keeps streams streaming.
func (r *sessionRecorder) Flush() {
	if !r.wrote {
		r.WriteHeader(http.StatusOK)
	}
	if f, ok := r.ResponseWriter.(http.Flusher); ok {
		f.Flush()
	}
}

// Unwrap lets http.ResponseController reach the connection.
func (r *sessionRecorder) Unwrap() http.ResponseWriter { return r.ResponseWriter }
