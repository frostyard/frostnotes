package syncer_test

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/frostyard/frostnotes/api"
	"github.com/frostyard/frostnotes/internal/secrets"
)

// The findings of plan 0004's Phase 7 review, each with its test.

func TestAHubClaimingAnotherHubsIDIsRefused(t *testing.T) {
	// A hub the user is led to pair with answers with the ID of the hub
	// the device already syncs with, which anyone can learn. The device
	// refuses it, keeping the real hub's token and URL.
	h, a, _ := twoDevices(t, map[string]string{"n.md": "one\n"})
	ctx := t.Context()
	hubs, err := a.srv.DB.SyncHubs(ctx)
	if err != nil || len(hubs) != 1 {
		t.Fatalf("SyncHubs = %+v, %v", hubs, err)
	}
	real := hubs[0]
	token, err := a.srv.Secrets.Get(ctx, secrets.HubToken(real.HubID))
	if err != nil {
		t.Fatal(err)
	}
	impostor := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.Method == http.MethodPost && r.URL.Path == "/sync/pair":
			w.WriteHeader(http.StatusCreated)
			_, _ = w.Write([]byte(`{"pairing":"p1","secret":"s","code":"ABCD-EFGH","hub":"` + real.HubID + `","expiresAt":"` +
				time.Now().Add(time.Minute).UTC().Format(time.RFC3339) + `"}`))
		case r.Method == http.MethodGet && r.URL.Path == "/sync/pair/p1":
			_, _ = w.Write([]byte(`{"state":"approved","token":"fnd_impostor","device":7,"hub":"` + real.HubID + `"}`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer impostor.Close()
	name := "a"
	p, err := a.c.Sync().Pair(ctx, &api.SyncPairParams{URL: impostor.URL, Name: &name})
	if err != nil {
		t.Fatal(err)
	}
	_, err = a.c.Sync().Paired(ctx, &api.SyncPairedParams{Pairing: p.Pairing})
	var ae *api.Error
	if !errors.As(err, &ae) || ae.Code != api.CodeConflict {
		t.Fatalf("pairing with a hub claiming another's ID = %v; want conflict", err)
	}
	if got, err := a.srv.Secrets.Get(ctx, secrets.HubToken(real.HubID)); err != nil || got != token {
		t.Fatalf("the real hub's token became %q, %v", got, err)
	}
	if hubs, err := a.srv.DB.SyncHubs(ctx); err != nil || len(hubs) != 1 || hubs[0].URL != real.URL {
		t.Fatalf("the device's hubs became %+v, %v", hubs, err)
	}
	a.write("n.md", "one\ntwo\n")
	a.srv.Syncer.Kick(a.vault)
	settle(t, h, a.node)
	if got := same(t, h, a.node)["n.md"]; got != "one\ntwo\n" {
		t.Fatalf("after the refusal, n.md = %q; want a's edit on the real hub", got)
	}
}

func TestAnAnswerAtAPathThisDeviceIgnores(t *testing.T) {
	// a ignores private/; b moves x.md there while a edits it. a's answer
	// puts the note where a does not hold files: the note leaves a's care
	// with a's edit merged on the hub, and a keeps syncing.
	h := newHub(t, map[string]string{"x.md": fiveLines, "y.md": "y\n"})
	a := newPartitioned(t, h, "a", map[string]string{".frostnotes/vault.yaml": "ignore:\n  - private/\n"})
	b := newPartitioned(t, h, "b", nil)
	settle(t, h, a.node, b.node)
	inOrder(t, h, []*device{b, a}, func() {
		b.move("x.md", "private/x.md")
		a.write("x.md", fiveLines+"six\n")
	})
	b.write("y.md", "y\nfrom b\n")
	b.srv.Syncer.Kick(b.vault)
	settle(t, h, a.node, b.node)
	hub := files(t, h.dir)
	if hub["private/x.md"] != fiveLines+"six\n" {
		t.Fatalf("the hub's private/x.md = %q; want a's edit", hub["private/x.md"])
	}
	if got := a.read("y.md"); got != "y\nfrom b\n" {
		t.Fatalf("a's y.md = %q; a stopped syncing", got)
	}
}

func TestALongNameWithinTheLimitSyncs(t *testing.T) {
	// A name near the 255-byte limit syncs, and the rest with it.
	h := newHub(t, map[string]string{"y.md": "y\n"})
	a := newPartitioned(t, h, "a", nil)
	settle(t, h, a.node)
	long := strings.Repeat("n", 250) + ".md"
	h.write(long, "long\n")
	h.write("y.md", "y\nagain\n")
	a.srv.Syncer.Kick(a.vault)
	settle(t, h, a.node)
	if got := a.read("y.md"); got != "y\nagain\n" {
		t.Fatalf("a's y.md = %q", got)
	}
	if got := a.read(long); got != "long\n" {
		t.Fatalf("a's long note = %q", got)
	}
}
