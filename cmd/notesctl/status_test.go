package main

import (
	"errors"
	"path/filepath"
	"strings"
	"testing"

	"github.com/frostyard/frostnotes/api"
	"github.com/frostyard/frostnotes/internal/config"
	"github.com/frostyard/frostnotes/internal/flatpakapp"
	"github.com/frostyard/frostnotes/internal/rpctest"
)

// inFlatpak makes this process look like one in the app's Flatpak, with
// its config, state and libexec directories in temporary ones. It answers
// the app's config and state directories.
func inFlatpak(t *testing.T) (cfg, state string) {
	t.Helper()
	cfgHome, stateHome := t.TempDir(), t.TempDir()
	t.Setenv("FLATPAK_ID", config.AppID)
	t.Setenv("XDG_CONFIG_HOME", cfgHome)
	t.Setenv("XDG_STATE_HOME", stateHome)
	t.Setenv("FROSTNOTES_LIBEXEC_DIR", t.TempDir())
	return filepath.Join(cfgHome, "frostnotes"), filepath.Join(stateHome, "frostnotes")
}

// outsideFlatpak clears what would make this process look like one in
// the Flatpak.
func outsideFlatpak(t *testing.T) {
	t.Helper()
	t.Setenv("FLATPAK_ID", "")
}

func writeFile(t *testing.T, path, text string) {
	t.Helper()
	if err := flatpakapp.WriteFile(path, []byte(text)); err != nil {
		t.Fatal(err)
	}
}

func TestStatusShowsVaultsTheirSyncAndHubs(t *testing.T) {
	outsideFlatpak(t)
	dev := &fakeSync{
		status: []api.SyncStatus{{VaultID: 1, URL: "https://hub.example", HubVault: "Notes", State: api.SyncStateIdle, Pending: 2, Conflicts: 1}},
		hubs:   []api.SyncHub{{ID: 1, Hub: "h1", URL: "https://hub.example", Device: 4, PairedAt: when}},
	}
	srv := syncServer(t, &fakeHub{}, dev, "notes", "work")
	out := notes(t, srv, "status")
	if first, _, _ := strings.Cut(out, "\n"); first != "notesd rpctest answers on "+srv.Socket {
		t.Errorf("status's first line = %q", first)
	}
	for _, want := range []string{
		`vault 1 "notes" at `,
		`: syncs with "Notes" at https://hub.example: idle, 2 pending, 1 conflicted` + "\n",
		`vault 2 "work" at `,
		": not synced\n",
		"paired with hub h1 at https://hub.example as device 4, since 2026-10-16\n",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("status lacks %q:\n%s", want, out)
		}
	}
	if strings.Contains(out, "background") {
		t.Errorf("status names the Flatpak's setting outside it:\n%s", out)
	}
}

func TestStatusSaysWhatToDoFirst(t *testing.T) {
	outsideFlatpak(t)
	srv := syncServer(t, &fakeHub{}, &fakeSync{})
	out := notes(t, srv, "status")
	for _, want := range []string{"no vaults: register a folder with: notesctl vault add PATH\n", "no hub: pair this device and sync a vault with: notesctl hub join URL NAME\n"} {
		if !strings.Contains(out, want) {
			t.Errorf("status lacks %q:\n%s", want, out)
		}
	}
}

// In the Flatpak, status shows the background setting beside notesd.
func TestStatusShowsTheBackgroundSettingInTheFlatpak(t *testing.T) {
	cfg, _ := inFlatpak(t)
	writeFile(t, filepath.Join(cfg, "background-off"), "off\n")
	srv := rpctest.Start(t)
	out := notes(t, srv, "status")
	if !strings.Contains(out, "background: off, notesd does not start at login; the desktop was not asked yet") {
		t.Errorf("status:\n%s", out)
	}
}

// When notesd does not answer in the Flatpak, status shows the end of
// notesd.log, the fix when the Flatpak portal did not start notesd, and
// how to start it, and fails.
func TestStatusExplainsWhyNotesdIsNotRunning(t *testing.T) {
	_, state := inFlatpak(t)
	log := filepath.Join(state, "notesd.log")
	writeFile(t, log, "notesd: starting\n"+flatpakapp.PortalFailure+"; see "+log+"\n")
	socket := filepath.Join(t.TempDir(), "notesd.sock")
	out, err := runNotesctl(t, "--socket", socket, "status")
	if !errors.Is(err, errNotRunning) {
		t.Fatalf("status with no notesd = %v, want %v", err, errNotRunning)
	}
	for _, want := range []string{
		"notesd does not answer on " + socket + ": ",
		"the end of notesd.log (" + log + "):\n  notesd: starting\n  " + flatpakapp.PortalFailure,
		"The Flatpak portal did not start notesd. To fix it:\n",
		"flatpak override --user --show org.frostyard.Frostnotes",
		"systemctl --user status flatpak-portal.service",
		"Start notesd with: flatpak run org.frostyard.Frostnotes\n",
		"background: on, notesd starts at login when the desktop allows it",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("status lacks %q:\n%s", want, out)
		}
	}
}

func TestStatusShowsAnyOtherEndOfTheLog(t *testing.T) {
	_, state := inFlatpak(t)
	writeFile(t, filepath.Join(state, "notesd.log"), "notesd: another notesd uses /data\n")
	out, err := runNotesctl(t, "--socket", filepath.Join(t.TempDir(), "notesd.sock"), "status")
	if !errors.Is(err, errNotRunning) || !strings.Contains(out, "  notesd: another notesd uses /data\n") || strings.Contains(out, "To fix it") {
		t.Fatalf("status = %v:\n%s", err, out)
	}
}

func TestStatusWithNoLog(t *testing.T) {
	_, state := inFlatpak(t)
	out, err := runNotesctl(t, "--socket", filepath.Join(t.TempDir(), "notesd.sock"), "status")
	if !errors.Is(err, errNotRunning) || !strings.Contains(out, "notesd.log is empty or missing: "+filepath.Join(state, "notesd.log")) {
		t.Fatalf("status = %v:\n%s", err, out)
	}
}

// Outside the Flatpak notesd's log is wherever its service manager keeps
// it: status says only that notesd does not answer.
func TestStatusOutsideTheFlatpakWhenNotesdIsNotRunning(t *testing.T) {
	outsideFlatpak(t)
	socket := filepath.Join(t.TempDir(), "notesd.sock")
	out, err := runNotesctl(t, "--socket", socket, "status")
	if !errors.Is(err, errNotRunning) {
		t.Fatalf("status = %v", err)
	}
	if lines := strings.Split(strings.TrimSpace(out), "\n"); len(lines) != 1 || !strings.HasPrefix(lines[0], "notesd does not answer on "+socket) {
		t.Errorf("status:\n%s", out)
	}
}

func TestStatusJSON(t *testing.T) {
	outsideFlatpak(t)
	srv := rpctest.Start(t)
	out := notes(t, srv, "status", "--json")
	if !strings.Contains(out, `"running": true`) || !strings.Contains(out, `"socket": "`+srv.Socket+`"`) {
		t.Errorf("status --json:\n%s", out)
	}
	out, err := runNotesctl(t, "--socket", filepath.Join(t.TempDir(), "none.sock"), "status", "--json")
	if !errors.Is(err, errNotRunning) || !strings.Contains(out, `"running": false`) {
		t.Errorf("status --json with no notesd = %v:\n%s", err, out)
	}
}
