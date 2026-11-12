package panes

import (
	"context"
	"errors"
	"path"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/egoist/mygo/ui"
	"github.com/frostyard/frostui"

	"github.com/frostyard/frostnotes/api"
	"github.com/frostyard/frostnotes/internal/desktop/catalog"
	"github.com/frostyard/frostnotes/internal/desktop/notes"
	"github.com/frostyard/frostnotes/internal/desktop/uikit"
	"github.com/frostyard/frostnotes/internal/markdown"
)

// API is what the panes read of notesd; api.NoteClient has it.
type API interface {
	Search(ctx context.Context, p *api.NoteSearchParams) ([]api.SearchHit, error)
	List(ctx context.Context, p *api.NoteListParams) ([]api.NoteEntry, error)
	Backlinks(ctx context.Context, p *api.NoteBacklinksParams) ([]api.Backlink, error)
	Links(ctx context.Context, p *api.NoteLinksParams) ([]api.Link, error)
	Read(ctx context.Context, p *api.NoteReadParams) (*api.Note, error)
	Tags(ctx context.Context, p *api.NoteTagsParams) ([]api.TagCount, error)
	Properties(ctx context.Context, p *api.NotePropertiesParams) ([]api.PropertyCount, error)
	Tasks(ctx context.Context, p *api.NoteTasksParams) ([]api.TaskEntry, error)
}

// Actions are what the panes ask of the window. where is a
// workspace.Here, NewTab, ToRight or Below.
type Actions struct {
	Open         func(path, where string)
	OpenAtLine   func(path string, line int, where string)
	OpenOrCreate func(target, where string)
	SearchFor    func(query string)
	JumpToLine   func(line int) // in the active note
	Report       func(error)
}

// Env is what every pane shares.
type Env struct {
	Loop    notes.Env
	API     func() API
	Vault   int64
	Catalog *catalog.Catalog
	Actions Actions
	// Active is the active tab's file, "" for none; ActiveText its text
	// as the editor holds it, ok false when it is not open as a note.
	Active     func() string
	ActiveText func() (string, bool)
	// Where turns the modifiers of a click into where it opens.
	Where func(mods ui.Modifiers) string
}

// call runs f off the UI thread and done on it.
func (e *Env) call(f func(ctx context.Context, a API) error, done func(error)) {
	a := e.API()
	if a == nil {
		return
	}
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		err := f(ctx, a)
		e.Loop.Post(func() { done(err) })
	}()
}

func (e *Env) report(err error) {
	if err != nil && e.Actions.Report != nil {
		e.Actions.Report(err)
	}
}

func (e *Env) where(c *ui.Context) string {
	if e.Where != nil {
		return e.Where(c.Modifiers())
	}
	return ""
}

// row is a clickable row of a pane: a button, so the keyboard and screen
// readers reach it.
func row(c *ui.Context, label string, selected bool) ui.Element {
	t := c.Theme()
	r := ui.Row(c).Gap(frostui.SpaceXS).Padding(3, frostui.SpaceS).Radius(frostui.RadiusChip).AlignItems(ui.Center).
		Role(ui.RoleButton).Focusable().Cursor(ui.CursorPointer).Label(label)
	if selected {
		uikit.SelectedRow(r, t)
	} else if r.Hovered() {
		r.TextColor(frostui.ColorsOf(t).HoverText)
	}
	return r
}

// mutedSize is frostui.Muted's size, at the platform's usual text size.
const mutedSize = 12.5

// snippet draws text with its matches in bold.
func snippet(c *ui.Context, parts []api.SnippetPart) ui.Element {
	spans := make([]ui.Span, len(parts))
	for i, p := range parts {
		spans[i] = ui.Span{Text: p.Text}
		if p.Match {
			spans[i].Weight = 700
		}
	}
	// frostui's Muted, which takes no spans.
	return ui.RichText(c, spans...).FontSize(frostui.TextSize(c, mutedSize)).TextColor(c.Theme().TextMuted).Wrap()
}

func titleOf(p string) string { return strings.TrimSuffix(path.Base(p), ".md") }

