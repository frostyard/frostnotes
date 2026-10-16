package main

// CONTRACT TEST for task card T-0020 (docs/tasks). Do not edit.

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/frostyard/clix"
	"github.com/frostyard/frostnotes/api"
	"github.com/frostyard/frostnotes/internal/rpctest"
)

// notesIn runs notesctl against srv with stdin, as runNotesctl does
// without one.
func notesIn(t *testing.T, srv *rpctest.Server, stdin string, args ...string) (string, error) {
	t.Helper()
	var out bytes.Buffer
	clix.Stdout = &out
	t.Cleanup(func() { clix.Stdout = nil })
	root := newRootCmd()
	root.SetArgs(append([]string{"--socket", srv.Socket}, args...))
	root.SetOut(&out)
	root.SetIn(strings.NewReader(stdin))
	err := (&clix.App{Version: "test"}).Run(root)
	return out.String(), err
}

func TestNoteWrite(t *testing.T) {
	fake := &fakeNotes{write: api.WriteResult{Path: "Plan.md", Hash: "h2"}}
	srv := notesServer(t, fake, "garden")
	content := "---\r\ntitle: x\r\n---\r\nno final newline"
	out, err := notesIn(t, srv, content, "write", "Plan.md", "--base", "h1")
	if err != nil || out != "wrote Plan.md h2\n" {
		t.Fatalf("write = %q, %v", out, err)
	}
	checkCall(t, fake.only(t), &api.NoteWriteParams{VaultID: 1, Path: "Plan.md", Base: "h1", Content: content})

	file := filepath.Join(t.TempDir(), "new.md")
	if err := os.WriteFile(file, []byte("from a file\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := notesIn(t, srv, "not this", "write", "Plan.md", "--base", "h1", "--file", file); err != nil {
		t.Fatal(err)
	}
	checkCall(t, fake.only(t), &api.NoteWriteParams{VaultID: 1, Path: "Plan.md", Base: "h1", Content: "from a file\n"})

	for conflicts, want := range map[int64]string{0: "merged Plan.md h2\n", 1: "merged Plan.md h2, 1 conflict\n", 3: "merged Plan.md h2, 3 conflicts\n"} {
		fake.write = api.WriteResult{Path: "Plan.md", Hash: "h2", Merged: true, Content: ptr("merged text"), Conflicts: conflicts}
		if out, err := notesIn(t, srv, "x", "write", "Plan.md", "--base", "h1"); err != nil || out != want {
			t.Errorf("a merged write with %d conflicts = %q, %v; want %q", conflicts, out, err, want)
		}
		fake.only(t)
	}
	out, err = notesIn(t, srv, "x", "--json", "write", "Plan.md", "--base", "h1")
	var got api.WriteResult
	if err != nil || json.Unmarshal([]byte(out), &got) != nil || !reflect.DeepEqual(got, fake.write) {
		t.Errorf("write --json = %q, %v", out, err)
	}
	fake.only(t)

	fake.write = api.WriteResult{Path: "New.md", Hash: "h3"}
	if out, err := notesIn(t, srv, "fresh", "write", "New.md", "--create"); err != nil || out != "created New.md h3\n" {
		t.Errorf("write --create = %q, %v", out, err)
	}
	checkCall(t, fake.only(t), &api.NoteCreateParams{VaultID: 1, Path: "New.md", Content: "fresh"})

	for _, tc := range []struct {
		args []string
		want string
	}{
		{[]string{"write", "Plan.md"}, "--base"},
		{[]string{"write", "Plan.md", "--base", "h1", "--create"}, "--create"},
		{[]string{"write", "Plan.md", "--base", "h1", "--file", filepath.Join(t.TempDir(), "missing")}, "missing"},
		{[]string{"write"}, "arg"},
	} {
		if _, err := notesIn(t, srv, "x", tc.args...); err == nil || !strings.Contains(err.Error(), tc.want) {
			t.Errorf("notesctl %q: %v, want an error mentioning %q", tc.args, err, tc.want)
		}
	}
	if len(fake.calls) != 0 {
		t.Errorf("refused writes still called notesd: %+v", fake.calls)
	}
}

func TestNoteMoveAndRemove(t *testing.T) {
	fake := &fakeNotes{move: api.MoveResult{Path: "Archive/Plan.md", Rewritten: []string{}}}
	srv := notesServer(t, fake, "garden")
	if out := notes(t, srv, "mv", "Plan.md", "Archive/Plan.md"); out != "moved Plan.md to Archive/Plan.md\n" {
		t.Errorf("mv = %q", out)
	}
	checkCall(t, fake.only(t), &api.NoteMoveParams{VaultID: 1, Path: "Plan.md", To: "Archive/Plan.md"})
	fake.move.Rewritten = []string{"Inbox.md", "Orchard/Index.md"}
	want := "moved Plan.md to Archive/Plan.md\nrewrote links in Inbox.md\nrewrote links in Orchard/Index.md\n"
	if out := notes(t, srv, "mv", "--update-links", "Plan.md", "Archive/Plan.md"); out != want {
		t.Errorf("mv --update-links = %q, want %q", out, want)
	}
	checkCall(t, fake.only(t), &api.NoteMoveParams{VaultID: 1, Path: "Plan.md", To: "Archive/Plan.md", UpdateLinks: ptr(true)})

	if out := notes(t, srv, "rm", "Plan.md", "--base", "h1"); out != "deleted Plan.md\n" {
		t.Errorf("rm = %q", out)
	}
	checkCall(t, fake.only(t), &api.NoteDeleteParams{VaultID: 1, Path: "Plan.md", Base: "h1"})
	if err := notesErr(t, srv, "rm", "Plan.md"); !strings.Contains(err.Error(), "--base") {
		t.Errorf("rm without --base: %v", err)
	}
	if err := notesErr(t, srv, "mv", "Plan.md"); !strings.Contains(err.Error(), "arg") {
		t.Errorf("mv with one path: %v", err)
	}
}

func TestNoteVersionsAndRevert(t *testing.T) {
	at := time.Date(2026, 10, 16, 12, 30, 0, 0, time.FixedZone("x", 3600))
	fake := &fakeNotes{versions: []api.Version{
		{ID: 9, Path: "Plan.md", Source: api.VersionSourceEditor, Author: "notesctl dev", At: at, Deleted: true},
		{ID: 8, Path: "Plan.md", Hash: "0123456789abcdef0123", Size: 5, Source: api.VersionSourceAgent, Author: "claude-code", At: at, Conflict: true},
		{ID: 7, Path: "Old/Plan.md", Hash: "fedcba9876543210ffff", Size: 4, Source: api.VersionSourceOutside, At: at.Add(-time.Hour)},
	}, write: api.WriteResult{Path: "Plan.md", Hash: "h4"}}
	srv := notesServer(t, fake, "garden")
	want := "9\t2026-10-16T11:30:00Z\teditor\tnotesctl dev\tdeleted\tPlan.md\n" +
		"8\t2026-10-16T11:30:00Z\tagent\tclaude-code\t0123456789ab conflict\tPlan.md\n" +
		"7\t2026-10-16T10:30:00Z\toutside\t-\tfedcba987654\tOld/Plan.md\n"
	if out := notes(t, srv, "versions", "Plan.md", "--limit", "3"); out != want {
		t.Errorf("versions =\n%s\nwant\n%s", out, want)
	}
	checkCall(t, fake.only(t), &api.NoteVersionsParams{VaultID: 1, Path: "Plan.md", Limit: ptr[int64](3)})
	var got []api.Version
	if err := json.Unmarshal([]byte(notes(t, srv, "--json", "versions", "Plan.md")), &got); err != nil || len(got) != 3 || got[1].Hash != fake.versions[1].Hash {
		t.Errorf("versions --json = %+v, %v", got, err)
	}
	checkCall(t, fake.only(t), &api.NoteVersionsParams{VaultID: 1, Path: "Plan.md"})

	if out := notes(t, srv, "revert", "Plan.md", "8", "--base", "h3"); out != "reverted Plan.md to version 8: h4\n" {
		t.Errorf("revert = %q", out)
	}
	checkCall(t, fake.only(t), &api.NoteRevertParams{VaultID: 1, Path: "Plan.md", Version: 8, Base: "h3"})
	notes(t, srv, "revert", "Gone.md", "3")
	checkCall(t, fake.only(t), &api.NoteRevertParams{VaultID: 1, Path: "Gone.md", Version: 3, Base: ""})
	fake.write = api.WriteResult{Path: "Plan.md", Hash: "h5", Merged: true, Content: ptr("x"), Conflicts: 2}
	if out := notes(t, srv, "revert", "Plan.md", "8", "--base", "h1"); out != "reverted Plan.md to version 8: h5, merged, 2 conflicts\n" {
		t.Errorf("a merged revert = %q", out)
	}
	fake.only(t)
	if err := notesErr(t, srv, "revert", "Plan.md", "eight"); !strings.Contains(err.Error(), "eight") {
		t.Errorf("revert with a bad version: %v", err)
	}
}
