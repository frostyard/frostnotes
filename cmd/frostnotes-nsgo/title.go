package main

import (
	"github.com/frostyard/frostnotes/internal/desktop/catalog"
	"github.com/frostyard/frostnotes/internal/desktop/notes"
	"github.com/frostyard/frostui/workspaceui"
	"github.com/nsgo-dev/nsgo/core"
	"github.com/nsgo-dev/nsgo/ui"
)

// The inline title: the note's name, as an editable title above it, as
// Obsidian and the MyGo client show it (internal/desktop/editor/noteview.go).
// Enter, or leaving the field, renames the file through the client's move
// (manage.go). The vault's Automatically update internal links says
// whether the move updates the links to it; without it the client asks.

// updateLinks is the question the inline title's move asks.
const updateLinks = "Update links?"

// pendingRename is a move of the inline title waiting for the answer.
type pendingRename struct {
	d        *notes.Document
	from, to string
	base     string
	dirty    bool
	reset    func()
}

// inlineTitle is the field of the note d's title.
func (w *workspace) inlineTitle(cx *core.Scope, d *notes.Document) core.Node {
	shown := d.Path()
	title := core.NewSignal(cx, catalog.Title(shown))
	// A move, here or elsewhere, shows the new name.
	cx.Effect(func() {
		w.revision.Get()
		if p := d.Path(); p != shown {
			shown = p
			title.Set(catalog.Title(p))
		}
	})
	reset := func() { title.Set(catalog.Title(d.Path())) }
	commit := func() {
		if w.rename != nil {
			return
		}
		to, ok := catalog.Renamed(d.Path(), title.GetUntracked())
		if !ok {
			reset()
			return
		}
		r := &pendingRename{d: d, from: d.Path(), to: to, base: d.Base(), dirty: d.Unsaved(), reset: reset}
		if w.vaultSettings != nil && w.vaultSettings.UpdateLinks {
			w.renameNote(r, true)
			return
		}
		w.rename = r
		w.action.Set(updateLinks)
	}
	// A heading of the note's own size, with no field's frame, as wide as
	// source mode's text and over it.
	colors := ui.ColorsOf(cx)
	style := workspaceui.Heading(1).Merge(core.Style{Width: core.Pct(100), MaxWidth: core.Pt(readableWidth), BorderWidth: 0, Background: core.Transparent})
	return core.View{Style: core.Style{AlignItems: core.AlignCenter, Padding: core.Edges{Bottom: workspaceui.SpaceS}}, Children: []core.Node{
		core.Input{Label: "Note title", Value: title.Get, OnChange: title.Set, Readonly: d.ReadOnly() != "",
			OnSubmit: func(string) { commit() }, OnBlur: commit, Style: style,
			StyleFunc: func() core.Style { return core.Style{Color: colors.Text(), PlaceholderColor: colors.Muted()} }},
	}}
}

// renameNote moves the note as the inline title asked, its links updated
// with update.
func (w *workspace) renameNote(r *pendingRename, update bool) {
	w.rename = nil
	w.manage(r.d, r.from, r.to, update, r.base, r.dirty, func(bool) {}, func(text string) {
		if text != "" {
			w.problem.Set(text)
			r.reset()
		}
	}, func() {})
}

// updateLinksDialog asks whether the inline title's move updates the
// links to the note, as the MyGo client asks when the vault does not
// update them by itself. Closing it leaves the name as it was.
func (w *workspace) updateLinksDialog(cx *core.Scope) core.Node {
	r := w.rename
	if r == nil {
		return nil
	}
	cx.Cleanup(func() {
		if w.rename == r {
			w.rename = nil
			r.reset()
		}
	})
	answer := func(update bool) func() {
		return func() {
			w.rename = nil
			w.action.Set("")
			w.renameNote(r, update)
		}
	}
	return ui.AlertDialog{Title: updateLinks, Description: `Update the links to "` + catalog.Title(r.from) + `" so they still lead to it?`,
		StyleFunc: workspaceui.DialogPanel(cx), Open: func() bool { return true }, OnOpenChange: w.closeDialog,
		Actions: []ui.AlertAction{
			{Text: "Don't update", Variant: ui.ButtonSecondary, OnPress: answer(false)},
			{Text: "Update", Default: true, OnPress: answer(true)}}}
}