// SearchDelay is how long after the last change the pane searches.
const SearchDelay = 250 * time.Millisecond

var propertyQuery = regexp.MustCompile(`^\[([^\]:]+)\]$`)

// Search is the Search pane (docs/specs/ui.md#search-pane-batch-2).
type Search struct {
	Env        *Env
	Query      string
	list       ui.ListState
	hits       []api.SearchHit
	asked      string
	done       string // the query hits answer
	err        error
	stop       func()
	focus      bool
	generation uint64
}

// Ask shows query in the pane and focuses it.
func (s *Search) Ask(query string) {
	s.Query, s.focus = query, true
	s.schedule()
}

func (s *Search) schedule() {
	if s.stop != nil {
		s.stop()
	}
	q := strings.TrimSpace(s.Query)
	s.generation++
	s.asked = q
	if q == "" {
		s.hits, s.done, s.err = nil, "", nil
		return
	}
	s.stop = s.Env.Loop.After(SearchDelay, func() {
		s.stop = nil
		s.run(q)
	})
}

// Refresh asks the current query again: the vault changed, or notesd came
// back.
func (s *Search) Refresh() {
	if strings.TrimSpace(s.Query) != "" {
		s.schedule()
	}
}

func (s *Search) run(q string) {
	gen := s.generation
	if s.Env.API() == nil {
		s.done, s.err = q, errors.New("not connected to notesd")
		return
	}
	var hits []api.SearchHit
	limit := int64(100)
	s.Env.call(func(ctx context.Context, a API) error {
		if m := propertyQuery.FindStringSubmatch(q); m != nil {
			key := strings.TrimSpace(m[1])
			list, err := a.List(ctx, &api.NoteListParams{VaultID: s.Env.Vault, Property: &key})
			for _, n := range list {
				hits = append(hits, api.SearchHit{Path: n.Path, Title: n.Title})
			}
			return err
		}
		var err error
		hits, err = a.Search(ctx, &api.NoteSearchParams{VaultID: s.Env.Vault, Query: q, Limit: &limit})
		return err
	}, func(err error) {
		if gen != s.generation || q != s.asked {
			return // an answer for another query is not shown
		}
		s.hits, s.done, s.err = hits, q, err
	})
}

// View draws the pane.
func (s *Search) View(c *ui.Context) ui.Element {
	return ui.Column(c).Fill().AlignItems(ui.Stretch).Gap(frostui.SpaceS).Padding(frostui.SpaceS).Children(func() {
		field := ui.SearchField(c, &s.Query).Label("Search in all files").Placeholder("Search")
		if s.focus {
			s.focus = false
			field.AutoFocus()
		}
		if field.Changed() {
			s.schedule()
		}
		switch {
		case s.err != nil:
			frostui.Problem(c, s.err.Error())
		case s.asked != "" && s.done != s.asked:
			frostui.Caption(c, "Searching…")
		case s.asked != "" && len(s.hits) == 0:
			frostui.Muted(c, "No results.")
		case s.asked != "":
			ui.List(c, &s.list, len(s.hits), func(i int) {
				h := s.hits[i]
				ui.Column(c).AlignItems(ui.Stretch).Children(func() {
					r := row(c, h.Title, false)
					if r.Clicked() {
						s.Env.Actions.Open(h.Path, s.Env.where(c))
					}
					r.Children(func() { ui.Text(c, h.Title).SingleLine() })
					if len(h.Snippet) > 0 {
						snippet(c, h.Snippet).Padding(0, frostui.SpaceS, frostui.SpaceXS)
					}
				})
			}).Grow(1)
		}
	})
}

// Mention is a note that links to the active note, or mentions it.
type Mention struct {
	Path, Title string
	Lines       []MentionLine
}

// MentionLine is where one mention is; Line is 0 for an unlinked one.
type MentionLine struct {
	Line  int
	Parts []api.SnippetPart
}

