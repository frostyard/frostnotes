package workspace

import (
	"math"
	"slices"
	"strconv"
	"strings"
)

// Where a view opens: in the active tab, a new tab, or a new group to the
// right or below.
const (
	Here    = "here"
	NewTab  = "tab"
	ToRight = "right"
	Below   = "down"
)

// The edges of a tab group, where a dropped tab makes a new split.
const (
	EdgeLeft   = "left"
	EdgeRight  = "right"
	EdgeTop    = "top"
	EdgeBottom = "bottom"
)

// WhereFor is where a click opens what it names, as Obsidian opens links:
// in the current tab, Ctrl in a new tab, Ctrl+Alt to the right, a middle
// click in a new tab.
func WhereFor(ctrl, alt bool, button int) string {
	switch {
	case ctrl && alt:
		return ToRight
	case ctrl || button == 1:
		return NewTab
	}
	return Here
}

// --- reading

// FindGroup returns the group with an ID, or nil.
func (w *Workspace) FindGroup(id string) *Node {
	for _, g := range Groups(w.Root) {
		if g.ID == id {
			return g
		}
	}
	return nil
}

// ActiveGroup is the group holding the window's active tab.
func (w *Workspace) ActiveGroup() *Node {
	if g := w.FindGroup(w.ActiveID); g != nil {
		return g
	}
	return Groups(w.Root)[0]
}

// ActiveTab is the window's active tab.
func (w *Workspace) ActiveTab() *Tab {
	g := w.ActiveGroup()
	return &g.Tabs[g.Active]
}

// FindTab returns a tab's group and index, or nil.
func (w *Workspace) FindTab(id string) (*Node, int) {
	for _, g := range Groups(w.Root) {
		for i, t := range g.Tabs {
			if t.ID == id {
				return g, i
			}
		}
	}
	return nil, -1
}

func (w *Workspace) takeID(prefix string) string {
	id := prefix + strconv.Itoa(w.NextID)
	w.NextID++
	return id
}

func capped(views []View) []View {
	if len(views) > MaxHistory {
		return views[len(views)-MaxHistory:]
	}
	return views
}

// --- opening and navigating

// Open shows v: in the active tab, which keeps its old view to go back to,
// unless it is pinned; in a new tab after the active one; or in a new
// group to the right of or below the active group.
func (w *Workspace) Open(v View, where string) {
	tab := w.ActiveTab()
	switch {
	case where == Here && !tab.Pinned:
		w.navigate(tab, v)
	case where == ToRight || where == Below:
		w.splitWith(w.ActiveGroup().ID, v, where)
	default:
		g := w.ActiveGroup()
		w.insertTab(g, g.Active+1, newTab(w.takeID("t"), v))
	}
	w.remember(v)
}

// SetView changes a tab's view without history: its scroll, cursor or
// mode.
func (w *Workspace) SetView(tabID string, v View) {
	if g, i := w.FindTab(tabID); g != nil {
		g.Tabs[i].View = v
	}
}

// Back shows the active tab's previous view.
func (w *Workspace) Back() {
	t := w.ActiveTab()
	if len(t.Back) == 0 {
		return
	}
	prev := t.Back[len(t.Back)-1]
	t.Back = t.Back[:len(t.Back)-1]
	t.Forward = capped(append(t.Forward, t.View))
	t.View = prev
}

// Forward shows the active tab's next view.
func (w *Workspace) Forward() {
	t := w.ActiveTab()
	if len(t.Forward) == 0 {
		return
	}
	next := t.Forward[len(t.Forward)-1]
	t.Forward = t.Forward[:len(t.Forward)-1]
	t.Back = capped(append(t.Back, t.View))
	t.View = next
}

// navigate shows v in a tab, keeping its old view to go back to; an empty
// tab's view is not kept.
func (w *Workspace) navigate(t *Tab, v View) {
	if t.View.Same(v) {
		t.View = v
		return
	}
	if t.View.Type != ViewEmpty {
		t.Back = capped(append(t.Back, t.View))
	}
	t.View, t.Forward = v, nil
}

// remember puts a file the view shows first among the recent files.
func (w *Workspace) remember(v View) {
	if v.Path == "" || v.Type == ViewHistory || v.Type == ViewMerge {
		return
	}
	recent := []string{v.Path}
	for _, r := range w.Recent {
		if r != v.Path {
			recent = append(recent, r)
		}
	}
	w.Recent = recent[:min(len(recent), MaxRecent)]
}

