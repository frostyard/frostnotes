package uikit

import (
	"go/parser"
	"go/token"
	"os"
	"slices"
	"strconv"
	"strings"
	"testing"

	"github.com/egoist/mygo/ui"
)

// The package moves to frostui by changing its import path: it may import
// only the standard library, MyGo, purego and frostui.
func TestImportsNothingOfTheApp(t *testing.T) {
	entries, err := os.ReadDir(".")
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range entries {
		if !strings.HasSuffix(e.Name(), ".go") {
			continue
		}
		f, err := parser.ParseFile(token.NewFileSet(), e.Name(), nil, parser.ImportsOnly)
		if err != nil {
			t.Fatal(err)
		}
		for _, imp := range f.Imports {
			p, _ := strconv.Unquote(imp.Path.Value)
			first, _, _ := strings.Cut(p, "/")
			switch {
			case !strings.Contains(first, "."):
			case strings.HasPrefix(p, "github.com/egoist/mygo"), strings.HasPrefix(p, "github.com/ebitengine/purego"), strings.HasPrefix(p, "github.com/frostyard/frostui"):
			default:
				t.Errorf("%s imports %s", e.Name(), p)
			}
		}
	}
}

func TestTheTabStrip(t *testing.T) {
	tabs := []Tab{{ID: "a", Title: "Alpha"}, {ID: "b", Title: "Beta"}, {ID: "p", Title: "Pinned", Pinned: true}}
	active := 0
	var closed, moved []string
	added := 0
	strip := func() TabStrip {
		return TabStrip{Label: "Tabs", Tabs: tabs, Active: active,
			Activate: func(id string) { active = slices.IndexFunc(tabs, func(x Tab) bool { return x.ID == id }) },
			Close:    func(id string) { closed = append(closed, id) },
			New:      func() { added++ },
			Move:     func(id string, i int) { moved = append(moved, id+strconv.Itoa(i)) },
			Menu:     func(m *ui.Menu, id string) { m.Item("Close " + id) },
		}
	}
	tt := ui.NewTester(func(c *ui.Context) { strip().View(c) }, 700, 60)
	beta, _ := tt.Find("Beta")
	x, y := beta.X+16, beta.Y+beta.H/2 // on its title: its middle is its close button
	tt.ClickAt(x, y)
	if active != 1 {
		t.Fatalf("active %d", active)
	}
	if err := tt.Click("Close Alpha"); err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(closed, []string{"a"}) {
		t.Errorf("%q", closed)
	}
	if _, found := tt.Find("Close Pinned"); found {
		t.Error("a pinned tab has a close button")
	}
	if err := tt.Click("New tab"); err != nil || added != 1 {
		t.Fatalf("%v %d", err, added)
	}
	tt.PressButton(1, x, y)
	tt.ReleaseButton(1, x, y)
	if !slices.Contains(tt.Menu(), "Close b") {
		t.Fatalf("%q", tt.Menu())
	}
	tt.CloseMenu()
	if a, ok := tt.AccessByRole(ui.RoleTab, "Beta"); !ok || !a.Selected {
		t.Errorf("the chosen tab: %+v %v", a, ok)
	}
}

func TestTheSplitView(t *testing.T) {
	leaf := func(name string) *SplitNode {
		return &SplitNode{ID: name, Leaf: func(c *ui.Context) { ui.Text(c, name) }}
	}
	root := &SplitNode{ID: "s", Row: true, Children: []*SplitNode{leaf("one"), leaf("two"), leaf("three")}, Sizes: []float64{0.5, 0.25, 0.25}}
	var state SplitState
	tt := ui.NewTester(func(c *ui.Context) {
		ui.Row(c).Fill().AlignItems(ui.Stretch).Children(func() { SplitView(c, root, &state, nil) })
	}, 800, 400)
	for _, name := range []string{"one", "two", "three"} {
		if !tt.HasText(name) {
			t.Errorf("no %s: %q", name, tt.Texts())
		}
	}
	one, _ := tt.Find("one")
	two, _ := tt.Find("two")
	if two.X < 380 || two.X > 420 || one.X != 0 {
		t.Errorf("the first takes half: %v %v", one, two)
	}
}

func TestThePaneTabs(t *testing.T) {
	var chosen string
	active := 0
	panes := []PaneTab{{"files", "Files", "folder"}, {"search", "Search", "system-search"}}
	tt := ui.NewTester(func(c *ui.Context) {
		PaneTabs(c, "Left sidebar", panes, active, func(id string) { chosen = id })
	}, 300, 60)
	if err := tt.Click("Search"); err != nil || chosen != "search" {
		t.Fatalf("%v %q", err, chosen)
	}
}