// LinkedGroups groups backlinks by their source, in notesd's order.
func LinkedGroups(backlinks []api.Backlink) []Mention {
	var out []Mention
	at := map[string]int{}
	for _, b := range backlinks {
		i, ok := at[b.Source]
		if !ok {
			i = len(out)
			at[b.Source] = i
			out = append(out, Mention{Path: b.Source, Title: titleOf(b.Source)})
		}
		parts := b.Context
		if len(parts) == 0 {
			parts = []api.SnippetPart{{Text: b.Link.Target, Match: true}}
		}
		out[i].Lines = append(out[i].Lines, MentionLine{Line: int(b.Link.Line), Parts: parts})
	}
	return out
}

// Backlinks is the Backlinks pane: linked and unlinked mentions of the
// active file.
type Backlinks struct {
	Env      *Env
	path     string
	linked   []Mention
	unlinked []Mention
	loaded   bool
	stale    int
}

// Refresh reads the pane again for the active file.
func (b *Backlinks) Refresh() {
	p := b.Env.Active()
	b.stale++
	gen := b.stale
	if p == "" {
		b.path, b.linked, b.unlinked, b.loaded = "", nil, nil, true
		return
	}
	var linked, unlinked []Mention
	vault := b.Env.Vault
	b.Env.call(func(ctx context.Context, a API) error {
		back, err := a.Backlinks(ctx, &api.NoteBacklinksParams{VaultID: vault, Path: p})
		if err != nil {
			return err
		}
		linked = LinkedGroups(back)
		has := map[string]bool{}
		for _, l := range linked {
			has[l.Path] = true
		}
		terms := []string{titleOf(p)}
		if strings.HasSuffix(strings.ToLower(p), ".md") {
			n, err := a.Read(ctx, &api.NoteReadParams{VaultID: vault, Path: p})
			if err != nil {
				return err
			}
			terms = append(terms, n.Aliases...)
		}
		limit := int64(100)
		seen := map[string]bool{}
		for _, term := range terms {
			if strings.TrimSpace(term) == "" {
				continue
			}
			hits, err := a.Search(ctx, &api.NoteSearchParams{VaultID: vault, Query: `"` + strings.ReplaceAll(term, `"`, "") + `"`, Limit: &limit})
			if err != nil {
				return err
			}
			for _, h := range hits {
				if h.Path != p && !has[h.Path] && !seen[h.Path] {
					seen[h.Path] = true
					unlinked = append(unlinked, Mention{Path: h.Path, Title: h.Title, Lines: []MentionLine{{Parts: h.Snippet}}})
				}
			}
		}
		return nil
	}, func(err error) {
		if gen != b.stale {
			return
		}
		b.Env.report(err)
		b.path, b.linked, b.unlinked, b.loaded = p, linked, unlinked, true
	})
}

// Count is how many notes link to the active file, for the status bar.
func (b *Backlinks) Count() int { return len(b.linked) }

func (b *Backlinks) group(c *ui.Context, title string, ms []Mention) {
	frostui.Strong(c, title+" "+strconv.Itoa(len(ms)))
	for _, m := range ms {
		ui.Column(c.Key(title + m.Path)).AlignItems(ui.Stretch).Children(func() {
			r := row(c, m.Title, false)
			if r.Clicked() {
				b.Env.Actions.Open(m.Path, b.Env.where(c))
			}
			r.Children(func() { ui.Text(c, m.Title).SingleLine() })
			for i, l := range m.Lines {
				line := row(c.Key(i), "Mention in "+m.Title, false)
				if line.Clicked() {
					if l.Line > 0 {
						b.Env.Actions.OpenAtLine(m.Path, l.Line, b.Env.where(c))
					} else {
						b.Env.Actions.Open(m.Path, b.Env.where(c))
					}
				}
				line.Children(func() { snippet(c, l.Parts) })
			}
		})
	}
}

// View draws the pane.
func (b *Backlinks) View(c *ui.Context) ui.Element {
	return ui.Scroll(c).Fill().Gap(frostui.SpaceS).Padding(frostui.SpaceS).Children(func() {
		if b.Env.Active() == "" {
			frostui.Muted(c, "No file is open.")
			return
		}
		b.group(c, "Linked mentions", b.linked)
		b.group(c, "Unlinked mentions", b.unlinked)
	})
}