// --- activating

// Activate makes a tab its group's active tab and its group the active
// group.
func (w *Workspace) Activate(tabID string) {
	if g, i := w.FindTab(tabID); g != nil {
		g.Active, w.ActiveID = i, g.ID
	}
}

// Cycle activates the next (1) or previous (-1) tab of the active group,
// wrapping.
func (w *Workspace) Cycle(delta int) {
	g := w.ActiveGroup()
	n := len(g.Tabs)
	g.Active = ((g.Active+delta)%n + n) % n
}

// GotoTab activates the active group's nth tab, counting from 1; past
// the end, nothing.
func (w *Workspace) GotoTab(n int) {
	if g := w.ActiveGroup(); n >= 1 && n <= len(g.Tabs) {
		g.Active = n - 1
	}
}

// GotoLastTab activates the active group's last tab.
func (w *Workspace) GotoLastTab() { w.GotoTab(len(w.ActiveGroup().Tabs)) }

// Pin pins or unpins a tab.
func (w *Workspace) Pin(tabID string, pinned bool) {
	if g, i := w.FindTab(tabID); g != nil {
		g.Tabs[i].Pinned = pinned
	}
}

// --- closing

// Close closes a tab, keeping it to reopen.
func (w *Workspace) Close(tabID string) {
	g, i := w.FindTab(tabID)
	if g == nil {
		return
	}
	closed := ClosedTab{Tab: g.Tabs[i], Group: g.ID, Index: i}
	w.removeTab(tabID, true)
	w.Closed = append(w.Closed, closed)
	if len(w.Closed) > MaxClosed {
		w.Closed = w.Closed[len(w.Closed)-MaxClosed:]
	}
}

// CloseOthers closes a group's unpinned tabs but one.
func (w *Workspace) CloseOthers(tabID string) {
	g, _ := w.FindTab(tabID)
	if g == nil {
		return
	}
	for _, t := range slices.Clone(g.Tabs) {
		if t.ID != tabID && !t.Pinned {
			w.Close(t.ID)
		}
	}
	w.Activate(tabID)
}

// CloseToTheRight closes a group's unpinned tabs after one.
func (w *Workspace) CloseToTheRight(tabID string) {
	g, i := w.FindTab(tabID)
	if g == nil {
		return
	}
	for _, t := range slices.Clone(g.Tabs[i+1:]) {
		if !t.Pinned {
			w.Close(t.ID)
		}
	}
}

// CloseAll closes a group's unpinned tabs.
func (w *Workspace) CloseAll(groupID string) {
	g := w.FindGroup(groupID)
	if g == nil {
		return
	}
	for _, t := range slices.Clone(g.Tabs) {
		if !t.Pinned {
			w.Close(t.ID)
		}
	}
}

// ReopenClosed reopens the tab closed last where it was, or after the
// active tab when its group is gone.
func (w *Workspace) ReopenClosed() {
	if len(w.Closed) == 0 {
		return
	}
	last := w.Closed[len(w.Closed)-1]
	w.Closed = w.Closed[:len(w.Closed)-1]
	if g := w.FindGroup(last.Group); g != nil {
		w.insertTab(g, min(last.Index, len(g.Tabs)), last.Tab)
		return
	}
	g := w.ActiveGroup()
	w.insertTab(g, g.Active+1, last.Tab)
}

// --- splitting and moving

// Split opens a copy of a tab's view in a new group to its right or below
// its group.
func (w *Workspace) Split(tabID, where string) {
	if g, i := w.FindTab(tabID); g != nil {
		w.splitWith(g.ID, g.Tabs[i].View, where)
	}
}

// MoveTab moves a tab into a group at an index, or, with an edge, to that
// edge of the group as a new group.
func (w *Workspace) MoveTab(tabID, groupID string, index int, edge string) {
	from, fi := w.FindTab(tabID)
	to := w.FindGroup(groupID)
	if from == nil || to == nil {
		return
	}
	tab := from.Tabs[fi]
	if edge != "" {
		if from == to && len(from.Tabs) == 1 {
			return
		}
		w.removeTab(tabID, false)
		g := &Node{Kind: KindTabs, ID: w.takeID("g"), Tabs: []Tab{tab}}
		w.Root = beside(w.Root, groupID, g, edge, w.takeID("s"))
		w.ActiveID = g.ID
		return
	}
	if from == to {
		if index > fi {
			index--
		}
		tabs := slices.Delete(slices.Clone(from.Tabs), fi, fi+1)
		index = max(0, min(index, len(tabs)))
		from.Tabs = slices.Insert(tabs, index, tab)
		w.Activate(tabID)
		return
	}
	w.removeTab(tabID, false)
	if dest := w.FindGroup(groupID); dest != nil {
		w.insertTab(dest, min(index, len(dest.Tabs)), tab)
	}
}

