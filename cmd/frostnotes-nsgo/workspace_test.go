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

// TestMain starts NSGo's test host, so the suite also runs in a real window:
// go tool nsgo test -target linux (make nsgo-live).
func TestMain(m *testing.M) { nsgotest.Main(m) }

// repoRoot finds the repository from the working directory, which is this
// package's directory under go test and a directory below it on a target.
func repoRoot(t *testing.T) string {
	t.Helper()
	dir, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	for {
		if _, err := os.Stat(filepath.Join(dir, "cmd", "notesd")); err == nil {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			t.Fatal("no cmd/notesd above the working directory")
		}
		dir = parent
	}
}

// waitTimers waits as app.WaitUntil does while the app's own timers run.
// Headless, NSGo's waits move the app's clock a frame between checks. On a
// target they leave it still, so a debounce or a retry that the app set with
// AfterFunc never comes due there. This moves the clock in both.
func waitTimers(t *testing.T, app *nsgotest.App, timeout time.Duration, cond func() bool) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for !cond() {
		if time.Now().After(deadline) {
			t.Fatalf("the condition did not hold after %v\n%s", timeout, app.Dump())
		}
		app.Advance(100 * time.Millisecond)
	}
}

// headlessStep reports whether the run is headless, and on a target logs why
// the step it guards is left out there. The rest of the test still runs in
// the real window.
func headlessStep(t *testing.T, why string) bool {
	t.Helper()
	if target := nsgotest.Target(); target != "" {
		t.Logf("left out on %s: %s", target, why)
		return false
	}
	return true
}

// typeLines types text with line breaks into a multiline field. On a target
// NSGo v0.3.0's Linux test host sends a line break as the Enter key, which
// submits a single-line field and does nothing in a text view, so there the
// text goes into the same widget as an input method's commit.
func typeLines(app *nsgotest.App, selector, text string) {
	if nsgotest.Target() == "" {
		app.Type(selector, text)
		return
	}
	app.Focus(selector)
	app.Commit(selector, text)
}

// Exercise the actual UI callbacks. Selecting the next note while the old
// editor is dirty must flush that editor's base and text to its own path.
func startWorkspace(t *testing.T) (*nsgotest.App, string) {
	t.Helper()
	app, vault := startVault(t, map[string]string{"First.md": "First\r\n", "Second.md": "Second\r\n"}, 1150, 760)
	app.WaitFor("label:First", 10*time.Second)
	return app, vault
}

// startVault writes files into a new vault named Vault, starts a notesd that
// holds only that vault, and opens the client on it in a window of the
// given size, with NSGo's options. A nil files starts the client on a notesd
// with no vault.
func startVault(t *testing.T, files map[string]string, width, height core.Points, options ...nsgotest.Option) (*nsgotest.App, string) {
	t.Helper()
	dir := t.TempDir()
	if err := os.Chmod(dir, 0700); err != nil {
		t.Fatal(err)
	}
	vault := filepath.Join(dir, "Vault")
	for name, text := range files {
		path := filepath.Join(vault, filepath.FromSlash(name))
		if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(text), 0600); err != nil {
			t.Fatal(err)
		}
	}
	socket := filepath.Join(dir, "n.sock")
	t.Setenv("FROSTNOTES_SOCKET", socket)
	t.Setenv("FROSTNOTES_DATA_DIR", filepath.Join(dir, "data"))
	binary := filepath.Join(dir, "notesd")
	build := exec.CommandContext(t.Context(), "go", "-C", repoRoot(t), "build", "-o", binary, "./cmd/notesd")
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
	if files != nil {
		if _, err := c.Vault().Add(ctx, &api.VaultAddParams{Path: vault}); err != nil {
			t.Fatal(err)
		}
	}
	return nsgotest.Start(t, root, append([]nsgotest.Option{nsgotest.Size(width, height)}, options...)...), vault
}

// The vault manager's choice of appearance shows at once and is kept on the
// device.
func TestAppearanceChoiceIsKept(t *testing.T) {
	app, _ := startWorkspace(t)
	app.Press("label:Manage vaults")
	app.WaitFor("role:tab Dark", 5*time.Second)
	app.Press("role:tab Dark")
	if app.Appearance != core.AppearanceDark {
		t.Fatalf("the app asked for appearance %v after Dark, want dark", app.Appearance)
	}
	if got, _ := app.Storage().GetItem("appearance"); got != "dark" {
		t.Fatalf("the kept appearance is %q, want dark", got)
	}
	app.Press("role:tab System")
	if _, kept := app.Storage().GetItem("appearance"); kept || app.Appearance != core.AppearanceSystem {
		t.Fatalf("after System the device keeps a choice (%v) or the app asks for %v", kept, app.Appearance)
	}
}

