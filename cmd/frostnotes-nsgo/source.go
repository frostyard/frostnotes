package main

import (
	"slices"

	"github.com/frostyard/frostnotes/internal/desktop/markup"
	"github.com/frostyard/frostnotes/internal/desktop/notes"
	"github.com/nsgo-dev/nsgo/base/editor"
	"github.com/nsgo-dev/nsgo/core"
	"github.com/nsgo-dev/nsgo/ui"
	uieditor "github.com/nsgo-dev/nsgo/ui/editor"
)

// readableWidth is a note's widest text under Readable line length, the
// MyGo client's (internal/desktop/editor.ReadableWidth). The kit has no
// token for a reading width: it is a measure of the text, not a look.
const readableWidth core.Points = 700

// sourcePrefs are the editor's preferences that source mode applies.
type sourcePrefs struct {
	LineNumbers, IndentGuides, FoldHeadings, FoldIndent  bool
	AutoPairBrackets, IndentWithTabs, ReadableLineLength bool
	TabSize                                              int
}

// editorDefaults are Obsidian's defaults, the MyGo client's
// (internal/desktop/prefs.Default, which imports what the web build cannot
// build; TestSourceTakesTheMyGoDefaults holds them equal). Settings for
// them come with the preferences (plan 0011, Phase 7).
func editorDefaults() sourcePrefs {
	return sourcePrefs{
		IndentGuides: true, FoldHeadings: true, FoldIndent: true, AutoPairBrackets: true,
		IndentWithTabs: true, ReadableLineLength: true, TabSize: 4,
	}
}

// sourceEditor is source mode: NSGo's code editor on the open note's
// document (internal/desktop/notes).
//
// The user's typing reaches the document as the user's edits, through
// Document.Local, and stays in the editor's undo history. Any other change
// of the document's text (a merge, another writer's change, a reload, the
// note read after it opened) never goes into the editor as a new value,
// which NSGo's editor would record as a step of its undo: the editor
// starts again on the new text, with its carets, folds and scroll mapped
// through the change. The user's undo history ends there, and Ctrl+Z never
// takes back another writer's change (ADR-0022; ADR-0049, decision 3).
type sourceEditor struct {
	d   *notes.Document
	ref *editor.Ref
	// last is the editor's text as the document last had it: typing since
	// is the user's.
	last string
	// text is the editor's text, which the folds follow.
	text *core.Signal[string]
	// starts counts the editor's starts; restore is what the next one puts
	// back.
	starts  *core.Signal[int]
	restore *restoreState
	focused bool
}

// restoreState is what an editor that starts again keeps of the one
// before: its selections, folds and scroll, mapped to the new text, and
// whether it had the keyboard.
type restoreState struct {
	sels   []editor.Selection
	folds  []editor.FoldRange
	scroll core.Point
	// top is the first line in view, mapped, and topY where its box was.
	top   editor.Position
	topY  core.Points
	hasY  bool
	focus bool
}

// sourceEditor is the source mode of document d.
func (w *workspace) sourceEditor(cx *core.Scope, d *notes.Document) core.Node {
	s := &sourceEditor{d: d, ref: &editor.Ref{}, last: d.Text()}
	s.text = core.NewSignal(cx, s.last)
	s.starts = core.NewSignal(cx, 0)
	writable := core.NewMemo(cx, func() bool {
		w.revision.Get()
		return d.Loaded() && d.Status() != notes.StatusMissing && d.ReadOnly() == ""
	})
	wasWritable := writable.GetUntracked()
	// What changed the document's text that the editor did not type, and
	// a change of whether the note can be edited, start the editor again.
	cx.Effect(func() {
		w.revision.Get()
		can := writable.Get()
		cx.Untrack(func() {
			if d.Text() != s.last || can != wasWritable {
				wasWritable = can
				s.restart()
			}
		})
	})
	folds := core.NewMemoFunc(cx, func() []editor.FoldRange {
		var out []editor.FoldRange
		for _, f := range markup.Folds(s.text.Get()) {
			out = append(out, editor.FoldRange{StartLine: f.Start, EndLine: f.End})
		}
		return out
	}, slices.Equal)
	p := editorDefaults()
	return core.View{Style: ui.FillRest.Merge(core.Style{AlignItems: core.AlignCenter}), Children: []core.Node{core.Dynamic(func(cx *core.Scope) core.Node {
		s.starts.Get()
		focus := s.restore != nil && s.restore.focus
		return uieditor.Editor{
			Label: "Markdown source", Ref: s.ref, DefaultValue: s.last, AutoFocus: focus,
			ReadOnly:    !core.Untracked(cx, writable.Get),
			Highlighter: func(string) editor.Highlighter { return obsidian{} },
			// Obsidian's defaults, the MyGo client's (internal/desktop/prefs):
			// no line numbers, indentation guides, folds by headings and
			// indentation, tabs of 4.
			NoLineNumbers: !p.LineNumbers, NoIndentGuides: !p.IndentGuides,
			NoFolding: !p.FoldHeadings && !p.FoldIndent, FoldRanges: folds.Get,
			TabSize: p.TabSize, HardTabs: p.IndentWithTabs, NoAutoClose: !p.AutoPairBrackets,
			SoftWrap: true,
			Style:    s.style(p.ReadableLineLength),
			OnChange: s.typed,
			OnLayout: func(core.Bounds) { s.restored() },
			OnFocus:  func() { s.focused = true },
			OnBlur:   func() { s.focused = false },
		}
	})}}
}

