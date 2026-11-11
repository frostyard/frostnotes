package panes

import (
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"

	"github.com/frostyard/frostnotes/api"
	"github.com/frostyard/frostnotes/internal/desktop/catalog"
	"github.com/frostyard/frostnotes/internal/desktop/desktoptest"
	"github.com/frostyard/frostnotes/internal/desktop/workspace"
)

func TestNaturalOrder(t *testing.T) {
	names := []string{"note 10", "Note 2", "note 1", "alpha", "note"}
	slices.SortFunc(names, compareNatural)
	if !slices.Equal(names, []string{"alpha", "note", "note 1", "Note 2", "note 10"}) {
		t.Errorf("%q", names)
	}
}

func TestBuildTree(t *testing.T) {
	at := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	files := []api.VaultFile{
		{Path: "b.md", ModifiedAt: at}, {Path: "a.md", ModifiedAt: at.Add(time.Hour)}, {Path: "d/x.md", ModifiedAt: at},
		{Path: "c.bin", ModifiedAt: at}, {Path: "p.png", ModifiedAt: at},
	}
	tr := BuildTree(files, []string{"d", "empty"}, SortAlphabetical, false)
	if !slices.Equal(tr.Roots, []string{"d", "empty", "a.md", "b.md", "p.png"}) {
		t.Errorf("%q", tr.Roots)
	}
	tr = BuildTree(files, []string{"d"}, SortModified, true)
	if !slices.Equal(tr.Roots, []string{"d", "a.md", "b.md", "c.bin", "p.png"}) {
		t.Errorf("modified: %q", tr.Roots)
	}
	tr = BuildTree(files, nil, SortAlphabeticalReverse, false)
	if !slices.Equal(tr.Roots, []string{"d", "p.png", "b.md", "a.md"}) {
		t.Errorf("reversed: %q", tr.Roots)
	}
	if n := tr.Nodes["p.png"]; n.Name != "p" || n.Extension != "png" || n.Type != "image" {
		t.Errorf("%+v", n)
	}
}

func TestTagTree(t *testing.T) {
	roots := TagTree([]api.TagCount{{Tag: "garden/fruit", Count: 2}, {Tag: "Garden", Count: 1}, {Tag: "zoo", Count: 9}}, false)
	if len(roots) != 2 || roots[0].Tag != "garden" || roots[0].Count != 3 || roots[0].Children[0].Tag != "garden/fruit" {
		t.Fatalf("%+v", roots[0])
	}
	if byCount := TagTree([]api.TagCount{{Tag: "a", Count: 1}, {Tag: "b", Count: 5}}, true); byCount[0].Tag != "b" {
		t.Error("by count")
	}
}

type fixture struct {
	*desktoptest.Notesd
	loop   *desktoptest.Loop
	env    *Env
	active string
	text   string
	opened []string
	lines  []int
	asked  []string
	create []string
}

func start(t *testing.T, files map[string]string) *fixture {
	t.Helper()
	f := &fixture{Notesd: desktoptest.StartNotesd(t), loop: desktoptest.NewLoop()}
	for p, c := range files {
		f.Create(p, c)
	}
	cat := catalog.New(f.loop, func() catalog.API {
		if a := f.AppNotes(); a != nil {
			return a
		}
		return nil
	}, f.Vault)
	cat.Reload()
	f.loop.Until(t, "the files", func() bool { return len(cat.Files()) == len(files) })
	f.env = &Env{Loop: f.loop, Vault: f.Vault, Catalog: cat,
		API: func() API {
			if a := f.AppNotes(); a != nil {
				return a
			}
			return nil
		},
		Active:     func() string { return f.active },
		ActiveText: func() (string, bool) { return f.text, f.text != "" },
		Actions: Actions{
			Open:         func(p, _ string) { f.opened = append(f.opened, p) },
			OpenAtLine:   func(p string, l int, _ string) { f.opened = append(f.opened, p); f.lines = append(f.lines, l) },
			OpenOrCreate: func(target, _ string) { f.create = append(f.create, target) },
			SearchFor:    func(q string) { f.asked = append(f.asked, q) },
			JumpToLine:   func(l int) { f.lines = append(f.lines, l) },
			Report:       func(err error) { t.Error(err) },
		}}
	return f
}

