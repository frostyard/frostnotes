package mcpserver_test

import (
	"context"
	"errors"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"testing"

	"github.com/frostyard/lodge/kit/kittest"
	kit "github.com/frostyard/lodge/kit/lodgeapp"
	sdk "github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/frostyard/frostnotes/api"
	"github.com/frostyard/frostnotes/internal/lodgeapp"
	"github.com/frostyard/frostnotes/internal/rpctest"
)

// Agents through lodge (docs/specs/mcp.md#through-lodge): the SDK's own
// HTTP client, over notesd's lodge app socket, with the headers lodge sets.

type lodgeNotes struct {
	srv          *rpctest.Server
	lodge        *kittest.Lodge
	local        *api.Client
	vault, other int64
}

func startLodge(t *testing.T) *lodgeNotes {
	t.Helper()
	srv := rpctest.Start(t)
	local := srv.Dial(t)
	add := func(files map[string]string) int64 {
		dir := t.TempDir()
		for p, c := range files {
			if err := os.WriteFile(filepath.Join(dir, p), []byte(c), 0o644); err != nil {
				t.Fatal(err)
			}
		}
		v, err := local.Vault().Add(t.Context(), &api.VaultAddParams{Path: dir})
		if err != nil {
			t.Fatal(err)
		}
		for p := range files {
			if _, err := local.Note().Read(t.Context(), &api.NoteReadParams{VaultID: v.ID, Path: p}); err != nil {
				t.Fatal(err)
			}
		}
		return v.ID
	}
	n := &lodgeNotes{srv: srv, local: local}
	n.vault = add(map[string]string{"Plan.md": "# Plan\n\nPlant the beds.\n"})
	n.other = add(map[string]string{"Secret.md": "secret\n"})
	n.lodge = kittest.Start(t, lodgeapp.ID, lodgeapp.Server(lodgeapp.Config{RPC: srv.RPC, Router: srv.Router,
		Blobs: srv.Blobs, Version: "test"}))
	return n
}

// asLodge sends every request to the socket with c's Lodge-* headers.
type asLodge struct {
	sock string
	c    kit.Caller
	base http.RoundTripper
}

func (a *asLodge) RoundTrip(r *http.Request) (*http.Response, error) {
	r = r.Clone(r.Context())
	a.c.SetHeaders(r.Header)
	return a.base.RoundTrip(r)
}

func (n *lodgeNotes) httpClient(c kit.Caller) *http.Client {
	sock := n.lodge.Socket
	tr := &http.Transport{DialContext: func(ctx context.Context, _, _ string) (net.Conn, error) {
		var d net.Dialer
		return d.DialContext(ctx, "unix", sock)
	}}
	return &http.Client{Transport: &asLodge{sock: sock, c: c, base: tr}}
}

// connect opens an MCP session as token with scope.
func (n *lodgeNotes) connect(t *testing.T, token, scope string) *sdk.ClientSession {
	t.Helper()
	cs, err := n.dial(t, token, scope)
	if err != nil {
		t.Fatal(err)
	}
	return cs
}

func (n *lodgeNotes) dial(t *testing.T, token, scope string) (*sdk.ClientSession, error) {
	t.Helper()
	client := sdk.NewClient(&sdk.Implementation{Name: "claude-code", Version: "test"}, nil)
	cs, err := client.Connect(t.Context(), &sdk.StreamableClientTransport{Endpoint: "http://lodge/mcp",
		HTTPClient: n.httpClient(kittest.Agent(token, scope))}, nil)
	if err == nil {
		t.Cleanup(func() { _ = cs.Close() })
	}
	return cs, err
}

func scope(id int64, write bool) string { return api.Scope{Vault: id, Write: write}.String() }

func toolNames(t *testing.T, cs *sdk.ClientSession) []string {
	t.Helper()
	res, err := cs.ListTools(t.Context(), nil)
	if err != nil {
		t.Fatal(err)
	}
	var names []string
	for _, tool := range res.Tools {
		names = append(names, tool.Name)
	}
	slices.Sort(names)
	return names
}

func call(t *testing.T, cs *sdk.ClientSession, tool string, args map[string]any) (*sdk.CallToolResult, string) {
	t.Helper()
	res, err := cs.CallTool(t.Context(), &sdk.CallToolParams{Name: tool, Arguments: args})
	if err != nil {
		t.Fatal(err)
	}
	var text strings.Builder
	for _, c := range res.Content {
		if tc, ok := c.(*sdk.TextContent); ok {
			text.WriteString(tc.Text)
		}
	}
	return res, text.String()
}

