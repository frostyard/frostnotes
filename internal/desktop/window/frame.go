package window

import (
	"strconv"

	"github.com/egoist/mygo/ui"
	"github.com/frostyard/frostui"

	"github.com/frostyard/frostnotes/api"
	"github.com/frostyard/frostnotes/internal/desktop/session"
	"github.com/frostyard/frostnotes/internal/desktop/uikit"
	"github.com/frostyard/frostnotes/internal/desktop/workspace"
)

// The ids of the window's panes.
const (
	paneLeft   = "left"
	paneCenter = "center"
	paneRight  = "right"
)

func (a *App) frame(c *ui.Context) {
	f := frostui.PaneFrame{
		SidebarPanes: &a.panes, Start: func() { a.brand(c) }, End: func() { a.barEnd(c) }, Content: func() { a.body(c) },
		Menu: a.mainMenu, Table: a.cmds, Commands: a.recentCommands(), Connection: a.connection(),
		Status: a.statusStart(), StatusEnd: a.statusEnd(), ToastCloseLabel: "Close",
	}
	if a.vault != nil {
		f.SidebarPane = paneLeft
	}
	a.layoutFromWorkspace()
	drawn := a.layoutDrawn()
	a.Shell.View(c, f)
	a.layoutToWorkspace(drawn)
	a.sets.view(c, a)
}

// connection is the status bar's first item: notesd, or the hub of a
// synced vault.
func (a *App) connection() *frostui.Connection {
	conn := &frostui.Connection{Daemon: "notesd", Reason: a.Session.Reason()}
	if a.syncInfo != nil && a.syncInfo.URL != "" {
		conn.Host = hostOf(a.syncInfo.URL)
	}
	switch {
	case a.vault == nil && (a.Session.State() != session.Connected || !a.listed):
		conn.State = frostui.Connecting
	case a.Session.State() != session.Connected:
		conn.State = frostui.Reconnecting
	case a.syncInfo != nil && a.syncInfo.State == api.SyncStateOffline:
		conn.State = frostui.Reconnecting
		if a.syncInfo.Error != nil {
			conn.Reason = *a.syncInfo.Error
		}
	}
	if a.cfg.Getenv != nil && conn.State == frostui.Connecting {
		if cause := notesdCause(a.cfg.Getenv); cause != "" {
			conn.OnDetails = func() {
				a.Shell.Info.Show("Why notesd is not running", func(c *ui.Context) { frostui.Body(c, cause).Wrap().Selectable() })
			}
		}
	}
	return conn
}

func (a *App) body(c *ui.Context) {
	if a.vault == nil {
		a.vm.view(c, a, false)
		return
	}
	a.main(c)
}

func (a *App) barEnd(c *ui.Context) {
	if a.vault == nil {
		return
	}
	if frostui.IconButton(c, "x-office-calendar", "Open today's daily note").Clicked() {
		a.run("daily-notes")
	}
	if frostui.IconButton(c, "document-new", "Create new note").Clicked() {
		a.run("file-explorer:new-file")
	}
	if frostui.IconButton(c, "sidebar-show-right", "Toggle right sidebar").Selected(a.Store.Workspace().Right.Open).Clicked() {
		a.updateWorkspace(func(w *workspace.Workspace) { w.ToggleSidebar(workspace.SideRight) })
	}
}

