package mcpserver

import (
	"encoding/json/v2"
	"os"
	"path/filepath"
	"strings"
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

// call calls a tool and returns its result. A protocol error fails the
// test: tools report failures as error results.
func (h *harness) call(name string, args map[string]any) *sdk.CallToolResult {
	h.t.Helper()
	res, err := h.cs.CallTool(h.t.Context(), &sdk.CallToolParams{Name: name, Arguments: args})
	if err != nil {
		h.t.Fatalf("%s: protocol error %v", name, err)
	}
	return res
}

// ok calls a tool that must succeed and decodes its structured result
// into out. It checks that the text content is the same JSON.
func (h *harness) ok(name string, args map[string]any, out any) {
	h.t.Helper()
	res := h.call(name, args)
	if res.IsError {
		h.t.Fatalf("%s(%v) failed: %s", name, args, resultText(h.t, res))
	}
	data, err := json.Marshal(res.StructuredContent)
	if err != nil {
		h.t.Fatal(err)
	}
	if err := json.Unmarshal(data, out); err != nil {
		h.t.Fatalf("%s: structured content %s: %v", name, data, err)
	}
	var fromText any
	if err := json.Unmarshal([]byte(resultText(h.t, res)), &fromText); err != nil {
		h.t.Fatalf("%s: the text content is not JSON: %v", name, err)
	}
}

// fails calls a tool that must fail with an error result, and returns its
// text.
func (h *harness) fails(name string, args map[string]any) string {
	h.t.Helper()
	res := h.call(name, args)
	text := resultText(h.t, res)
	if !res.IsError {
		h.t.Fatalf("%s(%v) succeeded with %s; want an error result", name, args, text)
	}
	return text
}

func resultText(t *testing.T, res *sdk.CallToolResult) string {
	t.Helper()
	var parts []string
	for _, c := range res.Content {
		text, ok := c.(*sdk.TextContent)
		if !ok {
			t.Fatalf("result content is %T, want text", c)
		}
		parts = append(parts, text.Text)
	}
	return strings.Join(parts, "\n")
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

// read returns a note through notesd, as a client other than the agent.
func (h *harness) read(p string) *api.Note {
	h.t.Helper()
	n, err := h.notes.Read(h.t.Context(), &api.NoteReadParams{VaultID: h.vault, Path: p})
	if err != nil {
		h.t.Fatalf("note.read(%s): %v", p, err)
	}
	return n
}

// newest returns a note's newest version.
func (h *harness) newest(p string) api.Version {
	h.t.Helper()
	vs, err := h.notes.Versions(h.t.Context(), &api.NoteVersionsParams{VaultID: h.vault, Path: p})
	if err != nil || len(vs) == 0 {
		h.t.Fatalf("note.versions(%s) = %v, %v", p, vs, err)
	}
	return vs[0]
}

func toolParams(name string, args map[string]any) *sdk.CallToolParams {
	return &sdk.CallToolParams{Name: name, Arguments: args}
}
