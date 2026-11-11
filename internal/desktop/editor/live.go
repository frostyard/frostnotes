package editor

import (
	"slices"
	"strings"

	"github.com/egoist/mygo/ui"
	code "github.com/egoist/mygo/ui/editor"

	"github.com/frostyard/frostnotes/internal/desktop/livepreview"
	"github.com/frostyard/frostnotes/internal/desktop/notes"
)

// ruleWidth is how many characters a horizontal rule draws.
const ruleWidth = 40

// lineSpan is the lines a selection touches, from first through last.
type lineSpan struct{ first, last int }

// liveState is what the editor showed last, so that a frame with the same
// text and selected lines does no work.
type liveState struct {
	scanner livepreview.Scanner
	on      bool
	version uint64
	lines   []lineSpan
	front   bool
	muted   ui.Color
}

// showLive sets what live preview shows (ADR-0039): the marks of the lines
// no selection touches hide, and bullets, tasks, quotes and rules stand in
// for theirs. In source mode it shows the text as it is.
func (e *NoteEditor) showLive(c *ui.Context) {
	l := &e.live
	if !e.Prefs.Live {
		if l.on {
			l.on = false
			e.State.SetReplacements(nil)
		}
		return
	}
	buf := e.b.Buffer()
	lines := e.selectedLines(buf)
	muted := c.Theme().TextMuted
	version := buf.Version()
	if l.on && l.version == version && l.front == e.HideFront && l.muted == muted && slices.Equal(l.lines, lines) {
		return
	}
	l.on, l.version, l.front, l.muted, l.lines = true, version, e.HideFront, muted, lines
	revealed := func(line int) bool {
		return slices.ContainsFunc(lines, func(s lineSpan) bool { return s.first <= line && line <= s.last })
	}
	pieces := l.scanner.Pieces(buf.String(), revealed, e.HideFront)
	out := make([]code.Replacement, len(pieces))
	for i, p := range pieces {
		out[i] = e.replacement(p, muted)
	}
	e.State.SetReplacements(out)
}

func (e *NoteEditor) selectedLines(buf *code.Buffer) []lineSpan {
	sels := e.State.Selections()
	out := make([]lineSpan, len(sels))
	for i, s := range sels {
		out[i] = lineSpan{buf.Pos(s.Start()).Line, buf.Pos(s.End()).Line}
	}
	return out
}

func (e *NoteEditor) replacement(p livepreview.Piece, muted ui.Color) code.Replacement {
	r := code.Replacement{Start: p.Start, End: p.End}
	switch p.Kind {
	case livepreview.Bullet:
		r.Text, r.Color = "•", muted
	case livepreview.Quote:
		r.Text, r.Color = "▏ ", muted
	case livepreview.Rule:
		r.Text, r.Color = strings.Repeat("─", ruleWidth), muted
	case livepreview.Task:
		return e.taskBox(p)
	}
	return r
}

// taskBox draws a check box in place of a task's "- [ ]"; a click toggles
// its one character, as the user's edit.
func (e *NoteEditor) taskBox(p livepreview.Piece) code.Replacement {
	label, mark, next := "Not done", "☐", "x"
	if p.Done {
		label, mark, next = "Done", "☑", " "
	}
	return code.Replacement{Start: p.Start, End: p.End, Label: label, Interactive: true,
		Content: func(c *ui.Context, el ui.Element, at code.Replacement) {
			el.Children(func() {
				b := ui.ButtonBase(c).Fill().Center().KeepFocus().Cursor(ui.CursorPointer).Label(label)
				b.Children(func() { ui.Text(c, mark) })
				if b.Clicked() {
					e.setTask(at, next)
				}
			})
		}}
}

// setTask writes a task's mark, the character before its closing bracket.
func (e *NoteEditor) setTask(at code.Replacement, mark string) {
	e.Run(func(text string, _ []Sel) Edit {
		cs, err := notes.ChangesOf(len(text), notes.Change{From: at.End - 2, To: at.End - 1, Insert: mark})
		if err != nil {
			return Edit{}
		}
		return Edit{Changes: cs}
	})
}
