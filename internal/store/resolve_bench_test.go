package store

import (
	"fmt"
	"path/filepath"
	"testing"
	"time"

	"github.com/frostyard/frostnotes/internal/markdown"
)

// openBench opens a migrated database in a temp dir with a fixed clock, as
// openTest does for a test.
func openBench(b *testing.B) *DB {
	b.Helper()
	d, err := Open(b.Context(), filepath.Join(b.TempDir(), "notesd.db"))
	if err != nil {
		b.Fatal(err)
	}
	b.Cleanup(func() { _ = d.Close() })
	d.Now = func() time.Time { return testNow }
	return d
}

// benchVault records a vault and one file at each path, and returns the
// vault's ID.
func benchVault(b *testing.B, d *DB, paths ...string) (int64, []File) {
	b.Helper()
	ctx := b.Context()
	var vaultID int64
	var files []File
	err := d.Tx(ctx, func(tx *Tx) error {
		v, err := tx.InsertVault(ctx, "v", "/v")
		if err != nil {
			return err
		}
		vaultID = v.ID
		for _, p := range paths {
			f, err := tx.PutFile(ctx, File{VaultID: vaultID, Path: p, Size: 1, ModTime: time.Unix(1, 0), Hash: "h"})
			if err != nil {
				return err
			}
			files = append(files, f)
		}
		return nil
	})
	if err != nil {
		b.Fatal(err)
	}
	return vaultID, files
}

// benchIndex indexes each note with content in one transaction.
func benchIndex(b *testing.B, d *DB, notes []File, content []byte) {
	b.Helper()
	ctx := b.Context()
	doc := markdown.Parse(content)
	err := d.Tx(ctx, func(tx *Tx) error {
		for _, f := range notes {
			if err := tx.IndexNote(ctx, f, content, doc); err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		b.Fatal(err)
	}
}

// BenchmarkResolveFileLinks measures one move of a note that 300 notes link
// to 3 times each, and the move back: every affected link is re-resolved
// against the same 2 names.
func BenchmarkResolveFileLinks(b *testing.B) {
	d := openBench(b)
	ctx := b.Context()
	paths := []string{"Garden/Plan.md"}
	for i := range 300 {
		paths = append(paths, fmt.Sprintf("Folder%d/Note%d.md", i%5, i))
	}
	_, files := benchVault(b, d, paths...)
	planID := files[0].ID
	benchIndex(b, d, files[1:], []byte("[[Plan]] [[Plan]] [[Plan]]\n"))

	for b.Loop() {
		err := d.Tx(ctx, func(tx *Tx) error {
			if _, err := tx.MoveFile(ctx, planID, "Garden/Calendar.md"); err != nil {
				return err
			}
			_, err := tx.MoveFile(ctx, planID, "Garden/Plan.md")
			return err
		})
		if err != nil {
			b.Fatal(err)
		}
	}
}

// BenchmarkIndexNoteRepeatedLinks measures re-indexing one note whose 50
// links name 5 targets.
func BenchmarkIndexNoteRepeatedLinks(b *testing.B) {
	d := openBench(b)
	ctx := b.Context()
	targets := []string{"Plan", "Harvest", "Index", "Log", "Notes"}
	paths := []string{"Inbox.md"}
	for _, name := range targets {
		paths = append(paths, "Garden/"+name+".md")
	}
	_, files := benchVault(b, d, paths...)
	note := files[0]
	var body []byte
	for i := range 50 {
		body = fmt.Appendf(body, "[[%s]] ", targets[i%len(targets)])
	}
	doc := markdown.Parse(body)

	for b.Loop() {
		if err := d.Tx(ctx, func(tx *Tx) error {
			return tx.IndexNote(ctx, note, body, doc)
		}); err != nil {
			b.Fatal(err)
		}
	}
}
