package vault

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/frostyard/frostnotes/api"
)

func TestCheckPath(t *testing.T) {
	for p, ok := range map[string]bool{
		"Plan.md":                true,
		"Orchard/North Block.md": true,
		"Café ☕/Plan.md":         true,
		".frostnotes/vault.yaml": true,
		"media/map.png":          true,
		"":                       false,
		"/etc/passwd":            false,
		"../outside.md":          false,
		"a/../b.md":              false,
		"./Plan.md":              false,
		"a//b.md":                false,
		"a/":                     false,
		".obsidian/app.json":     false,
		"a/.frostnotes/x":        false,
		"Orchard/.draft.md":      false,
		".trash/old.md":          false,
		"bad\xffname.md":         false,
		"nul\x00.md":             false,
	} {
		if err := CheckPath(p); (err == nil) != ok {
			t.Errorf("CheckPath(%q) = %v, want ok %v", p, err, ok)
		} else if err != nil && !errors.Is(err, ErrPath) {
			t.Errorf("CheckPath(%q) = %v, not ErrPath", p, err)
		}
	}
}

func TestReadRefusesSymlinks(t *testing.T) {
	outside := t.TempDir()
	if err := os.WriteFile(filepath.Join(outside, "secret.md"), []byte("secret"), 0o644); err != nil {
		t.Fatal(err)
	}
	tv := newTestVault(t, map[string]string{"Plan.md": "plan", "real/x.md": "x"}, nil)
	symlink(t, filepath.Join(outside, "secret.md"), filepath.Join(tv.dir, "link.md"))
	symlink(t, outside, filepath.Join(tv.dir, "away"))
	symlink(t, filepath.Join(tv.dir, "real"), filepath.Join(tv.dir, "inside"))
	tv.scan()
	for _, p := range []string{"link.md", "away/secret.md", "inside/x.md"} {
		if _, err := tv.v.Read(t.Context(), p); !errors.Is(err, api.ErrNotFound) && !errors.Is(err, api.ErrInvalidParams) {
			t.Errorf("Read(%s) through a symlink = %v, want notFound or invalidParams", p, err)
		}
	}
	for _, p := range []string{"away/new.md", "inside/new.md", "link.md"} {
		if _, err := tv.v.Create(t.Context(), p, []byte("x"), editor); err == nil {
			t.Errorf("Create(%s) through a symlink succeeded", p)
		}
	}
	if _, err := os.Stat(filepath.Join(outside, "new.md")); err == nil {
		t.Error("a file was created outside the vault")
	}
}

func TestScanIndexesAndFollowsChanges(t *testing.T) {
	tv := newTestVault(t, map[string]string{
		"A.md":                   "see [[B]] #tag\n",
		"B.md":                   "b\n",
		"sub/C.md":               "back to [[A]]\n",
		"img.png":                "\x89PNG",
		"skip/x.md":              "ignored",
		".obsidian/app.json":     "{}",
		".frostnotes/vault.yaml": "ignore: [\"skip/\"]\n",
	}, nil)
	tv.scan()
	ctx := t.Context()
	files, err := tv.db.Files(ctx, tv.v.id)
	if err != nil {
		t.Fatal(err)
	}
	var paths []string
	for _, f := range files {
		paths = append(paths, f.Path)
		if vs := tv.versions(f.ID); len(vs) != 1 || vs[0].Source != api.VersionSourceOutside || vs[0].Hash != f.Hash {
			t.Errorf("%s's versions after the first scan = %+v, want one outside version", f.Path, vs)
		}
	}
	if fmt.Sprint(paths) != "[A.md B.md img.png sub/C.md]" {
		t.Fatalf("indexed %v; want the notes and the attachment, not ignored, hidden or settings files", paths)
	}
	if got := tv.links("A.md"); got != "B -> B.md\n" {
		t.Errorf("A.md's links = %q", got)
	}
	if got := tv.takeEvents(); len(got) != 4 {
		t.Errorf("first scan's events = %q, want four note.changed", got)
	}
	s, err := tv.v.Status(ctx)
	if err != nil || s.State != api.ScanStateIdle || s.Notes != 3 || s.Attachments != 1 || s.ScannedAt == nil {
		t.Errorf("status = %+v, %v", s, err)
	}

	// An outside edit is a version; a touch is not.
	tv.edit("B.md", "b, edited\n")
	a := tv.row("A.md")
	if err := os.Chtimes(filepath.Join(tv.dir, "A.md"), tv.mtime, tv.mtime); err != nil {
		t.Fatal(err)
	}
	tv.scan()
	if got := tv.takeEvents(); fmt.Sprint(got) != "[note.changed B.md outside]" {
		t.Errorf("events after an edit and a touch = %q", got)
	}
	if n := len(tv.versions(tv.row("B.md").ID)); n != 2 {
		t.Errorf("B.md has %d versions after an outside edit, want 2", n)
	}
	if got := tv.row("A.md"); got.ID != a.ID || !got.ModTime.Equal(tv.mtime) || len(tv.versions(a.ID)) != 1 {
		t.Errorf("after a touch A.md = %+v with %d versions; want its new time and no new version", got, len(tv.versions(a.ID)))
	}

	// A rename is a move: the file keeps its ID and versions.
	c := tv.row("sub/C.md")
	if err := os.Rename(filepath.Join(tv.dir, "sub/C.md"), filepath.Join(tv.dir, "sub/D.md")); err != nil {
		t.Fatal(err)
	}
	tv.scan()
	if got := tv.row("sub/D.md"); got.ID != c.ID || len(tv.versions(c.ID)) != 1 {
		t.Errorf("after a rename sub/D.md = %+v; want file %d with its version", got, c.ID)
	}
	if got := tv.takeEvents(); fmt.Sprint(got) != "[note.moved sub/C.md sub/D.md]" {
		t.Errorf("events after a rename = %q", got)
	}

	// A deletion is a version too, and the links to the file let go.
	b := tv.row("B.md")
	if err := os.Remove(filepath.Join(tv.dir, "B.md")); err != nil {
		t.Fatal(err)
	}
	tv.scan()
	if tv.row("B.md").ID != 0 {
		t.Error("B.md is still indexed after its deletion")
	}
	if vs := tv.versions(b.ID); len(vs) != 3 || vs[0].Hash != "" || vs[0].Path != "B.md" {
		t.Errorf("B.md's versions after its deletion = %+v, want a deletion first", vs)
	}
	if got := tv.links("A.md"); got != "B -> \n" {
		t.Errorf("A.md's links after B.md went = %q", got)
	}
	if got := tv.takeEvents(); fmt.Sprint(got) != "[note.removed B.md]" {
		t.Errorf("events after a deletion = %q", got)
	}
}