// A start shows the appearance the device kept.
func TestKeptAppearanceShowsAtStart(t *testing.T) {
	app, _ := startVault(t, map[string]string{"First.md": "First\n"}, 1150, 760, nsgotest.Storage(map[string]string{"appearance": "light"}))
	app.WaitFor("label:First", 10*time.Second)
	if app.Appearance != core.AppearanceLight {
		t.Fatalf("the app asked for appearance %v at its start, want light", app.Appearance)
	}
	app.Press("label:Manage vaults")
	app.WaitFor("role:tab Light", 5*time.Second)
	if !app.Find("role:tab Light").Props.Selected {
		t.Fatalf("the choice does not show Light as chosen\n%s", app.Dump())
	}
}

func TestSwitchNoteDuringAutosave(t *testing.T) {
	app, vault := startWorkspace(t)
	app.Press("label:First")
	app.WaitFor("text:Saved", 10*time.Second)
	const source = "label:Markdown source"
	app.Clear(source)
	typeLines(app, source, "updated\n")
	app.Press("label:Second")
	app.WaitUntil(10*time.Second, func() bool { return app.Exists(source) && app.Find(source).Text == "Second\n" })
	if app.Exists("text:No vaults available") {
		t.Fatal("empty state alongside populated vault")
	}
	app.Clear(source)
	typeLines(app, source, "# Garden 🌱\n- [ ] prune\nSee [[First|orchard]].\n```\n- [ ] literal\n```\n")
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
	if headlessStep(t, "NSGo v0.3.0 reports a paste only headless: App.Paste skips the test on a target (2026-11-24)") {
		app.Paste(field, core.PasteEvent{Text: "\nseedling"})
		app.WaitUntil(time.Second, func() bool { return app.Exists(field) && strings.Contains(app.Find(field).Text, "seedling") })
		app.Key("mod+z")
		app.WaitUntil(time.Second, func() bool { return app.Exists(field) && app.Find(field).Text == "- [ ] prune" })
	}
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
	waitTimers(t, app, 5*time.Second, func() bool { return strings.Contains(app.Dump(), " · a scarlet quince") })
	app.Key("enter")
	app.WaitFor("text:Plants/Field notes.md", 5*time.Second)
}

func TestManageNotes(t *testing.T) {
	app, vault := startWorkspace(t)
	const field = "label:Markdown source"
	app.Press("label:First")
	app.WaitFor("text:Saved", 5*time.Second)
	app.Type(field, "draft")
	// Rename from a second client before autosave. The active buffer
	// follows note.moved and its save must land at the new path.
	c, _, err := api.Dial(t.Context(), os.Getenv("FROSTNOTES_SOCKET"), "other-window")
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()
	vs, err := c.Vault().List(t.Context(), &api.VaultListParams{})
	if err != nil {
		t.Fatal(err)
	}
	id := vs[0].ID
	_, err = c.Note().Move(t.Context(), &api.NoteMoveParams{VaultID: id, Path: "First.md", To: "Renamed.md"})
	if err != nil {
		t.Fatal(err)
	}
	app.WaitFor("text:Renamed.md", 5*time.Second)
	app.Press("label:Rename or move note")
	app.Clear("label:Destination path")
	app.Type("label:Destination path", "Plants/Renamed")
	app.Press("label:Move")
	app.WaitFor("text:Plants/Renamed.md", 5*time.Second)
	app.Type(field, "+latest")
	app.Press("label:Move to trash")
	app.Press("label:Cancel")
	app.Press("label:Move to trash")
	app.Press("label:Trash note")
	app.WaitFor("text:Select a note to edit", 5*time.Second)
	for _, p := range []string{"First.md", "Renamed.md", "Plants/Renamed.md"} {
		if _, err := os.Stat(filepath.Join(vault, p)); !os.IsNotExist(err) {
			t.Fatalf("%s after trash: %v", p, err)
		}
	}
	app.Press("label:Restore deleted note")
	app.Type("label:Deleted note path", "Plants/Renamed.md")
	app.Press("label:Restore")
	app.WaitUntil(5*time.Second, func() bool { return app.Exists(field) && app.Find(field).Text == "First\ndraft+latest" })
	// A second restore must never overwrite a live file.
	if err := restoreDeleted(t.Context(), c, id, "Plants/Renamed.md"); err == nil {
		t.Fatal("restored over a live note")
	}
}
