package mcpserver

import (
	"context"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"

	"github.com/frostyard/frostnotes/api"
	"github.com/frostyard/frostnotes/internal/rpctest"
	sdk "github.com/modelcontextprotocol/go-sdk/mcp"
)

// Each MCP session holds exactly one notesd connection, closed when the
// session ends: the SDK asks for a new session's server more than once,
// and a stray POST without a session must not leave a connection behind
// (the security review's finding 1).
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
	// A request without a session that opens none (the SDK's client sends
	// one before initialize) closes the connection it took at once.
	if n := live(); n != 0 {
		t.Errorf("%d notesd connections outlived every session", n)
	}
}
