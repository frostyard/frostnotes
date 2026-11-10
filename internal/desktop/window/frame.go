package window

import (
	"strconv"

	"github.com/egoist/mygo/ui"
	"github.com/frostyard/frostui"

	"github.com/frostyard/frostnotes/internal/desktop/session"
	"github.com/frostyard/frostnotes/internal/desktop/uikit"
	"github.com/frostyard/frostnotes/internal/desktop/workspace"
)

// The frame below is a thin stand-in for frostui's PaneShell, which comes
// in v0.2.0; it goes when that lands.

func (a *App) frame(c *ui.Context) {
	t := frostui.Adapt(c)
	a.Palette.RunPending()
	a.bindKeys(c)
	ui.Column(c).Fill().AlignItems(ui.Stretch).Background(t.Background).Children(func() {
		a.topBar(c)
		ui.Box(c).Height(1).Background(t.Border)
		ui.Column(c).Grow(1).MinHeight(0).AlignItems(ui.Stretch).Children(func() { a.body(c) })
		ui.Box(c).Height(1).Background(t.Border)
		a.statusBar(c)
	})
	frostui.TopToasts(c, "Close")
	a.Dialogs.View(c)
	a.Info.View(c)
	a.sets.view(c, a)
	a.Palette.View(c, a.paletteCommands())
}

func (a *App) body(c *ui.Context) {
	switch {
	case a.vault == nil && (a.Session.State() != session.Connected || !a.listed):
		a.connecting(c)
	case a.vault == nil:
		a.vm.view(c, a, false)
	default:
		a.main(c)
	}
}

func (a *App) connecting(c *ui.Context) {
	ui.Column(c).Fill().Center().Gap(frostui.SpaceS).Role(ui.RoleStatus).Children(func() {
		text := "Connecting to notesd…"
		if r := a.Session.Reason(); r != "" {
			text = "Reconnecting to notesd… (" + r + ")"
		}
		frostui.Muted(c, text).Wrap()
		if a.cfg.Getenv != nil {
			if cause := notesdCause(a.cfg.Getenv); cause != "" {
				frostui.Muted(c, cause).Wrap()
			}
		}
	})
}

func (a *App) topBar(c *ui.Context) {
	t := c.Theme()
	tb := c.TitleBar()
	ui.Row(c).Height(frostui.TopBarHeight).Padding(0, frostui.SpaceL+tb.Right, 0, frostui.SpaceL+tb.Left).Gap(frostui.SpaceM).AlignItems(ui.Center).
		Background(frostui.Chrome(t)).DragWindow().Children(func() {
		a.brand(c)
		if a.vault != nil && frostui.IconButton(c, "sidebar-show", "Toggle left sidebar").Selected(a.Store.Workspace().Left.Open).Clicked() {
			a.Store.Update(func(w *workspace.Workspace) { w.ToggleSidebar(workspace.SideLeft) })
		}
		ui.Spacer(c)
		field := ui.Row(c).Width(380).Height(32).Padding(0, 10).Gap(frostui.SpaceS).Radius(16).AlignItems(ui.Center).
			Background(t.Background).Border(1, t.Border).Role(ui.RoleButton).Focusable().Cursor(ui.CursorPointer).
			Label("Search or run a command").Tooltip("Command palette (Ctrl K)")
		if field.Clicked() {
			a.showPalette("")
		}
		field.Children(func() {
			frostui.Icon(c, "system-search").TextColor(t.TextMuted)
			frostui.Muted(c, "Search or run a command").Grow(1).SingleLine()
			frostui.Shortcut(c, "Ctrl", "K")
		})
		ui.Spacer(c)
		ui.Row(c).Gap(frostui.SpaceXS).AlignItems(ui.Center).Children(func() {
			if a.vault != nil {
				if frostui.IconButton(c, "x-office-calendar", "Open today's daily note").Clicked() {
					a.run("daily-notes")
				}
				if frostui.IconButton(c, "document-new", "Create new note").Clicked() {
					a.run("file-explorer:new-file")
				}
				if frostui.IconButton(c, "sidebar-show-right", "Toggle right sidebar").Selected(a.Store.Workspace().Right.Open).Clicked() {
					a.Store.Update(func(w *workspace.Workspace) { w.ToggleSidebar(workspace.SideRight) })
				}
			}
			frostui.IconButton(c, "view-more", "Main menu").Menu(a.mainMenu)
		})
	})
}

