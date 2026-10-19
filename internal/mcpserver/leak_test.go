package mcpserver

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"github.com/frostyard/frostnotes/api"
	"github.com/frostyard/frostnotes/internal/rpctest"
	sdk "github.com/modelcontextprotocol/go-sdk/mcp"
)

// Each MCP session holds exactly one notesd connection, opened by its
// initialize and closed when the session ends: the SDK asks for a new
// session's server more than once, and its client sends server/discover
// before initialize, neither of which may open another (the security
// reviews' finding on MCP sessions).
func TestASessionHoldsOneConnectionUntilItEnds(t *testing.T) {
	srv := rpctest.Start(t)
	var mu sync.Mutex
	var opened []*api.Client
	h := Handler(HTTPOptions{
		Caller: func(*http.Request) (string, string, bool) { return "claude", "vault=1", true },
		Open: func(ctx context.Context, token string, scope api.Scope) (*api.Client, error) {
			c := srv.DialAs(t, api.Identity{Caller: api.CallerAgent, Token: token, Scope: &scope})
			mu.Lock()
			opened = append(opened, c)
			mu.Unlock()
			return c, nil
		},
	})
	hs := httptest.NewServer(h)
	t.Cleanup(hs.Close)
	live := func() int {
		mu.Lock()
		defer mu.Unlock()
		n := 0
		for _, c := range opened {
			if c.RPC().Ping(t.Context(), &api.RPCPingParams{}) == nil {
				n++
			}
		}
		return n
	}
	for range 3 {
		client := sdk.NewClient(&sdk.Implementation{Name: "claude-code", Version: "test"}, nil)
		cs, err := client.Connect(t.Context(), &sdk.StreamableClientTransport{Endpoint: hs.URL}, nil)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := cs.ListTools(t.Context(), nil); err != nil {
			t.Fatal(err)
		}
		if n := live(); n != 1 {
			t.Fatalf("an open session holds %d notesd connections, want 1", n)
		}
		if err := cs.Close(); err != nil {
			t.Fatal(err)
		}
		if n := live(); n != 0 {
			t.Fatalf("after its DELETE, %d notesd connections are still open", n)
		}
	}
	mu.Lock()
	n := len(opened)
	mu.Unlock()
	if n != 3 {
		t.Errorf("three initializes opened %d notesd connections, want 3", n)
	}
	// A stray request without a session, initialize or not, opens nothing
	// that outlives it.
	for _, body := range []string{`{"jsonrpc":"2.0","id":1,"method":"tools/list"}`, `{"jsonrpc":"2.0","id":1,"method":"server/discover","params":{}}`, `not json`} {
		req, err := http.NewRequestWithContext(t.Context(), http.MethodPost, hs.URL, strings.NewReader(body))
		if err != nil {
			t.Fatal(err)
		}
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Accept", "application/json, text/event-stream")
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		_ = resp.Body.Close()
	}
	mu.Lock()
	n = len(opened)
	mu.Unlock()
	if n != 3 || live() != 0 {
		t.Errorf("stray requests opened %d notesd connections (%d live), want none", n-3, live())
	}
}
