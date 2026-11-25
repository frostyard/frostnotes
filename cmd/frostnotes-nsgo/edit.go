package main

import (
	"slices"

	"github.com/frostyard/frostnotes/internal/desktop/markup"
	"github.com/frostyard/frostnotes/internal/desktop/notes"
	"github.com/nsgo-dev/nsgo/base/editor"
	"github.com/nsgo-dev/nsgo/core"
)

// Source mode's own keys and commands: Obsidian's formatting commands
// (markup.Commands), smart lists, marks typed around a selection, find in
// the note and following a link, as the MyGo client has them
// (internal/desktop/editor/noteeditor.go and window/actions.go).
//
// An app's binding of a key that is made once the editor has the keyboard
// wins over the editor's own, which it makes as it takes the keyboard
// (TestAppKeysAroundTheEditor). So these keys are bound then, and Enter
// and Backspace fall back to the editor's own commands when they have
// nothing of a list to do. Inside the editor mod+K inserts a Markdown link,
// as in Obsidian and the MyGo client; elsewhere it stays the palette, and
// mod+O is the palette everywhere. mod+F and mod+H find in the note; outside
// the editor mod+F stays the notes tree's filter. NSGo's mod+D (select the
// next match) and mod+L (select the line) give way to Obsidian's Delete
// paragraph and Toggle checkbox status; its other keys of several
// selections stay: mod+Alt+Up and Down add a caret, mod+Shift+L selects
// every match and Alt+click adds a caret.

// bindKeys binds source mode's keys over the editor's, while it has the
// keyboard.
func (s *sourceEditor) bindKeys(cx *core.Scope) {
	bind := func(key string, repeat bool, fn func()) {
		cx.OnKeyWith(key, core.KeyOptions{When: core.KeyAlways, Repeat: repeat}, fn)
	}
	for _, c := range markup.Commands() {
		if c.Keys != "" {
			bind(c.Keys, false, func() { s.apply(c.Edit) })
		}
	}
	bind("enter", true, s.enter)
	bind("backspace", true, s.backspace)
	bind("alt+enter", false, s.followCaret)
	bind("mod+f", false, func() { s.openFind(false) })
	bind("mod+h", false, func() { s.openFind(true) })
}

// enter continues a list, a task or a quote (smart lists), and else
// breaks the line as the editor does.
func (s *sourceEditor) enter() {
	if s.writable() {
		if e, ok := markup.ContinueMarkup(s.last, s.markupSels()); ok {
			s.applyEdit(e)
			return
		}
	}
	s.ref.Run(editor.Newline)
}

// backspace takes an empty item's marker away (smart lists), and else
// deletes as the editor does.
func (s *sourceEditor) backspace() {
	if s.writable() {
		if e, ok := markup.DeleteMarkupBackward(s.last, s.markupSels()); ok {
			s.applyEdit(e)
			return
		}
	}
	s.ref.Run(editor.DeleteBackward)
}

// apply runs a formatting command on the editor's text and selections, as
// one step of its undo. It reports whether the command did anything.
func (s *sourceEditor) apply(cmd func(text string, sels []markup.Sel) markup.Edit) bool {
	if !s.writable() {
		return false
	}
	e := cmd(s.last, s.markupSels())
	if e.Changes.Empty() && e.Sels == nil {
		return false
	}
	s.applyEdit(e)
	return true
}

// applyEdit makes e's changes in the editor, as one step of its undo that
// reaches the document as typing does, and then sets e's selections.
func (s *sourceEditor) applyEdit(e markup.Edit) {
	text := s.last
	var edits []editor.Edit
	for _, c := range e.Changes.Changes() {
		edits = append(edits, editor.Edit{Range: editor.Range{Start: position(text, c.From), End: position(text, c.To)}, Text: c.Insert})
	}
	if len(edits) > 0 {
		s.ref.Edit(edits)
	}
	after, err := e.Changes.Apply(text)
	if err != nil || s.last != after || len(e.Sels) == 0 {
		return
	}
	sels := make([]editor.Selection, len(e.Sels))
	for i, sel := range e.Sels {
		sels[i] = editor.Selection{Anchor: position(after, sel.Anchor), Head: position(after, sel.Head)}
	}
	s.ref.SetSelections(sels)
}

