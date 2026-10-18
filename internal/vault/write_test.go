package vault

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/frostyard/frostnotes/api"
	"github.com/frostyard/frostnotes/internal/blobs"
	"github.com/frostyard/frostnotes/internal/store"
)

const three = "one\ntwo\nthree\n"

func (tv *testVault) read(p string) *api.Note {
	tv.t.Helper()
	n, err := tv.v.Read(tv.t.Context(), p)
	if err != nil {
		tv.t.Fatalf("Read(%s): %v", p, err)
	}
	return n
}

func (tv *testVault) write(p, base, content string, who Author) *api.WriteResult {
	tv.t.Helper()
	res, err := tv.v.Write(tv.t.Context(), p, base, []byte(content), who)
	if err != nil {
		tv.t.Fatalf("Write(%s): %v", p, err)
	}
	if got := tv.disk(p); blobs.ID([]byte(got)) != res.Hash {
		tv.t.Fatalf("Write(%s) returned hash %s, but the file holds %q", p, res.Hash, got)
	}
	return res
}

func sources(vs []store.Version) string {
	var parts []string
	for _, v := range vs {
		s := string(v.Source)
		if v.Conflict {
			s += "(conflict)"
		}
		if v.Hash == "" {
			s += "(deleted)"
		}
		parts = append(parts, s)
	}
	return strings.Join(parts, " ")
}

// ADR-0006's local writer pair: an agent's write against an outside edit
// that no scan has seen yet.
func TestWriteMergesAnUnscannedOutsideEdit(t *testing.T) {
	tv := newTestVault(t, map[string]string{"Plan.md": three}, nil)
	tv.scan()
	base := tv.read("Plan.md").Hash
	tv.edit("Plan.md", "one, in vim\ntwo\nthree\n")
	res := tv.write("Plan.md", base, "one\ntwo\nthree, by the agent\n", agent)
	want := "one, in vim\ntwo\nthree, by the agent\n"
	if !res.Merged || res.Content == nil || *res.Content != want || res.Conflicts != 0 {
		t.Fatalf("Write = %+v, want a clean merge to %q", res, want)
	}
	if got := tv.disk("Plan.md"); got != want {
		t.Fatalf("the file holds %q, want %q", got, want)
	}
	vs := tv.versions(tv.row("Plan.md").ID)
	if got := sources(vs); got != "agent outside outside" || vs[0].Author != "an agent" {
		t.Errorf("versions = %s (author %q), want the agent's merge, the vim edit and the first scan", got, vs[0].Author)
	}
	if got := tv.takeEvents(); fmt.Sprint(got) != "[note.changed Plan.md outside note.changed Plan.md outside note.changed Plan.md agent]" {
		t.Errorf("events = %q", got)
	}
}

// ADR-0006: just before the rename the disk is compared again, so an edit
// made while the write was staged is merged, not overwritten.
func TestWriteMergesAnEditMadeWhileStaged(t *testing.T) {
	tv := newTestVault(t, map[string]string{"Plan.md": three}, nil)
	tv.scan()
	base := tv.read("Plan.md").Hash
	edited := false
	tv.v.staged = func(p string) {
		if !edited {
			edited = true
			tv.edit(p, "one\ntwo, edited meanwhile\nthree\n")
		}
	}
	res := tv.write("Plan.md", base, "one\ntwo\nthree\nfour\n", editor)
	if want := "one\ntwo, edited meanwhile\nthree\nfour\n"; !res.Merged || tv.disk("Plan.md") != want {
		t.Fatalf("Write = %+v and the file holds %q, want %q", res, tv.disk("Plan.md"), want)
	}
	if entries, _ := os.ReadDir(tv.dir); len(entries) != 1 {
		t.Errorf("the vault holds %d entries, want only Plan.md: a staged file was left", len(entries))
	}
}

