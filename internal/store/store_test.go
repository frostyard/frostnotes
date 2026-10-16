package store

import (
	"errors"
	"path/filepath"
	"strings"
	"testing"

	"github.com/frostyard/frostnotes/api"
)

func TestOpenMigratesAndReopens(t *testing.T) {
	path := filepath.Join(t.TempDir(), "notesd.db")
	d, err := Open(t.Context(), path)
	if err != nil {
		t.Fatal(err)
	}
	ms, err := loadMigrations()
	if err != nil {
		t.Fatal(err)
	}
	if v, err := d.SchemaVersion(t.Context()); err != nil || v != len(ms) {
		t.Fatalf("SchemaVersion = %d, %v; want %d", v, err, len(ms))
	}
	_ = d.Close()
	d, err = Open(t.Context(), path)
	if err != nil {
		t.Fatalf("reopen: %v", err)
	}
	_ = d.Close()
}

func TestOpenRefusesNewerSchema(t *testing.T) {
	path := filepath.Join(t.TempDir(), "notesd.db")
	d, err := Open(t.Context(), path)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := d.db.Exec(`INSERT INTO schema_migrations VALUES (999, 'future.sql', 'x')`); err != nil {
		t.Fatal(err)
	}
	_ = d.Close()
	if _, err := Open(t.Context(), path); err == nil || !strings.Contains(err.Error(), "newer") {
		t.Fatalf("Open = %v, want a newer-schema error", err)
	}
}

