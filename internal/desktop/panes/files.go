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

// Files is the Files pane (docs/specs/ui.md#files-pane-batch-2): a vault's
// folders and files on MyGo's outline, which builds only the rows in view.
type Files struct {
	Env     *Env
	Store   *workspace.Store
	Actions FileActions
	// ShowUnsupported is the Show unsupported files preference.
	ShowUnsupported bool

	state    ui.OutlineState[string]
	tree     *Tree
	builtFor int
	files    int
	sort     string
	unsup    bool
	loaded   string
	renaming string
	newName  string
	focus    bool
	theme    *ui.Theme
}

func (f *Files) rebuild() {
	cat := f.Env.Catalog
	w := f.Store.Workspace()
	stamp := len(cat.Files()) + 31*len(cat.Folders())
	if f.tree != nil && stamp == f.files && f.sort == w.Files.Sort && f.unsup == f.ShowUnsupported && f.builtFor == f.generation() {
		return
	}
	f.tree = BuildTree(cat.Files(), cat.Folders(), w.Files.Sort, f.ShowUnsupported)
	f.files, f.sort, f.unsup, f.builtFor = stamp, w.Files.Sort, f.ShowUnsupported, f.generation()
}

// generation changes whenever the catalog's paths may have.
func (f *Files) generation() int {
	n := 0
	for _, x := range f.Env.Catalog.Files() {
		n += len(x.Path)
	}
	return n
}

// syncOpen loads the workspace's open folders when the vault changes, and
// writes back what the outline opened and closed.
func (f *Files) syncOpen() {
	if f.loaded != f.Store.Vault() {
		f.loaded = f.Store.Vault()
		f.setOpen(f.Store.Workspace().Files.Expanded)
		return
	}
	cur := slices.Sorted(f.state.Open.All())
	stored := slices.Sorted(slices.Values(f.Store.Workspace().Files.Expanded))
	if !slices.Equal(cur, stored) {
		f.Store.Update(func(w *workspace.Workspace) { w.Files.Expanded = cur })
	}
}

func (f *Files) setOpen(paths []string) {
	f.state.Open.Clear()
	for _, p := range paths {
		f.state.Open.Add(p)
	}
}

// Reveal opens every folder holding p, so its row shows.
func (f *Files) Reveal(p string) {
	f.Store.Update(func(w *workspace.Workspace) { w.RevealPath(p) })
	f.setOpen(f.Store.Workspace().Files.Expanded)
}

// Rename starts renaming p in place (F2).
func (f *Files) Rename(p string) {
	if f.tree == nil {
		f.rebuild()
	}
	if n := f.tree.Nodes[p]; n != nil {
		f.renaming, f.newName, f.focus = p, n.Name, true
	}
}

func (f *Files) commitRename(n *Node) {
	name := strings.TrimSpace(f.newName)
	p := f.renaming
	f.renaming = ""
	if name == "" || name == n.Name || strings.Contains(name, "/") {
		return
	}
	if f.Actions.Rename != nil {
		f.Actions.Rename(p, name, n.Type == "folder")
	}
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
		if f.state.Open.Len() > 0 {
			label = "Collapse all"
		}
		if uikit.ToolButton(c, "view-list-tree", label).Clicked() {
			if f.state.Open.Len() > 0 {
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

// drop moves a dragged row into a folder, "" being the vault's root.
func (f *Files) drop(el ui.Element, folder string) {
	t := el
	if _, over := ui.DragOver[string](t); over {
		t.Background(frostui.ToneFill(themeOf(f), frostui.Accent))
	}
	if p, ok := ui.Drop[string](t); ok && f.Actions.Move != nil && p != folder && parentOf(p) != folder && !strings.HasPrefix(folder+"/", p+"/") {
		n := f.tree.Nodes[p]
		f.Actions.Move(p, folder, n != nil && n.Type == "folder")
	}
}

func themeOf(f *Files) *ui.Theme { return f.theme }

func (f *Files) row(c *ui.Context, p string) {
	n := f.tree.Nodes[p]
	if n == nil {
		return
	}
	if f.renaming == p {
		in := ui.TextInput(c, &f.newName).Label("New name").Grow(1)
		if f.focus {
			f.focus = false
			in.AutoFocus()
		}
		if in.Submitted() {
			f.commitRename(n)
		}
		return
	}
	folder := n.Type == "folder"
	r := ui.Row(c).Grow(1).Gap(frostui.SpaceXS).AlignItems(ui.Center).Cursor(ui.CursorPointer).Role(ui.RoleButton).Label(n.Name).Drag(p)
	if p == f.Env.Active() {
		r.Background(f.theme.Surface).Radius(frostui.RadiusChip)
	}
	if folder {
		f.drop(r, p)
		r.ContextMenu(func(m *ui.Menu) { f.folderMenu(m, p) })
	} else {
		r.ContextMenu(func(m *ui.Menu) { f.fileMenu(m, p) })
	}
	if r.Clicked() {
		if folder {
			if f.state.Open.Has(p) {
				f.state.Open.Remove(p)
			} else {
				f.state.Open.Add(p)
			}
		} else {
			f.Env.Actions.Open(p, f.Env.where(c))
		}
	}
	r.Children(func() {
		ui.Text(c, n.Name).SingleLine().Grow(1)
		if !folder && n.Extension != "md" && n.Extension != "" {
			frostui.Pill(c, strings.ToUpper(n.Extension), frostui.Neutral)
		}
	})
}

// View draws the pane.
func (f *Files) View(c *ui.Context) ui.Element {
	f.rebuild()
	f.theme = c.Theme()
	f.syncOpen()
	return ui.Column(c).Fill().AlignItems(ui.Stretch).Children(func() {
		f.toolbar(c)
		list := ui.Outline(c, &f.state, f.tree.Roots, func(p string) []string {
			n := f.tree.Nodes[p]
			if n == nil || n.Type != "folder" {
				return nil
			}
			if n.Children == nil {
				return []string{}
			}
			return n.Children
		}, func(p string) { f.row(c, p) }).Grow(1)
		list.ContextMenu(func(m *ui.Menu) {
			if m.Item("New note").Chosen() && f.Actions.NewNote != nil {
				f.Actions.NewNote("")
			}
			if m.Item("New folder").Chosen() && f.Actions.NewFolder != nil {
				f.Actions.NewFolder("")
			}
		})
		f.drop(list, "")
	})
}