// Outgoing is the Outgoing links pane.
type Outgoing struct {
	Env   *Env
	links []api.Link
	stale int
}

// Refresh reads the active note's links again.
func (o *Outgoing) Refresh() {
	p := o.Env.Active()
	o.stale++
	gen := o.stale
	if p == "" || !strings.HasSuffix(strings.ToLower(p), ".md") {
		o.links = nil
		return
	}
	var links []api.Link
	o.Env.call(func(ctx context.Context, a API) (err error) {
		links, err = a.Links(ctx, &api.NoteLinksParams{VaultID: o.Env.Vault, Path: p})
		return err
	}, func(err error) {
		if gen == o.stale {
			o.Env.report(err)
			o.links = links
		}
	})
}

// View draws the pane; an unresolved link is dimmed, and clicking it
// creates the note.
func (o *Outgoing) View(c *ui.Context) ui.Element {
	t := c.Theme()
	return ui.Scroll(c).Fill().Gap(frostui.SpaceXS).Padding(frostui.SpaceS).Children(func() {
		if len(o.links) == 0 {
			frostui.Muted(c, "No outgoing links.")
			return
		}
		for i, l := range o.links {
			label := l.Target
			if label == "" {
				label = strings.TrimPrefix(l.Subpath, "#")
			}
			r := row(c.Key(i), label, false)
			if r.Clicked() {
				if l.Resolved != nil {
					o.Env.Actions.Open(*l.Resolved, o.Env.where(c))
				} else {
					o.Env.Actions.OpenOrCreate(l.Target, o.Env.where(c))
				}
			}
			r.Children(func() {
				txt := ui.Text(c, label).SingleLine()
				if l.Resolved == nil {
					txt.TextColor(t.TextMuted)
				}
			})
		}
	})
}

// Tags is the Tags pane: nested tags with counts; a click searches.
type Tags struct {
	Env        *Env
	ByCount    bool
	state      ui.OutlineState[*TagNode]
	chosen     int
	roots      []*TagNode
	counts     []api.TagCount
	generation uint64
}

// Refresh reads the tags again.
func (p *Tags) Refresh() {
	p.generation++
	gen := p.generation
	var got []api.TagCount
	p.Env.call(func(ctx context.Context, a API) (err error) {
		got, err = a.Tags(ctx, &api.NoteTagsParams{VaultID: p.Env.Vault})
		return err
	}, func(err error) {
		if gen != p.generation {
			return
		}
		p.Env.report(err)
		if err == nil {
			p.counts = got
			p.rebuild()
		}
	})
}

// rebuild carries expanded tags across refreshes by their canonical names.
func (p *Tags) rebuild() {
	open := map[string]bool{}
	for n := range p.state.Open.All() {
		open[strings.ToLower(n.Tag)] = true
	}
	p.roots = TagTree(p.counts, p.ByCount)
	p.state.Open.Clear()
	var restore func([]*TagNode)
	restore = func(nodes []*TagNode) {
		for _, n := range nodes {
			if open[strings.ToLower(n.Tag)] {
				p.state.Open.Add(n)
			}
			restore(n.Children)
		}
	}
	restore(p.roots)
}

// View draws the pane.
func (p *Tags) View(c *ui.Context) ui.Element {
	return ui.Column(c).Fill().AlignItems(ui.Stretch).Children(func() {
		sortBar(c, &p.ByCount, p.rebuild)
		if len(p.roots) == 0 {
			frostui.Muted(c, "No tags.").Padding(frostui.SpaceS)
			return
		}
		if p.state.List.Selected == nil {
			p.chosen = -1
			p.state.List.Selected = &p.chosen
		}
		list := ui.Outline(c, &p.state, p.roots, func(n *TagNode) []*TagNode {
			return n.Children
		}, func(n *TagNode) {
			r := ui.Row(c).Grow(1).Gap(frostui.SpaceXS).AlignItems(ui.Center).Cursor(ui.CursorPointer).Role(ui.RoleButton).Label("#" + n.Tag)
			if r.Clicked() {
				p.Env.Actions.SearchFor("tag:#" + n.Tag)
			}
			r.Children(func() {
				ui.Text(c, n.Name).SingleLine().Grow(1)
				frostui.Caption(c, strconv.FormatInt(n.Count, 10))
			})
		}).Grow(1)
		if list.Submitted() && p.chosen >= 0 {
			if n, ok := p.state.List.Key(p.chosen).(*TagNode); ok {
				p.Env.Actions.SearchFor("tag:#" + n.Tag)
			}
		}
	})
}