// wrapTyped wraps the selections in a Markdown mark typed over them (*, _,
// ~, = or `), as Obsidian's Auto-pair Markdown syntax does: the editor
// replaced them with the mark, and the mark goes around them instead, as
// one step of undo. before is the text before the typing, after the text
// with it.
func (s *sourceEditor) wrapTyped(before, after string) {
	if len(s.sels) == 0 || len(after) >= len(before) || !s.writable() {
		return
	}
	var sels []markup.Sel
	for _, sel := range s.sels {
		if sel.Anchor == sel.Head {
			return
		}
		sels = append(sels, markup.Sel{Anchor: offset(before, sel.Anchor), Head: offset(before, sel.Head)})
	}
	first := sels[0].From()
	for _, sel := range sels {
		first = min(first, sel.From())
	}
	if first >= len(after) {
		return
	}
	mark := after[first : first+1]
	wrap, ok := markup.WrapInMark(before, sels, mark)
	if !ok {
		return
	}
	// The selections replaced with the mark make after, or this was not a
	// mark typed over them.
	var changes []notes.Change
	for _, sel := range sels {
		changes = append(changes, notes.Change{From: sel.From(), To: sel.To(), Insert: mark})
	}
	cs, err := notes.ChangesOf(len(before), changes...)
	if err != nil {
		return
	}
	if typed, err := cs.Apply(before); err != nil || typed != after {
		return
	}
	s.w.cx.Post(func() {
		if s.last != after {
			return
		}
		// Undo takes the typing back, unless it joined typing before it,
		// which redo puts back; then the wrap is a step of its own.
		s.ref.Run(editor.Undo)
		if s.last != before {
			s.ref.Run(editor.Redo)
			if s.last != after {
				return
			}
			s.applyEdit(markup.Edit{Changes: completeWrap(before, after, sels, mark), Sels: wrap.Sels})
			return
		}
		s.applyEdit(wrap)
	})
}

// completeWrap is the changes that make the text where each selection
// was replaced by mark (after) into the text where mark wraps it: each
// selection's text and a second mark after its first.
func completeWrap(before, after string, sels []markup.Sel, mark string) notes.ChangeSet {
	sorted := slices.Clone(sels)
	slices.SortFunc(sorted, func(a, b markup.Sel) int { return a.From() - b.From() })
	var changes []notes.Change
	shift := 0
	for _, sel := range sorted {
		at := sel.From() - shift + len(mark)
		changes = append(changes, notes.Change{From: at, To: at, Insert: before[sel.From():sel.To()] + mark})
		shift += sel.To() - sel.From() - len(mark)
	}
	cs, err := notes.ChangesOf(len(after), changes...)
	if err != nil {
		return notes.EmptyChanges(len(after))
	}
	return cs
}

// markupSels are the editor's selections as byte offsets of its text.
func (s *sourceEditor) markupSels() []markup.Sel {
	ss := s.ref.Selections()
	out := make([]markup.Sel, len(ss))
	for i, sel := range ss {
		out[i] = markup.Sel{Anchor: offset(s.last, sel.Anchor), Head: offset(s.last, sel.Head)}
	}
	return out
}

// offset is p's byte offset in text; position is the editor's place of
// byte offset off.
func offset(text string, p editor.Position) int { return markup.Offset(text, p.Line, p.Column) }

func position(text string, off int) editor.Position {
	line, col := markup.LineCol(text, off)
	return editor.Position{Line: line, Column: col}
}

// editTools are source mode's commands in the palette, with their keys:
// the formatting commands, find, indenting, undo and following a link.
// Each is left out while source mode is not shown, or, for those that
// edit, while the note cannot be edited.
func (w *workspace) editTools() []tool {
	noSource := func() bool { return w.source == nil || w.live.GetUntracked() }
	cannotEdit := func() bool { return noSource() || !w.source.writable() }
	on := func(f func(*sourceEditor)) func() {
		return func() {
			if s := w.source; s != nil {
				f(s)
				w.cx.Post(s.ref.Focus)
			}
		}
	}
	run := func(c editor.Command) func() { return on(func(s *sourceEditor) { s.ref.Run(c) }) }
	out := []tool{
		{label: "Search current file", keys: "mod+f", off: noSource, run: func() {
			if s := w.source; s != nil {
				s.openFind(false)
			}
		}},
		{label: "Search & replace in current file", keys: "mod+h", off: cannotEdit, run: func() {
			if s := w.source; s != nil {
				s.openFind(true)
			}
		}},
	}
	for _, c := range markup.Commands() {
		out = append(out, tool{label: c.Title, keys: c.Keys, off: cannotEdit, run: on(func(s *sourceEditor) { s.apply(c.Edit) })})
	}
	out = append(out,
		tool{label: "Indent list", off: cannotEdit, run: run(editor.Indent)},
		tool{label: "Unindent list", off: cannotEdit, run: run(editor.Outdent)},
		tool{label: "Undo", keys: "mod+z", off: cannotEdit, run: run(editor.Undo)},
		tool{label: "Redo", keys: "mod+shift+z", off: cannotEdit, run: run(editor.Redo)},
		tool{label: "Follow link under cursor", keys: "alt+enter", off: func() bool { return noSource() || !w.source.linkAtCaret() },
			run: on((*sourceEditor).followCaret)},
	)
	return out
}

// caretOffset is the primary caret's byte offset in the editor's text.
func (s *sourceEditor) caretOffset() int {
	sels := s.ref.Selections()
	if len(sels) == 0 {
		return 0
	}
	return offset(s.last, sels[len(sels)-1].Head)
}

// linkAtCaret reports whether a link or a tag is under the caret.
func (s *sourceEditor) linkAtCaret() bool {
	at := s.caretOffset()
	if _, ok := markup.LinkAt(s.last, at); ok {
		return true
	}
	_, ok := markup.TagAt(s.last, at)
	return ok
}
