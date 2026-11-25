package main

import (
	"fmt"
	"strings"

	"github.com/frostyard/frostui/workspaceui"
	"github.com/nsgo-dev/nsgo/base/editor"
	"github.com/nsgo-dev/nsgo/core"
	"github.com/nsgo-dev/nsgo/icons/lucide"
	"github.com/nsgo-dev/nsgo/ui"
)

// Find and replace in the note (Ctrl+F, Ctrl+H), on NSGo's editor's search:
// the bar over source mode is the app's, of NSGo's controls, as the MyGo
// client's find bar was. The editor marks the matches and steps through
// them, and replaces one or all.

// finder is what the find bar holds. It outlives the editor's restarts
// (source.go), which read its search again.
type finder struct {
	open, replacing             *core.Signal[bool]
	query, with                 *core.Signal[string]
	caseSensitive, regex, whole *core.Signal[bool]
	result                      *core.Signal[editor.SearchResult]
	// field is the find field, and inField says it has the keyboard.
	field   *core.Ref
	inField *core.Signal[bool]
}

func newFinder(cx *core.Scope) *finder {
	return &finder{
		open: core.NewSignal(cx, false), replacing: core.NewSignal(cx, false),
		query: core.NewSignal(cx, ""), with: core.NewSignal(cx, ""),
		caseSensitive: core.NewSignal(cx, false), regex: core.NewSignal(cx, false), whole: core.NewSignal(cx, false),
		result: core.NewSignal(cx, editor.SearchResult{Current: -1}),
		field:  &core.Ref{}, inField: core.NewSignal(cx, false),
	}
}

// search is the editor's search: nothing while the bar is closed.
func (f *finder) search() editor.Search {
	if !f.open.Get() {
		return editor.Search{}
	}
	return editor.Search{Query: f.query.Get(), CaseSensitive: f.caseSensitive.Get(), Regex: f.regex.Get(), WholeWord: f.whole.Get()}
}

// openFind shows the find bar, with Search & replace its second row, and
// puts the keyboard in its field. From the editor, a selection on one line
// is what it finds.
func (s *sourceEditor) openFind(replace bool) {
	if sel := s.ref.SelectedText(); sel != "" && !strings.Contains(sel, "\n") {
		s.find.query.Set(sel)
	}
	s.showFind(replace)
}

// showFind shows the find bar as openFind does, its query as it is.
func (s *sourceEditor) showFind(replace bool) {
	f := s.find
	if replace && !s.writable() {
		replace = false
	}
	f.replacing.Set(replace || f.open.GetUntracked() && f.replacing.GetUntracked())
	f.open.Set(true)
	s.w.cx.Post(func() {
		f.field.Focus()
		f.field.SetSelection(0, len([]rune(f.query.GetUntracked())))
	})
}

// closeFind hides the bar and gives the keyboard back to the editor.
func (s *sourceEditor) closeFind() {
	s.find.open.Set(false)
	s.find.inField.Set(false)
	s.ref.Focus()
}

// findBar is the bar over source mode while find is open: the find field,
// the choices of case, whole words and regular expressions, where the
// caret is among the matches, the previous and next match and Close; with
// replace, the replacement, Replace and Replace all.
func (s *sourceEditor) findBar(readable bool) core.Node {
	f := s.find
	return core.Show{When: f.open.Get, Then: func(cx *core.Scope) core.Node {
		colors := ui.ColorsOf(cx)
		step := func(c editor.Command) func() { return func() { s.ref.Run(c) } }
		count := func() string {
			r := f.result.Get()
			switch {
			case r.Error != "":
				return "Not a regular expression"
			case f.query.Get() == "":
				return ""
			case r.Count == 0:
				return "No results"
			case r.Current >= 0:
				return fmt.Sprintf("%d of %d", r.Current+1, r.Count)
			}
			return fmt.Sprintf("%d results", r.Count)
		}
		toggle := func(label string, icon func() *core.SVG, on *core.Signal[bool]) core.Node {
			return ui.ToggleButton{Label: label, Icon: icon, On: on.Get, OnChange: on.Set}
		}
		width := core.Style{Width: core.Pct(100), Gap: workspaceui.SpaceXS, Padding: core.Edges{Bottom: workspaceui.SpaceS}}
		if readable {
			width.MaxWidth = core.Pt(readableWidth)
		}
		row := func(children ...core.Node) core.Node {
			return core.View{Style: core.Style{Direction: core.Row, AlignItems: core.AlignCenter, Gap: workspaceui.SpaceXS}, Children: children}
		}
		grow := core.Style{FlexGrow: 1, FlexShrink: 1, MinWidth: core.Pt(0)}
		// Escape closes the bar from its buttons, and from its fields
		// below; the editor's own Escape, which leaves 1 caret, is newer
		// while it has the keyboard.
		cx.OnKey("escape", s.closeFind)
		return core.View{Style: width, Children: []core.Node{
			// Its keys while a field has the keyboard: Escape, Shift+Enter finds
			// the previous match, mod+F and mod+H stay in the bar.
			core.Show{When: f.inField.Get, Then: func(cx *core.Scope) core.Node {
				bind := func(key string, fn func()) { cx.OnKeyWith(key, core.KeyOptions{When: core.KeyAlways}, fn) }
				bind("escape", s.closeFind)
				bind("shift+enter", step(editor.FindPrevious))
				bind("mod+f", func() { s.showFind(false) })
				bind("mod+h", func() { s.showFind(true) })
				return core.Group{}
			}},
			row(
				ui.Input{Ref: f.field, Label: "Find in note", Placeholder: "Find", Value: f.query.Get, OnChange: f.query.Set,
					OnSubmit: func(string) { s.ref.Run(editor.FindNext) },
					OnFocus:  func() { f.inField.Set(true) }, OnBlur: func() { f.inField.Set(false) }, Style: grow},
				toggle("Match case", lucide.CaseSensitive, f.caseSensitive),
				toggle("Match whole word", lucide.WholeWord, f.whole),
				toggle("Use regular expression", lucide.Regex, f.regex),
				core.Text{ContentFunc: count, MaxLines: 1, Style: ui.TypeCaption, StyleFunc: ui.Colored(colors.Muted)},
				ui.IconButton{Label: "Previous match", Icon: lucide.ChevronUp, OnPress: step(editor.FindPrevious)},
				ui.IconButton{Label: "Next match", Icon: lucide.ChevronDown, OnPress: step(editor.FindNext)},
				ui.IconButton{Label: "Close find", Icon: lucide.X, OnPress: s.closeFind},
			),
			core.Show{When: f.replacing.Get, Then: func(*core.Scope) core.Node {
				return row(
					ui.Input{Label: "Replace with", Placeholder: "Replace", Value: f.with.Get, OnChange: f.with.Set,
						OnSubmit: func(string) { s.ref.Replace(f.with.GetUntracked()) },
						OnFocus:  func() { f.inField.Set(true) }, OnBlur: func() { f.inField.Set(false) }, Style: grow},
					ui.Button{Text: "Replace", Variant: ui.ButtonSecondary, Disabled: func() bool { return !s.writable() },
						OnPress: func() { s.ref.Replace(f.with.GetUntracked()) }},
					ui.Button{Text: "Replace all", Variant: ui.ButtonSecondary, Disabled: func() bool { return !s.writable() },
						OnPress: func() { s.ref.ReplaceAll(f.with.GetUntracked()) }},
				)
			}},
		}}
	}}
}
