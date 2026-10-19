package main

import (
	"encoding/json/v2"
	"strings"
	"testing"

	"github.com/frostyard/frostnotes/api"
)

func TestHubURL(t *testing.T) {
	const moved = "https://lodge.example.ts.net/device/notes"
	hub := &fakeHub{status: api.HubStatus{Hub: "0123456789abcdef0123456789abcdef", URL: ptr(moved)}}
	srv := syncServer(t, hub, &fakeSync{})
	if out := notes(t, srv, "hub", "url", moved+"/"); out != "devices use "+moved+"\n" {
		t.Fatalf("hub url = %q", out)
	}
	checkCalls(t, hub.took(), &api.HubSetURLParams{URL: moved + "/"})
	var got api.HubStatus
	if err := json.Unmarshal([]byte(notes(t, srv, "--json", "hub", "url", moved)), &got); err != nil || got.URL == nil || *got.URL != moved {
		t.Fatalf("hub url --json = %+v, %v", got, err)
	}
	hub.took()
	hub.status.URL = nil
	if out := notes(t, srv, "hub", "url", ""); out != "devices use the URL they paired with\n" {
		t.Fatalf("hub url \"\" = %q", out)
	}
	checkCalls(t, hub.took(), &api.HubSetURLParams{URL: ""})
	hub.err = api.InvalidParams("http://lodge: a hub on another machine needs https://")
	if err := notesErr(t, srv, "hub", "url", "http://lodge"); err == nil || !strings.Contains(err.Error(), "needs https://") {
		t.Fatalf("hub url with a refused URL = %v", err)
	}
	if err := notesErr(t, srv, "hub", "url"); err == nil {
		t.Fatal("hub url without a URL succeeded")
	}
}

func TestHubStatusShowsTheURL(t *testing.T) {
	const moved = "https://lodge.example.ts.net/device/notes"
	hub := &fakeHub{status: api.HubStatus{Hub: "0123456789abcdef0123456789abcdef", URL: ptr(moved), Vaults: 1}}
	srv := syncServer(t, hub, &fakeSync{})
	want := "hub 0123456789abcdef0123456789abcdef\nnot listening: start notesd with --hub ADDR\ndevices use " + moved + "\n1 vaults, 0 devices, 0 connected\n"
	if out := notes(t, srv, "hub", "status"); out != want {
		t.Fatalf("hub status = %q, want %q", out, want)
	}
}

func TestHubRelocate(t *testing.T) {
	const old, moved = "https://hub.example.ts.net", "https://lodge.example.ts.net/device/notes"
	dev := &fakeSync{hubs: []api.SyncHub{{ID: 1, Hub: "0123456789abcdef0123456789abcdef", URL: moved, Device: 4, PairedAt: when}}}
	srv := syncServer(t, &fakeHub{}, dev)
	if out := notes(t, srv, "hub", "relocate", old, moved); out != "hub 0123456789abcdef0123456789abcdef is at "+moved+" now\n" {
		t.Fatalf("hub relocate = %q", out)
	}
	checkCalls(t, dev.took(), &api.SyncRelocateParams{Hub: old, URL: moved})
	var got api.SyncHub
	if err := json.Unmarshal([]byte(notes(t, srv, "--json", "hub", "relocate", old, moved)), &got); err != nil || got.URL != moved || got.Device != 4 {
		t.Fatalf("hub relocate --json = %+v, %v", got, err)
	}
	dev.took()
	dev.err = api.Conflict("the hub at https://relay.example does not hold this device's token")
	if err := notesErr(t, srv, "hub", "relocate", old, "https://relay.example"); err == nil || !strings.Contains(err.Error(), "does not hold") {
		t.Fatalf("a refused relocation = %v", err)
	}
	for _, args := range [][]string{{"hub", "relocate"}, {"hub", "relocate", old}, {"hub", "relocate", old, moved, "x"}} {
		if err := notesErr(t, srv, args...); err == nil {
			t.Errorf("%v succeeded", args)
		}
	}
}
