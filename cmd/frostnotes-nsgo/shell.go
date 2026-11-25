package main

import (
	"github.com/frostyard/frostnotes/internal/desktop/notes"
	"github.com/frostyard/frostui/workspaceui"
	"github.com/nsgo-dev/nsgo/core"
	"github.com/nsgo-dev/nsgo/icons/lucide"
	"github.com/nsgo-dev/nsgo/ui"
)

// The window's toolbar (the kit's standard for an app, rule 3). It holds
// what the row over the editor held: the open note's path and its state
// under the vault's name, and the note's actions as icon buttons, whose
// labels are their tooltips. New note is the window's one filled button.
// Without the room for them, the actions are one menu.

// toolbarRoom is the least width of the pane under the toolbar that shows
// the actions as buttons: New note, the editor's two modes, 4 icon
// buttons, the palette's field and the menu, beside a title.
const toolbarRoom core.Points = 760

// tool is an action of the toolbar: an icon button there, and an item of a
// narrow window's menu.
type tool struct {
	label string
	icon  func() *core.SVG
	keys  string
	// off dims it while there is nothing for it to act on.
	off func() bool
	run func()
}

func (t tool) button() core.Node {
	return ui.IconButton{Label: t.label, Icon: t.icon, Disabled: t.off, OnPress: t.run}
}

func (t tool) item() core.Node {
	return ui.MenuItem{Text: t.label, Icon: t.icon, Shortcut: t.keys, Disabled: t.off, OnSelect: t.run}
}

func buttons(tools []tool) []core.Node {
	nodes := make([]core.Node, len(tools))
	for i, t := range tools {
		nodes[i] = t.button()
	}
	return nodes
}

func items(tools []tool) []core.Node {
	nodes := make([]core.Node, len(tools))
	for i, t := range tools {
		nodes[i] = t.item()
	}
	return nodes
}

func (w *workspace) noVault() bool { return w.vault.Get() == 0 }

// openNote is the note the tools act on, as it is now.
func (w *workspace) openNote() *notes.Document {
	w.revision.Get()
	return w.doc.Get()
}

// noteTools act on the open note, vaultTools on the vault, and restTools
// are those a person seldom needs, which are always in the menu.
func (w *workspace) noteTools() []tool {
	noNote := func() bool { return w.openNote() == nil }
	return []tool{
		{label: "Save", icon: lucide.Save, keys: "mod+s", off: func() bool { d := w.openNote(); return d == nil || !d.Unsaved() }, run: func() {
			if d := w.doc.GetUntracked(); d != nil {
				d.RetryNow()
			}
		}},
		{label: moveNote, icon: lucide.Pencil, off: noNote, run: func() { w.showAction(moveNote) }},
		{label: trashNote, icon: lucide.Trash, off: noNote, run: func() { w.showAction(trashNote) }},
	}
}

func (w *workspace) vaultTools() []tool {
	return []tool{{label: vaultSearch, icon: lucide.FileSearch, keys: "mod+shift+f", off: w.noVault, run: func() { w.showAction(vaultSearch) }}}
}

// palette is the palette's way in for a window with no room for its field.
func (w *workspace) palette() tool {
	return tool{label: paletteLabel, icon: lucide.Search, keys: "mod+k", off: w.noVault, run: func() { w.showAction(quickOpen) }}
}

func (w *workspace) restTools() []tool {
	return []tool{
		{label: restoreNote, icon: lucide.ArchiveRestore, off: w.noVault, run: func() { w.showAction(restoreNote) }},
		{label: "Refresh vaults and notes", icon: lucide.RefreshCw, run: w.loadVaults},
	}
}

// subtitle is the open note's path and its state, "" with none open.
func (w *workspace) subtitle() string {
	d := w.openNote()
	if d == nil {
		return ""
	}
	return d.Path() + " · " + noteStatus(d)
}

func noteStatus(d *notes.Document) string {
	if reason := d.ReadOnly(); reason != "" {
		return reason
	}
	return [...]string{"Loading…", "Note is missing", "Saved", "Unsaved changes", "Saving…"}[d.Status()]
}

// header is the toolbar's start: the vault's name over the open note's
// path and state. While Settings is shown it names Settings.
func (w *workspace) header() core.Node {
	return core.Dynamic(func(cx *core.Scope) core.Node {
		colors := ui.ColorsOf(cx)
		title, subtitle := w.title, w.subtitle
		if w.settings.Get() {
			title, subtitle = func() string { return "Settings" }, func() string { return "" }
		}
		return core.View{Style: core.Style{FlexGrow: 1, FlexShrink: 1, MinWidth: core.Pt(0)}, Children: []core.Node{
			core.Text{ContentFunc: title, MaxLines: 1, Style: ui.TypeTitle},
			core.Show{When: func() bool { return subtitle() != "" }, Then: func(*core.Scope) core.Node {
				return core.Text{ContentFunc: subtitle, MaxLines: 1, Style: ui.TypeCaption, StyleFunc: ui.Colored(colors.Muted)}
			}}}}
	})
}

