package editor

import (
	"regexp"
	"strings"
	"time"

	"github.com/egoist/mygo/ui"
	code "github.com/egoist/mygo/ui/editor"
)

// Prefs are the editor's preferences (docs/specs/ui.md#settings-batch-2).
type Prefs struct {
	SmartLists       bool
	AutoPairMarkdown bool
	AutoPairBrackets bool
	LineNumbers      bool
	FoldHeadings     bool
	FoldIndent       bool
	IndentWithTabs   bool
	IndentGuides     bool
	TabSize          int
	FontSize         int  // 0 is the editor's own
	Live             bool // live preview, else source mode
}

// DefaultPrefs are the web app's defaults (app/src/data/prefs.ts).
func DefaultPrefs() Prefs {
	return Prefs{SmartLists: true, AutoPairMarkdown: true, AutoPairBrackets: true,
		FoldHeadings: true, FoldIndent: true, IndentWithTabs: true, IndentGuides: true, TabSize: 4, Live: true}
}

// Where a link opens.
type Where int

// The places a link opens.
const (
	WhereHere Where = iota
	WhereTab
	WhereRight
	WhereWindow
)

// Target is a note [[ completes to.
type Target struct {
	Insert string // the link text, as the vault's link format writes it
	Label  string // what the list shows
	Detail string // its folder, dimmed
}

// Sources is what the editor completes from: the vault's notes and tags,
// which the session keeps.
type Sources interface {
	// LinkTargets lists the notes [[ completes to for query, best first.
	LinkTargets(query string, limit int) []Target
	// Tags lists the vault's tags, by how many notes have them.
	Tags() []string
	// Subpaths lists the headings, or the block IDs, of the note target
	// leads to ("" is the note being edited), through done on the UI
	// thread; done is not called when it leads to no note.
	Subpaths(target string, blocks bool, done func([]Target))
}

// NoteEditor is one tab's editor of a note: MyGo's code editor on the
// note's shared buffer, with Obsidian's keys (docs/specs/ui.md#keyboard-map).
type NoteEditor struct {
	b       *Binding
	State   *code.State
	Prefs   Prefs
	Sources Sources
	// Follow opens a link: Alt + Enter, Ctrl + click and their variants.
	Follow func(l Link, where Where)
	// FollowTag searches for a tag, the same way.
	FollowTag func(tag string)
	// Preview builds a link's page preview in a hover; nil for none.
	Preview func(c *ui.Context, l Link)
	// Uploads and Clipboard let files be pasted and dropped; nil for
	// none. OnError shows what went wrong, Now is the clock.
	Uploads   Uploader
	Clipboard Clipboard
	OnError   func(error)
	Now       func() time.Time
	// HideFront hides the frontmatter in live preview, while the properties
	// panel stands in for it.
	HideFront bool

	completeNext bool
	live         liveState
}

// removed are the editor's keys Obsidian takes out of CodeMirror's
// (moving and duplicating lines), and keys the window owns (Ctrl + E,
// the reading view), which go on to the window's shortcuts.
var removed = []code.Binding{
	{Mods: ui.Alt, Key: ui.KeyUp, Command: code.NoCommand},
	{Mods: ui.Alt, Key: ui.KeyDown, Command: code.NoCommand},
	{Mods: ui.Alt | ui.Shift, Key: ui.KeyUp, Command: code.NoCommand},
	{Mods: ui.Alt | ui.Shift, Key: ui.KeyDown, Command: code.NoCommand},
	{Mods: ui.Ctrl, Key: ui.KeyE, Command: code.NoCommand},
}

// NewNoteEditor makes a tab's editor of a bound note.
func NewNoteEditor(b *Binding) *NoteEditor {
	e := &NoteEditor{b: b, Prefs: DefaultPrefs()}
	e.State = &code.State{
		Buffer:          b.Buffer(),
		SoftWrap:        true,
		Label:           "Note",
		NoScrollPastEnd: false,
		Bindings:        removed,
	}
	e.State.OnInput = e.input
	e.State.OnCompletion = e.complete
	e.State.OnDefinition = func(offset int) { e.followAt(offset, WhereTab) }
	e.State.OnHover = e.hover
	return e
}

// Binding is the note's binding.
func (e *NoteEditor) Binding() *Binding { return e.b }

