package mcpserver

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/frostyard/frostnotes/api"
	"github.com/frostyard/frostnotes/internal/rpctest"
)

// A session ended while its initialize is still finishing closes its
// notesd connection. Once the response's header names the session, lodge
// kit's mcpsession has bound it and may evict it at once, with a DELETE
// that can reach the handler before the initialize's goroutine resumes;
// the client must already be kept under the session's ID by then.
func TestASessionDeletedAsItOpensClosesItsConnection(t *testing.T) {
	srv := rpctest.Start(t)
	var opened []*api.Client
	h := Handler(HTTPOptions{
		Caller: func(*http.Request) (string, string, bool) { return "claude", "vault=1", true },
		Open: func(ctx context.Context, token string, scope api.Scope) (*api.Client, error) {
			c := srv.DialAs(t, api.Identity{Caller: api.CallerAgent, Token: token, Scope: &scope})
			opened = append(opened, c)
			return c, nil
		},
	})
	hs := h.(*httpSessions)
	deleted := 0
	hs.served = func(id string) {
		// The header is out: the session is evicted before this request's
		// handler goes on.
		req := httptest.NewRequest(http.MethodDelete, "/", nil)
		req.Header.Set(sessionHeader, id)
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, req)
		deleted = rec.Code
	}
	req := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(
		`{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2025-06-18","capabilities":{},"clientInfo":{"name":"claude-code","version":"test"}}}`))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json, text/event-stream")
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Header().Get(sessionHeader) == "" || len(opened) != 1 || deleted/100 != 2 {
		t.Fatalf("initialize = %d with session %q, %d connections opened, the delete = %d",
			rec.Code, rec.Header().Get(sessionHeader), len(opened), deleted)
	}
	if opened[0].RPC().Ping(t.Context(), &api.RPCPingParams{}) == nil {
		t.Error("the deleted session's notesd connection is still open")
	}
	hs.mu.Lock()
	kept := len(hs.clients)
	hs.mu.Unlock()
	if kept != 0 {
		t.Errorf("%d clients kept after the session was deleted", kept)
	}
}