func TestTheFilesPane(t *testing.T) {
	f := start(t, map[string]string{"Projects/Plan.md": "x", "Inbox.md": "y"})
	store := workspace.NewStore(f.loop, t.TempDir())
	store.Load("/vault")
	var renamed []string
	p := &Files{Env: f.env, Store: store, Actions: FileActions{Rename: func(a, b string, _ bool) { renamed = append(renamed, a+">"+b) }}}
	tt := ui.NewTester(func(c *ui.Context) { p.View(c) }, 300, 500)
	if !tt.HasText("Projects") || !tt.HasText("Inbox") || tt.HasText("Plan") {
		t.Fatalf("%q", tt.Texts())
	}
	if err := tt.Click("Projects"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	if !tt.HasText("Plan") {
		t.Fatalf("the folder did not open: %q", tt.Texts())
	}
	if err := tt.Click("Plan"); err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(f.opened, []string{"Projects/Plan.md"}) {
		t.Errorf("%q", f.opened)
	}
	tt.Frame()
	if !slices.Equal(store.Workspace().Files.Expanded, []string{"Projects"}) {
		t.Errorf("expanded %q", store.Workspace().Files.Expanded)
	}
	if err := tt.RightClick("Inbox"); err != nil {
		t.Fatal(err)
	}
	if !slices.Contains(tt.Menu(), "Rename…") {
		t.Fatalf("menu %q", tt.Menu())
	}
	if err := tt.ChooseMenuItem("Rename…"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	tt.Key(ui.Cmd, ui.KeyA)
	tt.Type("Later")
	tt.Key(0, ui.KeyEnter)
	if !slices.Equal(renamed, []string{"Inbox.md>Later"}) {
		t.Errorf("%q", renamed)
	}
}

func TestTheSearchPane(t *testing.T) {
	f := start(t, map[string]string{"Plan.md": "the seeds grow\n", "Other.md": "---\nowner: x\n---\nnothing\n"})
	s := &Search{Env: f.env}
	tt := ui.NewTester(func(c *ui.Context) { s.View(c) }, 300, 500)
	f.loop.Until(t, "the index", func() bool {
		s.Ask("seeds")
		f.loop.Advance(SearchDelay)
		f.loop.Until(t, "the answer", func() bool { return s.done == "seeds" })
		return len(s.hits) == 1
	})
	tt.Frame()
	if err := tt.Click("Plan"); err != nil {
		t.Fatalf("%v %q", err, tt.Texts())
	}
	if !slices.Equal(f.opened, []string{"Plan.md"}) {
		t.Errorf("%q", f.opened)
	}
	s.Ask("[owner]")
	f.loop.Advance(SearchDelay)
	f.loop.Until(t, "the property's notes", func() bool { return s.done == "[owner]" })
	if len(s.hits) != 1 || s.hits[0].Path != "Other.md" {
		t.Errorf("%+v", s.hits)
	}
}

func TestLinksPanes(t *testing.T) {
	f := start(t, map[string]string{"Plan.md": "[[Nowhere]] and [[Seeds]]\n", "Seeds.md": "see [[Plan]]\n", "Mention.md": "the Plan is here\n"})
	f.active = "Plan.md"
	b := &Backlinks{Env: f.env}
	f.loop.Until(t, "the backlinks", func() bool {
		b.Refresh()
		gen := b.stale
		f.loop.Until(t, "an answer", func() bool { return b.path == "Plan.md" && gen == b.stale })
		return b.Count() == 1 && len(b.unlinked) == 1
	})
	tt := ui.NewTester(func(c *ui.Context) { b.View(c) }, 300, 500)
	if !tt.HasText("Linked mentions 1") || !tt.HasText("Unlinked mentions 1") {
		t.Fatalf("%q", tt.Texts())
	}
	if err := tt.Click("Seeds"); err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(f.opened, []string{"Seeds.md"}) {
		t.Errorf("%q", f.opened)
	}
	o := &Outgoing{Env: f.env}
	o.Refresh()
	f.loop.Until(t, "the links", func() bool { return len(o.links) == 2 })
	tt2 := ui.NewTester(func(c *ui.Context) { o.View(c) }, 300, 500)
	if err := tt2.Click("Nowhere"); err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(f.create, []string{"Nowhere"}) {
		t.Errorf("%q", f.create)
	}
}

func TestCountsAndOutline(t *testing.T) {
	note := "---\nstatus: draft\ntags: [garden/beds]\n---\n# One\n\n## Two\n\n- [ ] do it\n"
	f := start(t, map[string]string{"Plan.md": note})
	tags := &Tags{Env: f.env}
	tags.Refresh()
	f.loop.Until(t, "the tags", func() bool { return len(tags.roots) == 1 })
	tt := ui.NewTester(func(c *ui.Context) { tags.View(c) }, 300, 400)
	if err := tt.Click("#garden"); err != nil {
		t.Fatalf("%v %q", err, tt.Texts())
	}
	if !slices.Equal(f.asked, []string{"tag:#garden"}) {
		t.Errorf("%q", f.asked)
	}
	props := &Properties{Env: f.env}
	props.Refresh()
	f.loop.Until(t, "the properties", func() bool { return len(props.counts) >= 1 })
	tp := ui.NewTester(func(c *ui.Context) { props.View(c) }, 300, 400)
	if err := tp.Click("status"); err != nil {
		t.Fatalf("%v %q", err, tp.Texts())
	}
	if f.asked[len(f.asked)-1] != "[status]" {
		t.Errorf("%q", f.asked)
	}
	tasks := &Tasks{Env: f.env}
	tasks.Refresh()
	f.loop.Until(t, "the tasks", func() bool { return len(tasks.tasks) == 1 })
	tk := ui.NewTester(func(c *ui.Context) { tasks.View(c) }, 300, 400)
	if err := tk.Click("do it"); err != nil {
		t.Fatalf("%v %q", err, tk.Texts())
	}
	if len(f.lines) != 1 || f.lines[0] != 9 {
		t.Errorf("%v", f.lines)
	}
	f.text = note
	o := &Outline{Env: f.env, CurrentLine: func() int { return 7 }}
	to := ui.NewTester(func(c *ui.Context) { o.View(c) }, 300, 400)
	if !to.HasText("One") || !to.HasText("Two") {
		t.Fatalf("%q", to.Texts())
	}
	if err := to.Click("Two"); err != nil {
		t.Fatal(err)
	}
	if f.lines[len(f.lines)-1] != 7 {
		t.Errorf("%v", f.lines)
	}
	if !strings.Contains(strings.Join(to.Texts(), " "), "One") {
		t.Error("the outline lost its rows")
	}
}

func TestARenameOfTheSameLengthRebuildsTheFiles(t *testing.T) {
	f := start(t, map[string]string{"aaa.md": "x"})
	files := &Files{Env: f.env, Store: workspace.NewStore(f.loop, t.TempDir())}
	files.rebuild()
	if _, err := f.Other.Note().Move(t.Context(), &api.NoteMoveParams{VaultID: f.Vault, Path: "aaa.md", To: "bbb.md"}); err != nil {
		t.Fatal(err)
	}
	generation := f.env.Catalog.Generation()
	f.env.Catalog.Reload()
	f.loop.Until(t, "the reload", func() bool { return f.env.Catalog.Generation() != generation })
	files.rebuild()
	if files.tree.Nodes["bbb.md"] == nil {
		t.Fatalf("the tree kept %q", files.tree.Roots)
	}
}

func TestANoteWithoutHeadingsIsParsedOnce(t *testing.T) {
	f := start(t, map[string]string{"a.md": "x"})
	f.text = "no headings\n"
	o := &Outline{Env: f.env}
	o.refresh()
	o.refresh()
	if o.parses != 1 {
		t.Fatalf("parsed %d times", o.parses)
	}
}

func TestTasksAreRowsOfAList(t *testing.T) {
	rows := taskRows([]api.TaskEntry{{Path: "a.md", Title: "A", Text: "1"}, {Path: "a.md", Title: "A", Text: "2"}, {Path: "b.md", Title: "B", Text: "3"}})
	if len(rows) != 5 || rows[0].title != "A" || rows[1].task.Text != "1" || rows[3].title != "B" {
		t.Fatalf("%+v", rows)
	}
}

func TestSearchOfflineAndAfterReconnect(t *testing.T) {
	f := start(t, map[string]string{"a.md": "findme\n"})
	real := f.env.API
	f.env.API = func() API { return nil }
	s := &Search{Env: f.env, Query: "findme"}
	s.schedule()
	f.loop.Advance(SearchDelay)
	if s.err == nil || s.done != "findme" {
		t.Fatalf("offline: done %q, err %v", s.done, s.err)
	}
	f.env.API = real
	f.loop.Until(t, "the results", func() bool {
		s.Refresh()
		f.loop.Advance(SearchDelay)
		f.loop.Until(t, "the answer", func() bool { return s.err == nil })
		return len(s.hits) == 1
	})
}

func TestTagsAnswerTheKeyboard(t *testing.T) {
	f := start(t, map[string]string{"a.md": "#one #two\n"})
	var searched []string
	f.env.Actions.SearchFor = func(q string) { searched = append(searched, q) }
	p := &Tags{Env: f.env}
	p.Refresh()
	f.loop.Until(t, "the tags", func() bool { return len(p.roots) == 2 })
	tt := ui.NewTester(func(c *ui.Context) { p.View(c) }, 300, 300)
	tt.Frame()
	for range 6 { // past the sort button, to the list
		tt.Key(0, ui.KeyTab)
		tt.Key(0, ui.KeyDown)
		if p.chosen >= 0 {
			break
		}
	}
	if p.chosen != 0 {
		t.Fatalf("Tab and Down chose row %d", p.chosen)
	}
	tt.Key(0, ui.KeyDown)
	tt.Key(0, ui.KeyEnter)
	if len(searched) != 1 || searched[0] != "tag:#two" {
		t.Fatalf("searched %q", searched)
	}
}