// ADR-0006's other local pair: two writers that started from one base.
func TestTwoWritersOnOneBase(t *testing.T) {
	tv := newTestVault(t, map[string]string{"Plan.md": three}, nil)
	tv.scan()
	base := tv.read("Plan.md").Hash
	first := tv.write("Plan.md", base, "one, first\ntwo\nthree\n", editor)
	if first.Merged || first.Content != nil {
		t.Fatalf("a write on the current base = %+v, want no merge", first)
	}
	second := tv.write("Plan.md", base, "one\ntwo\nthree, second\n", agent)
	if want := "one, first\ntwo\nthree, second\n"; !second.Merged || *second.Content != want {
		t.Fatalf("the second writer = %+v, want a merge to %q", second, want)
	}

	// A third writer on the same base changes the first line differently:
	// the file's side stands and the writer's content is a conflict version.
	sent := "one, third\ntwo\nthree\n"
	third := tv.write("Plan.md", base, sent, agent)
	if want := "one, first\ntwo\nthree, second\n"; third.Conflicts != 1 || *third.Content != want || tv.disk("Plan.md") != want {
		t.Fatalf("the conflicting writer = %+v, file %q", third, tv.disk("Plan.md"))
	}
	vs := tv.versions(tv.row("Plan.md").ID)
	if got := sources(vs); got != "agent(conflict) agent editor outside" {
		t.Fatalf("versions = %s", got)
	}
	if data, err := tv.blobs.Get(vs[0].Hash); err != nil || string(data) != sent {
		t.Errorf("the conflict version holds %q, %v; want what the writer sent", data, err)
	}
	if third.ConflictVersion == nil || *third.ConflictVersion != vs[0].ID {
		t.Errorf("the answer names conflict version %v, want %d: the one the editor's Compare reads", third.ConflictVersion, vs[0].ID)
	}
	if first.ConflictVersion != nil || second.ConflictVersion != nil {
		t.Error("a write without conflicts names a conflict version")
	}
	if third.Hash != tv.row("Plan.md").Hash || vs[1].Hash != third.Hash {
		t.Error("a write that changed nothing on disk moved the file's hash")
	}
}

func TestWriteRefusesABaseItCannotMerge(t *testing.T) {
	tv := newTestVault(t, map[string]string{"Plan.md": three, "img.png": "\x89PNG\xff"}, nil)
	tv.scan()
	if _, err := tv.v.Write(t.Context(), "Plan.md", strings.Repeat("0", 64), []byte("x"), editor); !errors.Is(err, api.ErrConflict) {
		t.Errorf("a base that is no version = %v, want conflict", err)
	}
	base := tv.read("Plan.md").Hash
	tv.write("Plan.md", base, "new\n", editor)
	if err := tv.blobs.Remove(base); err != nil {
		t.Fatal(err)
	}
	if _, err := tv.v.Write(t.Context(), "Plan.md", base, []byte("x"), editor); !errors.Is(err, api.ErrConflict) {
		t.Errorf("a pruned base = %v, want conflict", err)
	}
	img := tv.row("img.png").Hash
	tv.edit("img.png", "\x89PNG\xfe")
	if _, err := tv.v.Write(t.Context(), "img.png", img, []byte("text"), editor); !errors.Is(err, api.ErrConflict) {
		t.Errorf("a stale base of a binary file = %v, want conflict", err)
	}
	if _, err := tv.v.Write(t.Context(), "Missing.md", base, []byte("x"), editor); !errors.Is(err, api.ErrNotFound) {
		t.Errorf("a write to a missing file = %v, want notFound", err)
	}
	for _, p := range []string{"../x.md", ".obsidian/app.json", ".frostnotes/vault.yaml"} {
		if _, err := tv.v.Write(t.Context(), p, base, []byte("x"), editor); !errors.Is(err, api.ErrInvalidParams) {
			t.Errorf("Write(%s) = %v, want invalidParams", p, err)
		}
	}
}

