package store

// CONTRACT TEST for task card T-0026 (docs/tasks). Do not edit.

import (
	"errors"
	"testing"
	"time"

	"github.com/frostyard/frostnotes/api"
)

func TestDevices(t *testing.T) {
	d, _ := openTest(t)
	ctx := t.Context()
	if all, err := d.Devices(ctx); err != nil || all == nil || len(all) != 0 {
		t.Fatalf("Devices with none = %#v, %v; want a non-nil empty slice", all, err)
	}
	var laptop, phone HubDevice
	err := d.Tx(ctx, func(tx *Tx) error {
		var err error
		if laptop, err = tx.AddDevice(ctx, "laptop", "hash-laptop"); err != nil {
			return err
		}
		phone, err = tx.AddDevice(ctx, "phone", "hash-phone")
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	if laptop.ID == 0 || phone.ID <= laptop.ID || laptop.Name != "laptop" || laptop.TokenHash != "hash-laptop" ||
		!laptop.PairedAt.Equal(testNow) || !laptop.SeenAt.IsZero() || !laptop.RevokedAt.IsZero() {
		t.Fatalf("AddDevice = %+v, %+v", laptop, phone)
	}
	err = d.Tx(ctx, func(tx *Tx) error { _, err := tx.AddDevice(ctx, "copy", "hash-laptop"); return err })
	if !errors.Is(err, ErrConflict) {
		t.Fatalf("a second device with one token hash = %v, want ErrConflict", err)
	}
	if got, err := d.DeviceByToken(ctx, "hash-phone"); err != nil || got != phone {
		t.Fatalf("DeviceByToken = %+v, %v; want %+v", got, err, phone)
	}
	if _, err := d.DeviceByToken(ctx, "nobody"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("DeviceByToken(unknown) = %v, want ErrNotFound", err)
	}

	// Seen at most once a minute.
	at := testNow.Add(10 * time.Second)
	touch := func(at time.Time) bool {
		var changed bool
		if err := d.Tx(ctx, func(tx *Tx) error { var err error; changed, err = tx.TouchDevice(ctx, laptop.ID, at); return err }); err != nil {
			t.Fatal(err)
		}
		return changed
	}
	if !touch(at) {
		t.Fatal("the first TouchDevice changed nothing")
	}
	if touch(at.Add(59 * time.Second)) {
		t.Fatal("TouchDevice within a minute changed seen_at")
	}
	if !touch(at.Add(61 * time.Second)) {
		t.Fatal("TouchDevice a minute later changed nothing")
	}
	if got, _ := d.Device(ctx, laptop.ID); !got.SeenAt.Equal(at.Add(61 * time.Second).Truncate(time.Millisecond)) {
		t.Fatalf("SeenAt = %v, want %v", got.SeenAt, at.Add(61*time.Second))
	}

	revoke := func(id int64) error { return d.Tx(ctx, func(tx *Tx) error { return tx.RevokeDevice(ctx, id) }) }
	d.Now = func() time.Time { return testNow.Add(time.Hour) }
	if err := revoke(laptop.ID); err != nil {
		t.Fatal(err)
	}
	d.Now = func() time.Time { return testNow.Add(2 * time.Hour) }
	if err := revoke(laptop.ID); err != nil {
		t.Fatalf("revoking twice = %v, want nil", err)
	}
	if err := revoke(999); !errors.Is(err, ErrNotFound) {
		t.Fatalf("revoking device 999 = %v, want ErrNotFound", err)
	}
	if _, err := d.DeviceByToken(ctx, "hash-laptop"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("a revoked device's token = %v, want ErrNotFound", err)
	}
	got, err := d.Device(ctx, laptop.ID)
	if err != nil || got.TokenHash != "" || !got.RevokedAt.Equal(testNow.Add(time.Hour)) || got.Name != "laptop" {
		t.Fatalf("revoked Device = %+v, %v; want no token hash and the first revocation's time", got, err)
	}
	if _, err := d.Device(ctx, 999); !errors.Is(err, ErrNotFound) {
		t.Fatalf("Device(999) = %v, want ErrNotFound", err)
	}
	all, err := d.Devices(ctx)
	if err != nil || len(all) != 2 || all[0].ID != laptop.ID || all[1] != phone {
		t.Fatalf("Devices = %+v, %v; want both, by ID", all, err)
	}
	seen := got.SeenAt
	want := api.HubDevice{ID: laptop.ID, Name: "laptop", PairedAt: testNow, SeenAt: &seen, Connected: true, Revoked: true}
	if a := got.API(true); a.SeenAt == nil || !a.SeenAt.Equal(seen) || a.ID != want.ID || a.Name != want.Name ||
		!a.PairedAt.Equal(want.PairedAt) || !a.Connected || !a.Revoked {
		t.Fatalf("API = %+v, want %+v", a, want)
	}
	if a := phone.API(false); a.SeenAt != nil || a.Revoked || a.Connected {
		t.Fatalf("an unseen device's API = %+v; want no SeenAt", a)
	}
}
