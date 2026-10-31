package store

import (
	"testing"

	"github.com/frostyard/frostnotes/api"
)

// A head being applied is recorded with the agreed row's seq at that
// time, replaced by a later head of the same file, deleted, and dropped
// with its remote (ADR-0034).
func TestApplyingRecordsAHeadUntilItIsAgreed(t *testing.T) {
	dt := newDeviceTest(t)
	ctx := t.Context()
	var f File
	dt.tx(func(tx *Tx) {
		f = dt.file(tx, "f.md", "h1")
		dt.agree(tx, Agreed{HubFile: 7, FileID: f.ID, Seq: 3, Revision: 3, Path: "f.md", Hash: "h1"})
	})
	list := func() []Applying {
		t.Helper()
		rows, err := dt.d.Applying(ctx, dt.vault)
		if err != nil {
			t.Fatal(err)
		}
		return rows
	}
	if rows := list(); rows == nil || len(rows) != 0 {
		t.Fatalf("Applying = %#v, want a non-nil empty slice", rows)
	}
	put := Applying{VaultID: dt.vault, HubFile: 7, FileID: f.ID, From: "f.md", FromHash: "h1", BaseSeq: 99, Seq: 5, Revision: 5,
		Path: "g.md", Hash: "h2", At: "days/g.md", Conflicted: true, PushID: 4, Status: api.PushStatusMerged}
	dt.tx(func(tx *Tx) {
		got, err := tx.PutApplying(ctx, put)
		if err != nil {
			t.Fatal(err)
		}
		if got.BaseSeq != 3 {
			t.Fatalf("BaseSeq = %d, want the agreed row's 3", got.BaseSeq)
		}
		// A create's head has no agreed row: BaseSeq 0.
		if got, err := tx.PutApplying(ctx, Applying{VaultID: dt.vault, HubFile: 8, From: "n.md", Seq: 6, Revision: 6,
			Path: "n.md", Hash: "h3"}); err != nil || got.BaseSeq != 0 {
			t.Fatalf("PutApplying of a create = %+v, %v; want BaseSeq 0", got, err)
		}
	})
	rows := list()
	want := put
	want.BaseSeq = 3
	if len(rows) != 2 || rows[0] != want || rows[1].HubFile != 8 {
		t.Fatalf("Applying = %+v, want %+v and hub file 8's", rows, want)
	}
	if a := rows[0].Agreed(f.ID); a != (Agreed{VaultID: dt.vault, HubFile: 7, FileID: f.ID, Seq: 5, Revision: 5, Path: "g.md",
		Hash: "h2", Conflicted: true}) {
		t.Fatalf("Agreed = %+v", a)
	}
	// A later head of the file replaces the row.
	dt.tx(func(tx *Tx) {
		if _, err := tx.PutApplying(ctx, Applying{VaultID: dt.vault, HubFile: 7, FileID: f.ID, From: "f.md", Seq: 9,
			Revision: 9, Path: "f.md", Deleted: true}); err != nil {
			t.Fatal(err)
		}
	})
	if rows := list(); len(rows) != 2 || !rows[0].Deleted || rows[0].Seq != 9 || rows[0].PushID != 0 || rows[0].Status != "" {
		t.Fatalf("Applying after a later head = %+v", rows)
	}
	dt.tx(func(tx *Tx) {
		if err := tx.DeleteApplying(ctx, dt.vault, 7); err != nil {
			t.Fatal(err)
		}
		if err := tx.DeleteApplying(ctx, dt.vault, 70); err != nil {
			t.Fatalf("DeleteApplying of a missing row: %v", err)
		}
		if ok, err := tx.FileExists(ctx, f.ID); err != nil || !ok {
			t.Fatalf("FileExists(%d) = %v, %v", f.ID, ok, err)
		}
		if ok, err := tx.FileExists(ctx, f.ID+100); err != nil || ok {
			t.Fatalf("FileExists of a missing file = %v, %v", ok, err)
		}
	})
	if rows := list(); len(rows) != 1 || rows[0].HubFile != 8 {
		t.Fatalf("Applying after a delete = %+v, want hub file 8's", rows)
	}
	dt.tx(func(tx *Tx) {
		if err := tx.DeleteRemote(ctx, dt.vault); err != nil {
			t.Fatal(err)
		}
	})
	if rows := list(); len(rows) != 0 {
		t.Fatalf("Applying after the remote is gone = %+v", rows)
	}
}
