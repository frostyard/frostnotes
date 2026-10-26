package syncer_test

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/frostyard/frostnotes/api"
	"github.com/frostyard/frostnotes/internal/secrets"
	"github.com/frostyard/frostnotes/internal/store"
	"github.com/frostyard/frostnotes/internal/syncer"
)

// failingStore is a secret store that keeps nothing.
type failingStore struct{ err error }

func (f failingStore) Get(context.Context, string) (string, error) { return "", f.err }

func (f failingStore) Set(context.Context, string, string) error { return f.err }

func (f failingStore) Delete(context.Context, string) error { return f.err }

// noTokenStore keeps every secret but a hub token: it fails where the
// store of a device that cannot keep a token does.
type noTokenStore struct{ secrets.Store }

func (n noTokenStore) Set(ctx context.Context, key, value string) error {
	if strings.HasPrefix(key, "hub/") {
		return errors.New("the keyring refused the token")
	}
	return n.Store.Set(ctx, key, value)
}

// A pairing asks the secret store first: a store that cannot keep a token
// fails the pairing before the hub hears of the device, so the store's
// prompt comes before the hub knows it.
func TestPairChecksTheSecretStoreFirst(t *testing.T) {
	var requests atomic.Int64
	hubSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		requests.Add(1)
		http.Error(w, "the hub is not to be reached", http.StatusInternalServerError)
	}))
	defer hubSrv.Close()
	s := syncer.New(syncer.Config{Secrets: failingStore{err: errors.New("the keyring is locked")}})
	_, err := s.Service().Pair(t.Context(), &api.SyncPairParams{URL: hubSrv.URL})
	if err == nil {
		t.Fatal("pairing with a secret store that keeps nothing succeeded")
	}
	if !strings.Contains(err.Error(), "cannot keep a hub token") || !strings.Contains(err.Error(), "the keyring is locked") {
		t.Fatalf("the error = %v; want a device that cannot keep a hub token", err)
	}
	if n := requests.Load(); n != 0 {
		t.Fatalf("the hub got %d requests; want none", n)
	}
}

// A device whose store cannot keep the hub token withdraws the pairing it
// collected: the hub forgets the device, so a failed pairing leaves none
// behind.
func TestAPairingTheDeviceCannotKeepIsWithdrawn(t *testing.T) {
	h := newHub(t, map[string]string{"n.md": "one\n"})
	ctx := t.Context()
	dir := t.TempDir()
	db, err := store.Open(ctx, filepath.Join(dir, "notesd.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	file := secrets.NewFile(filepath.Join(dir, "secrets.json"))
	s := syncer.New(syncer.Config{DB: db, Secrets: noTokenStore{file}, Device: "leaky"})
	name := "leaky"
	p, err := s.Service().Pair(ctx, &api.SyncPairParams{URL: h.srv.HubURL, Name: &name})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := h.c.Hub().Approve(ctx, &api.HubApproveParams{Code: p.Code}); err != nil {
		t.Fatal(err)
	}
	_, err = s.Service().Paired(ctx, &api.SyncPairedParams{Pairing: p.Pairing})
	if err == nil {
		t.Fatal("a pairing whose token cannot be kept succeeded")
	}
	if !strings.Contains(err.Error(), "keep the device token") || !strings.Contains(err.Error(), "forgot") {
		t.Fatalf("the error = %v; want the token it could not keep and a hub that forgot the device", err)
	}
	if ds, err := h.c.Hub().Devices(ctx, &api.HubDevicesParams{}); err != nil || len(ds) != 0 {
		t.Fatalf("the hub's devices = %+v, %v; want none", ds, err)
	}
}
