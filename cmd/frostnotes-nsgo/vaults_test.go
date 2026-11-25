//go:build linux && integration

package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/frostyard/frostnotes/api"
	"github.com/frostyard/frostnotes/internal/evidence"
	"github.com/frostyard/frostnotes/internal/rpctest"
	"github.com/nsgo-dev/nsgo/nsgotest"
)

// Start with an empty device, approve its displayed code at a disposable hub,
// and restart the device on its original database and secret store.
func TestVaultSetup(t *testing.T) {
	bin := evidence.Build(t, "github.com/frostyard/frostnotes/cmd/notesd")
	dev := evidence.Start(t, bin, "device")
	t.Setenv("FROSTNOTES_SOCKET", dev.Socket)
	hub := rpctest.StartWith(t, rpctest.Options{HubListen: true})
	hc := hub.Dial(t)
	hv, err := hc.Vault().Add(t.Context(), &api.VaultAddParams{Path: t.TempDir(), Name: new("Garden")})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := hc.Hub().Serve(t.Context(), &api.HubServeParams{VaultID: hv.ID}); err != nil {
		t.Fatal(err)
	}
	rootDir := t.TempDir()
	app := nsgotest.Start(t, root, nsgotest.Size(1150, 760))
	app.WaitFor("text:Connected", 5*time.Second)
	app.Press("label:Create new vault")
	app.Type("label:Vault name", "Private")
	app.Type("label:Parent folder", rootDir)
	app.Press("label:Continue")
	app.WaitFor("label:Private", 5*time.Second)
	app.Press("label:Settings")
	app.Press("label:Open folder as vault")
	app.Type("label:Vault folder", dev.Dir)
	app.Press("label:Continue")
	app.WaitFor("label:"+filepath.Base(dev.Dir), 5*time.Second)
	app.Press("label:Settings")
	app.Press("label:Join a vault on a hub")
	app.Type("label:Hub URL", hub.HubURL)
	app.Type("label:Hub vault name", "Garden")
	app.Type("label:Parent folder", rootDir)
	app.Press("label:Continue")
	var code string
	app.WaitUntil(10*time.Second, func() bool {
		pending, err := hc.Hub().Pending(t.Context(), &api.HubPendingParams{})
		if err != nil || len(pending) == 0 {
			return false
		}
		code = pending[0].Code
		return strings.Contains(app.Dump(), "notesctl hub approve "+code)
	})
	if _, err := hc.Hub().Approve(t.Context(), &api.HubApproveParams{Code: code}); err != nil {
		t.Fatal(err)
	}
	app.WaitFor("label:Garden", 10*time.Second)
	app.WaitUntil(10*time.Second, func() bool { return strings.Contains(app.Dump(), "Garden · idle") })
	dev.Restart(t)
	_, err = hc.Note().Create(t.Context(), &api.NoteCreateParams{VaultID: hv.ID, Path: "Mint.md", Content: "Fresh mint"})
	if err != nil {
		t.Fatal(err)
	}
	// The session dials again on a timer of the app's clock.
	waitTimers(t, app, 25*time.Second, func() bool { return app.Exists("label:Mint") })
	states, err := dev.C.Sync().Status(t.Context(), &api.SyncStatusParams{})
	if err != nil || len(states) != 1 || states[0].URL != hub.HubURL {
		t.Fatalf("restarted pairing: %v, %v", states, err)
	}
	b, err := os.ReadFile(filepath.Join(rootDir, "Garden", "Mint.md"))
	if err != nil || string(b) != "Fresh mint" {
		t.Fatalf("synced bytes %q: %v", b, err)
	}
}