func (a *App) brand(c *ui.Context) {
	t := c.Theme()
	ui.Row(c).Gap(10).AlignItems(ui.Center).Children(func() {
		ui.Box(c).Size(30, 30).Radius(8).Background(t.Accent).Center().Children(func() {
			frostui.Icon(c, "org.frostyard.Frostnotes-symbolic").TextColor(t.AccentText)
		})
		ui.Column(c).Children(func() {
			ui.Text(c, "Frostnotes").Bold().SingleLine()
			if a.vault != nil {
				frostui.Caption(c, a.vault.Name).SingleLine().Role(ui.RoleButton).Focusable().Cursor(ui.CursorPointer).
					Label("Vault: " + a.vault.Name).Tooltip("Switch vault").Menu(a.vaultMenu)
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

// statusStart is sync and conflicts: the shell draws the connection first.
func (a *App) statusStart() []frostui.StatusItem {
	var items []frostui.StatusItem
	if a.syncInfo != nil {
		items = append(items, frostui.StatusItem{Icon: "view-refresh", Text: syncText(a.syncInfo), Tooltip: "Open Settings at Sync", Progress: -1,
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
				a.updateWorkspace(func(w *workspace.Workspace) { w.SelectPane(workspace.SideRight, "backlinks") })
			}})
	}
	if words, chars, ok := a.textCounts(); ok {
		items = append(items, frostui.StatusItem{Text: plural(words, "word"), Progress: -1}, frostui.StatusItem{Text: plural(chars, "character"), Progress: -1})
	}
	if t := a.Store.Workspace().ActiveTab().View; t.Type == workspace.ViewNote {
		items = append(items, frostui.StatusItem{Text: a.modeName(t), Tooltip: "Change the editing mode", Progress: -1, Menu: a.editingModeMenu})
	}
	return items
}

// main is the window under the top bar: the sidebars and the workspace.
func (a *App) main(c *ui.Context) {
	w := a.Store.Workspace()
	a.panes.View(c, []frostui.Pane{
		{ID: paneLeft, Label: "Left sidebar", Width: workspace.LeftWidth, Min: workspace.SidebarMin, Max: workspace.SidebarMax, Collapsible: true,
			Content: func() { a.sidebar(c, workspace.SideLeft, &w.Left) }},
		{ID: paneCenter, Label: "Workspace", Min: 320, Content: func() { a.workspaceView(c) }},
		{ID: paneRight, Label: "Right sidebar", Width: workspace.RightWidth, Min: workspace.SidebarMin, Max: workspace.SidebarMax, Collapsible: true,
			Content: func() { a.sidebar(c, workspace.SideRight, &w.Right) }},
	}).Grow(1)
	if a.vm.open {
		a.vm.view(c, a, true)
	}
}

// layoutFromWorkspace shows the workspace's sidebars: the workspace is
// what is saved.
func (a *App) layoutFromWorkspace() {
	w := a.Store.Workspace()
	a.panes.Layout = frostui.PaneLayout{
		Widths:    map[string]float32{paneLeft: float32(w.Left.Width), paneRight: float32(w.Right.Width)},
		Collapsed: map[string]bool{},
	}
	if !w.Left.Open {
		a.panes.Layout.Collapsed[paneLeft] = true
	}
	if !w.Right.Open {
		a.panes.Layout.Collapsed[paneRight] = true
	}
}

// paneLayout is one sidebar's layout as the frame starts drawing it.
type paneLayout struct {
	width     float32
	collapsed bool
}

// layoutDrawn is the sidebars' layout the frame starts with.
func (a *App) layoutDrawn() map[string]paneLayout {
	out := make(map[string]paneLayout, 2)
	for _, id := range []string{paneLeft, paneRight} {
		out[id] = paneLayout{a.panes.Layout.Widths[id], a.panes.Layout.Collapsed[id]}
	}
	return out
}

// layoutToWorkspace keeps what the panes changed in this frame. A button
// that changed the workspace itself in this frame keeps its change: only
// what moved since the frame started is copied back.
func (a *App) layoutToWorkspace(drawn map[string]paneLayout) {
	if a.vault == nil {
		return
	}
	w := a.Store.Workspace()
	sync := func(id, side string, s *workspace.Sidebar) {
		if width := float64(a.panes.Layout.Widths[id]); width != 0 && a.panes.Layout.Widths[id] != drawn[id].width && width != s.Width {
			a.updateWorkspace(func(w *workspace.Workspace) { w.SetSidebarWidth(side, width) })
		}
		if collapsed := a.panes.Layout.Collapsed[id]; collapsed != drawn[id].collapsed && !collapsed != s.Open {
			a.updateWorkspace(func(w *workspace.Workspace) { w.ToggleSidebar(side) })
		}
	}
	sync(paneLeft, workspace.SideLeft, &w.Left)
	sync(paneRight, workspace.SideRight, &w.Right)
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
		return uikit.PaneTab{ID: id, Title: "Tags", Icon: "tag"}
	case "properties":
		return uikit.PaneTab{ID: id, Title: "All properties", Icon: "view-list"}
	case "outline":
		return uikit.PaneTab{ID: id, Title: "Outline", Icon: "format-justify-left"}
	}
	return uikit.PaneTab{ID: id, Title: "Tasks", Icon: "object-select"}
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
			a.updateWorkspace(func(w *workspace.Workspace) { w.SelectPane(side, id) })
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

// editingModeMenu selects a specific view instead of cycling between two modes.
func (a *App) editingModeMenu(m *ui.Menu) {
	current := a.modeName(a.Store.Workspace().ActiveTab().View)
	for _, mode := range []struct{ name, pref string }{{"Reading view", ""}, {"Live Preview", "live"}, {"Source mode", "source"}} {
		if m.Item(mode.name).Checked(current == mode.name).Chosen() {
			a.chooseEditingMode(mode.pref)
		}
	}
}

func (a *App) chooseEditingMode(pref string) {
	if pref != "" {
		a.setPref("editingMode", pref)
	}
	a.Store.Update(func(w *workspace.Workspace) {
		tab := w.ActiveTab()
		if tab.View.Type != workspace.ViewNote {
			return
		}
		v := tab.View
		v.Mode = workspace.ModeRead
		if pref != "" {
			v.Mode = workspace.ModeEdit
		}
		w.SetView(tab.ID, v)
	})
}