// View draws the editor.
func (e *NoteEditor) View(c *ui.Context) ui.Element {
	e.b.Sync()
	d := e.b.Document()
	st := e.State
	st.ReadOnly = !d.Loaded() || d.ReadOnly() != ""
	st.LineNumbers = code.LineNumbersOff
	if e.Prefs.LineNumbers {
		st.LineNumbers = code.LineNumbersOn
	}
	st.NoAutoClose = !e.Prefs.AutoPairBrackets
	st.NoFolding = !e.Prefs.FoldHeadings && !e.Prefs.FoldIndent
	st.TabSize = max(1, e.Prefs.TabSize)
	st.NoIndentGuides = !e.Prefs.IndentGuides
	if e.Prefs.FontSize > 0 {
		st.Font = ui.Font{Family: "monospace", Size: float32(e.Prefs.FontSize)}
	}
	st.Indent = code.IndentSpaces
	if e.Prefs.IndentWithTabs {
		st.Indent = code.IndentTabs
	}
	if e.completeNext {
		e.completeNext = false
		e.openCompletions(st.Cursor())
	}
	e.showLive(c)
	el := code.View(c, st)
	e.acceptDrops(el)
	return el
}

func (e *NoteEditor) sels() []Sel {
	ss := e.State.Selections()
	out := make([]Sel, len(ss))
	for i, s := range ss {
		out[i] = Sel{s.Anchor, s.Head}
	}
	return out
}

func (e *NoteEditor) setSels(sels []Sel) {
	out := make([]code.Selection, len(sels))
	for i, s := range sels {
		out[i] = code.Selection{Anchor: s.Anchor, Head: s.Head}
	}
	if len(out) > 0 {
		e.State.SetSelections(out...)
	}
}

// Run applies a command's edit as the user's: one step of undo, saved as
// typing is.
func (e *NoteEditor) Run(cmd func(text string, sels []Sel) Edit) bool {
	e.b.Sync()
	d := e.b.Document()
	if e.State.ReadOnly || !d.Loaded() || d.ReadOnly() != "" {
		return false
	}
	ed := cmd(d.Text(), e.sels())
	if ed.Changes.Empty() && ed.Sels == nil {
		return false
	}
	d.Edit(ed.Changes)
	e.setSels(ed.Sels)
	return true
}

// Undo takes back the user's newest step, never another writer's change,
// and puts the selections where they were.
func (e *NoteEditor) Undo() bool { return e.step(code.Undo) }

// Redo makes the newest undone step again.
func (e *NoteEditor) Redo() bool { return e.step(code.Redo) }

func (e *NoteEditor) step(cmd code.Command) bool {
	e.b.Sync()
	done := e.State.Run(cmd)
	e.b.Sync()
	return done
}

func (e *NoteEditor) input(ev ui.InputEvent) bool {
	switch ev.Kind {
	case ui.InputCommand:
		if ev.Text == "paste" {
			return e.paste()
		}
	case ui.InputKeyDown:
		return e.key(ev.Mods, ev.Key)
	case ui.InputText:
		if e.Prefs.AutoPairMarkdown {
			if e.Run(func(text string, sels []Sel) Edit {
				ed, _ := WrapInMark(text, sels, ev.Text)
				return ed
			}) {
				return true
			}
		}
		if strings.HasSuffix(ev.Text, "[") || strings.HasSuffix(ev.Text, "#") || strings.HasSuffix(ev.Text, "^") {
			e.completeNext = true // after the editor has typed it
		}
	}
	return false
}

// key runs the editor's own keys; it reports whether it took the key.
func (e *NoteEditor) key(mods ui.Modifiers, key ui.Key) bool {
	cmd, alt, shift := ui.Cmd, ui.Alt, ui.Shift
	run := func(f func(string, []Sel) Edit) bool { e.Run(f); return true }
	switch {
	case mods == cmd && key == ui.KeyV, mods == shift && key == ui.KeyInsert:
		return e.paste()
	case mods == cmd && key == ui.KeyB:
		return run(func(t string, s []Sel) Edit { return ToggleMark(t, s, "**") })
	case mods == cmd && key == ui.KeyI:
		return run(func(t string, s []Sel) Edit { return ToggleMark(t, s, "*") })
	case mods == cmd && key == ui.KeyK:
		return run(InsertLink)
	case mods == cmd && key == ui.KeyL:
		return run(ToggleChecklist)
	case mods == cmd && key == ui.KeySlash:
		return run(ToggleComment)
	case mods == cmd && key == ui.KeyD:
		return run(DeleteParagraph)
	case mods == cmd && key == ui.KeyS:
		e.b.Sync()
		e.b.Document().Save()
		return true
	case mods == alt && key == ui.KeyEnter:
		return e.followAt(e.State.Cursor(), WhereHere)
	case mods == cmd && key == ui.KeyEnter:
		return e.followAt(e.State.Cursor(), WhereTab)
	case mods == cmd|alt && key == ui.KeyEnter:
		return e.followAt(e.State.Cursor(), WhereRight)
	case mods == cmd|alt|shift && key == ui.KeyEnter:
		return e.followAt(e.State.Cursor(), WhereWindow)
	case mods == 0 && key == ui.KeyEnter && e.Prefs.SmartLists:
		return e.Run(func(t string, s []Sel) Edit {
			ed, _ := ContinueMarkup(t, s)
			return ed
		})
	case mods == 0 && key == ui.KeyBackspace && e.Prefs.SmartLists:
		return e.Run(func(t string, s []Sel) Edit {
			ed, _ := DeleteMarkupBackward(t, s)
			return ed
		})
	}
	return false
}

