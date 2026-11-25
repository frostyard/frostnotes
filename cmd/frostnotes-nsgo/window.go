package main

import (
	"encoding/json"
	"time"

	"github.com/frostyard/frostui/workspaceui"
	"github.com/nsgo-dev/nsgo/core"
	"github.com/nsgo-dev/nsgo/desktop"
)

// The window is kept on the device (the kit's standard for an app, rule
// 8): its size, its place where the platform says it, and the width of
// the navigation. A start shows the window as it was left. The choice is
// the device's, as the appearance is, and lives in the client's storage.

const (
	windowKey = "window"
	panesKey  = "pane-widths"
	// The least size of a kept window: under it the window opens at its
	// own first size.
	minWidth, minHeight core.Points = 480, 360
)

// keptWindow is the window as the device keeps it.
type keptWindow struct {
	Width, Height core.Points
	X, Y          core.Points
	// Placed says X and Y are a place the platform reported.
	Placed bool
}

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

// keepWindow shows the window as the device kept it, and keeps it as it
// changes. A browser's window is the browser's, and is not kept.
func (w *workspace) keepWindow(cx *core.Scope) {
	if !desktop.IsAvailable(cx) {
		return
	}
	if k, ok := load[keptWindow](cx, windowKey); ok && k.Width >= minWidth && k.Height >= minHeight {
		desktop.SetWindowFrame(cx, core.WindowFrame{Width: k.Width, Height: k.Height, X: k.X, Y: k.Y, Move: k.Placed})
	}
	var pending func() bool
	first := true
	cx.Effect(func() {
		size, frame, large := cx.Env().Window(), desktop.Frame(cx), desktop.Maximized(cx)
		if first {
			// The size the window opens at is not a change of it.
			first = false
			return
		}
		// A maximized window keeps the size it had before.
		if large || size.Width < minWidth || size.Height < minHeight {
			return
		}
		k := keptWindow{Width: size.Width, Height: size.Height, X: frame.X, Y: frame.Y, Placed: frame.Move}
		if pending != nil {
			pending()
		}
		// A drag of the window's edge reports many sizes: keep the last.
		pending = cx.AfterFunc(300*time.Millisecond, func() { w.save(windowKey, k) })
	})
}

// paneWidths are the kept widths of the shell's panes, and savePanes keeps
// them as a drag of a divider changes them.
func (w *workspace) paneWidths() workspaceui.PaneWidths {
	widths, _ := load[workspaceui.PaneWidths](w.cx, panesKey)
	return widths
}

func (w *workspace) savePanes(widths workspaceui.PaneWidths) { w.save(panesKey, widths) }
