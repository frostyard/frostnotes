package store

// CONTRACT TEST for task card T-0014 (docs/tasks). Do not edit.

import (
	"errors"
	"testing"
	"time"
)

func TestFileKindAndName(t *testing.T) {
	for _, tc := range []struct {
		path string
		kind FileKind
		name string
	}{
		{"Plan.md", FileNote, "plan.md"},
		{"Orchard/North Block/Plan.md", FileNote, "plan.md"},
		{"Notes.MD", FileAttachment, "notes.md"},
		{"media/Soil-Map.PNG", FileAttachment, "soil-map.png"},
		{"Members/Ilsa/CAFÉ.md", FileNote, "café.md"},
		{"x.md/file", FileAttachment, "file"},
	} {
		if k := (File{Path: tc.path}).Kind(); k != tc.kind {
			t.Errorf("Kind(%q) = %q, want %q", tc.path, k, tc.kind)
		}
		if n := FileName(tc.path); n != tc.name {
			t.Errorf("FileName(%q) = %q, want %q", tc.path, n, tc.name)
		}
	}
}

// fileTest is a database with one vault and helpers over a transaction.
type fileTest struct {
	t     *testing.T
	d     *DB
	vault int64
}

func newFileTest(t *testing.T) *fileTest {
	d, _ := openTest(t)
	ft := &fileTest{t: t, d: d}
	ft.tx(func(tx *Tx) {
		v, err := tx.InsertVault(t.Context(), "v", "/v")
		if err != nil {
			t.Fatal(err)
		}
		ft.vault = v.ID
	})
	return ft
}

func (ft *fileTest) tx(fn func(*Tx)) {
	ft.t.Helper()
	if err := ft.d.Tx(ft.t.Context(), func(tx *Tx) error { fn(tx); return nil }); err != nil {
		ft.t.Fatal(err)
	}
}

// put records a file, and a notes row for a note, as a scan would.
func (ft *fileTest) put(tx *Tx, path string) File {
	ft.t.Helper()
	f, err := tx.PutFile(ft.t.Context(), File{VaultID: ft.vault, Path: path, Size: 1, ModTime: time.Unix(1, 0), Hash: "h"})
	if err != nil {
		ft.t.Fatalf("PutFile(%s): %v", path, err)
	}
	if f.Kind() == FileNote {
		if _, err := tx.ExecContext(ft.t.Context(), `INSERT OR IGNORE INTO notes (file_id, title, props, prop_error, aliases, words)
			VALUES (?, 't', '{}', '', '[]', 0)`, f.ID); err != nil {
			ft.t.Fatal(err)
		}
	}
	return f
}

// link adds an unresolved link row, as IndexNote would before resolving.
func (ft *fileTest) link(tx *Tx, from File, ord int, target, name string) {
	ft.t.Helper()
	if _, err := tx.ExecContext(ft.t.Context(), `INSERT INTO links (file_id, ord, kind, target, subpath, alias, embed, key,
		start, stop, line, name, dest_id) VALUES (?, ?, 'wiki', ?, '', '', 0, '', 0, 1, 1, ?, NULL)`, from.ID, ord, target, name); err != nil {
		ft.t.Fatal(err)
	}
}

// dest is the path link ord of from resolves to, or "".
func (ft *fileTest) dest(from File, ord int) string {
	ft.t.Helper()
	var p *string
	err := ft.d.db.QueryRowContext(ft.t.Context(), `SELECT f.path FROM links l LEFT JOIN files f ON f.id = l.dest_id
		WHERE l.file_id = ? AND l.ord = ?`, from.ID, ord).Scan(&p)
	if err != nil {
		ft.t.Fatal(err)
	}
	if p == nil {
		return ""
	}
	return *p
}

