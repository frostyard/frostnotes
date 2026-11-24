//go:build linux && integration

package main

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	"github.com/frostyard/frostnotes/api"
	"github.com/nsgo-dev/nsgo/nsgotest"
)

// Exercise the actual UI callbacks. Selecting the next note while the old
// editor is dirty must flush that editor's base and text to its own path.
func TestSwitchNoteDuringAutosave(t *testing.T) {
	dir := t.TempDir()
	if err := os.Chmod(dir, 0700); err != nil {
		t.Fatal(err)
	}
	vault := filepath.Join(dir, "Vault")
	if err := os.Mkdir(vault, 0700); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"First", "Second"} {
		if err := os.WriteFile(filepath.Join(vault, name+".md"), []byte(name+"\r\n"), 0600); err != nil {
			t.Fatal(err)
		}
	}
	socket := filepath.Join(dir, "n.sock")
	t.Setenv("FROSTNOTES_SOCKET", socket)
	t.Setenv("FROSTNOTES_DATA_DIR", filepath.Join(dir, "data"))
	binary := filepath.Join(dir, "notesd")
	build := exec.CommandContext(t.Context(), "go", "-C", "../..", "build", "-o", binary, "./cmd/notesd")
	if output, err := build.CombinedOutput(); err != nil {
		t.Fatalf("build notesd: %v\n%s", err, output)
	}
	daemon := exec.Command(binary)
	daemon.Stderr = os.Stderr
	if err := daemon.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = daemon.Process.Kill(); _ = daemon.Wait() })
	ctx, cancel := context.WithTimeout(t.Context(), 10*time.Second)
	defer cancel()
	var c *api.Client
	for {
		var err error
		c, _, err = api.Dial(ctx, socket, "workspace-test")
		if err == nil {
			break
		}
		select {
		case <-ctx.Done():
			t.Fatal(err)
		case <-time.After(10 * time.Millisecond):
		}
	}
	defer c.Close()
	if _, err := c.Vault().Add(ctx, &api.VaultAddParams{Path: vault}); err != nil {
		t.Fatal(err)
	}
	app := nsgotest.Start(t, root, nsgotest.Size(1150, 760))
	app.WaitFor("label:First", 10*time.Second)
	app.Press("label:First")
	app.WaitFor("text:Saved", 10*time.Second)
	const source = "label:Markdown source"
	app.Clear(source)
	app.Type(source, "updated\n")
	app.Press("label:Second")
	app.WaitUntil(10*time.Second, func() bool { return app.Exists(source) && app.Find(source).Text == "Second\n" })
	if app.Exists("text:No vaults available") {
		t.Fatal("empty state alongside populated vault")
	}
	got, err := os.ReadFile(filepath.Join(vault, "First.md"))
	if err != nil || string(got) != "updated\r\n" {
		t.Fatalf("first note = %q, %v", got, err)
	}
}