func sortBar(c *ui.Context, byCount *bool, changed func()) {
	ui.Row(c).Padding(frostui.SpaceXS, frostui.SpaceS).AlignItems(ui.Center).Children(func() {
		ui.Spacer(c)
		label := "Sort by count"
		if *byCount {
			label = "Sort by name"
		}
		if frostui.IconButton(c, "view-sort-ascending", label).Clicked() {
			*byCount = !*byCount
			changed()
		}
	})
}

// Properties is the All properties pane.
type Properties struct {
	Env        *Env
	ByCount    bool
	list       ui.ListState
	counts     []api.PropertyCount
	generation uint64
}

// Refresh reads the properties again.
func (p *Properties) Refresh() {
	p.generation++
	gen := p.generation
	var got []api.PropertyCount
	p.Env.call(func(ctx context.Context, a API) (err error) {
		got, err = a.Properties(ctx, &api.NotePropertiesParams{VaultID: p.Env.Vault})
		return err
	}, func(err error) {
		if gen != p.generation {
			return
		}
		p.Env.report(err)
		if err == nil {
			p.counts = got
			p.sort()
		}
	})
}

func (p *Properties) sort() {
	slices.SortStableFunc(p.counts, func(a, b api.PropertyCount) int {
		if p.ByCount && a.Count != b.Count {
			return int(b.Count - a.Count)
		}
		return strings.Compare(strings.ToLower(a.Key), strings.ToLower(b.Key))
	})
}

// Types are the vault's property types, by key, for the properties panel.
func (p *Properties) Types() map[string]string {
	out := make(map[string]string, len(p.counts))
	for _, c := range p.counts {
		out[c.Key] = c.Type
	}
	return out
}

// View draws the pane.
func (p *Properties) View(c *ui.Context) ui.Element {
	return ui.Column(c).Fill().AlignItems(ui.Stretch).Children(func() {
		sortBar(c, &p.ByCount, p.sort)
		if len(p.counts) == 0 {
			frostui.Muted(c, "No properties.").Padding(frostui.SpaceS)
			return
		}
		ui.List(c, &p.list, len(p.counts), func(i int) {
			pc := p.counts[i]
			r := row(c, pc.Key, false)
			if r.Clicked() {
				p.Env.Actions.SearchFor("[" + pc.Key + "]")
			}
			r.Children(func() {
				ui.Text(c, pc.Key).SingleLine().Grow(1)
				frostui.Caption(c, strconv.FormatInt(pc.Count, 10))
			})
		}).Grow(1)
	})
}

// Outline is the Outline pane: the active note's headings, nested.
type Outline struct {
	Env *Env
	// CurrentLine is the caret's line, from 1, to mark its heading.
	CurrentLine func() int
	state       ui.OutlineState[*heading]
	roots       []*heading
	text        string
	chosen      int
	parsed      bool
	parses      int // for the tests
}

type heading struct {
	markdown.Heading
	children []*heading
}

// Headings nests a note's headings by level.
func nestHeadings(hs []markdown.Heading) []*heading {
	var roots []*heading
	var stack []*heading
	for _, h := range hs {
		n := &heading{Heading: h}
		for len(stack) > 0 && stack[len(stack)-1].Level >= h.Level {
			stack = stack[:len(stack)-1]
		}
		if len(stack) == 0 {
			roots = append(roots, n)
		} else {
			parent := stack[len(stack)-1]
			parent.children = append(parent.children, n)
		}
		stack = append(stack, n)
	}
	return roots
}