// A scan in batches of two leaves the index a single transaction would.
func TestScanBatches(t *testing.T) {
	files := map[string]string{}
	for i := range 9 {
		files[fmt.Sprintf("n%d/Note %d.md", i%3, i)] = fmt.Sprintf("# %d\nlinks [[Note %d]] and [[Note %d]] #t%d\n", i, (i+1)%9, (i+4)%9, i%2)
	}
	dir := makeVault(t, files)
	small := openTestVault(t, dir, t.TempDir(), func(c *Config) { c.Batch = 2 })
	small.scan()
	whole := openTestVault(t, dir, t.TempDir(), nil)
	whole.scan()
	a, err := small.db.IndexSnapshot(t.Context(), small.v.id)
	if err != nil {
		t.Fatal(err)
	}
	b, err := whole.db.IndexSnapshot(t.Context(), whole.v.id)
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(a, b) {
		t.Fatalf("batches of 2 give a different index:\n%s\nwant\n%s", strings.Join(a, "\n"), strings.Join(b, "\n"))
	}
	if !slices.ContainsFunc(a, func(row string) bool {
		return strings.HasPrefix(row, "link n0/Note 0.md 0 ") && strings.HasSuffix(row, "'n1/Note 1.md'")
	}) {
		t.Errorf("a link to a note in a later batch did not resolve:\n%s", strings.Join(a, "\n"))
	}
}

func TestScanFailsOnUnreadableFolder(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("root reads every folder")
	}
	tv := newTestVault(t, map[string]string{"A.md": "a", "locked/B.md": "b"}, nil)
	tv.scan()
	locked := filepath.Join(tv.dir, "locked")
	if err := os.Chmod(locked, 0); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(locked, 0o755) })
	if _, err := tv.v.runScan(t.Context(), false); err == nil {
		t.Fatal("a scan with an unreadable folder succeeded")
	}
	if tv.row("locked/B.md").ID == 0 {
		t.Error("a failed scan removed a note it could not see")
	}
	if s, _ := tv.v.Status(t.Context()); s.State != api.ScanStateFailed || s.Error == nil {
		t.Errorf("status after a failed scan = %+v", s)
	}
}

func TestScanRemovesWriteTemps(t *testing.T) {
	tv := newTestVault(t, map[string]string{
		"A.md": "a", "sub/.frostnotes-0123456789abcdef.tmp": "partial", "sub/.frostnotes-other.tmp": "someone else's",
	}, nil)
	tv.scan()
	if tv.exists("sub/.frostnotes-0123456789abcdef.tmp") {
		t.Error("a write's temporary file survived a scan")
	}
	if !tv.exists("sub/.frostnotes-other.tmp") {
		t.Error("a scan removed a dot-file that is not a write's")
	}
}

