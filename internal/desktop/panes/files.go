package panes

import (
	"maps"
	"slices"
	"strings"

	"github.com/egoist/mygo/ui"
	"github.com/frostyard/frostui"

	"github.com/frostyard/frostnotes/internal/desktop/uikit"
	"github.com/frostyard/frostnotes/internal/desktop/workspace"
)

// FileActions are what the Files pane asks of the window.
type FileActions struct {
	NewNote     func(folder string)
	NewFolder   func(parent string)
	Rename      func(path, name string, folder bool)
	Move        func(path, toFolder string, folder bool)
	MoveTo      func(path string, folder bool) // asks where
	Delete      func(path string, folder bool)
	CopyPath    func(path string)
	OpenDefault func(path string)
	History     func(path string)
}

// section is the Files list's one section, which has no header.
const section = "files"

// Files is the Files pane (docs/specs/ui.md#files-pane-batch-2): a vault's
// folders and files on frostui's source list, which builds only the rows
// in view.
type Files struct {
	Env     *Env
	Store   *workspace.Store
	Actions FileActions
	// ShowUnsupported is the Show unsupported files preference.
	ShowUnsupported bool

	list           frostui.SourceList[string]
	made           bool
	tree           *Tree
	builtFor       int
	sort           string
	unsup          bool
	loaded         string
	active         string
	openGeneration uint64
}

func (f *Files) rebuild() {
	cat := f.Env.Catalog
	w := f.Store.Workspace()
	if f.tree != nil && f.sort == w.Files.Sort && f.unsup == f.ShowUnsupported && f.builtFor == cat.Generation() {
		return
	}
	f.tree = BuildTree(cat.Files(), cat.Folders(), w.Files.Sort, f.ShowUnsupported)
	f.sort, f.unsup, f.builtFor = w.Files.Sort, f.ShowUnsupported, cat.Generation()
}

// syncOpen loads the workspace's open folders when the vault changes, and
// writes back what the list opened and closed.
func (f *Files) syncOpen() {
	if f.loaded != f.Store.Vault() {
		f.loaded = f.Store.Vault()
		f.setOpen(f.Store.Workspace().Files.Expanded)
		return
	}
	generation := f.list.OpenGeneration()
	if generation == f.openGeneration {
		return
	}
	f.openGeneration = generation
	cur := f.list.OpenItems(section)
	slices.Sort(cur)
	stored := f.Store.Workspace().Files.Expanded
	if !slices.Equal(cur, stored) {
		f.Store.Update(func(w *workspace.Workspace) { w.Files.Expanded = cur })
	}
}

func (f *Files) setOpen(paths []string) {
	f.list.CollapseAll()
	for _, p := range paths {
		f.list.SetOpen(section, p, true)
	}
}

// Reveal opens every folder holding p, so its row shows.
func (f *Files) Reveal(p string) {
	f.Store.Update(func(w *workspace.Workspace) { w.RevealPath(p) })
	f.setOpen(f.Store.Workspace().Files.Expanded)
	f.list.Select(section, p)
}

// Rename starts renaming p in place (F2).
func (f *Files) Rename(p string) {
	f.Reveal(p)
	f.list.StartRename(section, p)
}

func (f *Files) isFolder(p string) bool {
	n := f.tree.Nodes[p]
	return n != nil && n.Type == "folder"
}

func (f *Files) rename(p, name string) {
	if strings.Contains(name, "/") || f.Actions.Rename == nil {
		return
	}
	f.Actions.Rename(p, name, f.isFolder(p))
}

func (f *Files) activate(p string, mods ui.Modifiers) {
	if f.isFolder(p) {
		f.list.SetOpen(section, p, !f.list.IsOpen(section, p))
		return
	}
	f.Env.Actions.Open(p, workspace.WhereFor(mods&ui.Cmd != 0, mods&ui.Alt != 0, 0))
}

func (f *Files) children(p string) []string {
	n := f.tree.Nodes[p]
	if n == nil || n.Type != "folder" {
		return nil
	}
	if n.Children == nil {
		return []string{}
	}
	return n.Children
}

func (f *Files) sourceRow(p string) frostui.SourceRow {
	n := f.tree.Nodes[p]
	if n == nil {
		return frostui.SourceRow{}
	}
	row := frostui.SourceRow{Title: n.Name}
	switch n.Type {
	case "folder":
		row.Icon = "folder"
	case "image":
		row.Icon = "image-x-generic"
	default:
		row.Icon = "text-x-generic"
	}
	if n.Type != "folder" && n.Extension != "md" && n.Extension != "" {
		row.Tag = strings.ToUpper(n.Extension)
	}
	return row
}

func (f *Files) menu(m *ui.Menu, p, _ string) {
	if f.isFolder(p) {
		f.folderMenu(m, p)
	} else {
		f.fileMenu(m, p)
	}
}

// accepts says whether folder takes the dragged row.
func (f *Files) accepts(folder, p string) bool {
	return f.isFolder(folder) && p != folder && parentOf(p) != folder && !strings.HasPrefix(folder+"/", p+"/")
}

func (f *Files) move(p, folder string) {
	if f.Actions.Move != nil {
		f.Actions.Move(p, folder, f.isFolder(p))
	}
}

