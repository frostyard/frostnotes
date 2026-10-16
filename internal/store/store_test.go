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
