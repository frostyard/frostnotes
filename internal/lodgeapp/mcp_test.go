package lodgeapp_test

import (
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/frostyard/lodge/kit/kittest"
	"github.com/frostyard/lodge/kit/mcpsession"

	"github.com/frostyard/frostnotes/api"
	"github.com/frostyard/frostnotes/internal/lodgeapp"
	"github.com/frostyard/frostnotes/internal/rpctest"
)

// An MCP session that ends without its client's DELETE, evicted past its
// token's cap or swept once idle, closes the notesd connection it opened
// (the platform review's N7: lodgeapp.Server's sessions end so, and Serve
// closes their sweep).
func TestAnEvictedOrIdleMCPSessionClosesItsConnection(t *testing.T) {
	srv := rpctest.Start(t)
	local := srv.Dial(t)
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "Plan.md"), []byte("plan\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	v, err := local.Vault().Add(t.Context(), &api.VaultAddParams{Path: dir})
	if err != nil {
		t.Fatal(err)
	}
	var mu sync.Mutex
	var opened []*api.Client
	var now atomic.Int64 // the sessions' clock, in Unix nanoseconds
	now.Store(time.Date(2026, 10, 19, 12, 0, 0, 0, time.UTC).UnixNano())
	const idle = time.Hour
	s := lodgeapp.Server(lodgeapp.Config{RPC: srv.RPC, Router: srv.Router, Blobs: srv.Blobs, Version: "test",
		Sessions: mcpsession.Options{Max: 2, Idle: idle, Every: 10 * time.Millisecond,
			Now: func() time.Time { return time.Unix(0, now.Load()) }},
		Opened: func(c *api.Client) {
			mu.Lock()
			defer mu.Unlock()
			opened = append(opened, c)
		}})
	if m, ok := s.MCP.(*mcpsession.Sessions); ok {
		t.Cleanup(func() { _ = m.Close() })
	}
	l := kittest.Start(t, lodgeapp.ID, s)
	agent := kittest.Agent("claude", api.Scope{Vault: v.ID}.String())

	// initialize, without the client's stream: the session is idle once answered.
	initialize := func() {
		t.Helper()
		body := `{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2025-06-18","capabilities":{},"clientInfo":{"name":"t","version":"1"}}}`
		req, err := http.NewRequestWithContext(t.Context(), http.MethodPost, "/mcp", strings.NewReader(body))
		if err != nil {
			t.Fatal(err)
		}
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Accept", "application/json, text/event-stream")
		resp, err := l.Do(req, agent)
		if err != nil {
			t.Fatal(err)
		}
		_ = resp.Body.Close()
		if resp.StatusCode != http.StatusOK || resp.Header.Get("Mcp-Session-Id") == "" {
			t.Fatalf("initialize = %s, session %q", resp.Status, resp.Header.Get("Mcp-Session-Id"))
		}
	}
	conn := func(i int) *api.Client {
		t.Helper()
		mu.Lock()
		defer mu.Unlock()
		if i >= len(opened) {
			t.Fatalf("%d notesd connections opened, want at least %d", len(opened), i+1)
		}
		return opened[i]
	}
	closed := func(c *api.Client, within time.Duration) bool {
		select {
		case <-c.Done():
			return true
		case <-time.After(within):
			return false
		}
	}

	initialize()
	initialize()
	// A third session on the token evicts the least recently used: the first.
	initialize()
	if !closed(conn(0), 10*time.Second) {
		t.Fatal("the evicted session's notesd connection is still open")
	}
	for i := 1; i <= 2; i++ {
		if closed(conn(i), 0) {
			t.Fatalf("live session %d's notesd connection closed", i+1)
		}
	}
	// An hour later the sweep ends the two idle sessions.
	now.Add(int64(idle + time.Minute))
	for i := 1; i <= 2; i++ {
		if !closed(conn(i), 10*time.Second) {
			t.Fatalf("idle session %d's notesd connection is still open", i+1)
		}
	}
}