func TestCreate(t *testing.T) {
	tv := newTestVault(t, map[string]string{"Plan.md": "plan", "A.md": "[[New Idea]]"}, nil)
	tv.scan()
	res, err := tv.v.Create(t.Context(), "Ideas/Deep/New Idea.md", []byte("idea\n"), agent)
	if err != nil {
		t.Fatal(err)
	}
	if tv.disk("Ideas/Deep/New Idea.md") != "idea\n" || res.Hash != blobs.ID([]byte("idea\n")) {
		t.Fatalf("Create = %+v; file %q", res, tv.disk("Ideas/Deep/New Idea.md"))
	}
	if got := tv.links("A.md"); got != "New Idea -> Ideas/Deep/New Idea.md\n" {
		t.Errorf("a waiting link did not resolve to the new note: %q", got)
	}
	if _, err := tv.v.Create(t.Context(), "Plan.md", []byte("x"), editor); !errors.Is(err, api.ErrConflict) {
		t.Errorf("Create over a file = %v, want conflict", err)
	}
	if _, err := tv.v.Create(t.Context(), "Ideas", []byte("x"), editor); !errors.Is(err, api.ErrConflict) {
		t.Errorf("Create over a folder = %v, want conflict", err)
	}
	if _, err := tv.v.Create(t.Context(), "Plan.md/x.md", []byte("x"), editor); !errors.Is(err, api.ErrInvalidParams) {
		t.Errorf("Create under a file = %v, want invalidParams", err)
	}
	if tv.disk("Plan.md") != "plan" {
		t.Error("a refused create changed a file")
	}
}

func TestDeleteAndRevert(t *testing.T) {
	tv := newTestVault(t, map[string]string{"Plan.md": three, "A.md": "[[Plan]]"}, nil)
	tv.scan()
	row := tv.row("Plan.md")
	first := row.Hash
	tv.edit("Plan.md", "edited elsewhere\n")
	if err := tv.v.Delete(t.Context(), "Plan.md", first, editor); !errors.Is(err, api.ErrConflict) {
		t.Fatalf("a delete on a stale base = %v, want conflict: an edit beats a delete", err)
	}
	if !tv.exists("Plan.md") {
		t.Fatal("a refused delete removed the file")
	}
	current := tv.read("Plan.md").Hash
	if err := tv.v.Delete(t.Context(), "Plan.md", current, editor); err != nil {
		t.Fatal(err)
	}
	if tv.exists("Plan.md") || tv.row("Plan.md").ID != 0 {
		t.Fatal("the file or its row survived a delete")
	}
	vs := tv.versions(row.ID)
	if got := sources(vs); got != "editor(deleted) outside outside" {
		t.Fatalf("versions after a delete = %s", got)
	}
	if got := tv.links("A.md"); got != "Plan -> \n" {
		t.Errorf("A.md's link after the delete = %q", got)
	}

	// The first version comes back at the file's last path, with its ID.
	oldest := vs[len(vs)-1]
	if _, err := tv.v.Revert(t.Context(), "Plan.md", oldest.ID, "x", editor); !errors.Is(err, api.ErrConflict) {
		t.Errorf("reverting a deleted file with a base = %v, want conflict", err)
	}
	res, err := tv.v.Revert(t.Context(), "Plan.md", oldest.ID, "", editor)
	if err != nil {
		t.Fatal(err)
	}
	if got := tv.row("Plan.md"); got.ID != row.ID || tv.disk("Plan.md") != three || res.Hash != first {
		t.Fatalf("revert brought back %+v holding %q", got, tv.disk("Plan.md"))
	}
	if got := tv.links("A.md"); got != "Plan -> Plan.md\n" {
		t.Errorf("A.md's link after the revert = %q", got)
	}

	// Reverting a file that exists is a write with a base.
	tv.write("Plan.md", first, "later\n", editor)
	edited := vs[1]
	res, err = tv.v.Revert(t.Context(), "Plan.md", edited.ID, blobs.ID([]byte("later\n")), editor)
	if err != nil || tv.disk("Plan.md") != "edited elsewhere\n" || res.Merged {
		t.Fatalf("Revert = %+v, %v; file %q", res, err, tv.disk("Plan.md"))
	}
	if _, err := tv.v.Revert(t.Context(), "A.md", edited.ID, tv.row("A.md").Hash, editor); !errors.Is(err, api.ErrNotFound) {
		t.Errorf("reverting to another file's version = %v, want notFound", err)
	}
	if _, err := tv.v.Revert(t.Context(), "Plan.md", vs[0].ID, res.Hash, editor); !errors.Is(err, api.ErrInvalidParams) {
		t.Errorf("reverting to a deletion = %v, want invalidParams", err)
	}
}

