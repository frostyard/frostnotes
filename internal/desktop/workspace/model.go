// Package workspace is the desktop app's workspace model
// (docs/design/app.md#workspace, plan 0009): a tree of splits and tab
// groups, two sidebars of panes, the tabs closed lately, the files opened
// lately and the Files pane's sort and open folders. It ports
// app/src/workspace's model and operations, which the web app keeps; here
// the operations change a workspace in place on the UI thread, and the
// store keeps one per vault in a versioned JSON file (ADR-0021).
package workspace

import (
	"path"
	"slices"
	"strings"
)

// The workspace's limits, as the web app's.
const (
	MaxHistory  = 50
	MaxClosed   = 20
	MaxRecent   = 50
	MaxExpanded = 1000

	// Sidebar widths (docs/specs/ui.md#layout).
	SidebarMin = 200
	SidebarMax = 600
	LeftWidth  = 260
	RightWidth = 300
)

// The kinds of view.
const (
	ViewEmpty   = "empty"
	ViewNote    = "note"
	ViewHistory = "history"
	ViewMerge   = "merge"
	ViewFile    = "file"
)

// The modes of a note view.
const (
	ModeEdit = "edit"
	ModeRead = "read"
)

// MergeSide is the other side of a merge view: a version of the file, a
// hub revision, or the text the editor held when notesd refused its base
// (in memory only: a stored workspace drops it).
type MergeSide struct {
	Kind    string `json:"kind"` // version, revision or held
	Version int64  `json:"version,omitzero"`
	Seq     int64  `json:"seq,omitzero"`
}

// View is what a tab shows.
type View struct {
	Type   string     `json:"type"`
	Path   string     `json:"path,omitzero"`
	Mode   string     `json:"mode,omitzero"`
	Scroll float64    `json:"scroll,omitzero"`
	Cursor int        `json:"cursor,omitzero"`
	Other  *MergeSide `json:"other,omitzero"`
}

// Empty is the view of an empty tab.
func Empty() View { return View{Type: ViewEmpty} }

// Note is a note's view in a mode.
func Note(path, mode string) View { return View{Type: ViewNote, Path: path, Mode: mode} }

// Same reports whether two views show the same thing, ignoring scroll and
// cursor.
func (v View) Same(w View) bool {
	if v.Type != w.Type {
		return false
	}
	switch v.Type {
	case ViewEmpty:
		return true
	case ViewNote:
		return v.Path == w.Path && v.Mode == w.Mode
	case ViewMerge:
		return v.Path == w.Path && (v.Other == nil) == (w.Other == nil) && (v.Other == nil || *v.Other == *w.Other)
	}
	return v.Path == w.Path
}

// Title is a tab's title as its strip shows it.
func (v View) Title() string {
	base := path.Base(v.Path)
	name := strings.TrimSuffix(base, ".md")
	switch v.Type {
	case ViewNote:
		return name
	case ViewFile:
		return base
	case ViewHistory:
		return "History of " + name
	case ViewMerge:
		return "Merge " + name
	}
	return "New tab"
}

// Tab is a tab: its view and the views before and after it.
type Tab struct {
	ID   string `json:"id"`
	View View   `json:"view"`
	// Back and Forward are the views to go back and forward to, the
	// nearest last; at most MaxHistory.
	Back    []View `json:"back"`
	Forward []View `json:"forward"`
	Pinned  bool   `json:"pinned"`
}

// The kinds of node.
const (
	KindSplit = "split"
	KindTabs  = "tabs"
)

// The directions of a split: side by side, or stacked.
const (
	Row    = "row"
	Column = "column"
)

// Node is a split or a tab group. A split lays out at least two children
// with sizes that sum to 1; a group holds at least one tab and its active
// one.
type Node struct {
	Kind      string    `json:"kind"`
	ID        string    `json:"id"`
	Direction string    `json:"direction,omitzero"`
	Children  []*Node   `json:"children,omitzero"`
	Sizes     []float64 `json:"sizes,omitzero"`
	Tabs      []Tab     `json:"tabs,omitzero"`
	Active    int       `json:"active"`
}

