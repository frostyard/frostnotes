package store

import (
	"strings"
	"testing"
)

// A hub revision whose content is kept holds its blob, though no version
// or file names it any more, until retention marks it pruned (ADR-0019).
func TestKeptRevisionsHoldTheirBlobs(t *testing.T) {
	d, _ := openTest(t)
	ctx := t.Context()
	old, kept := strings.Repeat("a", 64), strings.Repeat("b", 64)
	err := d.Tx(ctx, func(tx *Tx) error {
		v, err := tx.InsertVault(ctx, "hub", "/v/hub")
		if err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx, `INSERT INTO hub_vaults (vault_id, sync_id, served_at) VALUES (?, 's', 'x')`, v.ID); err != nil {
			return err
		}
		for seq, hash := range []string{old, kept} {
			if _, err := tx.ExecContext(ctx, `INSERT INTO revisions (vault_id, seq, file_id, op, path, hash, size, base, source, author, at, pruned)
				VALUES (?, ?, 1, 'put', 'a.md', ?, 1, 0, 'editor', '', '2026-01-01T00:00:00.000Z', ?)`, v.ID, seq+1, hash, seq == 0); err != nil {
				return err
			}
		}
		if _, err := tx.AddVersion(ctx, Version{VaultID: v.ID, FileID: 1, Path: "a.md", Hash: kept, Size: 1, Source: "editor"}); err != nil {
			return err
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	used, err := d.UsedHashes(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if !used[kept] || used[old] {
		t.Fatalf("used = %v; want the kept revision's hash and not the pruned one's", used)
	}
	var freed []string
	err = d.Tx(ctx, func(tx *Tx) error {
		var err error
		freed, err = tx.PruneVersions(ctx, testNow.AddDate(1, 0, 0))
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(freed) != 0 {
		t.Fatalf("PruneVersions freed %v; a kept revision still names it", freed)
	}
}