// followAt opens the link or tag at offset; it reports whether there was
// one.
func (e *NoteEditor) followAt(offset int, where Where) bool {
	e.b.Sync()
	text := e.b.Document().Text()
	if l, ok := LinkAt(text, offset); ok && e.Follow != nil {
		e.Follow(l, where)
		return true
	}
	if tag, ok := TagAt(text, offset); ok && e.FollowTag != nil {
		e.FollowTag(tag)
		return true
	}
	return false
}

func (e *NoteEditor) hover(r code.HoverRequest) {
	if e.Preview == nil {
		return
	}
	l, ok := LinkAt(e.b.Buffer().String(), r.Offset)
	if !ok || l.URL != "" {
		return
	}
	e.State.ShowHover(l.From, l.To, "", func(c *ui.Context) { e.Preview(c, l) })
}

var (
	openLinkRe = regexp.MustCompile(`!?\[\[([^\[\]|#\n]*)$`)
	openSubRe  = regexp.MustCompile(`!?\[\[([^\[\]|#\n]*)#(\^?)([^\[\]|\n]*)$`)
	openTagRe  = regexp.MustCompile(`(?:^|\s)#([\p{L}\p{N}_/-]*)$`)
)

func (e *NoteEditor) complete(r code.CompletionRequest) { e.openCompletions(r.Offset) }

// openCompletions lists the notes after [[ and the tags after #, at the
// caret.
func (e *NoteEditor) openCompletions(at int) {
	if e.Sources == nil {
		return
	}
	text := e.b.Buffer().String()
	at = max(0, min(at, len(text)))
	before := text[strings.LastIndexByte(text[:at], '\n')+1 : at]
	if m := openSubRe.FindStringSubmatch(before); m != nil {
		e.completeSubpath(at, m[1], m[2] != "", m[3], strings.HasPrefix(text[at:], "]]"))
		return
	}
	if m := openLinkRe.FindStringSubmatchIndex(before); m != nil {
		query := before[m[2]:m[3]]
		start := at - len(query)
		closed := strings.HasPrefix(text[at:], "]]")
		var items []code.Completion
		for _, t := range e.Sources.LinkTargets(query, 50) {
			insert := t.Insert
			if !closed {
				insert += "]]"
			}
			items = append(items, code.Completion{Label: t.Label, Detail: t.Detail, Insert: insert,
				FilterText: t.Label, Kind: code.CompletionModule})
		}
		e.State.ShowCompletions(start, items)
		return
	}
	if m := openTagRe.FindStringSubmatchIndex(before); m != nil {
		query := strings.ToLower(before[m[2]:m[3]])
		start := at - (m[3] - m[2])
		var items []code.Completion
		for _, tag := range e.Sources.Tags() {
			if query == "" || fuzzyMatch(strings.ToLower(tag), query) {
				items = append(items, code.Completion{Label: tag, Insert: tag, Kind: code.CompletionKeyword})
			}
			if len(items) == 50 {
				break
			}
		}
		e.State.ShowCompletions(start, items)
	}
}

// completeSubpath lists a note's headings or block IDs after [[target#.
func (e *NoteEditor) completeSubpath(at int, target string, blocks bool, query string, closed bool) {
	version := e.b.Buffer().Version()
	start := at - len(query)
	e.Sources.Subpaths(target, blocks, func(found []Target) {
		if e.b.Buffer().Version() != version {
			return // the text moved on; the next key asks again
		}
		var items []code.Completion
		for _, t := range found {
			insert := t.Insert
			if !closed {
				insert += "]]"
			}
			items = append(items, code.Completion{Label: t.Label, Detail: t.Detail, Insert: insert, Kind: code.CompletionField})
		}
		e.State.ShowCompletions(start, items)
	})
}

// fuzzyMatch reports whether every rune of query appears in s in order.
func fuzzyMatch(s, query string) bool {
	for _, r := range query {
		i := strings.IndexRune(s, r)
		if i < 0 {
			return false
		}
		s = s[i+len(string(r)):]
	}
	return true
}