func (a *App) brand(c *ui.Context) {
	t := c.Theme()
	ui.Row(c).Gap(10).AlignItems(ui.Center).Children(func() {
		ui.Box(c).Size(30, 30).Radius(8).Background(t.Accent).Center().Children(func() {
			frostui.Icon(c, "accessories-text-editor").TextColor(t.AccentText)
		})
		ui.Column(c).Children(func() {
			ui.Text(c, "Frostnotes").Bold().SingleLine()
			if a.vault != nil {
				ui.MenuButton(c, "", a.vaultMenu).Label("Vault: " + a.vault.Name).Tooltip("Switch vault").Children(func() {
					frostui.Caption(c, a.vault.Name).SingleLine()
				})
			}
		})
	})
}

func (a *App) mainMenu(m *ui.Menu) {
	item := func(title, id string) {
		if m.Item(title).Chosen() {
			a.run(id)
		}
	}
	item("Command palette", "command-palette:open")
	item("Quick switcher", "switcher:open")
	item("Manage vaults…", "app:open-vault")
	item("Settings", "app:open-settings")
	if m.Item("Keyboard shortcuts").Chosen() {
		a.showShortcuts()
	}
	item("Help", "app:open-help")
	if m.Item("About Frostnotes").Chosen() {
		a.showAbout()
	}
	m.Separator()
	item("Close window", "workspace:close-window")
	if m.Item("Quit").Shortcut(ui.Cmd, ui.KeyQ).Chosen() && a.cfg.Quit != nil {
		a.cfg.Quit()
	}
}

func (a *App) vaultMenu(m *ui.Menu) {
	for i := range a.vaults {
		v := &a.vaults[i]
		if m.Item(v.Name).Checked(a.vault != nil && v.ID == a.vault.ID).Chosen() {
			a.showVault(v)
		}
	}
	m.Separator()
	if m.Item("Manage vaults…").Chosen() {
		a.vm.open = true
	}
}

func (a *App) statusBar(c *ui.Context) {
	t := c.Theme()
	ui.Row(c).Height(28).Padding(0, frostui.SpaceM).Gap(frostui.SpaceL).AlignItems(ui.Center).Background(frostui.Chrome(t)).Role(ui.RoleStatus).Children(func() {
		for i, it := range a.statusStart() {
			statusItem(c.Key([2]any{"s", i}), it)
		}
		ui.Spacer(c)
		for i, it := range a.statusEnd() {
			statusItem(c.Key([2]any{"e", i}), it)
		}
	})
}

func statusItem(c *ui.Context, it frostui.StatusItem) {
	t := c.Theme()
	color := t.TextMuted
	if it.Tone != frostui.Neutral {
		color = frostui.ToneColor(t, it.Tone)
	}
	row := ui.Row(c).Gap(6).Padding(2, 4).Radius(4).AlignItems(ui.Center)
	if it.Tooltip != "" {
		row.Tooltip(it.Tooltip)
	}
	if it.OnClick != nil {
		label := it.Text
		if it.Tooltip != "" {
			label = it.Tooltip
		}
		row.Role(ui.RoleButton).Focusable().Cursor(ui.CursorPointer).Label(label)
		if row.Clicked() {
			it.OnClick()
		}
	}
	row.Children(func() {
		if it.Icon != "" {
			frostui.Icon(c, it.Icon).FontSize(12).TextColor(color)
		}
		ui.Text(c, it.Text).FontSize(12).TextColor(color).SingleLine()
	})
}

// statusStart is the connection, always first, then sync and conflicts.
func (a *App) statusStart() []frostui.StatusItem {
	var items []frostui.StatusItem
	place := "Local"
	if a.syncInfo != nil && a.syncInfo.URL != "" {
		place = hostOf(a.syncInfo.URL)
	}
	conn := frostui.StatusItem{Icon: "network-idle", Text: "● " + place, Tooltip: "Connected to notesd", Progress: -1}
	if a.Session.State() != session.Connected {
		conn.Tone, conn.Tooltip = frostui.Warning, "Reconnecting: "+a.Session.Reason()
	}
	items = append(items, conn)
	if a.syncInfo != nil {
		items = append(items, frostui.StatusItem{Icon: "emblem-synchronizing", Text: syncText(a.syncInfo), Tooltip: "Open Settings at Sync", Progress: -1,
			OnClick: func() { a.sets.show("sync") }})
	}
	if n := len(a.conflict); n > 0 {
		items = append(items, frostui.StatusItem{Icon: "dialog-warning", Text: plural(n, "conflict"), Tone: frostui.Warning, Tooltip: "Show the conflicts", Progress: -1,
			OnClick: a.showConflicts})
	}
	return items
}

func plural(n int, one string) string {
	if n == 1 {
		return "1 " + one
	}
	return strconv.Itoa(n) + " " + one + "s"
}