// Resize sets a split's sizes, normalized to sum to 1, none under 5%.
func (w *Workspace) Resize(splitID string, sizes []float64) {
	if n := findNode(w.Root, splitID); n != nil && n.Kind == KindSplit && len(sizes) == len(n.Children) {
		n.Sizes = normalize(sizes)
	}
}

func (w *Workspace) splitWith(groupID string, v View, where string) {
	t := newTab(w.takeID("t"), v)
	g := &Node{Kind: KindTabs, ID: w.takeID("g"), Tabs: []Tab{t}}
	edge := EdgeBottom
	if where == ToRight {
		edge = EdgeRight
	}
	w.Root = beside(w.Root, groupID, g, edge, w.takeID("s"))
	w.ActiveID = g.ID
}

// beside puts node at a group's edge: in its parent split when that runs
// the same way, sharing the group's size, else in a new split around it.
func beside(root *Node, groupID string, node *Node, edge, splitID string) *Node {
	dir := Column
	if edge == EdgeLeft || edge == EdgeRight {
		dir = Row
	}
	after := edge == EdgeRight || edge == EdgeBottom
	var visit func(n *Node) *Node
	visit = func(n *Node) *Node {
		if n.Kind == KindTabs {
			if n.ID != groupID {
				return n
			}
			children := []*Node{n, node}
			if !after {
				children = []*Node{node, n}
			}
			return &Node{Kind: KindSplit, ID: splitID, Direction: dir, Children: children, Sizes: []float64{0.5, 0.5}}
		}
		for i, c := range n.Children {
			if c.Kind == KindTabs && c.ID == groupID && n.Direction == dir {
				half := n.Sizes[i] / 2
				at := i
				if after {
					at = i + 1
				}
				n.Children = slices.Insert(n.Children, at, node)
				n.Sizes = slices.Replace(n.Sizes, i, i+1, half, half)
				return n
			}
		}
		for i, c := range n.Children {
			n.Children[i] = visit(c)
		}
		return n
	}
	return visit(root)
}

// --- sidebars

func (w *Workspace) sidebar(side string) *Sidebar {
	if side == SideRight {
		return &w.Right
	}
	return &w.Left
}

// ToggleSidebar opens or closes a sidebar.
func (w *Workspace) ToggleSidebar(side string) {
	s := w.sidebar(side)
	s.Open = !s.Open
}

// SetSidebarWidth sets a sidebar's width, between SidebarMin and
// SidebarMax.
func (w *Workspace) SetSidebarWidth(side string, width float64) {
	w.sidebar(side).Width = math.Round(max(SidebarMin, min(SidebarMax, width)))
}

// SelectPane shows a pane in its sidebar, opening the sidebar.
func (w *Workspace) SelectPane(side, pane string) {
	s := w.sidebar(side)
	if i := slices.Index(s.Panes, pane); i >= 0 {
		s.Active, s.Open = i, true
	}
}

// --- the Files pane

// SetFolderOpen opens or closes a folder in the Files pane.
func (w *Workspace) SetFolderOpen(folder string, open bool) {
	rest := slices.DeleteFunc(slices.Clone(w.Files.Expanded), func(p string) bool { return p == folder })
	if open {
		rest = append(rest, folder)
		if len(rest) > MaxExpanded {
			rest = rest[len(rest)-MaxExpanded:]
		}
	}
	w.Files.Expanded = rest
}

// RevealPath opens every folder holding p, so its row shows.
func (w *Workspace) RevealPath(p string) {
	parts := strings.Split(p, "/")
	for i := 1; i < len(parts); i++ {
		f := strings.Join(parts[:i], "/")
		if !slices.Contains(w.Files.Expanded, f) {
			w.SetFolderOpen(f, true)
		}
	}
}

// --- following the vault

