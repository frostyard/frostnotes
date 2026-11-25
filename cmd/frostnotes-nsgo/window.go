package main

import (
	"encoding/json"
	"time"

	"github.com/frostyard/frostui/workspaceui"
	"github.com/nsgo-dev/nsgo/core"
)

// The window is kept on the device (the kit's standard for an app, rule
// 8): its size and its place, which the kit keeps (workspaceui.KeepWindow),
// the width of the navigation, and the width of the notes' tree. A start
// shows the window as it was left. The choice is the device's, as the
// appearance is, and lives in the client's storage.

const (
	panesKey = "pane-widths"
	treeKey  = "tree-width"
	// The tree's first width.
	treeFirst core.Points = 230
	// The least size of a kept window: under it the window opens at its
	// own first size.
	minWidth, minHeight core.Points = 480, 360
)

func load[T any](cx *core.Scope, key string) (T, bool) {
	var v T
	text, ok := cx.Storage().GetItem(key)
	if !ok || json.Unmarshal([]byte(text), &v) != nil {
		var zero T
		return zero, false
	}
	return v, true
}

func (w *workspace) save(key string, v any) {
	text, err := json.Marshal(v)
	if err == nil {
		err = w.cx.Storage().SetItem(key, string(text))
	}
	if err != nil {
		w.problem.Set(err.Error())
	}
}

// paneWidths are the kept widths of the shell's panes, and savePanes keeps
// them as a drag of a divider changes them.
func (w *workspace) paneWidths() workspaceui.PaneWidths {
	widths, _ := load[workspaceui.PaneWidths](w.cx, panesKey)
	return widths
}

func (w *workspace) savePanes(widths workspaceui.PaneWidths) { w.save(panesKey, widths) }

// treeWidth is the kept width of the notes' tree, its first width where
// the device keeps none.
func (w *workspace) treeWidth() core.Points {
	if width, ok := load[core.Points](w.cx, treeKey); ok && width > 0 {
		return width
	}
	return treeFirst
}

// keepTree keeps the tree's width once a drag of its edge has rested: a
// drag reports many widths.
func (w *workspace) keepTree(width core.Points) {
	if w.treePending != nil {
		w.treePending()
	}
	w.treePending = w.cx.AfterFunc(200*time.Millisecond, func() { w.save(treeKey, width) })
}
