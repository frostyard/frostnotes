package main

import (
	"context"
	"fmt"
	"slices"
	"strings"
	"time"
	"unicode/utf16"

	"github.com/frostyard/frostnotes/api"
	"github.com/frostyard/frostnotes/internal/desktop/livepreview"
	"github.com/frostyard/frostnotes/internal/desktop/notes"
	"github.com/frostyard/frostui/workspaceui"
	"github.com/nsgo-dev/nsgo/core"
	"github.com/nsgo-dev/nsgo/ui"
)

// Positions in the document and scanner count bytes. Only the native text
// field's selection uses UTF-16.
type previewLine struct {
	number, start, heading int
	raw                    string
	pieces                 []livepreview.Piece
}

func previewLines(scanner *livepreview.Scanner, source string) []previewLine {
	scanned := scanner.Scan(source, livepreview.Options{})
	var lines []previewLine
	start, next := 0, 0
	for raw := range strings.SplitSeq(source, "\n") {
		line := previewLine{number: len(lines), start: start, raw: raw}
		end := start + len(raw)
		for next < len(scanned.Pieces) && scanned.Pieces[next].Start < end {
			p := scanned.Pieces[next]
			next++
			if p.Start >= start && p.End <= end {
				p.Start -= start
				p.End -= start
				line.pieces = append(line.pieces, p)
			}
		}
		lines = append(lines, line)
		start = end + 1
	}
	for _, s := range scanned.Styles {
		lines[s.Line].heading = s.Heading
	}
	return lines
}

type inlineView struct {
	fieldRef   *core.Ref
	d          *notes.Document
	active     *core.Signal[int]
	lines      *core.Memo[[]previewLine]
	column     int
	expected   string
	undo, redo []notes.Change
	follow     func(string)
}

func (w *workspace) inlineEditor(cx *core.Scope, d *notes.Document) core.Node {
	e := &inlineView{d: d, active: core.NewSignal(cx, -1), expected: d.Text(), follow: func(target string) { w.followWiki(d, target) }}
	for _, key := range []string{"mod+z", "mod+shift+z", "ctrl+y"} {
		cx.OnKeyWith(key, core.KeyOptions{When: core.KeyAlways}, func() {
			if key == "mod+z" {
				e.history(&e.undo, &e.redo)
			} else {
				e.history(&e.redo, &e.undo)
			}
		})
	}
	var scanner livepreview.Scanner
	source := core.NewMemo(cx, func() string { w.revision.Get(); return d.Text() })
	e.lines = core.NewMemoFunc(cx, func() []previewLine { return previewLines(&scanner, source.Get()) }, nil)
	cx.Effect(func() {
		if source.Get() != e.expected {
			e.undo = nil
			e.redo = nil
			e.expected = d.Text()
			e.active.Set(-1)
		}
	})
	return core.List{Count: func() int { return len(e.lines.Get()) }, Row: e.row, EstimatedSize: 26, Style: ui.FillRest}
}

func (e *inlineView) row(cx *core.Scope, row core.ListRow) core.Node {
	item := func() previewLine {
		rows := e.lines.Get()
		i := row.Index()
		if i < 0 || i >= len(rows) {
			return previewLine{number: -1}
		}
		return rows[i]
	}
	return core.Show{When: func() bool { return e.active.Get() >= 0 && e.active.Get() == row.Index() }, Then: func(cx *core.Scope) core.Node { return e.field(cx, item) }, Else: func(cx *core.Scope) core.Node {
		// Only rebuild the displayed runs when this row's appearance changes.
		line := core.NewMemoFunc(cx, item, func(a, b previewLine) bool {
			return a.number == b.number && a.raw == b.raw && a.heading == b.heading && slices.Equal(a.pieces, b.pieces)
		})
		return core.Dynamic(func(cx *core.Scope) core.Node {
			l := line.Get()
			if l.number < 0 {
				return nil
			}
			return e.renderLine(cx, l)
		})
	}}
}

func (e *inlineView) writable() bool {
	return e.d.Loaded() && e.d.ReadOnly() == "" && e.d.Status() != notes.StatusMissing && !needsRecovery(e.d)
}

func (e *inlineView) activate(line, col int) {
	if e.writable() {
		e.column = col
		e.active.Set(line)
	}
}