// statusEnd is backlinks, words, characters and the editing mode.
func (a *App) statusEnd() []frostui.StatusItem {
	if a.vault == nil {
		return nil
	}
	var items []frostui.StatusItem
	if a.activeFile() != "" {
		items = append(items, frostui.StatusItem{Text: plural(a.backlinks.Count(), "backlink"), Tooltip: "Open Backlinks", Progress: -1,
			OnClick: func() {
				a.Store.Update(func(w *workspace.Workspace) { w.SelectPane(workspace.SideRight, "backlinks") })
			}})
	}
	if words, chars, ok := a.textCounts(); ok {
		items = append(items, frostui.StatusItem{Text: plural(words, "word"), Progress: -1}, frostui.StatusItem{Text: plural(chars, "character"), Progress: -1})
	}
	if t := a.Store.Workspace().ActiveTab().View; t.Type == workspace.ViewNote {
		items = append(items, frostui.StatusItem{Text: a.modeName(t), Tooltip: "Change the editing mode", Progress: -1, OnClick: a.toggleMode})
	}
	return items
}

// main is the window under the top bar: the sidebars and the workspace.
func (a *App) main(c *ui.Context) {
	w := a.Store.Workspace()
	total, _ := c.Size()
	center := func() {
		ui.Column(c).Grow(1).Shrink(1).MinWidth(320).AlignItems(ui.Stretch).Children(func() { a.workspaceView(c) })
	}
	left := 0.0
	if w.Left.Open {
		left = w.Left.Width
	}
	withRight := func() {
		if !w.Right.Open {
			center()
			return
		}
		pos := total - float32(left+w.Right.Width)
		el := ui.Split(c.Key("right"), &pos, center, func() { a.sidebar(c, workspace.SideRight, &w.Right) }).Grow(1)
		if el.Changed() {
			width := float64(total-pos) - left
			a.Store.Update(func(w *workspace.Workspace) { w.SetSidebarWidth(workspace.SideRight, width) })
		}
	}
	ui.Row(c).Grow(1).MinHeight(0).AlignItems(ui.Stretch).Children(func() {
		if !w.Left.Open {
			withRight()
			return
		}
		pos := float32(w.Left.Width)
		el := ui.Split(c.Key("left"), &pos, func() { a.sidebar(c, workspace.SideLeft, &w.Left) }, withRight).Grow(1)
		if el.Changed() {
			a.Store.Update(func(w *workspace.Workspace) { w.SetSidebarWidth(workspace.SideLeft, float64(pos)) })
		}
	})
	if a.vm.open {
		a.vm.view(c, a, true)
	}
}

func paneTab(id string) uikit.PaneTab {
	switch id {
	case "files":
		return uikit.PaneTab{ID: id, Title: "Files", Icon: "folder"}
	case "search":
		return uikit.PaneTab{ID: id, Title: "Search", Icon: "system-search"}
	case "backlinks":
		return uikit.PaneTab{ID: id, Title: "Backlinks", Icon: "go-previous"}
	case "outgoing":
		return uikit.PaneTab{ID: id, Title: "Outgoing links", Icon: "go-next"}
	case "tags":
		return uikit.PaneTab{ID: id, Title: "Tags", Icon: "bookmark-new"}
	case "properties":
		return uikit.PaneTab{ID: id, Title: "All properties", Icon: "document-properties"}
	case "outline":
		return uikit.PaneTab{ID: id, Title: "Outline", Icon: "view-list-tree"}
	}
	return uikit.PaneTab{ID: id, Title: "Tasks", Icon: "checkbox-checked"}
}

func (a *App) sidebar(c *ui.Context, side string, s *workspace.Sidebar) {
	t := c.Theme()
	ui.Column(c).Fill().AlignItems(ui.Stretch).Background(frostui.Chrome(t)).Children(func() {
		var tabs []uikit.PaneTab
		for _, p := range s.Panes {
			tabs = append(tabs, paneTab(p))
		}
		label := "Left sidebar"
		if side == workspace.SideRight {
			label = "Right sidebar"
		}
		uikit.PaneTabs(c, label, tabs, s.Active, func(id string) {
			a.Store.Update(func(w *workspace.Workspace) { w.SelectPane(side, id) })
		})
		ui.Column(c).Grow(1).MinHeight(0).AlignItems(ui.Stretch).Children(func() { a.pane(c, s.Panes[s.Active]) })
	})
}

func (a *App) pane(c *ui.Context, id string) {
	switch id {
	case "files":
		a.files.View(c)
	case "search":
		a.search.View(c)
	case "backlinks":
		a.backlinks.View(c)
	case "outgoing":
		a.outgoing.View(c)
	case "tags":
		a.tagsPane.View(c)
	case "properties":
		a.properties.View(c)
	case "outline":
		a.outline.View(c)
	case "tasks":
		a.tasks.View(c)
	}
}
