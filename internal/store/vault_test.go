package store

import (
	"errors"
	"testing"
)

func TestVaultsRoundTrip(t *testing.T) {
	d, events := openTest(t)
	ctx := t.Context()
	var notes, work Vault
	err := d.Tx(ctx, func(tx *Tx) error {
		var err error
		if work, err = tx.InsertVault(ctx, "work", "/v/work"); err != nil {
			return err
		}
		notes, err = tx.InsertVault(ctx, "Notes", "/v/notes")
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	if !notes.CreatedAt.Equal(testNow) || notes.ID == work.ID {
		t.Fatalf("inserted %+v and %+v", notes, work)
	}
	got, err := d.GetVault(ctx, work.ID)
	if err != nil || got != work {
		t.Fatalf("GetVault = %+v, %v; want %+v", got, err, work)
	}
	list, err := d.ListVaults(ctx)
	if err != nil || len(list) != 2 || list[0] != notes || list[1] != work {
		t.Fatalf("ListVaults = %+v, %v; want Notes then work", list, err)
	}
	if len(*events) != 2 || (*events)[0].Event != "vault.added" {
		t.Fatalf("events = %+v", *events)
	}
}

func TestDuplicateVaultPathConflicts(t *testing.T) {
	d, _ := openTest(t)
	ctx := t.Context()
	insert := func(tx *Tx) error { _, err := tx.InsertVault(ctx, "a", "/v/a"); return err }
	if err := d.Tx(ctx, insert); err != nil {
		t.Fatal(err)
	}
	if err := d.Tx(ctx, insert); !errors.Is(err, ErrConflict) {
		t.Fatalf("duplicate path = %v, want ErrConflict", err)
	}
}

func TestDeleteVault(t *testing.T) {
	d, events := openTest(t)
	ctx := t.Context()
	var v Vault
	if err := d.Tx(ctx, func(tx *Tx) error { var err error; v, err = tx.InsertVault(ctx, "a", "/v/a"); return err }); err != nil {
		t.Fatal(err)
	}
	if err := d.Tx(ctx, func(tx *Tx) error { return tx.DeleteVault(ctx, v.ID) }); err != nil {
		t.Fatal(err)
	}
	if _, err := d.GetVault(ctx, v.ID); !errors.Is(err, ErrNotFound) {
		t.Fatalf("GetVault after delete = %v, want ErrNotFound", err)
	}
	if err := d.Tx(ctx, func(tx *Tx) error { return tx.DeleteVault(ctx, v.ID) }); !errors.Is(err, ErrNotFound) {
		t.Fatalf("second delete = %v, want ErrNotFound", err)
	}
	if last := (*events)[len(*events)-1]; last.Event != "vault.removed" {
		t.Fatalf("last event = %+v", last)
	}
}