func TestReindexKeepsIDsAndVersions(t *testing.T) {
	tv := newTestVault(t, map[string]string{"A.md": "[[B]]", "B.md": "b"}, nil)
	tv.scan()
	a := tv.row("A.md")
	before, _ := tv.db.IndexSnapshot(t.Context(), tv.v.id)
	tv.takeEvents()
	if _, err := tv.v.runScan(t.Context(), true); err != nil {
		t.Fatal(err)
	}
	after, _ := tv.db.IndexSnapshot(t.Context(), tv.v.id)
	if !slices.Equal(before, after) || tv.row("A.md").ID != a.ID || len(tv.versions(a.ID)) != 1 {
		t.Errorf("reindex changed the index, an ID or the versions")
	}
	if got := tv.takeEvents(); len(got) != 0 {
		t.Errorf("reindex of unchanged files emitted %q", got)
	}
}

// equalsRebuild fails unless tv's index is what a fresh scan of its folder
// builds.
func (tv *testVault) equalsRebuild() {
	tv.t.Helper()
	got, err := tv.db.IndexSnapshot(tv.t.Context(), tv.v.id)
	if err != nil {
		tv.t.Fatal(err)
	}
	fresh := openTestVault(tv.t, tv.dir, tv.t.TempDir(), nil)
	fresh.scan()
	want, err := fresh.db.IndexSnapshot(tv.t.Context(), fresh.v.id)
	if err != nil {
		tv.t.Fatal(err)
	}
	if !slices.Equal(got, want) {
		tv.t.Fatalf("the index differs from a rebuild:\n%s\nwant\n%s", strings.Join(got, "\n"), strings.Join(want, "\n"))
	}
}

// A note that moves to a new name takes it as its title, in the index and
// in search, whoever moved it.
func TestMovesRetitleNotes(t *testing.T) {
	tv := newTestVault(t, map[string]string{"A.md": "a", "B.md": "[[A]]"}, nil)
	tv.scan()
	if _, err := tv.v.Move(t.Context(), "A.md", "Sub/Renamed.md", true, editor); err != nil {
		t.Fatal(err)
	}
	if err := os.Rename(filepath.Join(tv.dir, "B.md"), filepath.Join(tv.dir, "C.md")); err != nil {
		t.Fatal(err)
	}
	tv.scan()
	if n := tv.read("Sub/Renamed.md"); n.Title != "Renamed" {
		t.Errorf("title after a move = %q", n.Title)
	}
	tv.equalsRebuild()
}

// Files the settings come to ignore leave the index, and the API does not
// let them in again.
func TestFilesTheSettingsIgnoreLeaveTheIndex(t *testing.T) {
	tv := newTestVault(t, map[string]string{"A.md": "[[x]]", "Archive/x.md": "old"}, nil)
	tv.scan()
	tv.edit(".frostnotes/vault.yaml", "ignore: [\"Archive/\"]\n")
	tv.scan()
	if tv.row("Archive/x.md").ID != 0 {
		t.Fatal("an ignored note is still indexed")
	}
	if _, err := tv.v.Create(t.Context(), "Archive/new.md", []byte("x"), editor); !errors.Is(err, api.ErrInvalidParams) {
		t.Errorf("Create in an ignored folder = %v, want invalidParams", err)
	}
	if _, err := tv.v.Read(t.Context(), "Archive/x.md"); !errors.Is(err, api.ErrInvalidParams) {
		t.Errorf("Read of an ignored note = %v, want invalidParams", err)
	}
	if _, err := tv.v.Move(t.Context(), "A.md", "Archive/A.md", false, editor); !errors.Is(err, api.ErrInvalidParams) {
		t.Errorf("Move into an ignored folder = %v, want invalidParams", err)
	}
	tv.equalsRebuild()
}

// On a file system that ignores case, a path spelled otherwise than the
// file is not a second file. Checked with a stand-in: exactName refuses
// a spelling the folder does not list.
func TestExactName(t *testing.T) {
	tv := newTestVault(t, map[string]string{"Notes/Plan.md": "plan"}, nil)
	r, err := tv.v.open()
	if err != nil {
		t.Fatal(err)
	}
	defer r.Close()
	for p, want := range map[string]bool{"Notes/Plan.md": true, "Notes/plan.md": false, "notes/Plan.md": false, "Notes/None.md": false} {
		if got := exactName(r, p); got != want {
			t.Errorf("exactName(%s) = %v, want %v", p, got, want)
		}
	}
}

// An empty vault folder, as an unmounted share leaves, is not every file
// deleted.
func TestScanOfAnEmptyFolderRemovesNothing(t *testing.T) {
	tv := newTestVault(t, map[string]string{"A.md": "a", "sub/B.md": "b"}, nil)
	tv.scan()
	for _, p := range []string{"A.md", "sub/B.md"} {
		if err := os.Remove(filepath.Join(tv.dir, p)); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := tv.v.runScan(t.Context(), false); err == nil || !strings.Contains(err.Error(), "mounted") {
		t.Fatalf("a scan of an empty folder = %v, want an error asking whether it is mounted", err)
	}
	if tv.row("A.md").ID == 0 || tv.row("sub/B.md").ID == 0 {
		t.Error("a scan of an empty folder removed notes")
	}
}
