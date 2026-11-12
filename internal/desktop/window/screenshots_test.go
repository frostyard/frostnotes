//go:build integration

package window

import (
	"image/png"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/frostyard/frostui"

	"github.com/frostyard/frostnotes/internal/desktop/workspace"
)

// TestMetainfoScreenshots takes the Flatpak metainfo's screenshots and the
// README's dark editor (make screenshots) at 1280 x 800, on a notesd with a
// copy of the made-up vault in dev/screenshots/vault: no real person,
// address or note.
func TestMetainfoScreenshots(t *testing.T) {
	dir := os.Getenv("FROSTNOTES_SCREENSHOTS")
	if dir == "" {
		t.Skip("make screenshots sets FROSTNOTES_SCREENSHOTS")
	}
	// The brand tile draws the app's logo, as cmd/frostnotes registers it,
	// rather than the kit's stand-in for an unknown icon.
	logo, err := os.ReadFile(filepath.Join("..", "..", "..", "cmd", "frostnotes", "logo.svg"))
	if err != nil {
		t.Fatal(err)
	}
	frostui.SetIcons(frostui.NewIcons(map[string][]byte{"org.frostyard.Frostnotes-symbolic": logo}))
	f := start(t)
	f.tt.SetSize(1280, 800)
	root := filepath.Join("..", "..", "..", "dev", "screenshots", "vault")
	err = filepath.WalkDir(root, func(p string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		data, err := os.ReadFile(p)
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(root, p)
		if err != nil {
			return err
		}
		f.Create(rel, string(data))
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	f.openVault()
	f.until("the catalog", func() bool { return f.a.hasFile("Projects/Cabin Weekend.md") })
	f.a.vault.Name = "Notebook"

	settle := func() {
		for range 40 {
			f.loop.Advance(time.Second)
			f.tt.Frame()
			f.loop.Drain()
		}
	}
	shot := func(name string) {
		t.Helper()
		settle()
		out, err := os.Create(filepath.Join(dir, name+".png"))
		if err != nil {
			t.Fatal(err)
		}
		defer func() { _ = out.Close() }()
		if err := png.Encode(out, f.tt.Image()); err != nil {
			t.Fatal(err)
		}
	}
	// open opens p with the caret just after the first after in its text,
	// or at its end when after is empty, and shows its top.
	open := func(p, after string) {
		t.Helper()
		f.a.open(p, workspace.Here)
		f.until(p, func() bool {
			e := f.a.activeEditor()
			return f.a.activeFile() == p && e != nil && e.Binding().Document().Loaded()
		})
		e := f.a.activeEditor()
		text := e.Binding().Document().Text()
		at := len(text)
		if after != "" {
			i := strings.Index(text, after)
			if i < 0 {
				t.Fatalf("no %q in %s", after, p)
			}
			at = i + len(after)
		}
		e.State.Select(at, at)
		settle()
		e.State.ScrollTo(0)
	}

	open("Projects/Cabin Weekend.md", "")
	shot("editor")

	// The caret ends the last step: on the empty line after the callout,
	// the caret's line would draw as an empty band.
	open("Recipes/Winter Squash Soup.md", "black pepper.")
	f.a.searchFor("soup")
	shot("search")

	open("Projects/Cabin Weekend.md", "")
	f.a.Store.Update(func(w *workspace.Workspace) {
		w.SelectPane(workspace.SideLeft, "files")
		w.SelectPane(workspace.SideRight, "tasks")
	})
	shot("tasks")

	f.tt.SetDark(true)
	open("Projects/Cabin Weekend.md", "")
	f.a.Store.Update(func(w *workspace.Workspace) { w.SelectPane(workspace.SideRight, "backlinks") })
	shot("editor-dark")

	open("Reading/The Quiet Orchard.md", "")
	f.a.run("markdown:toggle-preview")
	f.a.Store.Update(func(w *workspace.Workspace) { w.SelectPane(workspace.SideRight, "outline") })
	f.until("the reading view", func() bool { return f.a.activeEditor() == nil })
	shot("dark")
}
