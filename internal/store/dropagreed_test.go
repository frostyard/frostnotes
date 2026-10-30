package store

import (
	"errors"
	"testing"

	"github.com/frostyard/frostnotes/api"
)

// DropAgreed keeps a file's agreed row while a conflict version of the
// file waits for an answered push, and drops it as DeleteAgreed does
// otherwise.
func TestDropAgreedWaitsForConflictVersions(t *testing.T) {
	dt := newDeviceTest(t)
	ctx := t.Context()
	var kept, plain File
	dt.tx(func(tx *Tx) {
		kept, plain = dt.file(tx, "a.md", "h1"), dt.file(tx, "b.md", "h2")
		dt.agree(tx, Agreed{HubFile: 7, FileID: kept.ID, Seq: 2, Revision: 2, Path: "a.md", Hash: "h1"})
		dt.agree(tx, Agreed{HubFile: 8, FileID: plain.ID, Seq: 3, Revision: 3, Path: "b.md", Hash: "h2"})
		if _, err := tx.AddVersion(ctx, Version{VaultID: dt.vault, FileID: kept.ID, Path: "a.md", Hash: "lost", Size: 1,
			Source: api.VersionSourceEditor, Conflict: true}); err != nil {
			t.Fatal(err)
		}
	})
	drop := func(hubFile int64) {
		t.Helper()
		dt.tx(func(tx *Tx) {
			if err := tx.DropAgreed(ctx, dt.vault, hubFile); err != nil {
				t.Fatal(err)
			}
		})
	}
	agreed := func(hubFile int64) bool {
		t.Helper()
		_, err := dt.d.AgreedByHubFile(ctx, dt.vault, hubFile)
		if err != nil && !errors.Is(err, ErrNotFound) {
			t.Fatal(err)
		}
		return err == nil
	}
	drop(8)
	drop(8) // a missing row is not an error
	if agreed(8) {
		t.Fatal("DropAgreed kept the row of a file without conflict versions")
	}
	drop(7)
	if !agreed(7) {
		t.Fatal("DropAgreed dropped the row of a file whose conflict version no push carried")
	}
	var p LocalPush
	dt.tx(func(tx *Tx) {
		var err error
		if p, err = tx.AddPush(ctx, LocalPush{VaultID: dt.vault, FileID: kept.ID, HubFile: 7, Base: 2, Path: "a.md", Hash: "lost",
			Conflict: true}); err != nil {
			t.Fatal(err)
		}
	})
	drop(7)
	if !agreed(7) {
		t.Fatal("DropAgreed dropped the row of a file whose conflict version's push has no answer")
	}
	dt.tx(func(tx *Tx) {
		if err := tx.AnswerPush(ctx, dt.vault, p.PushID, api.PushStatusAccepted, ""); err != nil {
			t.Fatal(err)
		}
	})
	drop(7)
	if agreed(7) {
		t.Fatal("DropAgreed kept the row of a file whose conflict version the hub has")
	}
}
