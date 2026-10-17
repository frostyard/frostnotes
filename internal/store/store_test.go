package store

import (
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
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
	checkRecordedChecksums(t, d, ms)
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
	if _, err := d.db.Exec(`INSERT INTO schema_migrations (version, name, applied_at, checksum)
		VALUES (999, 'future.sql', 'x', 'x')`); err != nil {
		t.Fatal(err)
	}
	_ = d.Close()
	if _, err := Open(t.Context(), path); err == nil || !strings.Contains(err.Error(), "newer") {
		t.Fatalf("Open = %v, want a newer-schema error", err)
	}
}

// A database that applied an earlier text of 0001_init.sql, edited in place
// since, is refused with what to do rather than run without the tables
// the code expects.
func TestOpenRefusesEditedMigration(t *testing.T) {
	path := filepath.Join(t.TempDir(), "notesd.db")
	d, err := Open(t.Context(), path)
	if err != nil {
		t.Fatal(err)
	}
	earlier := sha256.Sum256([]byte("-- an earlier 0001_init.sql"))
	if _, err := d.db.Exec(`UPDATE schema_migrations SET checksum = ? WHERE version = 1`,
		hex.EncodeToString(earlier[:])); err != nil {
		t.Fatal(err)
	}
	_ = d.Close()
	_, err = Open(t.Context(), path)
	checkStaleSchemaError(t, err, path)
}

// A database from before schema_migrations kept checksums opens and adopts
// them when its schema is what the migrations create today.
func TestOpenAdoptsChecksumsOfCurrentSchema(t *testing.T) {
	ms, err := loadMigrations()
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(t.TempDir(), "notesd.db")
	writeUncheckedDB(t, path, ms)
	d, err := Open(t.Context(), path)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	checkRecordedChecksums(t, d, ms)
	_ = d.Close()
	d, err = Open(t.Context(), path)
	if err != nil {
		t.Fatalf("reopen: %v", err)
	}
	_ = d.Close()
}

// The database that ran without M1's tables: M0's notesd applied an
// earlier 0001_init.sql, before checksums were recorded.
func TestOpenRefusesUncheckedStaleSchema(t *testing.T) {
	path := filepath.Join(t.TempDir(), "notesd.db")
	writeUncheckedDB(t, path, []migration{{version: 1, name: "0001_init.sql", sql: `CREATE TABLE vaults (
		id INTEGER PRIMARY KEY AUTOINCREMENT, name TEXT NOT NULL, path TEXT NOT NULL UNIQUE,
		created_at TEXT NOT NULL) STRICT`}})
	_, err := Open(t.Context(), path)
	checkStaleSchemaError(t, err, path)
}

// writeUncheckedDB writes a database at path that applied ms as notesd did
// before schema_migrations had a checksum column.
func writeUncheckedDB(t *testing.T, path string, ms []migration) {
	t.Helper()
	db, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = db.Close() }()
	if _, err := db.ExecContext(t.Context(), `CREATE TABLE schema_migrations (
		version INTEGER PRIMARY KEY, name TEXT NOT NULL, applied_at TEXT NOT NULL) STRICT`); err != nil {
		t.Fatal(err)
	}
	for _, m := range ms {
		if _, err := db.ExecContext(t.Context(), m.sql); err != nil {
			t.Fatalf("%s: %v", m.name, err)
		}
		if _, err := db.ExecContext(t.Context(), `INSERT INTO schema_migrations VALUES (?, ?, 'x')`,
			m.version, m.name); err != nil {
			t.Fatal(err)
		}
	}
}

func checkRecordedChecksums(t *testing.T, d *DB, ms []migration) {
	t.Helper()
	for _, m := range ms {
		var sum string
		if err := d.db.QueryRowContext(t.Context(), `SELECT checksum FROM schema_migrations WHERE version = ?`,
			m.version).Scan(&sum); err != nil || sum != m.checksum {
			t.Fatalf("%s: recorded checksum %q, %v; want %q", m.name, sum, err, m.checksum)
		}
	}
}

func checkStaleSchemaError(t *testing.T, err error, path string) {
	t.Helper()
	if err == nil {
		t.Fatal("Open succeeded; want the database refused")
	}
	for _, want := range []string{path, "0001_init.sql", "predates an in-place edit", "move it aside"} {
		if !strings.Contains(err.Error(), want) {
			t.Fatalf("Open = %v; want it to name %q", err, want)
		}
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
	exec(`INSERT INTO tags (file_id, ord, folded, name, front, line) VALUES (1, 0, 'x', 'X', 0, 1)`)
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

// Migrations are append-only (plan 0004), and notesd refuses a database
// that applied another text of a migration, so editing a released file,
// even a comment in it, strands every database that applied it. Add a
// migration instead; pin its checksum here when it lands.
func TestReleasedMigrationsKeepTheirText(t *testing.T) {
	pinned := map[string]string{
		"0001_init.sql":                 "617de380f7086b64a19b197f0bf34c8c967fae4f0d91bcc4802489f47d2b409c",
		"0002_sync.sql":                 "46eea9c7b99b9c157624d951dffed855891aede980eaae1fad492dfdd67e9cc7",
		"0003_sync_conflict_pushes.sql": "7c95bd389e9cba771256a8016d51abe712604cf954e961b597bf1f758b642ed1",
	}
	ms, err := loadMigrations()
	if err != nil {
		t.Fatal(err)
	}
	for _, m := range ms {
		want, ok := pinned[m.name]
		switch {
		case !ok:
			t.Errorf("%s is not pinned: add its checksum %s to this test", m.name, m.checksum)
		case m.checksum != want:
			t.Errorf("%s changed (checksum %s, released as %s): restore it and add a new migration", m.name, m.checksum, want)
		}
	}
}
