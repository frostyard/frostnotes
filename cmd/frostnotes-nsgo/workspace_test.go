//go:build linux && integration

package main

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/frostyard/frostnotes/api"
	"github.com/nsgo-dev/nsgo/core"
	"github.com/nsgo-dev/nsgo/nsgotest"
)

// Exercise the actual UI callbacks. Selecting the next note while the old
// editor is dirty must flush that editor's base and text to its own path.
func startWorkspace(t *testing.T) (*nsgotest.App, string) {
	t.Helper()
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
	t.Cleanup(func() { _ = c.Close() })
	if _, err := c.Vault().Add(ctx, &api.VaultAddParams{Path: vault}); err != nil {
		t.Fatal(err)
	}
	app := nsgotest.Start(t, root, nsgotest.Size(1150, 760))
	app.WaitFor("label:First", 10*time.Second)
	return app, vault
}

func TestSwitchNoteDuringAutosave(t *testing.T) {
	app, vault := startWorkspace(t)
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
	app.Clear(source)
	app.Type(source, "# Garden 🌱\n- [ ] prune\nSee [[First|orchard]].\n```\n- [ ] literal\n```\n")
	app.Press("label:Live preview")
	app.WaitFor("label:Edit line 1", time.Second)
	app.Press("label:Task on line 2")
	app.Press("label:Edit line 2")
	const field = "label:Markdown line"
	app.WaitUntil(time.Second, func() bool { return app.Exists(field) && app.Find(field).Text == "- [x] prune" })
	app.Focus(field)
	app.Key("up")
	app.WaitUntil(time.Second, func() bool { return app.Find(field).Text == "# Garden 🌱" })
	app.Key("mod+z")
	app.WaitUntil(time.Second, func() bool { return app.Find(field).Text == "- [ ] prune" })
	app.Key("mod+shift+z")
	app.WaitUntil(time.Second, func() bool { return app.Find(field).Text == "- [x] prune" })
	app.Key("mod+z")
	app.WaitUntil(time.Second, func() bool { return app.Find(field).Text == "- [ ] prune" })
	// A multiline paste creates real document lines and leaves the field
	// on the last inserted line. Undo restores the original task marker.
	app.Paste(field, core.PasteEvent{Text: "\nseedling"})
	app.WaitUntil(time.Second, func() bool { return app.Exists(field) && strings.Contains(app.Find(field).Text, "seedling") })
	app.Key("mod+z")
	app.WaitUntil(time.Second, func() bool { return app.Exists(field) && app.Find(field).Text == "- [ ] prune" })
	if app.Exists("label:Task on line 5") {
		t.Fatal("task inside code fence")
	}
	app.Press("label:Open First")
	app.WaitUntil(10*time.Second, func() bool { return app.Exists("text:First.md") })
	second, err := os.ReadFile(filepath.Join(vault, "Second.md"))
	if err != nil || string(second) != "# Garden 🌱\r\n- [ ] prune\r\nSee [[First|orchard]].\r\n```\r\n- [ ] literal\r\n```\r\n" {
		t.Fatalf("inline source on disk = %q, %v", second, err)
	}
	got, err := os.ReadFile(filepath.Join(vault, "First.md"))
	if err != nil || string(got) != "updated\r\n" {
		t.Fatalf("first note = %q, %v", got, err)
	}
}

func TestCreateAndFindNote(t *testing.T) {
	app, vault := startWorkspace(t)
	app.Press("label:New note")
	app.Type("label:Note path", "Plants/Field notes")
	app.Press("label:Create")
	app.WaitFor("text:Plants/Field notes.md", 5*time.Second)
	app.WaitFor("text:Saved", 5*time.Second)
	app.Type("label:Markdown source", "a scarlet quince")
	app.Press("label:First")
	app.WaitFor("text:First.md", 5*time.Second)
	b, err := os.ReadFile(filepath.Join(vault, "Plants", "Field notes.md"))
	if err != nil || string(b) != "a scarlet quince" {
		t.Fatalf("new note = %q, %v", b, err)
	}
	app.Press("label:New note")
	app.Type("label:Note path", "Plants/Field notes.md")
	app.Press("label:Create")
	app.WaitUntil(5*time.Second, func() bool { return strings.Contains(app.Dump(), "Plants/Field notes.md exists") })
	app.Press("label:Cancel")
	app.Key("mod+o")
	app.Type("label:Find note", "Plants/Field")
	app.Key("enter")
	app.WaitFor("text:Plants/Field notes.md", 5*time.Second)
	app.Press("label:First")
	app.Key("mod+shift+f")
	app.Type("label:Find note", "scarlet")
	app.WaitUntil(5*time.Second, func() bool { return strings.Contains(app.Dump(), " · a scarlet quince") })
	app.Key("enter")
	app.WaitFor("text:Plants/Field notes.md", 5*time.Second)
}