// ADR-0014: bytes that are not UTF-8 read as U+FFFD, and survive a write
// that leaves the words around them as they were.
func TestBytesThatAreNotUTF8SurviveWrites(t *testing.T) {
	raw := "caf\xe9 au lait\nsecond line\n"
	tv := newTestVault(t, map[string]string{"Menu.md": raw}, nil)
	tv.scan()
	n := tv.read("Menu.md")
	if n.Content != "caf� au lait\nsecond line\n" || n.Hash != blobs.ID([]byte(raw)) {
		t.Fatalf("Read = %q (%s), want U+FFFD for the stray byte and the hash of the bytes", n.Content, n.Hash)
	}
	res := tv.write("Menu.md", n.Hash, "caf� au lait\nsecond line, edited\n", editor)
	if want := "caf\xe9 au lait\nsecond line, edited\n"; tv.disk("Menu.md") != want || res.Merged {
		t.Fatalf("an edit elsewhere left %q (%+v), want %q", tv.disk("Menu.md"), res, want)
	}
	tv.write("Menu.md", res.Hash, "caf� au chocolat\nsecond line, edited\n", editor)
	if want := "caf\xe9 au chocolat\nsecond line, edited\n"; tv.disk("Menu.md") != want {
		t.Fatalf("an edit beside the stray byte left %q, want %q", tv.disk("Menu.md"), want)
	}
	tv.write("Menu.md", tv.row("Menu.md").Hash, "café au chocolat\nsecond line, edited\n", editor)
	if want := "café au chocolat\nsecond line, edited\n"; tv.disk("Menu.md") != want {
		t.Fatalf("fixing the word left %q, want %q", tv.disk("Menu.md"), want)
	}
}

func TestMoveUpdatesLinks(t *testing.T) {
	tv := newTestVault(t, map[string]string{
		"A.md":             "see [[Plan]], [the plan](Notes/Plan.md) and ![[Plan#Goals|goals]]\n",
		"B.md":             "---\nup: \"[[Plan]]\"\n---\nunrelated [[Other]]\n",
		"Other.md":         "other",
		"Notes/Plan.md":    "# Goals\nme [[Plan]], beside [[./Sibling]]\n",
		"Notes/Sibling.md": "sibling",
	}, nil)
	tv.scan()
	id := tv.row("Notes/Plan.md").ID
	res, err := tv.v.Move(t.Context(), "Notes/Plan.md", "Archive/Plan 2.md", true, editor)
	if err != nil {
		t.Fatal(err)
	}
	if fmt.Sprint(res.Rewritten) != "[A.md Archive/Plan 2.md B.md]" || res.Path != "Archive/Plan 2.md" {
		t.Fatalf("Move = %+v", res)
	}
	if tv.row("Archive/Plan 2.md").ID != id || tv.exists("Notes/Plan.md") {
		t.Fatal("the moved file lost its ID or stayed")
	}
	for p, want := range map[string]string{
		"A.md":              "see [[Plan 2]], [the plan](Plan%202.md) and ![[Plan 2#Goals|goals]]\n",
		"B.md":              "---\nup: \"[[Plan 2]]\"\n---\nunrelated [[Other]]\n",
		"Archive/Plan 2.md": "# Goals\nme [[Plan 2]], beside [[Sibling]]\n",
	} {
		if got := tv.disk(p); got != want {
			t.Errorf("%s = %q, want %q", p, got, want)
		}
	}
	if got := tv.links("A.md"); got != "Plan 2 -> Archive/Plan 2.md\nPlan 2.md -> Archive/Plan 2.md\nPlan 2#Goals -> Archive/Plan 2.md\n" {
		t.Errorf("A.md's links = %q", got)
	}
	if _, err := tv.v.Move(t.Context(), "A.md", "B.md", false, editor); !errors.Is(err, api.ErrConflict) {
		t.Errorf("a move onto a file = %v, want conflict", err)
	}
	if _, err := tv.v.Move(t.Context(), "Nope.md", "C.md", false, editor); !errors.Is(err, api.ErrNotFound) {
		t.Errorf("a move of a missing file = %v, want notFound", err)
	}
	// Without updateLinks only the file moves.
	before := tv.disk("A.md")
	if _, err := tv.v.Move(t.Context(), "Archive/Plan 2.md", "Plan 3.md", false, editor); err != nil {
		t.Fatal(err)
	}
	if tv.disk("A.md") != before {
		t.Error("a move without updateLinks rewrote a note")
	}
}