// Keep inverse edits in source coordinates. A server update invalidates local
// undo before another command can apply it against a different base.
func (e *inlineView) apply(c notes.Change) (notes.Change, bool) {
	base := e.d.Text()
	if !e.writable() || base != e.expected || c.From < 0 || c.To < c.From || c.To > len(base) {
		return notes.Change{}, false
	}
	cs, err := notes.ChangesOf(len(base), c)
	if err != nil || !e.d.EditIfText(base, cs) {
		return notes.Change{}, false
	}
	e.expected = e.d.Text()
	return notes.Change{From: c.From, To: c.From + len(c.Insert), Insert: strings.Clone(base[c.From:c.To])}, true
}

func (e *inlineView) edit(c notes.Change) bool {
	inverse, ok := e.apply(c)
	if ok {
		e.undo = append(e.undo, inverse)
		e.redo = nil
	}
	return ok
}

func (e *inlineView) history(from, to *[]notes.Change) {
	if len(*from) == 0 {
		return
	}
	c := (*from)[len(*from)-1]
	inverse, ok := e.apply(c)
	if !ok {
		return
	}
	*from = (*from)[:len(*from)-1]
	*to = append(*to, inverse)
	if e.active.GetUntracked() >= 0 {
		prefix := e.expected[:c.From+len(c.Insert)]
		line := strings.Count(prefix, "\n")
		column := units(prefix[strings.LastIndexByte(prefix, '\n')+1:])
		if e.active.GetUntracked() == line && e.fieldRef != nil {
			e.fieldRef.SetSelection(column, column)
		}
		e.activate(line, column)
	}
}

func units(s string) int {
	n := 0
	for _, r := range s {
		n += utf16.RuneLen(r)
	}
	return n
}

func (e *inlineView) field(cx *core.Scope, item func() previewLine) core.Node {
	ref := new(core.Ref)
	e.fieldRef = ref
	cx.Cleanup(func() {
		if e.fieldRef == ref {
			e.fieldRef = nil
		}
	})
	focused := core.NewSignal(cx, false)
	initial := min(e.column, units(item().raw))
	sel := core.NewSignal(cx, core.TextRange{Start: initial, End: initial})
	// Bind only while the field has focus. Leave ordinary deletion and
	// horizontal navigation within the line to the platform text field.
	cx.Effect(func() {
		if !focused.Get() {
			return
		}
		row := item()
		r := sel.Get()
		bind := func(key string, fn func()) {
			cx.OnKeyWith(key, core.KeyOptions{When: core.KeyAlways, Repeat: true}, fn)
		}
		last := len(e.lines.Get()) - 1
		if row.number > 0 {
			bind("up", func() { e.activate(row.number-1, r.Start) })
			if r.Start == 0 && r.End == 0 {
				prev := e.lines.Get()[row.number-1]
				bind("left", func() { e.activate(row.number-1, units(prev.raw)) })
				bind("backspace", func() {
					if e.edit(notes.Change{From: row.start - 1, To: row.start}) {
						e.activate(row.number-1, units(prev.raw))
					}
				})
			}
		}
		if row.number < last {
			bind("down", func() { e.activate(row.number+1, r.Start) })
			if r.Start == units(row.raw) && r.End == r.Start {
				bind("right", func() { e.activate(row.number+1, 0) })
				bind("delete", func() { e.edit(notes.Change{From: row.start + len(row.raw), To: row.start + len(row.raw) + 1}) })
			}
		}
	})
	return core.Input{Label: "Markdown line", Ref: ref, OnFocus: func() { focused.Set(true) }, OnBlur: func() { focused.Set(false) }, AutoFocus: true, Multiline: true, NoAutoCorrect: true, Style: ui.TypeBody.Merge(core.Style{MinHeight: core.Pt(28)}),
		Value: func() string { return item().raw }, SelectedRange: func() core.TextRange { return core.TextRange{Start: initial, End: initial} }, OnSelectionChange: sel.Set,
		OnChange: func(value string) {
			row := item()
			if row.raw == value || e.active.GetUntracked() != row.number {
				return
			}
			diff := notes.Diff(row.raw, value).Changes()
			if len(diff) == 0 {
				return
			}
			// One field contains one source line. A paste can insert multiple
			// lines, moving the native field to the end of the inserted text.
			first, last := diff[0], diff[len(diff)-1]
			insertedEnd := len(value) - (len(row.raw) - last.To)
			if !e.edit(notes.Change{From: row.start + first.From, To: row.start + last.To, Insert: value[first.From:insertedEnd]}) {
				return
			}
			if strings.Contains(value, "\n") {
				prefix := value[:insertedEnd]
				e.activate(row.number+strings.Count(prefix, "\n"), units(prefix[strings.LastIndexByte(prefix, '\n')+1:]))
			}
		}}
}

