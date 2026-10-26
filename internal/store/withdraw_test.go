package store

import (
	"errors"
	"testing"
)

// A device that never made a request is deleted whole; one that made a
// request, or was revoked, stays, since the hub's revisions may name it.
func TestDeleteUnseenDevice(t *testing.T) {
	d, _ := openTest(t)
	ctx := t.Context()
	var unseen, seen, revoked HubDevice
	err := d.Tx(ctx, func(tx *Tx) error {
		var err error
		if unseen, err = tx.AddDevice(ctx, "unseen", "hash-unseen"); err != nil {
			return err
		}
		if seen, err = tx.AddDevice(ctx, "seen", "hash-seen"); err != nil {
			return err
		}
		if revoked, err = tx.AddDevice(ctx, "revoked", "hash-revoked"); err != nil {
			return err
		}
		if _, err := tx.TouchDevice(ctx, seen.ID, testNow); err != nil {
			return err
		}
		return tx.RevokeDevice(ctx, revoked.ID)
	})
	if err != nil {
		t.Fatal(err)
	}
	del := func(id int64) bool {
		t.Helper()
		var ok bool
		if err := d.Tx(ctx, func(tx *Tx) error {
			var err error
			ok, err = tx.DeleteUnseenDevice(ctx, id)
			return err
		}); err != nil {
			t.Fatal(err)
		}
		return ok
	}
	if del(seen.ID) {
		t.Fatal("a device that made a request was deleted")
	}
	if del(revoked.ID) {
		t.Fatal("a revoked device was deleted")
	}
	if !del(unseen.ID) {
		t.Fatal("an unseen device was not deleted")
	}
	if _, err := d.Device(ctx, unseen.ID); !errors.Is(err, ErrNotFound) {
		t.Fatalf("the unseen device after deletion = %v, want ErrNotFound", err)
	}
	if del(unseen.ID) {
		t.Fatal("deleting a device twice reported a deletion")
	}
	if all, err := d.Devices(ctx); err != nil || len(all) != 2 || all[0].ID != seen.ID || all[1].ID != revoked.ID {
		t.Fatalf("Devices = %+v, %v; want the seen and the revoked one", all, err)
	}
}
