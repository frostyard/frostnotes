package syncer_test

import (
	"errors"
	"path/filepath"
	"strings"
	"testing"

	"github.com/frostyard/frostnotes/api"
	"github.com/frostyard/frostnotes/internal/secrets"
	"github.com/frostyard/frostnotes/internal/store"
	"github.com/frostyard/frostnotes/internal/syncer"
)

// A pairing that cannot record its hub puts back the token the device kept:
// a pairing again with a hub this device already syncs with must not lose a
// token that works. The hub still forgets the new device.
func TestAFailedPairingKeepsTheOldToken(t *testing.T) {
	for _, tc := range []struct {
		name string
		old  string // the token the store holds before the pairing; "" for none
	}{{"an old token", "fnd_the_old_token"}, {"no token", ""}} {
		t.Run(tc.name, func(t *testing.T) {
			h := newHub(t, map[string]string{"n.md": "one\n"})
			ctx := t.Context()
			st, err := h.c.Hub().Status(ctx, &api.HubStatusParams{})
			if err != nil {
				t.Fatal(err)
			}
			dir := t.TempDir()
			db, err := store.Open(ctx, filepath.Join(dir, "notesd.db"))
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() { _ = db.Close() })
			file := secrets.NewFile(filepath.Join(dir, "secrets.json"))
			key := secrets.HubToken(st.Hub)
			if tc.old != "" {
				if err := file.Set(ctx, key, tc.old); err != nil {
					t.Fatal(err)
				}
			}
			// The hub's row cannot be written, as in a database that is full
			// or on a file system that refuses writes.
			err = db.Tx(ctx, func(tx *store.Tx) error {
				_, err := tx.ExecContext(ctx, `CREATE TRIGGER no_sync_hubs BEFORE INSERT ON sync_hubs
					BEGIN SELECT RAISE(ABORT, 'sync_hubs is not writable'); END`)
				return err
			})
			if err != nil {
				t.Fatal(err)
			}
			s := syncer.New(syncer.Config{DB: db, Secrets: file, Device: "laptop"})
			name := "laptop"
			p, err := s.Service().Pair(ctx, &api.SyncPairParams{URL: h.srv.HubURL, Name: &name})
			if err != nil {
				t.Fatal(err)
			}
			if _, err := h.c.Hub().Approve(ctx, &api.HubApproveParams{Code: p.Code}); err != nil {
				t.Fatal(err)
			}
			_, err = s.Service().Paired(ctx, &api.SyncPairedParams{Pairing: p.Pairing})
			if err == nil {
				t.Fatal("a pairing whose hub row cannot be written succeeded")
			}
			if !strings.Contains(err.Error(), "sync_hubs is not writable") {
				t.Fatalf("the pairing's error = %v; want the failed row write", err)
			}
			got, err := file.Get(ctx, key)
			switch {
			case tc.old == "" && !errors.Is(err, secrets.ErrNotFound):
				t.Fatalf("the hub token after the failed pairing = %q, %v; want none", got, err)
			case tc.old != "" && (err != nil || got != tc.old):
				t.Fatalf("the hub token after the failed pairing = %q, %v; want %q back", got, err, tc.old)
			}
			if hs, err := db.SyncHubs(ctx); err != nil || len(hs) != 0 {
				t.Fatalf("the device's hubs = %+v, %v; want none", hs, err)
			}
			if ds, err := h.c.Hub().Devices(ctx, &api.HubDevicesParams{}); err != nil || len(ds) != 0 {
				t.Fatalf("the hub's devices = %+v, %v; want none: the new pairing is withdrawn", ds, err)
			}
		})
	}
}
