//go:build linux && integration

package main

import (
	"encoding/json"
	"slices"
	"testing"
	"time"

	"github.com/frostyard/frostui/workspaceui"
	"github.com/nsgo-dev/nsgo/core"
	"github.com/nsgo-dev/nsgo/nsgotest"
)

// windowKey is where the kit keeps the window (workspaceui.KeepWindow),
// and keptWindow what it keeps there.
const windowKey = "window"

type keptWindow struct{ Width, Height core.Points }

// desktopWindow is a desktop's environment, where the window is the app's
// to keep.
var desktopWindow = nsgotest.Environment(core.Environment{OS: "linux", Idiom: core.IdiomDesktop, PixelRatio: 1})

// TestWindowSizeIsKept checks that a change of the window's size is kept
// on the device, and that a size under the least one is not.
func TestWindowSizeIsKept(t *testing.T) {
	app, _ := startVault(t, map[string]string{"First.md": "First\n"}, 1150, 760, desktopWindow)
	app.WaitFor("label:First", 10*time.Second)
	if _, kept := app.Storage().GetItem(windowKey); kept {
		t.Fatal("the window's first size is kept before it changed")
	}
	kept := func() (k keptWindow, ok bool) {
		text, ok := app.Storage().GetItem(windowKey)
		return k, ok && json.Unmarshal([]byte(text), &k) == nil
	}
	app.Resize(980, 700)
	waitTimers(t, app, 5*time.Second, func() bool { k, ok := kept(); return ok && k.Width == 980 && k.Height == 700 })
	app.Resize(300, 200)
	app.Advance(time.Second)
	if k, _ := kept(); k.Width != 980 || k.Height != 700 {
		t.Errorf("a window 300 by 200 replaced the kept size: %+v", k)
	}
}

// TestKeptWindowAndPanesShowAtStart checks that a start shows the window
// at the size the device kept and the navigation at its kept width, and
// that a drag of the navigation's edge is kept.
func TestKeptWindowAndPanesShowAtStart(t *testing.T) {
	app, _ := startVault(t, map[string]string{"First.md": "First\n"}, 1150, 760, desktopWindow,
		nsgotest.Storage(map[string]string{windowKey: `{"Width":1000,"Height":720}`, panesKey: `{"Navigation":300}`}))
	app.WaitFor("label:First", 10*time.Second)
	app.WaitUntil(5*time.Second, func() bool { return app.Find("id:sidebar-panel").Bounds().Size.Width == 300 })
	// Headless, the host records what the app asked of its window and
	// keeps its size; a real window takes the size.
	if nsgotest.Target() == "" {
		asked := slices.ContainsFunc(app.Window, func(r nsgotest.WindowRequest) bool {
			return r.Action == nsgotest.WindowSetFrame && r.Frame.Width == 1000 && r.Frame.Height == 720
		})
		if !asked {
			t.Errorf("the app did not ask for its kept window, 1000 by 720: %+v", app.Window)
		}
	} else {
		app.WaitUntil(5*time.Second, func() bool {
			size := app.Find("id:frame").Bounds().Size
			return size.Width == 1000 && size.Height == 720
		})
	}
	app.Drag("label:Resize navigation", core.Point{X: 40})
	app.Advance(time.Second)
	var saved workspaceui.PaneWidths
	app.WaitUntil(5*time.Second, func() bool {
		text, _ := app.Storage().GetItem(panesKey)
		return json.Unmarshal([]byte(text), &saved) == nil && saved.Navigation > 300
	})
}

// TestFilterNarrowsTheTree checks the filter over the notes tree: it
// leaves the notes whose title or path holds its words, inside their
// folders, says so when none does, and mod+F puts the keyboard in it from
// the editor.
func TestFilterNarrowsTheTree(t *testing.T) {
	app, _ := startVault(t, map[string]string{"Welcome.md": "Welcome\n", "Plants/Fig.md": "Fig\n", "Plants/Herbs/Basil.md": "Basil\n"}, 1150, 760)
	row := func(name string) string { return "id:notes-tree >> label:" + name }
	app.WaitFor(row("Basil"), 10*time.Second)
	const filter = "label:Filter notes"
	app.Type(filter, "bas")
	app.WaitForGone(row("Fig"), 5*time.Second)
	for _, name := range []string{"Plants", "Herbs", "Basil"} {
		app.Find(row(name))
	}
	if app.Exists(row("Welcome")) {
		t.Error("a note the filter does not match is in the tree")
	}
	// A folder's name matches the notes under it, by their path.
	app.Clear(filter)
	app.Type(filter, "herbs")
	app.WaitFor(row("Basil"), 5*time.Second)
	app.Clear(filter)
	app.Type(filter, "quince")
	app.WaitFor("text:No matching notes", 5*time.Second)
	app.Clear(filter)
	app.WaitFor(row("Welcome"), 5*time.Second)
	app.Press(row("Fig"))
	app.WaitFor("text:Plants/Fig.md · Saved", 5*time.Second)
	app.Focus("label:Markdown source")
	app.Key("mod+f")
	app.WaitUntil(5*time.Second, func() bool { return app.Find(filter).Focused })
}

// TestTreeWidthIsKept checks that the notes' tree opens at the width the
// device kept, and that a drag of its edge is kept.
func TestTreeWidthIsKept(t *testing.T) {
	app, _ := startVault(t, map[string]string{"First.md": "First\n"}, 1150, 760, desktopWindow,
		nsgotest.Storage(map[string]string{treeKey: `300`}))
	app.WaitFor("label:First", 10*time.Second)
	edge := func() core.Points { return app.Find("label:Resize file tree").Bounds().Origin.X }
	first := edge()
	app.Drag("label:Resize file tree", core.Point{X: 40})
	app.Advance(time.Second)
	var kept core.Points
	app.WaitUntil(5*time.Second, func() bool {
		text, _ := app.Storage().GetItem(treeKey)
		return json.Unmarshal([]byte(text), &kept) == nil && kept > 300
	})
	if moved := edge() - first; moved <= 0 {
		t.Errorf("the tree's edge moved %v after a drag of 40 to the right", moved)
	}
}

// TestTreeOpensAtItsFirstWidth checks a first start, and that the kept
// width is where the tree's edge is.
func TestTreeOpensAtItsFirstWidth(t *testing.T) {
	wide, _ := startVault(t, map[string]string{"First.md": "First\n"}, 1150, 760, desktopWindow,
		nsgotest.Storage(map[string]string{treeKey: `300`}))
	wide.WaitFor("label:First", 10*time.Second)
	kept := wide.Find("label:Resize file tree").Bounds().Origin.X
	app, _ := startVault(t, map[string]string{"First.md": "First\n"}, 1150, 760, desktopWindow)
	app.WaitFor("label:First", 10*time.Second)
	if first := app.Find("label:Resize file tree").Bounds().Origin.X; kept-first != 300-treeFirst {
		t.Errorf("the tree's edge is at %v with 300 kept and at %v with nothing kept: want them %v apart", kept, first, 300-treeFirst)
	}
	if _, saved := app.Storage().GetItem(treeKey); saved {
		t.Error("the tree's width is kept before it changed")
	}
}