// M1's full-text index relies on modernc's FTS5 with diacritic folding and
// contentless deletes; this pins both before the index exists.
func TestFTS5Available(t *testing.T) {
	d, _ := openTest(t)
	ctx := t.Context()
	err := d.Tx(ctx, func(tx *Tx) error {
		if _, err := tx.ExecContext(ctx, `CREATE VIRTUAL TABLE probe_fts USING fts5(title, body,
			content='', contentless_delete=1, tokenize='unicode61 remove_diacritics 2')`); err != nil {
			return err
		}
		_, err := tx.ExecContext(ctx, `INSERT INTO probe_fts (rowid, title, body) VALUES (7, 'Café notes', 'naïve plans')`)
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	var rowid int64
	if err := d.db.QueryRowContext(ctx, `SELECT rowid FROM probe_fts WHERE probe_fts MATCH 'cafe AND naive'`).Scan(&rowid); err != nil || rowid != 7 {
		t.Fatalf("diacritic-folded match = %d, %v; want 7", rowid, err)
	}
	if err := d.Tx(ctx, func(tx *Tx) error {
		_, err := tx.ExecContext(ctx, `DELETE FROM probe_fts WHERE rowid = 7`)
		return err
	}); err != nil {
		t.Fatalf("contentless delete: %v", err)
	}
}

func TestTxEmitsDurableEventsAfterCommit(t *testing.T) {
	d, events := openTest(t)
	ctx := t.Context()
	err := d.Tx(ctx, func(tx *Tx) error {
		if err := tx.Emit(ctx, api.VaultRemoved{ID: 1}); err != nil {
			return err
		}
		return tx.Emit(ctx, api.VaultRemoved{ID: 2})
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(*events) != 2 || (*events)[0].Seq != 1 || (*events)[1].Seq != 2 || (*events)[0].Event != "vault.removed" {
		t.Fatalf("events = %+v", *events)
	}
	latest, err := d.LatestSeq(ctx)
	if err != nil || latest != 2 {
		t.Fatalf("LatestSeq = %d, %v", latest, err)
	}
	replay, err := d.ChangesSince(ctx, 1)
	if err != nil || len(replay) != 1 || replay[0].Seq != 2 || string(replay[0].Data) != `{"id":2}` {
		t.Fatalf("ChangesSince(1) = %+v, %v", replay, err)
	}
}

func TestTxRollbackDropsEvents(t *testing.T) {
	d, events := openTest(t)
	ctx := t.Context()
	boom := errors.New("boom")
	err := d.Tx(ctx, func(tx *Tx) error {
		if err := tx.Emit(ctx, api.VaultRemoved{ID: 1}); err != nil {
			return err
		}
		return boom
	})
	if !errors.Is(err, boom) {
		t.Fatalf("Tx = %v", err)
	}
	if len(*events) != 0 {
		t.Fatalf("rolled-back events delivered: %+v", *events)
	}
	if latest, _ := d.LatestSeq(ctx); latest != 0 {
		t.Fatalf("rolled-back event kept seq %d", latest)
	}
}

func TestPruneKeepsLatestSeq(t *testing.T) {
	d, _ := openTest(t)
	ctx := t.Context()
	for i := range 5 {
		if err := d.Tx(ctx, func(tx *Tx) error { return tx.Emit(ctx, api.VaultRemoved{ID: int64(i)}) }); err != nil {
			t.Fatal(err)
		}
	}
	if err := d.PruneChanges(ctx, 2); err != nil {
		t.Fatal(err)
	}
	oldest, _ := d.OldestSeq(ctx)
	latest, _ := d.LatestSeq(ctx)
	if oldest != 4 || latest != 5 {
		t.Fatalf("after prune oldest=%d latest=%d, want 4 and 5", oldest, latest)
	}
}

// The index's rows leave with their note, file or vault, the search row
// through a trigger that cascades fire too; a link to a deleted file
// becomes unresolved; versions outlive their file.
func TestSchemaCascades(t *testing.T) {
	d, _ := openTest(t)
	ctx := t.Context()
	exec := func(q string, args ...any) {
		t.Helper()
		if err := d.Tx(ctx, func(tx *Tx) error { _, err := tx.ExecContext(ctx, q, args...); return err }); err != nil {
			t.Fatalf("%s: %v", q, err)
		}
	}
	count := func(table string) int {
		t.Helper()
		var n int
		if err := d.db.QueryRowContext(ctx, `SELECT count(*) FROM `+table).Scan(&n); err != nil {
			t.Fatal(err)
		}
		return n
	}
	exec(`INSERT INTO vaults (id, name, path, created_at) VALUES (1, 'v', '/v', 'x')`)
	exec(`INSERT INTO files (id, vault_id, path, name, kind, size, mtime, hash) VALUES
		(1, 1, 'A.md', 'a.md', 'note', 1, 1, 'h1'), (2, 1, 'B.md', 'b.md', 'note', 1, 1, 'h2')`)
	exec(`INSERT INTO notes (file_id, title, props, prop_error, aliases, words) VALUES
		(1, 'A', '{}', '', '[]', 0), (2, 'B', '{}', '', '[]', 0)`)
	exec(`INSERT INTO links (file_id, ord, kind, target, subpath, alias, embed, key, start, stop, line, name, dest_id)
		VALUES (1, 0, 'wiki', 'B', '', '', 0, '', 0, 5, 1, 'b', 2)`)
	exec(`INSERT INTO tags (file_id, folded, name, front, line) VALUES (1, 'x', 'X', 0, 1)`)
	exec(`INSERT INTO notes_fts (rowid, title, aliases, body, props) VALUES (1, 'A', '', 'apple', ''), (2, 'B', '', 'butter', '')`)
	exec(`INSERT INTO versions (vault_id, file_id, path, hash, size, at, source, author) VALUES (1, 2, 'B.md', 'h2', 1, 'x', 'outside', '')`)

	exec(`DELETE FROM files WHERE id = 2`)
	var dest *int64
	if err := d.db.QueryRowContext(ctx, `SELECT dest_id FROM links`).Scan(&dest); err != nil || dest != nil {
		t.Fatalf("link to a deleted file: dest_id %v, %v; want NULL", dest, err)
	}
	if n := count(`notes_fts WHERE notes_fts MATCH 'butter'`); n != 0 {
		t.Fatalf("a deleted file's search row remains")
	}
	if n := count(`versions`); n != 1 {
		t.Fatalf("versions = %d, want the deleted file's version kept", n)
	}

	exec(`DELETE FROM vaults WHERE id = 1`)
	for _, table := range []string{"files", "notes", "links", "tags", "notes_fts", "versions"} {
		if n := count(table); n != 0 {
			t.Errorf("%s keeps %d rows after its vault is removed", table, n)
		}
	}
}