func (o *Outline) refresh() {
	text, ok := o.Env.ActiveText()
	if !ok {
		o.roots, o.text, o.parsed = nil, "", false
		return
	}
	if o.parsed && text == o.text {
		return
	}
	o.text, o.parsed = text, true
	o.parses++
	o.roots = nestHeadings(markdown.Parse([]byte(text)).Headings)
	var openAll func([]*heading)
	openAll = func(hs []*heading) {
		for _, h := range hs {
			if len(h.children) > 0 {
				o.state.Open.Add(h)
			}
			openAll(h.children)
		}
	}
	o.state.Open.Clear()
	openAll(o.roots)
}

func (o *Outline) current(line int) *heading {
	var found *heading
	var walk func([]*heading)
	walk = func(hs []*heading) {
		for _, h := range hs {
			if h.Line <= line {
				found = h
			}
			walk(h.children)
		}
	}
	walk(o.roots)
	return found
}

// View draws the pane.
func (o *Outline) View(c *ui.Context) ui.Element {
	o.refresh()
	t := c.Theme()
	return ui.Column(c).Fill().AlignItems(ui.Stretch).Children(func() {
		if len(o.roots) == 0 {
			frostui.Muted(c, "No headings.").Padding(frostui.SpaceS)
			return
		}
		var cur *heading
		if o.CurrentLine != nil {
			cur = o.current(o.CurrentLine())
		}
		if o.state.List.Selected == nil {
			o.chosen = -1
			o.state.List.Selected = &o.chosen
		}
		list := ui.Outline(c, &o.state, o.roots, func(h *heading) []*heading { return h.children }, func(h *heading) {
			r := ui.Row(c).Grow(1).AlignItems(ui.Center).Cursor(ui.CursorPointer).Role(ui.RoleButton).Label(h.Text)
			if r.Clicked() {
				o.Env.Actions.JumpToLine(h.Line)
			}
			r.Children(func() {
				txt := ui.Text(c, h.Text).SingleLine()
				if h == cur {
					txt.TextColor(t.Accent)
				}
			})
		}).Grow(1)
		if list.Submitted() && o.chosen >= 0 {
			if h, ok := o.state.List.Key(o.chosen).(*heading); ok {
				o.Env.Actions.JumpToLine(h.Line)
			}
		}
	})
}

// Tasks is the Tasks pane: the vault's open tasks, by note.
type Tasks struct {
	Env        *Env
	tasks      []api.TaskEntry
	rows       []taskRow
	list       ui.ListState
	generation uint64
}

// taskRow is a row of the list: a note's title, or one of its tasks.
type taskRow struct {
	title string
	task  *api.TaskEntry
}

func taskRows(tasks []api.TaskEntry) []taskRow {
	var out []taskRow
	last := ""
	for i := range tasks {
		if tasks[i].Path != last {
			last = tasks[i].Path
			out = append(out, taskRow{title: tasks[i].Title})
		}
		out = append(out, taskRow{task: &tasks[i]})
	}
	return out
}

// Refresh reads the tasks again.
func (p *Tasks) Refresh() {
	p.generation++
	gen := p.generation
	var got []api.TaskEntry
	p.Env.call(func(ctx context.Context, a API) (err error) {
		got, err = a.Tasks(ctx, &api.NoteTasksParams{VaultID: p.Env.Vault})
		return err
	}, func(err error) {
		if gen != p.generation {
			return
		}
		p.Env.report(err)
		if err == nil {
			p.tasks = got
			p.rows = taskRows(got)
		}
	})
}

// View draws the pane.
func (p *Tasks) View(c *ui.Context) ui.Element {
	return ui.Column(c).Fill().AlignItems(ui.Stretch).Padding(frostui.SpaceS).Children(func() {
		if len(p.rows) == 0 {
			frostui.Muted(c, "No open tasks.")
			return
		}
		ui.List(c, &p.list, len(p.rows), func(i int) {
			item := p.rows[i]
			if item.task == nil {
				frostui.Strong(c, item.title)
				return
			}
			r := row(c, item.task.Text, false)
			if r.Clicked() {
				p.Env.Actions.OpenAtLine(item.task.Path, int(item.task.Line), p.Env.where(c))
			}
			r.Children(func() { ui.Text(c, item.task.Text).Wrap() })
		}).Grow(1)
	})
}
