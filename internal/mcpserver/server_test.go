package mcpserver

import (
	"fmt"
	"slices"
	"strings"
	"testing"
)

var (
	readTools  = []string{"get_backlinks", "list_notes", "read_note", "search_notes"}
	writeTools = []string{"append_to_daily_note", "create_note", "edit_note"}
)

func (h *harness) toolNames() []string {
	h.t.Helper()
	res, err := h.cs.ListTools(h.t.Context(), nil)
	if err != nil {
		h.t.Fatal(err)
	}
	var names []string
	for _, tool := range res.Tools {
		names = append(names, tool.Name)
		if tool.Description == "" {
			h.t.Errorf("%s has no description", tool.Name)
		}
		readOnly := slices.Contains(readTools, tool.Name)
		if tool.Annotations == nil || tool.Annotations.ReadOnlyHint != readOnly {
			h.t.Errorf("%s's read-only hint is not %v", tool.Name, readOnly)
		}
		if readOnly && !strings.Contains(tool.Description, Notice) {
			h.t.Errorf("%s's description does not say that note text is data", tool.Name)
		}
	}
	slices.Sort(names)
	return names
}

func TestServerOffersEveryTool(t *testing.T) {
	h := newHarness(t, map[string]string{"Plan.md": "plan"}, false)
	if got, want := h.toolNames(), slices.Sorted(slices.Values(append(slices.Clone(readTools), writeTools...))); !slices.Equal(got, want) {
		t.Fatalf("tools = %v, want %v", got, want)
	}
	init := h.cs.InitializeResult()
	if init.ServerInfo.Name != ServerName || !strings.Contains(init.Instructions, Notice) {
		t.Errorf("server = %+v, instructions %q", init.ServerInfo, init.Instructions)
	}
}

// --read-only offers the read tools only, and a write tool cannot be
// called at all.
func TestReadOnlyServerRefusesEveryWrite(t *testing.T) {
	h := newHarness(t, map[string]string{"Plan.md": "plan"}, true)
	if got := h.toolNames(); !slices.Equal(got, readTools) {
		t.Fatalf("read-only tools = %v, want %v", got, readTools)
	}
	for _, name := range writeTools {
		args := map[string]any{"path": "New.md", "content": "x", "text": "x", "append": "x"}
		res, err := h.cs.CallTool(t.Context(), toolParams(name, args))
		if err == nil && !res.IsError {
			t.Errorf("a read-only server ran %s", name)
		}
	}
	if got := h.disk("Plan.md"); got != "plan" {
		t.Errorf("Plan.md = %q after a read-only session", got)
	}
	if !strings.Contains(h.cs.InitializeResult().Instructions, "read-only") {
		t.Error("a read-only server's instructions do not say so")
	}
}

func TestAuthorIsTheMCPClient(t *testing.T) {
	if got := author(nil); got != "mcp" {
		t.Errorf("author without a session = %q, want mcp", got)
	}
	source, name := agentWrite(nil)
	if fmt.Sprint(*source, " ", *name) != "agent mcp" {
		t.Errorf("agentWrite = %s %s", *source, *name)
	}
}
