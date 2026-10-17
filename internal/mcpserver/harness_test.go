package mcpserver

import (
	"encoding/json/v2"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/frostyard/frostnotes/api"
	"github.com/frostyard/frostnotes/internal/rpctest"
	"github.com/frostyard/frostnotes/internal/vault"
	sdk "github.com/modelcontextprotocol/go-sdk/mcp"
)

// The test harness every tool test uses: a real notesd (rpctest) with one
// vault of files in a temporary folder, a server for that vault, and an
// MCP client session connected to it over in-memory transports. No port,
// no subprocess; everything stops when the test ends.

// clientName is the MCP client's name, which writes record as their author.
const clientName = "harness-agent"

type harness struct {
	t     *testing.T
	srv   *rpctest.Server
	notes api.NoteClient // notesd's note domain, for checking what tools did
	vault int64
	dir   string
	cs    *sdk.ClientSession
}

// newHarness writes files (path → content) into a new vault, waits for its
// first scan, and connects a client to a server for it.
func newHarness(t *testing.T, files map[string]string, readOnly bool) *harness {
	t.Helper()
	dir := t.TempDir()
	for p, content := range files {
		full := filepath.Join(dir, filepath.FromSlash(p))
		if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(full, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	srv := rpctest.StartWith(t, rpctest.Options{Vault: vault.Config{Debounce: 20 * time.Millisecond}})
	ctx := t.Context()
	// One connection waits for the scan; the server gets its own, which
	// takes no events, as notesctl mcp's does.
	waiter := srv.Dial(t)
	if _, err := waiter.Events().Subscribe(ctx, &api.EventsSubscribeParams{}); err != nil {
		t.Fatal(err)
	}
	v, err := waiter.Vault().Add(ctx, &api.VaultAddParams{Path: dir})
	if err != nil {
		t.Fatal(err)
	}
	waitScan(t, waiter, v.ID)
	_ = waiter.Close()

	c := srv.Dial(t)
	server := newServer(Options{Client: c, VaultID: v.ID, ReadOnly: readOnly, Version: "test"})
	ct, st := sdk.NewInMemoryTransports()
	ss, err := server.Connect(ctx, st, nil)
	if err != nil {
		t.Fatal(err)
	}
	cs, err := sdk.NewClient(&sdk.Implementation{Name: clientName, Version: "1.0"}, nil).Connect(ctx, ct, nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_ = cs.Close()
		_ = ss.Wait()
	})
	return &harness{t: t, srv: srv, notes: c.Note(), vault: v.ID, dir: dir, cs: cs}
}

func waitScan(t *testing.T, c *api.Client, id int64) {
	t.Helper()
	timeout := time.After(time.Minute)
	for {
		select {
		case env := <-c.Notifications():
			var ev api.VaultScan
			if env.Event == "vault.scan" && json.Unmarshal(env.Data, &ev) == nil && ev.Status.VaultID == id &&
				ev.Status.State != api.ScanStateScanning {
				if ev.Status.State == api.ScanStateFailed {
					t.Fatalf("scan failed: %s", *ev.Status.Error)
				}
				return
			}
		case <-timeout:
			t.Fatal("no finished scan within a minute")
		}
	}
}

// disk returns a vault file's content.
func (h *harness) disk(p string) string {
	h.t.Helper()
	data, err := os.ReadFile(filepath.Join(h.dir, filepath.FromSlash(p)))
	if err != nil {
		h.t.Fatal(err)
	}
	return string(data)
}

func toolParams(name string, args map[string]any) *sdk.CallToolParams {
	return &sdk.CallToolParams{Name: name, Arguments: args}
}
