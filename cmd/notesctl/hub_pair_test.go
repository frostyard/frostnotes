package main

import (
	"strings"
	"testing"

	"github.com/frostyard/frostnotes/api"
)

// hub pair pairs the device, or pairs it again, and joins no vault.
func TestHubPair(t *testing.T) {
	dev := &fakeSync{
		pairing: api.Pairing{Pairing: "p1", Code: "7K4M-Q2XD", Hub: "h1", URL: "https://hub.example", ExpiresAt: when},
		states:  []api.PairingState{api.PairingStatePending, api.PairingStateApproved},
	}
	srv := syncServer(t, &fakeHub{}, dev)
	out := notes(t, srv, "hub", "pair", "https://hub.example/", "--device", "laptop")
	if out != "approve this device on the hub: notesctl hub approve 7K4M-Q2XD\npaired with hub h1\n" {
		t.Errorf("output = %q", out)
	}
	checkCalls(t, dev.took(),
		&api.SyncPairParams{URL: "https://hub.example", Name: ptr("laptop")},
		&api.SyncPairedParams{Pairing: "p1"},
		&api.SyncPairedParams{Pairing: "p1"},
	)
}

func TestHubPairExpired(t *testing.T) {
	dev := &fakeSync{
		pairing: api.Pairing{Pairing: "p1", Code: "7K4M-Q2XD", Hub: "h1", URL: "https://hub.example", ExpiresAt: when},
		states:  []api.PairingState{api.PairingStateExpired},
	}
	srv := syncServer(t, &fakeHub{}, dev)
	if err := notesErr(t, srv, "hub", "pair", "https://hub.example"); !strings.Contains(err.Error(), "expired") {
		t.Errorf("hub pair = %v", err)
	}
	checkCalls(t, dev.took(), &api.SyncPairParams{URL: "https://hub.example"}, &api.SyncPairedParams{Pairing: "p1"})
}
