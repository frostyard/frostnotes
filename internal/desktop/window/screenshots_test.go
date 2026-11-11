//go:build integration

package window

import (
	"image/png"
	"io/fs"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/frostyard/frostui"

	"github.com/frostyard/frostnotes/internal/desktop/workspace"
)

// TestMetainfoScreenshots takes the Flatpak metainfo's screenshots (make
// screenshots) at 1280 x 800, on a notesd with a copy of the made-up vault
// in dev/screenshots/vault: no real person, address or note.
func TestMetainfoScreenshots(t *testing.T) {
	dir := os.Getenv("FROSTNOTES_SCREENSHOTS")
	if dir == "" {
		t.Skip("make screenshots sets FROSTNOTES_SCREENSHOTS")
	}
	frostui.SetIcons(frostui.NewIcons(nil))
	f := start(t)
	f.tt.SetSize(1280, 800)
	root := filepath.Join("..", "..", "..", "dev", "screenshots", "vault")
	err := filepath.WalkDir(root, func(p string, d fs.DirEntry, err error) error {
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
	open := func(p string) {
		t.Helper()
		f.a.open(p, workspace.Here)
		f.until(p, func() bool {
			e := f.a.activeEditor()
			return f.a.activeFile() == p && e != nil && e.Binding().Document().Loaded()
		})
		e := f.a.activeEditor()
		end := len(e.Binding().Document().Text())
		e.State.Select(end, end)
		settle()
		e.State.ScrollTo(0)
	}

	open("Projects/Cabin Weekend.md")
	shot("editor")

	open("Recipes/Winter Squash Soup.md")
	f.a.searchFor("soup")
	shot("search")

	open("Projects/Cabin Weekend.md")
	f.a.Store.Update(func(w *workspace.Workspace) {
		w.SelectPane(workspace.SideLeft, "files")
		w.SelectPane(workspace.SideRight, "tasks")
	})
	shot("tasks")

	f.tt.SetDark(true)
	open("Reading/The Quiet Orchard.md")
	f.a.run("markdown:toggle-preview")
	f.a.Store.Update(func(w *workspace.Workspace) { w.SelectPane(workspace.SideRight, "outline") })
	f.until("the reading view", func() bool { return f.a.activeEditor() == nil })
	shot("dark")
}