// A read scope's session lists the four read tools and reads its vault.
func TestAReadScopeServesTheReadTools(t *testing.T) {
	n := startLodge(t)
	cs := n.connect(t, "claude-laptop", scope(n.vault, false))
	if got := toolNames(t, cs); !slices.Equal(got, []string{"get_backlinks", "list_notes", "read_note", "search_notes"}) {
		t.Fatalf("tools = %v, want the four read tools", got)
	}
	if res, text := call(t, cs, "read_note", map[string]any{"path": "Plan.md"}); res.IsError || !strings.Contains(text, "Plant the beds.") {
		t.Fatalf("read_note = %v %s", res.IsError, text)
	}
	// A write tool it does not list is no tool at all.
	if res, err := cs.CallTool(t.Context(), &sdk.CallToolParams{Name: "create_note",
		Arguments: map[string]any{"path": "New.md", "content": "x"}}); err == nil && !res.IsError {
		t.Fatal("create_note under a read scope succeeded")
	}
	if _, err := n.local.Note().Read(t.Context(), &api.NoteReadParams{VaultID: n.vault, Path: "New.md"}); !errors.Is(err, api.ErrNotFound) {
		t.Fatalf("New.md after a read scope's create = %v", err)
	}
}

// A write scope's writes name the token, not the MCP client.
func TestAWriteScopeWritesAsItsToken(t *testing.T) {
	n := startLodge(t)
	cs := n.connect(t, "claude-laptop", scope(n.vault, true))
	if got := toolNames(t, cs); len(got) != 7 {
		t.Fatalf("tools = %v, want seven", got)
	}
	if res, text := call(t, cs, "edit_note", map[string]any{"path": "Plan.md", "append": "Water them.\n"}); res.IsError {
		t.Fatalf("edit_note: %s", text)
	}
	vs, err := n.local.Note().Versions(t.Context(), &api.NoteVersionsParams{VaultID: n.vault, Path: "Plan.md"})
	if err != nil || vs[0].Source != api.VersionSourceAgent || vs[0].Author != "claude-laptop" {
		t.Fatalf("the edit's version = %+v, %v; want agent claude-laptop", vs[0], err)
	}
}

// The scope binds the session's connection in the engine: a write sent
// down an agent connection under a read scope is methodNotFound, and
// another vault notFound, whatever a tool might send.
func TestTheEngineHoldsTheAgentToItsScope(t *testing.T) {
	n := startLodge(t)
	cfg := lodgeapp.Config{RPC: n.srv.RPC, Router: n.srv.Router}
	read, err := lodgeapp.Agent(t.Context(), cfg, "claude-laptop", api.Scope{Vault: n.vault})
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = read.Close() }()
	if _, err := read.Note().Create(t.Context(), &api.NoteCreateParams{VaultID: n.vault, Path: "New.md", Content: "x"}); !errors.Is(err, api.ErrMethodNotFound) {
		t.Errorf("a write under a read scope = %v, want methodNotFound", err)
	}
	if _, err := read.Note().Read(t.Context(), &api.NoteReadParams{VaultID: n.other, Path: "Secret.md"}); !errors.Is(err, api.ErrNotFound) {
		t.Errorf("a read of another vault = %v, want notFound", err)
	}
	if _, err := read.Vault().List(t.Context(), &api.VaultListParams{}); !errors.Is(err, api.ErrMethodNotFound) {
		t.Errorf("vault.list as an agent = %v, want methodNotFound", err)
	}
	// Through MCP, another vault is out of reach: the session serves its own.
	cs := n.connect(t, "claude-laptop", scope(n.other+100, true))
	if res, text := call(t, cs, "read_note", map[string]any{"path": "Secret.md"}); !res.IsError || !strings.Contains(text, "notFound") {
		t.Errorf("a session for a vault that does not exist read %v %s", res.IsError, text)
	}
}