func (e *inlineView) renderLine(cx *core.Scope, l previewLine) core.Node {
	style := ui.TypeBody.Merge(core.Style{MinHeight: core.Pt(26)})
	if l.heading > 0 {
		style = style.Merge(workspaceui.Heading(l.heading))
	}
	spans := []core.Span{}
	// TextCaretAt counts rendered UTF-16 units. Record where each changed
	// run starts and ends so clicking after hidden markup lands in the source.
	type projection struct{ start, end, width int }
	var replacements []projection
	cursor := 0
	for _, p := range l.pieces {
		if cursor < p.Start {
			spans = append(spans, core.Span{Content: l.raw[cursor:p.Start]})
		}
		raw := l.raw[p.Start:p.End]
		span := core.Span{Content: raw}
		switch p.Kind {
		case livepreview.Hide:
			span.Content = ""
		case livepreview.Task:
			span.Attachment = &ui.Checkbox{Style: core.Style{VerticalAlign: core.VerticalAlignMiddle}, Label: fmt.Sprintf("Task on line %d", l.number+1), Checked: func() bool { return p.Done }, Disabled: func() bool { return !e.writable() }, OnChange: func(on bool) {
				lines := e.lines.GetUntracked()
				if l.number >= len(lines) || lines[l.number].raw != l.raw {
					return
				}
				mark := lines[l.number].start + p.Start + strings.LastIndexByte(raw, '[') + 1
				end := lines[l.number].start + p.Start + strings.LastIndexByte(raw, ']')
				replacement := " "
				if on {
					replacement = "x"
				}
				e.edit(notes.Change{From: mark, To: end, Insert: replacement})
			}}
		case livepreview.Link:
			if p.Target != "" && strings.HasPrefix(raw, "[[") {
				span.Content = p.Label
				span.Attachment = &ui.Link{Text: p.Label, Label: "Open " + p.Target, OnPress: func() { e.follow(p.Target) }}
			}
		case livepreview.Bullet:
			span.Content = "•"
		case livepreview.Quote:
			span.Content = "│ "
		case livepreview.Dim:
			span.Content = p.Label
			span.StyleFunc = func() core.Style { return core.Style{Color: ui.ColorsOf(cx).Faint()} }
		}
		if span.Content != "" || span.Attachment != nil {
			spans = append(spans, span)
		}
		width := units(span.Content)
		if span.Attachment != nil {
			width = 1
		}
		replacements = append(replacements, projection{p.Start, p.End, width})
		cursor = p.End
	}
	if cursor < len(l.raw) {
		spans = append(spans, core.Span{Content: l.raw[cursor:]})
	}
	if len(spans) == 0 {
		spans = append(spans, core.Span{Content: " "})
	}
	ref := new(core.Ref)
	reveal := func(press core.PressEvent) {
		col, _, ok := ref.TextCaretAt(press.Position)
		if !ok || press.Keyboard {
			e.activate(l.number, 0)
			return
		}
		src, shown, prev := 0, 0, 0
		for _, r := range replacements {
			unchanged := units(l.raw[prev:r.start])
			if col < shown+unchanged {
				break
			}
			src += unchanged
			shown += unchanged
			if col < shown+r.width {
				break
			}
			src += units(l.raw[r.start:r.end])
			shown += r.width
			prev = r.end
		}
		e.activate(l.number, src+col-shown)
	}
	return core.View{Role: core.RoleButton, Label: fmt.Sprintf("Edit line %d", l.number+1), OnPress: reveal, Children: []core.Node{core.Text{Ref: ref, Spans: spans, Style: style}}}
}

func (w *workspace) followWiki(d *notes.Document, target string) {
	c := w.session.Client()
	if c == nil {
		w.problem.Set("Reconnect to open this wiki link.")
		return
	}
	w.navigation++
	request := w.navigation
	params := api.NoteResolveParams{VaultID: d.Vault(), From: d.Path(), Targets: []string{target}}
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
		defer cancel()
		hits, err := c.Note().Resolve(ctx, &params)
		w.cx.Post(func() {
			if w.navigation != request || w.doc.GetUntracked() != d || w.session.Client() != c {
				return
			}
			if err != nil {
				w.problem.Set(err.Error())
				return
			}
			if len(hits) == 0 || hits[0].Path == nil {
				w.problem.Set(fmt.Sprintf("Could not find the note %q.", target))
				return
			}
			w.open(*hits[0].Path)
		})
	}()
}