func TestPutFile(t *testing.T) {
	ft := newFileTest(t)
	ctx := t.Context()
	mtime := time.Unix(1_760_000_000, 123_456_789)
	var a File
	ft.tx(func(tx *Tx) {
		var err error
		a, err = tx.PutFile(ctx, File{ID: 99, VaultID: ft.vault, Path: "a.md", Size: 3, ModTime: mtime, Hash: "h1"})
		if err != nil {
			t.Fatal(err)
		}
		if a.ID == 0 || a.ID == 99 || a.Path != "a.md" || a.Size != 3 || !a.ModTime.Equal(mtime) || a.Hash != "h1" {
			t.Fatalf("PutFile = %+v", a)
		}
		if got, err := tx.FileByPath(ctx, ft.vault, "a.md"); err != nil || got.ID != a.ID {
			t.Fatalf("Tx.FileByPath inside the transaction = %+v, %v", got, err)
		}
	})
	got, err := ft.d.FileByPath(ctx, ft.vault, "a.md")
	if err != nil || got.ID != a.ID || !got.ModTime.Equal(mtime) || got.ModTime.Nanosecond() != 123_456_789 {
		t.Fatalf("FileByPath = %+v, %v; want %+v to the nanosecond", got, err, a)
	}

	later := mtime.Add(time.Second)
	ft.tx(func(tx *Tx) {
		b, err := tx.PutFile(ctx, File{VaultID: ft.vault, Path: "a.md", Size: 5, ModTime: later, Hash: "h2"})
		if err != nil || b.ID != a.ID || b.Size != 5 || !b.ModTime.Equal(later) || b.Hash != "h2" {
			t.Fatalf("PutFile on an existing path = %+v, %v; want the same ID, updated", b, err)
		}
		ft.put(tx, "B.md")
		ft.put(tx, "media/map.png")
	})

	files, err := ft.d.Files(ctx, ft.vault)
	if err != nil {
		t.Fatal(err)
	}
	var paths []string
	for _, f := range files {
		paths = append(paths, f.Path)
	}
	if want := []string{"B.md", "a.md", "media/map.png"}; !equalStrings(paths, want) {
		t.Fatalf("Files = %q, want %q (byte order)", paths, want)
	}
	if notes, attachments, err := ft.d.FileCounts(ctx, ft.vault); err != nil || notes != 2 || attachments != 1 {
		t.Fatalf("FileCounts = %d, %d, %v; want 2, 1", notes, attachments, err)
	}
	if _, err := ft.d.FileByPath(ctx, ft.vault, "A.md"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("FileByPath of another case = %v, want ErrNotFound", err)
	}
	if files, err := ft.d.Files(ctx, ft.vault+1); err != nil || len(files) != 0 {
		t.Fatalf("Files of another vault = %+v, %v", files, err)
	}
}