// style is the editor's box: the pane's height, and at most readableWidth
// wide, in the middle, with Readable line length.
func (s *sourceEditor) style(readable bool) core.Style {
	st := ui.FillRest.Merge(core.Style{Width: core.Pct(100)})
	if readable {
		st.MaxWidth = core.Pt(readableWidth)
	}
	return st
}

// typed takes the user's edit in the editor to the document.
func (s *sourceEditor) typed(text string) {
	if text == s.last {
		return
	}
	before, now := s.last, s.d.Text()
	cs := notes.Diff(before, text)
	// The editor holds text from here on, whatever the document does with
	// it: a change of the document's that differs starts the editor again.
	s.last = text
	s.text.Set(text)
	if now == before {
		s.d.Local(cs)
	} else if mapped, err := cs.Map(notes.Diff(before, now), false); err == nil {
		// Another writer's change reached the document before the editor
		// started again on it: the typing goes over that change.
		s.d.Local(mapped)
	}
	if s.d.Text() != s.last {
		// The document refused the edit, or holds another writer's change
		// too: the editor starts again on the document's text.
		s.restart()
	}
}

// restart starts the editor again on the document's text, keeping what
// the editor before showed, mapped through the change from s.last.
func (s *sourceEditor) restart() {
	from, to := s.last, s.d.Text()
	cs := notes.Diff(from, to)
	pos := func(p editor.Position) editor.Position {
		line, col := markup.LineCol(to, cs.MapPos(markup.Offset(from, p.Line, p.Column), -1))
		return editor.Position{Line: line, Column: col}
	}
	// What the editor shows, or, when it started again and has not been
	// laid out since, what it is still to show.
	was := s.restore
	if was == nil {
		was = &restoreState{focus: s.focused, scroll: s.ref.ScrollOffset(), sels: s.ref.Selections(), folds: s.ref.Folds()}
		top := s.ref.VisibleRange().Start
		if y, ok := s.rowY(top); ok {
			was.top, was.topY, was.hasY = top, y, true
		}
	}
	r := &restoreState{focus: was.focus, scroll: was.scroll, top: pos(was.top), topY: was.topY, hasY: was.hasY}
	for _, sel := range was.sels {
		r.sels = append(r.sels, editor.Selection{Anchor: pos(sel.Anchor), Head: pos(sel.Head)})
	}
	for _, f := range was.folds {
		start, end := pos(editor.Position{Line: f.StartLine}), pos(editor.Position{Line: f.EndLine})
		if end.Line > start.Line {
			r.folds = append(r.folds, editor.FoldRange{StartLine: start.Line, EndLine: end.Line})
		}
	}
	s.restore = r
	s.last = to
	s.text.Set(to)
	s.starts.Set(s.starts.GetUntracked() + 1)
}

// restored puts back what the editor before showed, once the new one is
// laid out.
func (s *sourceEditor) restored() {
	r := s.restore
	if r == nil {
		return
	}
	s.restore = nil
	s.ref.SetFolds(r.folds)
	if len(r.sels) > 0 {
		s.ref.SetSelections(r.sels)
	}
	s.ref.SetScrollOffset(r.scroll)
	if !r.hasY {
		return
	}
	// The first line in view stays where it was, whatever came or went
	// above it.
	if y, ok := s.rowY(r.top); ok {
		s.ref.SetScrollOffset(core.Point{X: r.scroll.X, Y: r.scroll.Y + y - r.topY})
	}
}

// rowY is where the row of p is in the editor's box, its scrolling
// applied. An empty range has no box, so it measures p to the next line's
// start.
func (s *sourceEditor) rowY(p editor.Position) (core.Points, bool) {
	b := s.ref.RangeBounds(editor.Range{Start: p, End: editor.Position{Line: p.Line + 1}})
	if len(b) == 0 {
		return 0, false
	}
	return b[0].Bounds.Origin.Y, true
}

// obsidian is the app's highlighting of a note: Obsidian's Markdown, as
// markup.Highlight reads it, in the kinds of NSGo's palette. NSGo v0.3.0
// names no bold or italic kind for an app's tokens (its KindNamed), so
// emphasis and strong text keep the text's color and their marks are
// dimmed as Obsidian's source mode dims them.
type obsidian struct{}

// Highlight implements editor.Highlighter.
func (obsidian) Highlight(lines []string, from, to int) [][]editor.Token {
	runs := markup.Highlight(lines, from, to)
	out := make([][]editor.Token, len(runs))
	for i, line := range runs {
		for _, t := range line {
			if k := tokenKind(t.Kind); k != "" {
				out[i] = append(out[i], editor.Token{Start: t.Start, End: t.End, Kind: string(k)})
			}
		}
	}
	return out
}

// tokenKind is the palette's kind of markup's, "" for the text's color.
func tokenKind(k markup.Kind) editor.TokenKind {
	switch k {
	case markup.KindHeading:
		return "title"
	case markup.KindLink, markup.KindURL, markup.KindWikilink, markup.KindEmbed, markup.KindFootnote:
		return "built"
	case markup.KindTag, markup.KindCallout:
		return "keyword"
	case markup.KindCode, markup.KindValue:
		return "string"
	case markup.KindMath:
		return "number"
	case markup.KindProperty, markup.KindHighlight:
		return "attr"
	case markup.KindStrike:
		return "deletion"
	case markup.KindFrontmatter, markup.KindMarks, markup.KindComment, markup.KindBlockID, markup.KindQuote, markup.KindList, markup.KindRule:
		return "comment"
	}
	return ""
}
