package workspace

import (
	"fmt"
	"slices"
	"testing"
)

func note(p string) View { return Note(p, ModeEdit) }

// titles are the active group's tabs, as the web app's tests name them.
func titles(w *Workspace) []string {
	var out []string
	for _, t := range w.ActiveGroup().Tabs {
		if t.View.Type == ViewNote {
			out = append(out, t.View.Path)
		} else {
			out = append(out, t.View.Type)
		}
	}
	return out
}

// with opens each path in a new tab of a fresh workspace, so the first
// tab stays empty.
func with(paths ...string) *Workspace {
	w := New()
	for _, p := range paths {
		w.Open(note(p), NewTab)
	}
	return w
}

func eq(t *testing.T, what string, got, want []string) {
	t.Helper()
	if !slices.Equal(got, want) {
		t.Errorf("%s: %q, want %q", what, got, want)
	}
}

// The cases of app/src/workspace/ops.test.ts.
func TestOpening(t *testing.T) {
	w := New()
	w.Open(note("a.md"), Here)
	w.Open(note("b.md"), Here)
	if tab := w.ActiveTab(); tab.View != note("b.md") || len(tab.Back) != 1 || tab.Back[0] != note("a.md") {
		t.Errorf("tab %+v", tab)
	}
	eq(t, "recent", w.Recent, []string{"b.md", "a.md"})

	w = with("a.md", "b.md")
	w.Activate(w.ActiveGroup().Tabs[1].ID)
	w.Pin(w.ActiveTab().ID, true)
	w.Open(note("c.md"), Here)
	eq(t, "a pinned tab's link", titles(w), []string{"empty", "a.md", "c.md", "b.md"})

	w = New()
	w.Open(note("a.md"), Here)
	w.Open(note("b.md"), ToRight)
	if w.Root.Kind != KindSplit || w.Root.Direction != Row || !slices.Equal(w.Root.Sizes, []float64{0.5, 0.5}) {
		t.Fatalf("root %+v", w.Root)
	}
	w.Open(note("c.md"), Below)
	if w.Root.Children[1].Kind != KindSplit || w.Root.Children[1].Direction != Column || w.ActiveTab().View != note("c.md") {
		t.Fatalf("below: %+v", w.Root.Children[1])
	}
	w.Activate(Groups(w.Root)[0].Tabs[0].ID)
	w.Open(note("d.md"), ToRight)
	if len(w.Root.Children) != 3 || !slices.Equal(w.Root.Sizes, []float64{0.25, 0.25, 0.5}) {
		t.Errorf("a third group to the right: %v", w.Root.Sizes)
	}
}

func TestBackAndForward(t *testing.T) {
	w := New()
	for i := 0; i <= MaxHistory+5; i++ {
		w.Open(note(fmt.Sprintf("%d.md", i)), Here)
	}
	if n := len(w.ActiveTab().Back); n != MaxHistory {
		t.Fatalf("%d back", n)
	}
	w.Back()
	w.Back()
	if w.ActiveTab().View != note(fmt.Sprintf("%d.md", MaxHistory+3)) {
		t.Fatalf("back to %+v", w.ActiveTab().View)
	}
	w.Forward()
	if w.ActiveTab().View != note(fmt.Sprintf("%d.md", MaxHistory+4)) {
		t.Fatalf("forward to %+v", w.ActiveTab().View)
	}
	w.Open(note("x.md"), Here)
	if len(w.ActiveTab().Forward) != 0 {
		t.Fatal("forward kept after a new view")
	}
}

func TestActivating(t *testing.T) {
	w := with("a.md", "b.md", "c.md")
	c := w.Clone()
	c.Cycle(1)
	if c.ActiveTab().View.Type != ViewEmpty {
		t.Error("cycling did not wrap")
	}
	c = w.Clone()
	c.Cycle(-1)
	if c.ActiveTab().View != note("b.md") {
		t.Error("cycling back")
	}
	c = w.Clone()
	c.GotoTab(2)
	if c.ActiveTab().View != note("a.md") {
		t.Error("goto 2")
	}
	c.GotoTab(9)
	if c.ActiveTab().View != note("a.md") {
		t.Error("goto past the end moved")
	}
	c.GotoLastTab()
	if c.ActiveTab().View != note("c.md") {
		t.Error("goto last")
	}
}

func TestClosing(t *testing.T) {
	w := with("a.md", "b.md", "c.md")
	w.GotoTab(2)
	w.Close(w.ActiveTab().ID)
	eq(t, "after closing a.md", titles(w), []string{"empty", "b.md", "c.md"})
	if w.ActiveTab().View != note("b.md") {
		t.Errorf("active %+v", w.ActiveTab().View)
	}

	w = New()
	w.Open(note("a.md"), Here)
	id := w.ActiveTab().ID
	w.Close(id)
	eq(t, "the last tab", titles(w), []string{"empty"})
	w.ReopenClosed()
	eq(t, "reopened", titles(w), []string{"a.md", "empty"})
	if w.ActiveTab().ID != id {
		t.Error("the reopened tab is not active")
	}

	w = New()
	for i := range MaxClosed + 3 {
		w.Open(note(fmt.Sprintf("%d.md", i)), NewTab)
		w.Close(w.ActiveTab().ID)
	}
	if len(w.Closed) != MaxClosed {
		t.Errorf("%d closed kept", len(w.Closed))
	}

	w = New()
	w.Open(note("a.md"), Here)
	w.Open(note("b.md"), ToRight)
	w.Close(w.ActiveTab().ID)
	if w.Root.Kind != KindTabs || w.ActiveTab().View != note("a.md") {
		t.Fatalf("the split did not collapse: %+v", w.Root)
	}
	w.ReopenClosed()
	eq(t, "reopened where its group was", titles(w), []string{"a.md", "b.md"})
}

