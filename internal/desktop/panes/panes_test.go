package panes

import (
	"context"
	"slices"
	"strconv"
	"strings"
	"sync/atomic"
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

func TestRenameRevealsAClosedFolder(t *testing.T) {
	f := start(t, map[string]string{"Closed/Plan.md": "note"})
	store := workspace.NewStore(f.loop, t.TempDir())
	store.Load("/vault")
	p := &Files{Env: f.env, Store: store, Actions: FileActions{Rename: func(string, string, bool) {}}}
	tt := ui.NewTester(func(c *ui.Context) { p.View(c) }, 300, 500)
	p.Rename("Closed/Plan.md")
	tt.Frame()
	if _, ok := tt.Find("Name"); !ok {
		t.Fatal("rename field was not revealed")
	}
}

func TestTagExpansionSurvivesRefreshAndSort(t *testing.T) {
	p := &Tags{counts: []api.TagCount{{Tag: "garden/beds", Count: 1}}}
	p.rebuild()
	p.state.Open.Add(p.roots[0])
	p.counts = []api.TagCount{{Tag: "garden/beds", Count: 2}, {Tag: "other", Count: 3}}
	p.ByCount = true
	p.rebuild()
	for _, n := range p.roots {
		if n.Tag == "garden" && !p.state.Open.Has(n) {
			t.Fatal("expanded tag collapsed")
		}
	}
}

type queuedPaneLoop struct {
	*desktoptest.Loop
	posted chan func()
}

func (l *queuedPaneLoop) Post(f func()) { l.posted <- f }

type numberedPaneAPI struct {
	API
	calls atomic.Int64
}

func (a *numberedPaneAPI) Tags(context.Context, *api.NoteTagsParams) ([]api.TagCount, error) {
	return []api.TagCount{{Tag: "tag", Count: a.calls.Add(1)}}, nil
}
func (a *numberedPaneAPI) Properties(context.Context, *api.NotePropertiesParams) ([]api.PropertyCount, error) {
	return []api.PropertyCount{{Key: "key", Count: a.calls.Add(1)}}, nil
}
func (a *numberedPaneAPI) Tasks(context.Context, *api.NoteTasksParams) ([]api.TaskEntry, error) {
	return []api.TaskEntry{{Path: "a.md", Line: a.calls.Add(1)}}, nil
}
func (a *numberedPaneAPI) Search(context.Context, *api.NoteSearchParams) ([]api.SearchHit, error) {
	return []api.SearchHit{{Path: strconv.FormatInt(a.calls.Add(1), 10)}}, nil
}

func TestPaneRefreshDiscardsOlderAnswers(t *testing.T) {
	for _, name := range []string{"tags", "properties", "tasks", "search"} {
		t.Run(name, func(t *testing.T) {
			loop := &queuedPaneLoop{Loop: desktoptest.NewLoop(), posted: make(chan func(), 2)}
			client := &numberedPaneAPI{}
			env := &Env{Loop: loop, API: func() API { return client }}
			var refresh func()
			var value func() int64
			switch name {
			case "tags":
				p := &Tags{Env: env}
				refresh = p.Refresh
				value = func() int64 { return p.counts[0].Count }
			case "properties":
				p := &Properties{Env: env}
				refresh = p.Refresh
				value = func() int64 { return p.counts[0].Count }
			case "tasks":
				p := &Tasks{Env: env}
				refresh = p.Refresh
				value = func() int64 { return p.tasks[0].Line }
			case "search":
				p := &Search{Env: env, Query: "same"}
				refresh = func() { p.Refresh(); loop.Advance(SearchDelay) }
				value = func() int64 { n, _ := strconv.ParseInt(p.hits[0].Path, 10, 64); return n }
			}
			refresh()
			first := <-loop.posted
			refresh()
			second := <-loop.posted
			second()
			first()
			if value() != 2 {
				t.Fatalf("older %s answer replaced current result", name)
			}
		})
	}
}

func TestFilesClicksDoNotDependOnHoverOrSelection(t *testing.T) {
	f := start(t, map[string]string{"A.md": "a", "B.md": "b"})
	store := workspace.NewStore(f.loop, t.TempDir())
	store.Load("/vault")
	p := &Files{Env: f.env, Store: store}
	var places []string
	f.env.Actions.Open = func(path, where string) { f.opened = append(f.opened, path); places = append(places, where) }
	tt := ui.NewTester(func(c *ui.Context) { p.View(c) }, 300, 500)
	tt.Frame()
	if err := tt.Click("A"); err != nil {
		t.Fatal(err)
	}
	tt.Key(0, ui.KeyDown)
	tt.Frame()
	if !slices.Equal(f.opened, []string{"A.md"}) {
		t.Fatalf("arrow activated a file: %v", f.opened)
	}
	tt.Key(0, ui.KeyEnter)
	tt.Frame()
	if !slices.Equal(f.opened, []string{"A.md", "B.md"}) {
		t.Fatalf("Enter: %v", f.opened)
	}
	r, ok := tt.Find("A")
	if !ok {
		t.Fatal("missing A")
	}
	tt.PressButton(2, r.X+2, r.Y+2)
	tt.ReleaseButton(2, r.X+2, r.Y+2)
	tt.Frame()
	if !slices.Equal(places, []string{workspace.Here, workspace.Here, workspace.NewTab}) {
		t.Fatalf("destinations: %v", places)
	}
}

func TestAMiddleClickOpensAFileOnlyWhenReleasedOnIt(t *testing.T) {
	f := start(t, map[string]string{"A.md": "a", "B.md": "b", "Dir/C.md": "c"})
	store := workspace.NewStore(f.loop, t.TempDir())
	store.Load("/vault")
	p := &Files{Env: f.env, Store: store}
	var places []string
	f.env.Actions.Open = func(path, where string) { f.opened = append(f.opened, path); places = append(places, where) }
	tt := ui.NewTester(func(c *ui.Context) { p.View(c) }, 300, 500)
	tt.Frame()
	at := func(name string) (float32, float32) {
		t.Helper()
		r, ok := tt.Find(name)
		if !ok {
			t.Fatalf("missing %s", name)
		}
		return r.X + 2, r.Y + 2
	}
	ax, ay := at("A")
	bx, by := at("B")
	tt.PressButton(2, ax, ay) // released over another row: no click
	tt.Move(bx, by)
	tt.ReleaseButton(2, bx, by)
	tt.Frame()
	dx, dy := at("Dir")
	tt.PressButton(2, dx, dy) // a folder does not open
	tt.ReleaseButton(2, dx, dy)
	tt.Frame()
	if len(f.opened) != 0 {
		t.Fatalf("opened %v", f.opened)
	}
	tt.PressButton(2, bx, by)
	tt.ReleaseButton(2, bx, by)
	tt.Frame()
	if !slices.Equal(f.opened, []string{"B.md"}) || !slices.Equal(places, []string{workspace.NewTab}) {
		t.Fatalf("opened %v in %v", f.opened, places)
	}
}

func TestIdleFilesOpenSyncDoesNotAllocate(t *testing.T) {
	store := workspace.NewStore(desktoptest.NewLoop(), t.TempDir())
	store.Load("/vault")
	p := &Files{Store: store}
	p.syncOpen()
	for i := range 1000 {
		p.list.SetOpen(section, strconv.Itoa(i), true)
	}
	p.syncOpen()
	if n := testing.AllocsPerRun(20, p.syncOpen); n != 0 {
		t.Fatalf("idle folder persistence allocates %v times", n)
	}
}