// setUp fills in the list once.
func (f *Files) setUp() {
	if f.made {
		return
	}
	f.made = true
	f.list.Label = "Files"
	f.list.Children = f.children
	f.list.Row = f.sourceRow
	f.list.Menu = f.menu
	f.list.OnClick = f.activate
	f.list.OnMiddleClick = func(p string, mods ui.Modifiers) {
		if !f.isFolder(p) {
			f.Env.Actions.Open(p, workspace.WhereFor(false, mods&ui.Alt != 0, 1))
		}
	}
	f.list.Rename = f.rename
	f.list.DragItems = func(items []string) any { return items[0] }
	frostui.DropOn(&f.list, f.accepts, func(folder, p string, _ ui.Modifiers) { f.move(p, folder) })
}

func (f *Files) toolbar(c *ui.Context) {
	ui.Row(c).Gap(frostui.SpaceXS).Padding(frostui.SpaceXS, frostui.SpaceS).AlignItems(ui.Center).Children(func() {
		if uikit.ToolButton(c, "document-new", "New note").Clicked() && f.Actions.NewNote != nil {
			f.Actions.NewNote("")
		}
		if uikit.ToolButton(c, "folder-new", "New folder").Clicked() && f.Actions.NewFolder != nil {
			f.Actions.NewFolder("")
		}
		uikit.ToolButton(c, "view-sort-ascending", "Change sort order").Menu(f.sortMenu)
		label := "Expand all"
		if len(f.list.OpenItems(section)) > 0 {
			label = "Collapse all"
		}
		if uikit.ToolButton(c, "view-list", label).Clicked() {
			if len(f.list.OpenItems(section)) > 0 {
				f.setOpen(nil)
			} else {
				f.setOpen(f.folders())
			}
		}
	})
}

func (f *Files) folders() []string {
	var out []string
	for _, p := range slices.Sorted(maps.Keys(f.tree.Nodes)) {
		if p != "" && f.tree.Nodes[p].Type == "folder" {
			out = append(out, p)
		}
	}
	return out
}

func (f *Files) sortMenu(m *ui.Menu) {
	for _, s := range []struct{ id, title string }{
		{SortAlphabetical, "File name (A to Z)"}, {SortAlphabeticalReverse, "File name (Z to A)"},
		{SortModified, "Modified time (new to old)"}, {SortModifiedReverse, "Modified time (old to new)"},
	} {
		if m.Item(s.title).Checked(f.Store.Workspace().Files.Sort == s.id).Chosen() {
			f.Store.Update(func(w *workspace.Workspace) { w.Files.Sort = s.id })
		}
	}
}

func (f *Files) fileMenu(m *ui.Menu, p string) {
	a := f.Env.Actions
	if m.Item("Open in new tab").Chosen() {
		a.Open(p, workspace.NewTab)
	}
	if m.Item("Open to the right").Chosen() {
		a.Open(p, workspace.ToRight)
	}
	m.Separator()
	if m.Item("Rename…").Chosen() {
		f.Rename(p)
	}
	if m.Item("Move file to…").Chosen() && f.Actions.MoveTo != nil {
		f.Actions.MoveTo(p, false)
	}
	m.Separator()
	if m.Item("Copy path").Chosen() && f.Actions.CopyPath != nil {
		f.Actions.CopyPath(p)
	}
	if m.Item("Open version history").Chosen() && f.Actions.History != nil {
		f.Actions.History(p)
	}
	if m.Item("Open in default app").Chosen() && f.Actions.OpenDefault != nil {
		f.Actions.OpenDefault(p)
	}
	m.Separator()
	if m.Item("Delete").Chosen() && f.Actions.Delete != nil {
		f.Actions.Delete(p, false)
	}
}

func (f *Files) folderMenu(m *ui.Menu, p string) {
	if m.Item("New note").Chosen() && f.Actions.NewNote != nil {
		f.Actions.NewNote(p)
	}
	if m.Item("New folder").Chosen() && f.Actions.NewFolder != nil {
		f.Actions.NewFolder(p)
	}
	m.Separator()
	if m.Item("Rename…").Chosen() {
		f.Rename(p)
	}
	if m.Item("Move folder to…").Chosen() && f.Actions.MoveTo != nil {
		f.Actions.MoveTo(p, true)
	}
	m.Separator()
	if m.Item("Delete").Chosen() && f.Actions.Delete != nil {
		f.Actions.Delete(p, true)
	}
}

// View draws the pane.
func (f *Files) View(c *ui.Context) ui.Element {
	f.rebuild()
	f.setUp()
	f.syncOpen()
	if active := f.Env.Active(); active != f.active {
		f.active = active
		if active != "" {
			f.list.Select(section, active)
		}
	}
	return ui.Column(c).Fill().AlignItems(ui.Stretch).Children(func() {
		f.toolbar(c)
		list := f.list.View(c, []frostui.SourceSection[string]{{ID: section, Items: f.tree.Roots}}).Grow(1)
		list.ContextMenu(func(m *ui.Menu) {
			if m.Item("New note").Chosen() && f.Actions.NewNote != nil {
				f.Actions.NewNote("")
			}
			if m.Item("New folder").Chosen() && f.Actions.NewFolder != nil {
				f.Actions.NewFolder("")
			}
		})
		if p, ok := ui.Drop[string](list); ok && parentOf(p) != "" {
			f.move(p, "")
		}
	})
}