// The sidebars and their panes (docs/specs/ui.md#sidebars-and-panes).
const (
	SideLeft  = "left"
	SideRight = "right"
)

// Panes are each sidebar's panes, in order.
var Panes = map[string][]string{
	SideLeft:  {"files", "search"},
	SideRight: {"backlinks", "outgoing", "tags", "properties", "outline", "tasks"},
}

// Sidebar is a sidebar: its panes, the selected one, its width and
// whether it is open.
type Sidebar struct {
	Panes  []string `json:"panes"`
	Active int      `json:"active"`
	Width  float64  `json:"width"`
	Open   bool     `json:"open"`
}

// ClosedTab is a closed tab and where it was, to reopen it.
type ClosedTab struct {
	Tab   Tab    `json:"tab"`
	Group string `json:"group"`
	Index int    `json:"index"`
}

// FilesPane is the Files pane's state: its sort order and its open
// folders.
type FilesPane struct {
	Sort     string   `json:"sort"`
	Expanded []string `json:"expanded"`
}

// Version is the stored workspace's version.
const Version = 1

// Workspace is one vault's layout.
type Workspace struct {
	Version  int         `json:"version"`
	Root     *Node       `json:"root"`
	ActiveID string      `json:"activeGroup"` // the group holding the active tab
	Left     Sidebar     `json:"left"`
	Right    Sidebar     `json:"right"`
	Closed   []ClosedTab `json:"closed"`
	Recent   []string    `json:"recent"`
	Files    FilesPane   `json:"files"`
	NextID   int         `json:"nextId"`
}

// New is a vault's first layout: one empty tab, both sidebars open on
// their first pane.
func New() *Workspace {
	return &Workspace{
		Version:  Version,
		Root:     &Node{Kind: KindTabs, ID: "g1", Tabs: []Tab{newTab("t2", Empty())}},
		ActiveID: "g1",
		Left:     Sidebar{Panes: slices.Clone(Panes[SideLeft]), Width: LeftWidth, Open: true},
		Right:    Sidebar{Panes: slices.Clone(Panes[SideRight]), Width: RightWidth, Open: true},
		Files:    FilesPane{Sort: "alphabetical"},
		NextID:   3,
	}
}

func newTab(id string, v View) Tab { return Tab{ID: id, View: v} }

// Groups lists the tab groups under n, left to right and top to bottom.
func Groups(n *Node) []*Node {
	if n.Kind == KindTabs {
		return []*Node{n}
	}
	var out []*Node
	for _, c := range n.Children {
		out = append(out, Groups(c)...)
	}
	return out
}

// Clone is a deep copy of the workspace.
func (w *Workspace) Clone() *Workspace {
	c := *w
	c.Root = w.Root.clone()
	c.Left.Panes = slices.Clone(w.Left.Panes)
	c.Right.Panes = slices.Clone(w.Right.Panes)
	c.Closed = make([]ClosedTab, len(w.Closed))
	for i, t := range w.Closed {
		c.Closed[i] = ClosedTab{Tab: t.Tab.clone(), Group: t.Group, Index: t.Index}
	}
	c.Recent = slices.Clone(w.Recent)
	c.Files.Expanded = slices.Clone(w.Files.Expanded)
	return &c
}

func (n *Node) clone() *Node {
	c := *n
	c.Sizes = slices.Clone(n.Sizes)
	c.Children = make([]*Node, len(n.Children))
	for i, ch := range n.Children {
		c.Children[i] = ch.clone()
	}
	c.Tabs = make([]Tab, len(n.Tabs))
	for i, t := range n.Tabs {
		c.Tabs[i] = t.clone()
	}
	if n.Children == nil {
		c.Children = nil
	}
	if n.Tabs == nil {
		c.Tabs = nil
	}
	return &c
}

func (t Tab) clone() Tab {
	t.Back = slices.Clone(t.Back)
	t.Forward = slices.Clone(t.Forward)
	return t
}
