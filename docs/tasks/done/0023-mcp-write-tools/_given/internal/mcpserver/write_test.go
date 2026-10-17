package mcpserver

// CONTRACT TEST for task card T-0023 (docs/tasks). Do not edit.

import (
	"strings"
	"testing"
	"time"

	"github.com/frostyard/frostnotes/api"
)

// versions returns a note's versions, newest first.
func (h *harness) versions(p string) []api.Version {
	h.t.Helper()
	vs, err := h.notes.Versions(h.t.Context(), &api.NoteVersionsParams{VaultID: h.vault, Path: p})
	if err != nil || len(vs) == 0 {
		h.t.Fatalf("note.versions(%s) = %v, %v", p, vs, err)
	}
	return vs
}

// byAgent fails unless p's newest version is the MCP client's.
func (h *harness) byAgent(p string) {
	h.t.Helper()
	if v := h.versions(p)[0]; v.Source != api.VersionSourceAgent || v.Author != clientName {
		h.t.Errorf("%s's newest version is by %s %q, want agent %q", p, v.Source, v.Author, clientName)
	}
}

func TestCreateNote(t *testing.T) {
	h := newHarness(t, map[string]string{"Plan.md": "plan"}, false)
	var out createOut
	h.ok("create_note", map[string]any{"path": "Ideas/New Idea.md", "content": "# New idea\n"}, &out)
	if out.Path != "Ideas/New Idea.md" || h.disk(out.Path) != "# New idea\n" || out.Hash != h.notesRead(out.Path).Hash {
		t.Fatalf("create_note = %+v; the file holds %q", out, h.disk(out.Path))
	}
	h.byAgent(out.Path)
	for path, want := range map[string]string{"Plan.md": "conflict", "notes.txt": ".md", "../x.md": "invalidParams"} {
		if got := h.fails("create_note", map[string]any{"path": path, "content": "x"}); !strings.Contains(got, want) {
			t.Errorf("create_note(%s) = %q, want an error with %s", path, got, want)
		}
	}
	if h.disk("Plan.md") != "plan" {
		t.Error("a refused create_note changed a note")
	}
}

func TestEditNote(t *testing.T) {
	h := newHarness(t, map[string]string{
		"Plan.md":  "---\nstatus: draft\n---\none\ntwo\n",
		"Twice.md": "x and x",
	}, false)
	edit := func(args map[string]any) editOut {
		t.Helper()
		var out editOut
		h.ok("edit_note", args, &out)
		if out.Path != args["path"] || out.Hash != h.notesRead(out.Path).Hash {
			t.Fatalf("edit_note(%v) = %+v; its hash is not the note's", args, out)
		}
		h.byAgent(out.Path)
		return out
	}
	edit(map[string]any{"path": "Plan.md", "old_text": "two", "new_text": "2"})
	edit(map[string]any{"path": "Plan.md", "append": "three"})
	edit(map[string]any{"path": "Plan.md", "prepend": "zero"})
	edit(map[string]any{"path": "Plan.md", "properties": map[string]any{"status": "done", "owner": "wren"}})
	if got, want := h.disk("Plan.md"), "---\nstatus: done\nowner: wren\n---\nzero\none\n2\nthree\n"; got != want {
		t.Fatalf("Plan.md = %q, want %q", got, want)
	}

	// The tool edits what the note holds now: an edit someone else made
	// first stays.
	cur := h.notesRead("Plan.md")
	if _, err := h.notes.Write(t.Context(), &api.NoteWriteParams{VaultID: h.vault, Path: "Plan.md", Base: cur.Hash,
		Content: cur.Content + "four, by hand\n"}); err != nil {
		t.Fatal(err)
	}
	if out := edit(map[string]any{"path": "Plan.md", "append": "five"}); out.Merged {
		t.Errorf("an edit of the current note reported a merge: %+v", out)
	}
	if got := h.disk("Plan.md"); !strings.HasSuffix(got, "three\nfour, by hand\nfive\n") {
		t.Errorf("Plan.md after edits on both sides = %q", got)
	}

	before := h.disk("Plan.md")
	for _, tc := range []struct {
		args map[string]any
		want string
	}{
		{map[string]any{"path": "Twice.md", "old_text": "x", "new_text": "y"}, "2 times"},
		{map[string]any{"path": "Plan.md"}, "exactly one"},
		{map[string]any{"path": "Plan.md", "append": "a", "prepend": "b"}, "exactly one"},
		{map[string]any{"path": "Missing.md", "append": "a"}, "notFound"},
	} {
		if got := h.fails("edit_note", tc.args); !strings.Contains(got, tc.want) {
			t.Errorf("edit_note(%v) = %q, want an error with %q", tc.args, got, tc.want)
		}
	}
	if h.disk("Plan.md") != before || h.disk("Twice.md") != "x and x" {
		t.Error("a refused edit changed a note")
	}

	// The user undoes the agent's last edit with note.revert.
	vs := h.versions("Plan.md")
	if _, err := h.notes.Revert(t.Context(), &api.NoteRevertParams{VaultID: h.vault, Path: "Plan.md", Version: vs[1].ID,
		Base: vs[0].Hash}); err != nil {
		t.Fatal(err)
	}
	if got := h.disk("Plan.md"); strings.HasSuffix(got, "five\n") || !strings.HasSuffix(got, "four, by hand\n") {
		t.Errorf("Plan.md after reverting the agent's edit = %q", got)
	}
}

func TestAppendToDailyNote(t *testing.T) {
	h := newHarness(t, map[string]string{
		".obsidian/daily-notes.json": `{"folder": "Journal", "format": "YYYY-MM-DD", "template": "Templates/Daily"}`,
		"Templates/Daily.md":         "# {{title}}\n",
	}, false)
	var out dailyOut
	h.ok("append_to_daily_note", map[string]any{"text": "- bought quince", "date": "2026-10-16"}, &out)
	if out.Path != "Journal/2026-10-16.md" || !out.Created || out.Hash != h.notesRead(out.Path).Hash {
		t.Fatalf("append_to_daily_note = %+v", out)
	}
	h.ok("append_to_daily_note", map[string]any{"text": "- pruned the pear\n", "date": "2026-10-16"}, &out)
	if out.Created || h.disk(out.Path) != "# 2026-10-16\n- bought quince\n- pruned the pear\n" {
		t.Fatalf("a second append = %+v; the note holds %q", out, h.disk(out.Path))
	}
	for _, v := range h.versions(out.Path) {
		if v.Source != api.VersionSourceAgent || v.Author != clientName {
			t.Errorf("version %+v is not the agent's, though the agent made the note too", v)
		}
	}

	// Without a date, today's.
	before := time.Now().Format(time.DateOnly)
	h.ok("append_to_daily_note", map[string]any{"text": "today"}, &out)
	if after := time.Now().Format(time.DateOnly); out.Path != "Journal/"+before+".md" && out.Path != "Journal/"+after+".md" {
		t.Errorf("an append without a date went to %s, want today's note", out.Path)
	}
	for _, args := range []map[string]any{{"text": "x", "date": "16/10/2026"}, {"text": ""}} {
		if got := h.fails("append_to_daily_note", args); got == "" {
			t.Errorf("append_to_daily_note(%v) failed without a reason", args)
		}
	}
}