func equalStrings(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func TestPutFileResolvesWaitingLinks(t *testing.T) {
	ft := newFileTest(t)
	var inbox, index File
	ft.tx(func(tx *Tx) {
		inbox, index = ft.put(tx, "Inbox.md"), ft.put(tx, "Orchard/Index.md")
		ft.link(tx, inbox, 0, "Plan", "plan")
		ft.link(tx, inbox, 1, "Plan.md", "plan")
		ft.link(tx, index, 0, "Plan", "plan")
		ft.link(tx, inbox, 2, "soil-map.png", "soil-map.png")
		ft.put(tx, "Plan.md")
	})
	for _, c := range []struct {
		from File
		ord  int
		want string
	}{{inbox, 0, "Plan.md"}, {inbox, 1, "Plan.md"}, {index, 0, "Plan.md"}, {inbox, 2, ""}} {
		if got := ft.dest(c.from, c.ord); got != c.want {
			t.Errorf("after Plan.md: link %d of %s resolves to %q, want %q", c.ord, c.from.Path, got, c.want)
		}
	}
	ft.tx(func(tx *Tx) {
		ft.put(tx, "Orchard/Plan.md")
		ft.put(tx, "Orchard/media/soil-map.png")
	})
	if got := ft.dest(index, 0); got != "Orchard/Plan.md" {
		t.Errorf("Orchard/Index.md's [[Plan]] = %q after Orchard/Plan.md, want the one in its folder", got)
	}
	if got := ft.dest(inbox, 0); got != "Plan.md" {
		t.Errorf("Inbox.md's [[Plan]] = %q, want Plan.md still", got)
	}
	if got := ft.dest(inbox, 2); got != "Orchard/media/soil-map.png" {
		t.Errorf("Inbox.md's [[soil-map.png]] = %q", got)
	}
}

func TestResolveLink(t *testing.T) {
	ft := newFileTest(t)
	ctx := t.Context()
	ft.tx(func(tx *Tx) {
		for _, p := range []string{"Inbox.md", "Plan.md", "Orchard/Plan.md", "Orchard/Index.md", "Recipes/Apple Butter.md"} {
			ft.put(tx, p)
		}
		for _, c := range []struct{ source, target, want string }{
			{"Orchard/Index.md", "Plan", "Orchard/Plan.md"},
			{"Inbox.md", "Plan", "Plan.md"},
			{"Inbox.md", "apple butter", "Recipes/Apple Butter.md"},
			{"Inbox.md", "", "Inbox.md"},
			{"Inbox.md", "Nowhere", ""},
			{"Inbox.md", "../Plan", ""},
		} {
			f, ok, err := tx.ResolveLink(ctx, ft.vault, c.source, c.target)
			if err != nil {
				t.Fatal(err)
			}
			if ok != (c.want != "") || f.Path != c.want || (ok && f.ID == 0) {
				t.Errorf("ResolveLink(%q, %q) = %+v, %t; want %q", c.source, c.target, f, ok, c.want)
			}
		}
	})
}

func TestMoveFile(t *testing.T) {
	ft := newFileTest(t)
	ctx := t.Context()
	var inbox, index, plan File
	ft.tx(func(tx *Tx) {
		// Links come first: putting a file resolves the links waiting for it.
		inbox, index = ft.put(tx, "Inbox.md"), ft.put(tx, "Orchard/Index.md")
		ft.link(tx, inbox, 0, "Orchard/Plan", "plan")
		ft.link(tx, index, 0, "Plan", "plan")
		ft.link(tx, index, 1, "Harvest", "harvest")
		ft.put(tx, "Plan.md")
		plan = ft.put(tx, "Orchard/Plan.md")
		ft.put(tx, "Garden/Plan.md")
	})
	if got := ft.dest(index, 0); got != "Orchard/Plan.md" {
		t.Fatalf("setup: [[Plan]] from Orchard/Index.md = %q", got)
	}

	ft.tx(func(tx *Tx) {
		moved, err := tx.MoveFile(ctx, plan.ID, "Orchard/Harvest.md")
		if err != nil || moved.ID != plan.ID || moved.Path != "Orchard/Harvest.md" {
			t.Fatalf("MoveFile = %+v, %v", moved, err)
		}
	})
	if _, err := ft.d.FileByPath(ctx, ft.vault, "Orchard/Plan.md"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("old path: %v, want ErrNotFound", err)
	}
	for _, c := range []struct {
		from File
		ord  int
		want string
	}{{inbox, 0, ""}, {index, 0, "Plan.md"}, {index, 1, "Orchard/Harvest.md"}} {
		if got := ft.dest(c.from, c.ord); got != c.want {
			t.Errorf("after the move: link %d of %s = %q, want %q", c.ord, c.from.Path, got, c.want)
		}
	}

	// A moved note's own links resolve from its new folder.
	ft.tx(func(tx *Tx) {
		if _, err := tx.MoveFile(ctx, index.ID, "Garden/Index.md"); err != nil {
			t.Fatal(err)
		}
	})
	if got := ft.dest(index, 0); got != "Garden/Plan.md" {
		t.Errorf("[[Plan]] from Garden/Index.md = %q, want Garden/Plan.md", got)
	}

	err := ft.d.Tx(ctx, func(tx *Tx) error { _, err := tx.MoveFile(ctx, index.ID, "Plan.md"); return err })
	if !errors.Is(err, ErrConflict) {
		t.Errorf("MoveFile onto a taken path = %v, want ErrConflict", err)
	}
	err = ft.d.Tx(ctx, func(tx *Tx) error { _, err := tx.MoveFile(ctx, 999, "x.md"); return err })
	if !errors.Is(err, ErrNotFound) {
		t.Errorf("MoveFile of a missing file = %v, want ErrNotFound", err)
	}

	// A note renamed to an attachment is no longer a note.
	ft.tx(func(tx *Tx) {
		if _, err := tx.MoveFile(ctx, inbox.ID, "Inbox.txt"); err != nil {
			t.Fatal(err)
		}
	})
	var n int
	if err := ft.d.db.QueryRowContext(ctx, `SELECT count(*) FROM notes WHERE file_id = ?`, inbox.ID).Scan(&n); err != nil || n != 0 {
		t.Errorf("notes rows of a note moved to Inbox.txt = %d, %v; want 0", n, err)
	}
	if notes, attachments, _ := ft.d.FileCounts(ctx, ft.vault); notes != 4 || attachments != 1 {
		t.Errorf("FileCounts = %d, %d; want 4, 1", notes, attachments)
	}
}

func TestDeleteFile(t *testing.T) {
	ft := newFileTest(t)
	ctx := t.Context()
	var index, plan File
	ft.tx(func(tx *Tx) {
		index = ft.put(tx, "Orchard/Index.md")
		ft.link(tx, index, 0, "Plan", "plan")
		ft.put(tx, "Plan.md")
		plan = ft.put(tx, "Orchard/Plan.md")
		ft.link(tx, plan, 0, "Index", "index")
		if _, err := tx.ExecContext(ctx, `INSERT INTO versions (vault_id, file_id, path, hash, size, at, source, author)
			VALUES (?, ?, 'Orchard/Plan.md', 'h', 1, 'x', 'outside', '')`, ft.vault, plan.ID); err != nil {
			t.Fatal(err)
		}
	})
	if got := ft.dest(index, 0); got != "Orchard/Plan.md" {
		t.Fatalf("setup: [[Plan]] from Orchard/Index.md = %q", got)
	}
	ft.tx(func(tx *Tx) {
		if err := tx.DeleteFile(ctx, plan.ID); err != nil {
			t.Fatal(err)
		}
	})
	if got := ft.dest(index, 0); got != "Plan.md" {
		t.Errorf("[[Plan]] after Orchard/Plan.md is deleted = %q, want Plan.md", got)
	}
	var links, versions int
	if err := ft.d.db.QueryRowContext(ctx, `SELECT (SELECT count(*) FROM links WHERE file_id = ?), (SELECT count(*) FROM versions)`,
		plan.ID).Scan(&links, &versions); err != nil || links != 0 || versions != 1 {
		t.Errorf("after delete: %d of its links, %d versions, %v; want 0 and 1", links, versions, err)
	}
	if _, err := ft.d.FileByPath(ctx, ft.vault, "Orchard/Plan.md"); !errors.Is(err, ErrNotFound) {
		t.Errorf("FileByPath after delete = %v", err)
	}
	err := ft.d.Tx(ctx, func(tx *Tx) error { return tx.DeleteFile(ctx, plan.ID) })
	if !errors.Is(err, ErrNotFound) {
		t.Errorf("second delete = %v, want ErrNotFound", err)
	}
}