// bar is a row of the toolbar's parts, which never wraps.
func bar(children ...core.Node) core.Node {
	return core.View{Style: core.Style{Direction: core.Row, AlignItems: core.AlignCenter, Gap: 8, FlexShrink: 0}, Children: children}
}

// actions is the toolbar's end. With the room: New note, the editor's two
// modes, the note's and the vault's actions, the palette as a field with
// its keys, and the menu of the rest. Without it: New note and one menu of
// everything.
func (w *workspace) actions(cx *core.Scope) core.Node {
	narrow := core.NewMemo(cx, func() bool {
		window := cx.Env().Window().Width
		room := w.contentWidth.Get()
		if room <= 0 {
			// Before the first layout: the window less the navigation.
			room = window - 246
		}
		return window < 720 || room < toolbarRoom
	})
	create := func() { w.showAction(newNote) }
	return core.Dynamic(func(*core.Scope) core.Node {
		if w.settings.Get() {
			return bar(ui.Button{Text: "Done", Label: "Close settings", Variant: ui.ButtonSecondary, OnPress: func() { w.settings.Set(false) }})
		}
		if narrow.Get() {
			return bar(ui.IconButton{Label: newNote, Icon: lucide.FilePlus, Variant: ui.IconButtonPrimary, Disabled: w.noVault, OnPress: create}, w.moreMenu(true))
		}
		return bar(
			ui.Button{Text: newNote, Icon: lucide.FilePlus, Disabled: w.noVault, OnPress: create},
			// The modes are icons, named by their tooltips, so that the
			// palette's field has its room.
			ui.Tabs[bool]{Variant: ui.TabsSegmented, Label: "Editor", Value: w.live.Get, OnChange: w.live.Set,
				Options: []ui.Option[bool]{mode(false, "Source", lucide.Code), mode(true, "Live preview", lucide.Eye)}},
			ui.Toolbar{Label: "Note and vault", Children: []core.Node{
				ui.ToolbarGroup{Label: "Note", Children: buttons(w.noteTools())},
				ui.ToolbarSeparator{},
				ui.ToolbarGroup{Label: "Vault", Children: buttons(w.vaultTools())}}},
			ui.SidebarButton{Text: paletteLabel, Icon: lucide.Search, Shortcut: "mod+k", Variant: ui.SidebarButtonField, OnPress: w.palette().run},
			w.moreMenu(false))
	})
}

// mode is one of the editor's 2 modes in the toolbar: its icon, named by a
// tooltip and to assistive technology.
func mode(live bool, name string, icon func() *core.SVG) ui.Option[bool] {
	return ui.Option[bool]{Value: live, Label: name, Content: core.Dynamic(func(cx *core.Scope) core.Node {
		return ui.Tooltip{Text: name, Trigger: ui.Icon(icon, 16, ui.ColorsOf(cx).Text)}
	})}
}

// moreMenu is the toolbar's menu: the actions a person seldom needs, and,
// with all set, every action the toolbar has no room to show.
func (w *workspace) moreMenu(all bool) core.Node {
	var children []core.Node
	if all {
		children = append(children, core.Dynamic(func(*core.Scope) core.Node {
			if w.live.Get() {
				return ui.MenuItem{Text: "Show source", Icon: lucide.Code, OnSelect: func() { w.live.Set(false) }}
			}
			return ui.MenuItem{Text: "Show live preview", Icon: lucide.Eye, OnSelect: func() { w.live.Set(true) }}
		}))
		children = append(children, items(w.noteTools())...)
		children = append(children, ui.MenuSeparator{})
		children = append(children, w.palette().item())
		children = append(children, items(w.vaultTools())...)
		children = append(children, ui.MenuSeparator{})
	}
	children = append(children, items(w.restTools())...)
	return ui.Menu{Label: "More", Trigger: ui.Button{Label: "More", Icon: lucide.Ellipsis, Variant: ui.ButtonGhost}, Children: children}
}

// sidebarFoot is the navigation's foot: where the app is connected, and,
// on a desktop, the way to Settings.
func (w *workspace) sidebarFoot() core.Node {
	return core.Dynamic(func(*core.Scope) core.Node {
		tone := workspaceui.Warning
		if w.connection.Get() == connected {
			tone = workspaceui.Success
		}
		children := []core.Node{workspaceui.ConnectionItem{Tone: tone, Text: w.connectionLabel}, ui.Spacer{}}
		return core.View{Style: core.Style{Direction: core.Row, AlignItems: core.AlignCenter, Width: core.Pct(100)}, Children: append(children, w.settingsButton()...)}
	})
}