func TestDailyNotes(t *testing.T) {
	tv := newTestVault(t, map[string]string{
		".obsidian/daily-notes.json": `{"folder": "Journal/", "format": "YYYY/MM-DD", "template": "Templates/Daily"}`,
		"Templates/Daily.md":         "# {{title}}\n{{date}} was a {{date:dddd}}; made at {{time}}\n",
	}, nil)
	tv.scan()
	date := time.Date(2026, 10, 16, 0, 0, 0, 0, time.UTC)
	d, err := tv.v.Daily(t.Context(), date, agent)
	if err != nil {
		t.Fatal(err)
	}
	want := "# 10-16\n2026/10-16 was a Friday; made at 09:30\n"
	if !d.Created || d.Note.Path != "Journal/2026/10-16.md" || d.Note.Content != want {
		t.Fatalf("Daily = created %v, %s %q; want Journal/2026/10-16.md %q", d.Created, d.Note.Path, d.Note.Content, want)
	}
	again, err := tv.v.Daily(t.Context(), date, agent)
	if err != nil || again.Created || again.Note.Hash != d.Note.Hash {
		t.Fatalf("Daily again = %+v, %v; want the same note, not created", again, err)
	}
	if vs := tv.versions(tv.row(d.Note.Path).ID); sources(vs) != "agent" {
		t.Errorf("the daily note's versions = %s", sources(vs))
	}

	// vault.yaml's settings win over Obsidian's, setting by setting.
	tv.edit(".frostnotes/vault.yaml", "daily:\n  folder: Days\n  template: \"\"\n")
	d, err = tv.v.Daily(t.Context(), date.AddDate(0, 0, 1), agent)
	if err != nil || d.Note.Path != "Days/2026/10-17.md" || d.Note.Content != "" {
		t.Fatalf("Daily with vault.yaml = %+v, %v", d, err)
	}
}

func TestDefaultDailyNote(t *testing.T) {
	tv := newTestVault(t, map[string]string{}, nil)
	d, err := tv.v.Daily(t.Context(), time.Date(2026, 3, 9, 0, 0, 0, 0, time.UTC), editor)
	if err != nil || d.Note.Path != "2026-03-09.md" || !d.Created {
		t.Fatalf("Daily without settings = %+v, %v; want 2026-03-09.md at the root", d, err)
	}
}

func TestPrune(t *testing.T) {
	tv := newTestVault(t, map[string]string{"Plan.md": "v1\n", "Gone.md": "gone\n"}, nil)
	tv.scan()
	v1 := tv.row("Plan.md").Hash
	v2 := tv.write("Plan.md", v1, "v2\n", editor).Hash
	gone := tv.row("Gone.md")
	if err := tv.v.Delete(t.Context(), "Gone.md", gone.Hash, editor); err != nil {
		t.Fatal(err)
	}
	*tv.clock = tv.clock.Add(29 * 24 * time.Hour)
	v3 := tv.write("Plan.md", v2, "v3\n", editor).Hash
	orphan, err := tv.blobs.Put(t.Context(), bytes.NewReader([]byte("a crash left me")))
	if err != nil {
		t.Fatal(err)
	}
	*tv.clock = tv.clock.Add(2 * 24 * time.Hour)
	m := &Manager{cfg: *tv.v.cfg, vaults: map[int64]*Vault{tv.v.id: tv.v}}
	if err := m.Prune(t.Context()); err != nil {
		t.Fatal(err)
	}
	if got := sources(tv.versions(tv.row("Plan.md").ID)); got != "editor" {
		t.Errorf("Plan.md's versions after the prune = %s, want only the one younger than 30 days", got)
	}
	if got := tv.versions(gone.ID); len(got) != 0 {
		t.Errorf("a deleted file's old versions survived the prune: %s", sources(got))
	}
	for hash, want := range map[string]bool{v1: false, v2: false, v3: true, gone.Hash: false, orphan: false} {
		if ok, _ := tv.blobs.Has(hash); ok != want {
			t.Errorf("blob %s kept = %v, want %v", hash[:8], ok, want)
		}
	}
}