// A folder scope holds through lodge: the engine refuses what it does
// not allow, in any spelling, and says what the token may write.
func TestAFolderScopeHoldsThroughLodge(t *testing.T) {
	n := startLodge(t)
	cs := n.connect(t, "porter", "vault="+strconv.FormatInt(n.vault, 10)+";write=Porter/;except=Porter/Routines/;daily")
	if got := toolNames(t, cs); len(got) != 7 {
		t.Fatalf("tools = %v, want seven", got)
	}
	if res, text := call(t, cs, "create_note", map[string]any{"path": "Porter/Memory.md", "content": "memory\n"}); res.IsError {
		t.Fatalf("create_note in Porter/: %s", text)
	}
	for _, p := range []string{"Porter/Routines/Brief.md", "porter/ROUTINES/Brief.md", "Elsewhere.md"} {
		res, text := call(t, cs, "create_note", map[string]any{"path": p, "content": "x\n"})
		if !res.IsError || !strings.Contains(text, "outside this token's writes") || !strings.Contains(text, "under Porter/ except Porter/Routines/") {
			t.Errorf("create_note %s = %v %s", p, res.IsError, text)
		}
	}
	if res, text := call(t, cs, "edit_note", map[string]any{"path": "Plan.md", "append": "x\n"}); !res.IsError || !strings.Contains(text, "outside this token's writes") {
		t.Errorf("edit_note Plan.md = %v %s", res.IsError, text)
	}
	if _, err := n.local.Note().Read(t.Context(), &api.NoteReadParams{VaultID: n.vault, Path: "Porter/Routines/Brief.md"}); !errors.Is(err, api.ErrNotFound) {
		t.Errorf("Porter/Routines/Brief.md after the refusals = %v", err)
	}
}

// A session belongs to its token and scope: another token's request for
// it is 404, and a scope that does not parse is 403.
func TestASessionBelongsToItsToken(t *testing.T) {
	n := startLodge(t)
	cs := n.connect(t, "claude-laptop", scope(n.vault, false))
	id := cs.ID()
	if id == "" {
		t.Fatal("no session ID")
	}
	for _, c := range []kit.Caller{kittest.Agent("other", scope(n.vault, false)), kittest.Agent("claude-laptop", scope(n.vault, true))} {
		req, err := http.NewRequestWithContext(t.Context(), http.MethodPost, "http://lodge/mcp",
			strings.NewReader(`{"jsonrpc":"2.0","id":9,"method":"tools/list"}`))
		if err != nil {
			t.Fatal(err)
		}
		req.Header.Set("Mcp-Session-Id", id)
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Accept", "application/json, text/event-stream")
		resp, err := n.httpClient(c).Do(req)
		if err != nil {
			t.Fatal(err)
		}
		_ = resp.Body.Close()
		if resp.StatusCode != http.StatusNotFound {
			t.Errorf("session %s as %s %s = %d, want 404", id, c.Token, c.Scope, resp.StatusCode)
		}
	}
	for _, bad := range []string{"vault=x", "all", "vault=1;admin", "", "vault=1;except=A/", "vault=1;daily;write",
		"vault=1;write=A", "vault=1;write=Résumés/", "vault=1;write=../"} {
		if _, err := n.dial(t, "claude-laptop", bad); err == nil {
			t.Errorf("a session with scope %q opened", bad)
		}
	}
	// The session still works for its own token.
	if got := toolNames(t, cs); len(got) != 4 {
		t.Errorf("tools after the refusals = %v", got)
	}
}

// A token keeps eight sessions: a ninth evicts the least recently used.
func TestANinthSessionEvictsTheLeastRecentlyUsed(t *testing.T) {
	n := startLodge(t)
	var sessions []*sdk.ClientSession
	for i := range 8 {
		cs := n.connect(t, "claude-laptop", scope(n.vault, false))
		if got := toolNames(t, cs); len(got) != 4 {
			t.Fatalf("session %d: tools = %v", i, got)
		}
		sessions = append(sessions, cs)
	}
	// Use all but the first again, so it is the least recently used.
	for _, cs := range sessions[1:] {
		toolNames(t, cs)
	}
	ninth := n.connect(t, "claude-laptop", scope(n.vault, false))
	toolNames(t, ninth)
	if _, err := sessions[0].ListTools(t.Context(), nil); err == nil {
		t.Error("the least recently used session still answers after a ninth opened")
	}
	for i, cs := range sessions[1:] {
		if _, err := cs.ListTools(t.Context(), nil); err != nil {
			t.Errorf("session %s: %v", strconv.Itoa(i+1), err)
		}
	}
	// Another token's sessions are its own.
	other := n.connect(t, "other", scope(n.vault, false))
	toolNames(t, other)
	if _, err := sessions[1].ListTools(t.Context(), nil); err != nil {
		t.Errorf("another token's session evicted one of claude-laptop's: %v", err)
	}
}