func TestClosingSparesPinnedTabs(t *testing.T) {
	w := with("a.md", "b.md", "c.md", "d.md")
	tabs := w.ActiveGroup().Tabs
	a, b, c := tabs[1].ID, tabs[2].ID, tabs[3].ID
	w.Pin(c, true)
	x := w.Clone()
	x.CloseOthers(b)
	eq(t, "close others", titles(x), []string{"b.md", "c.md"})
	x = w.Clone()
	x.CloseToTheRight(a)
	eq(t, "close to the right", titles(x), []string{"empty", "a.md", "c.md"})
	x = w.Clone()
	x.CloseAll(w.ActiveID)
	eq(t, "close all", titles(x), []string{"c.md"})
	w.Pin(c, false)
	w.CloseAll(w.ActiveID)
	eq(t, "close all, nothing pinned", titles(w), []string{"empty"})
}

func TestSplittingAndMoving(t *testing.T) {
	w := New()
	w.Open(note("a.md"), Here)
	w.Split(w.ActiveTab().ID, Below)
	if w.Root.Direction != Column || w.ActiveID != Groups(w.Root)[1].ID || Groups(w.Root)[1].Tabs[0].View != note("a.md") {
		t.Fatalf("split: %+v", w.Root)
	}

	w = with("a.md", "b.md", "c.md")
	a := w.ActiveGroup().Tabs[1].ID
	w.MoveTab(a, w.ActiveID, 4, "")
	eq(t, "moved to the end", titles(w), []string{"empty", "b.md", "c.md", "a.md"})
	if w.ActiveTab().ID != a {
		t.Error("the moved tab is not active")
	}

	w = with("a.md", "b.md")
	b := w.ActiveGroup().Tabs[2].ID
	g := w.ActiveID
	w.MoveTab(b, g, 0, EdgeLeft)
	if w.Root.Kind != KindSplit || w.Root.Direction != Row || w.Root.Children[0].Tabs[0].ID != b {
		t.Fatalf("moved to the left edge: %+v", w.Root)
	}
	w.MoveTab(b, g, 1, "")
	if w.Root.Kind != KindTabs {
		t.Fatal("moving the only tab of a group back did not collapse the split")
	}

	w.Resize("none", []float64{1})
	w = New()
	w.Open(note("a.md"), ToRight)
	w.Resize(w.Root.ID, []float64{3, 1})
	if !slices.Equal(w.Root.Sizes, []float64{0.75, 0.25}) {
		t.Errorf("sizes %v", w.Root.Sizes)
	}
	w.Resize(w.Root.ID, []float64{1, 0})
	if w.Root.Sizes[1] < 0.04 {
		t.Errorf("a size under 5%%: %v", w.Root.Sizes)
	}
}

func TestSidebarsAndFiles(t *testing.T) {
	w := New()
	w.ToggleSidebar(SideRight)
	if w.Right.Open {
		t.Error("toggle")
	}
	w.SelectPane(SideRight, "tags")
	if !w.Right.Open || w.Right.Panes[w.Right.Active] != "tags" {
		t.Error("select pane")
	}
	w.SetSidebarWidth(SideLeft, 9999)
	w.SetSidebarWidth(SideRight, 10)
	if w.Left.Width != SidebarMax || w.Right.Width != SidebarMin {
		t.Errorf("widths %v %v", w.Left.Width, w.Right.Width)
	}
	w.RevealPath("a/b/c.md")
	eq(t, "revealed", w.Files.Expanded, []string{"a", "a/b"})
	w.SetFolderOpen("a", false)
	eq(t, "closed a", w.Files.Expanded, []string{"a/b"})
}

func TestFollowingTheVault(t *testing.T) {
	w := with("Notes/a.md", "b.md")
	w.Open(note("Notes/c.md"), Here)
	w.RevealPath("Notes/a.md")
	w.ApplyMove("Notes", "Archive")
	eq(t, "titles", titles(w), []string{"empty", "Archive/a.md", "Archive/c.md"})
	if w.ActiveTab().Back[0] != note("b.md") {
		t.Errorf("history %+v", w.ActiveTab().Back)
	}
	eq(t, "expanded", w.Files.Expanded, []string{"Archive"})
	w.ApplyDelete("b.md")
	if len(w.ActiveTab().Back) != 0 || slices.Contains(w.Recent, "b.md") {
		t.Error("a deleted file stayed in history")
	}
}

func TestCloneOwnsMergeSides(t *testing.T) {
	w := New()
	v := View{Type: ViewMerge, Path: "a.md", Other: &MergeSide{Kind: "version", Version: 1}}
	w.Root.Tabs[0].View = v
	w.Root.Tabs[0].Back = []View{v}
	w.Root.Tabs[0].Forward = []View{v}
	w.Closed = []ClosedTab{{Tab: w.Root.Tabs[0]}}
	clone := w.Clone()
	for _, tab := range []*Tab{&clone.Root.Tabs[0], &clone.Closed[0].Tab} {
		tab.View.Other.Version = 2
		tab.Back[0].Other.Version = 3
		tab.Forward[0].Other.Version = 4
	}
	if v.Other.Version != 1 {
		t.Fatal("clone changed the original merge side")
	}
	if clone.Root.Tabs[0].View.Other.Version != 2 {
		t.Fatal("cloned views share a merge side")
	}
}