// ApplyMove follows a file or folder moved from one path to another, in
// every view, history and list.
func (w *Workspace) ApplyMove(from, to string) {
	move := func(p string) string {
		if p == from {
			return to
		}
		if strings.HasPrefix(p, from+"/") {
			return to + p[len(from):]
		}
		return p
	}
	view := func(v View) View {
		if v.Path != "" {
			v.Path = move(v.Path)
		}
		return v
	}
	tab := func(t *Tab) {
		t.View = view(t.View)
		for i := range t.Back {
			t.Back[i] = view(t.Back[i])
		}
		for i := range t.Forward {
			t.Forward[i] = view(t.Forward[i])
		}
	}
	for _, g := range Groups(w.Root) {
		for i := range g.Tabs {
			tab(&g.Tabs[i])
		}
	}
	for i := range w.Closed {
		tab(&w.Closed[i].Tab)
	}
	for i, r := range w.Recent {
		w.Recent[i] = move(r)
	}
	for i, e := range w.Files.Expanded {
		w.Files.Expanded[i] = move(e)
	}
}

// ApplyDelete forgets a deleted file in history and the recent files; a
// tab showing it stays, saying so, until it is closed or the file comes
// back.
func (w *Workspace) ApplyDelete(p string) {
	keep := func(v View) bool { return v.Path == p }
	for _, g := range Groups(w.Root) {
		for i := range g.Tabs {
			g.Tabs[i].Back = slices.DeleteFunc(g.Tabs[i].Back, keep)
			g.Tabs[i].Forward = slices.DeleteFunc(g.Tabs[i].Forward, keep)
		}
	}
	w.Recent = slices.DeleteFunc(w.Recent, func(r string) bool { return r == p })
}

// --- the tree

func normalize(sizes []float64) []float64 {
	out := make([]float64, len(sizes))
	sum := 0.0
	for i, s := range sizes {
		if math.IsNaN(s) || math.IsInf(s, 0) {
			s = 0.05
		}
		out[i] = max(0.05, s)
		sum += out[i]
	}
	for i := range out {
		out[i] /= sum
	}
	return out
}

func findNode(n *Node, id string) *Node {
	if n.ID == id {
		return n
	}
	for _, c := range n.Children {
		if f := findNode(c, id); f != nil {
			return f
		}
	}
	return nil
}

// removeNode takes the node with an ID out of the tree; a split left with
// one child becomes that child.
func removeNode(root *Node, id string) *Node {
	var visit func(n *Node) *Node
	visit = func(n *Node) *Node {
		if n.ID == id {
			return nil
		}
		if n.Kind == KindTabs {
			return n
		}
		var children []*Node
		var sizes []float64
		for i, c := range n.Children {
			if k := visit(c); k != nil {
				children = append(children, k)
				sizes = append(sizes, n.Sizes[i])
			}
		}
		switch len(children) {
		case 0:
			return nil
		case 1:
			return children[0]
		}
		n.Children, n.Sizes = children, normalize(sizes)
		return n
	}
	if r := visit(root); r != nil {
		return r
	}
	return root
}

// insertTab puts a tab into a group at an index and activates it.
func (w *Workspace) insertTab(g *Node, index int, t Tab) {
	g.Tabs = slices.Insert(g.Tabs, index, t)
	g.Active, w.ActiveID = index, g.ID
}

// removeTab takes a tab out of its group. A group left empty goes, and the
// active group passes to the one in its place; the last group keeps an
// empty tab instead, unless keepLast is false (a move out of it).
func (w *Workspace) removeTab(tabID string, keepLast bool) {
	g, i := w.FindTab(tabID)
	if g == nil {
		return
	}
	if len(g.Tabs) > 1 {
		g.Tabs = slices.Delete(g.Tabs, i, i+1)
		if i < g.Active || g.Active == len(g.Tabs) {
			g.Active--
		}
		g.Active = max(0, g.Active)
		return
	}
	all := Groups(w.Root)
	if len(all) == 1 {
		if keepLast {
			g.Tabs, g.Active = []Tab{newTab(w.takeID("t"), Empty())}, 0
		}
		return
	}
	position := slices.Index(all, g)
	w.Root = removeNode(w.Root, g.ID)
	if w.ActiveID == g.ID {
		left := Groups(w.Root)
		w.ActiveID = left[min(position, len(left)-1)].ID
	}
}
