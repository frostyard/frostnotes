package hub_test

import (
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/frostyard/frostnotes/api"
	"github.com/frostyard/frostnotes/internal/blobs"
)

// The findings of plan 0004's Phase 7 review, each with its test.

func TestARevocationWhileADeviceConnectsClosesIt(t *testing.T) {
	// The device is revoked after its token passed the check and before
	// its connection is counted among the device's: the connection must
	// not outlive the revocation.
	var h *testHub
	var device atomic.Int64
	var armed atomic.Bool
	var once sync.Once
	h = newHubWith(t, func(point string) bool {
		if point == "connecting" && armed.Load() {
			once.Do(func() {
				if err := h.c.Hub().Revoke(t.Context(), &api.HubRevokeParams{Device: device.Load()}); err != nil {
					t.Error(err)
				}
			})
		}
		return false
	})
	token, id := h.pair("phone")
	device.Store(id)
	armed.Store(true)
	c, err := h.dial(token)
	if err == nil {
		_, err = c.Log().Vaults(t.Context(), &api.LogVaultsParams{})
	}
	if err == nil {
		t.Fatal("a connection made as its device was revoked still serves the log")
	}
}

func TestANameTheHubCannotHoldRefusesOnlyItsItem(t *testing.T) {
	// One item of a push names a file longer than a file system holds:
	// that item is refused as invalid, and the push's other item applies.
	h := newHub(t)
	token, _ := h.pair("phone")
	c, err := h.dial(token)
	if err != nil {
		t.Fatal(err)
	}
	ctx := t.Context()
	rep, err := c.Log().Join(ctx, &api.LogJoinParams{Vault: h.vault.Sync})
	if err != nil {
		t.Fatal(err)
	}
	hash := blobs.ID([]byte("one\n")) // a.md's, which the hub holds
	long := strings.Repeat("x", 300) + ".md"
	answers, err := c.Log().Push(ctx, &api.LogPushParams{Vault: h.vault.Sync, Replica: rep.ID, Items: []api.PushItem{
		{Push: 1, Path: long, Hash: &hash, Source: api.VersionSourceEditor, At: time.Now()},
		{Push: 2, Path: "b.md", Hash: &hash, Source: api.VersionSourceEditor, At: time.Now()},
	}})
	if err != nil {
		t.Fatalf("a push with a name too long fails whole: %v", err)
	}
	if answers[0].Status != api.PushStatusRefused || answers[0].Reason == nil || *answers[0].Reason != api.RefuseReasonInvalid {
		t.Fatalf("the long name's answer = %+v; want refused, invalid", answers[0])
	}
	if answers[1].Status != api.PushStatusAccepted {
		t.Fatalf("the other item's answer = %+v; want accepted", answers[1])
	}
}

func TestContentTheHubHoldsForAnotherVaultIsNotADevicesToName(t *testing.T) {
	// The hub's notesd holds a note of a vault it does not serve. A device
	// may not learn that it holds those bytes, nor name them in a push to
	// read them back, unless it uploads them itself.
	h := newHub(t)
	ctx := t.Context()
	private := t.TempDir()
	secret := "a private note\n"
	if err := os.WriteFile(filepath.Join(private, "p.md"), []byte(secret), 0o644); err != nil {
		t.Fatal(err)
	}
	pv, err := h.c.Vault().Add(ctx, &api.VaultAddParams{Path: private})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := h.c.Note().Read(ctx, &api.NoteReadParams{VaultID: pv.ID, Path: "p.md"}); err != nil {
		t.Fatal(err)
	}
	hash := blobs.ID([]byte(secret))
	if ok, err := h.srv.Blobs.Has(hash); err != nil || !ok {
		t.Fatalf("the hub's store lacks the private note: %v", err)
	}
	token, _ := h.pair("phone")
	c, err := h.dial(token)
	if err != nil {
		t.Fatal(err)
	}
	rep, err := c.Log().Join(ctx, &api.LogJoinParams{Vault: h.vault.Sync})
	if err != nil {
		t.Fatal(err)
	}
	if missing, err := c.Log().Missing(ctx, &api.LogMissingParams{Vault: h.vault.Sync, Hashes: []string{hash}}); err != nil ||
		len(missing) != 1 {
		t.Fatalf("log.missing of another vault's content = %v, %v; want it missing", missing, err)
	}
	push := func(id int64) api.PushAnswer {
		t.Helper()
		answers, err := c.Log().Push(ctx, &api.LogPushParams{Vault: h.vault.Sync, Replica: rep.ID, Items: []api.PushItem{
			{Push: id, Path: "copy.md", Hash: &hash, Source: api.VersionSourceEditor, At: time.Now()},
		}})
		if err != nil {
			t.Fatal(err)
		}
		return answers[0]
	}
	if a := push(1); a.Status != api.PushStatusRefused || a.Reason == nil || *a.Reason != api.RefuseReasonMissing {
		t.Fatalf("a push naming content the device never uploaded = %+v; want refused, missing", a)
	}
	if resp := h.do(http.MethodGet, "/sync/blobs/"+hash, token, nil, nil); resp.StatusCode != http.StatusNotFound {
		t.Fatalf("GET of another vault's content = %s, want 404", resp.Status)
	}
	// With the bytes in hand the device uploads them, and its push names
	// its own upload.
	if resp := h.do(http.MethodPut, "/sync/blobs/"+hash, token, strings.NewReader(secret),
		http.Header{"Content-Type": {"application/octet-stream"}}); resp.StatusCode != http.StatusCreated {
		t.Fatalf("PUT = %s, want 201 whether or not the hub held it", resp.Status)
	}
	if a := push(2); a.Status != api.PushStatusAccepted {
		t.Fatalf("a push naming the device's own upload = %+v; want accepted", a)
	}
}
