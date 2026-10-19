package syncer_test

import (
	"encoding/json/v2"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"net/http/httputil"
	"net/url"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/frostyard/frostnotes/api"
	"github.com/frostyard/frostnotes/internal/hub"
)

// behind serves the hub's endpoints under prefix on another loopback
// port, as lodge serves them under /device/notes, and counts the requests
// that reach it.
func behind(t *testing.T, h *node, prefix string) (string, *atomic.Int64) {
	t.Helper()
	var n atomic.Int64
	inner := http.StripPrefix(prefix, h.srv.Hub.Handler())
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		n.Add(1)
		inner.ServeHTTP(w, r)
	}))
	t.Cleanup(srv.Close)
	return srv.URL + prefix, &n
}

// hubURL is the URL the device records for its one hub.
func hubURL(t *testing.T, d *node) string {
	t.Helper()
	hs, err := d.c.Sync().Hubs(t.Context(), &api.SyncHubsParams{})
	if err != nil || len(hs) != 1 {
		t.Fatalf("sync.hubs = %+v, %v", hs, err)
	}
	return hs[0].URL
}

// A device asks log.hub at each connection and moves to the URL the hub
// records, with its token and sync state; its edits then go there.
func TestADeviceFollowsTheHubsURL(t *testing.T) {
	h := newHub(t, map[string]string{"n.md": fiveLines})
	a := newPartitioned(t, h, "a", nil)
	settle(t, h, a.node)
	moved, reached := behind(t, h, "/device/notes")
	if _, err := h.c.Hub().SetURL(t.Context(), &api.HubSetURLParams{URL: moved}); err != nil {
		t.Fatal(err)
	}
	a.net.cut() // the next connection asks log.hub
	a.net.heal()
	a.srv.Syncer.Kick(a.vault)
	a.write("n.md", fiveLines+"six\n")
	settle(t, h, a.node)
	if got := hubURL(t, a.node); got != moved {
		t.Fatalf("the device's hub URL = %s, want %s", got, moved)
	}
	if reached.Load() == 0 {
		t.Fatal("no request reached the hub's new URL")
	}
	if got := same(t, h, a.node); got["n.md"] != fiveLines+"six\n" {
		t.Fatalf("n.md = %q after the move", got["n.md"])
	}
	// The loop may be syncing again by now; its URL is the point.
	st, err := a.c.Sync().Status(t.Context(), &api.SyncStatusParams{VaultID: &a.vault})
	if err != nil || st[0].URL != moved {
		t.Fatalf("sync.status = %+v, %v", st, err)
	}
}

// fake answers POST /sync/hub with answer(nonce), and fails the test if
// a device sends its token.
func fake(t *testing.T, answer func(req hub.RelocationRequest) any) string {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "" {
			t.Errorf("a relocation request carries %q", r.Header.Get("Authorization"))
		}
		body, _ := io.ReadAll(r.Body)
		if strings.Contains(string(body), "fnd_") {
			t.Errorf("a relocation request carries a token: %s", body)
		}
		var req hub.RelocationRequest
		if r.URL.Path != "/device/notes/sync/hub" || json.Unmarshal(body, &req) != nil {
			http.NotFound(w, r)
			return
		}
		_ = json.MarshalWrite(w, answer(req))
	}))
	t.Cleanup(srv.Close)
	return srv.URL + "/device/notes"
}

// sync.relocate keeps a new URL only when the hub there proves itself.
func TestRelocateChecksTheHubsProof(t *testing.T) {
	h := newHub(t, map[string]string{"n.md": fiveLines})
	a := newDevice(t, h, "a", nil)
	settle(t, h, a)
	ctx := t.Context()
	old := hubURL(t, a)
	moved, _ := behind(t, h, "/device/notes")
	relocate := func(to string) error {
		_, err := a.c.Sync().Relocate(ctx, &api.SyncRelocateParams{Hub: old, URL: to})
		return err
	}
	refused := func(name, to string) {
		t.Helper()
		if err := relocate(to); !errors.Is(err, api.ErrConflict) {
			t.Errorf("%s: relocate = %v, want conflict", name, err)
		}
		if got := hubURL(t, a); got != old {
			t.Fatalf("%s moved the device to %s", name, got)
		}
	}
	refused("a hub that records no URL", moved)
	if _, err := h.c.Hub().SetURL(ctx, &api.HubSetURLParams{URL: moved}); err != nil {
		t.Fatal(err)
	}
	st, err := h.c.Hub().Status(ctx, &api.HubStatusParams{})
	if err != nil {
		t.Fatal(err)
	}

	// A relay forwards the nonce to the real hub: the proof is genuine,
	// but it names the real hub's URL, not the relay's.
	target, _ := url.Parse(moved)
	relay := httptest.NewServer(http.StripPrefix("/device/notes", httputil.NewSingleHostReverseProxy(target)))
	t.Cleanup(relay.Close)
	refused("a relay", relay.URL+"/device/notes")
	// The same hub at a URL it does not record.
	other, _ := behind(t, h, "/other")
	refused("a URL the hub does not record", other)
	// A host that claims another hub ID, or forges the proof.
	refused("another hub", fake(t, func(req hub.RelocationRequest) any {
		return hub.Relocation{Hub: strings.Repeat("f", 32), URL: "", Proof: ""}
	}))
	var forged string
	forged = fake(t, func(req hub.RelocationRequest) any {
		return hub.Relocation{Hub: st.Hub, URL: forged, Proof: hub.Proof(hub.TokenKey("fnd_guess"), st.Hub, forged, req.Nonce)}
	})
	refused("a forged proof", forged)
	refused("an unpaired hub", func() string {
		h2 := newHub(t, nil)
		u, _ := behind(t, h2, "/device/notes")
		if _, err := h2.c.Hub().SetURL(ctx, &api.HubSetURLParams{URL: u}); err != nil {
			t.Fatal(err)
		}
		return u
	}())
	if _, err := a.c.Sync().Relocate(ctx, &api.SyncRelocateParams{Hub: "http://127.0.0.1:1", URL: moved}); !errors.Is(err, api.ErrNotFound) {
		t.Errorf("relocating a hub the device never paired = %v, want notFound", err)
	}
	if _, err := a.c.Sync().Relocate(ctx, &api.SyncRelocateParams{Hub: old, URL: "http://lodge.example.ts.net/device/notes"}); !errors.Is(err, api.ErrInvalidParams) {
		t.Errorf("relocating to plain http on another host = %v, want invalidParams", err)
	}

	// The real hub at its recorded URL: the device moves and syncs there.
	got, err := a.c.Sync().Relocate(ctx, &api.SyncRelocateParams{Hub: old, URL: moved})
	if err != nil || got.URL != moved || got.Hub != st.Hub {
		t.Fatalf("relocate = %+v, %v", got, err)
	}
	a.write("n.md", fiveLines+"six\n")
	settle(t, h, a)
	if files := same(t, h, a); files["n.md"] != fiveLines+"six\n" {
		t.Fatalf("n.md = %q after relocating", files["n.md"])
	}
}