func TestWriteKeepsTheFileMode(t *testing.T) {
	tv := newTestVault(t, map[string]string{"Private.md": "secret\n"}, nil)
	full := filepath.Join(tv.dir, "Private.md")
	if err := os.Chmod(full, 0o600); err != nil {
		t.Fatal(err)
	}
	tv.scan()
	tv.write("Private.md", tv.row("Private.md").Hash, "still secret\n", editor)
	if fi, err := os.Stat(full); err != nil || fi.Mode().Perm() != 0o600 {
		t.Fatalf("mode after a write = %v, %v; want 0600", fi.Mode().Perm(), err)
	}
}

// Links can nest: a clickable image is a link around an image, and both
// name the file. A move rewrites both.
func TestMoveRewritesNestedLinks(t *testing.T) {
	tv := newTestVault(t, map[string]string{"N.md": "[![p](pic.png)](pic.png) and [see [[Plan]]](Plan.md)\n", "pic.png": "\x89PNG", "Plan.md": "plan"}, nil)
	tv.scan()
	for from, to := range map[string]string{"pic.png": "img/pic2.png", "Plan.md": "Notes/Plan 2.md"} {
		if _, err := tv.v.Move(t.Context(), from, to, true, editor); err != nil {
			t.Fatalf("Move(%s): %v", from, err)
		}
	}
	if got, want := tv.disk("N.md"), "[![p](pic2.png)](pic2.png) and [see [[Plan 2]]](Plan%202.md)\n"; got != want {
		t.Errorf("N.md = %q, want %q", got, want)
	}
}

// A client that goes away once its write is staged changes nothing: the
// file, its version and the writer's losing text are all recorded.
func TestWriteSurvivesACancelledClient(t *testing.T) {
	tv := newTestVault(t, map[string]string{"Plan.md": three}, nil)
	tv.scan()
	base := tv.read("Plan.md").Hash
	tv.write("Plan.md", base, "one, first\ntwo\nthree\n", editor)
	ctx, cancel := context.WithCancel(t.Context())
	cancelled := false
	tv.v.staged = func(string) { cancel(); cancelled = true }
	res, err := tv.v.Write(ctx, "Plan.md", base, []byte("one, second\ntwo\nthree, second\n"), agent)
	if err != nil || !cancelled {
		t.Fatalf("a write whose client went away after staging = %+v, %v (staged: %v)", res, err, cancelled)
	}
	want := "one, first\ntwo\nthree, second\n"
	if res.Conflicts != 1 || tv.disk("Plan.md") != want || res.Hash != tv.row("Plan.md").Hash {
		t.Fatalf("Write = %+v; file %q, index %s", res, tv.disk("Plan.md"), tv.row("Plan.md").Hash)
	}
	if got := sources(tv.versions(tv.row("Plan.md").ID)); got != "agent agent(conflict) editor outside" {
		t.Fatalf("versions = %s, want the merge and the losing text kept", got)
	}
}

// A vault being removed refuses changes before its row goes, so no write
// lands on disk that its index could not record.
func TestRemovedVaultRefusesChanges(t *testing.T) {
	tv := newTestVault(t, map[string]string{"Plan.md": "plan"}, nil)
	tv.scan()
	tv.v.setRemoved(true)
	if _, err := tv.v.Create(t.Context(), "New.md", []byte("x"), editor); !errors.Is(err, api.ErrNotFound) {
		t.Errorf("Create on a removed vault = %v, want notFound", err)
	}
	if _, err := tv.v.Write(t.Context(), "Plan.md", tv.row("Plan.md").Hash, []byte("x"), editor); !errors.Is(err, api.ErrNotFound) {
		t.Errorf("Write on a removed vault = %v, want notFound", err)
	}
	if tv.exists("New.md") || tv.disk("Plan.md") != "plan" {
		t.Error("a removed vault's files changed")
	}
}
