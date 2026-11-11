package editor

import (
	"path"
	"slices"
	"strings"

	"github.com/egoist/mygo/ui"
	code "github.com/egoist/mygo/ui/editor"

	"github.com/frostyard/frostnotes/internal/desktop/livepreview"
	"github.com/frostyard/frostnotes/internal/desktop/mathview"
	"github.com/frostyard/frostnotes/internal/desktop/media"
	"github.com/frostyard/frostnotes/internal/desktop/notes"
)

// ruleWidth is how many characters a horizontal rule draws.
const ruleWidth = 40

// lineSpan is the lines a selection touches, from first through last.
type lineSpan struct{ first, last int }

// liveState is what the editor showed last, so that a frame with the same
// text and selected lines does no work.
type liveState struct {
	math    mathview.Cache
	scanner livepreview.Scanner
	on      bool
	version uint64
	lines   []lineSpan
	front   bool
	muted   ui.Color

	resolved   map[string]bool // links' targets, until the vault changes
	widths     map[string]float32
	widthsFont ui.Font
	hovered    int // the link under Ctrl, one past its start; 0 for none
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
			e.State.SetBlocks(nil)
			e.State.SetLineStyles(nil)
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
	res := l.scanner.Scan(buf.String(), livepreview.Options{Revealed: revealed, HideFront: e.HideFront, Blocks: e.Rendered != nil, Embed: e.embedKind})
	out := make([]code.Replacement, len(res.Pieces))
	for i, p := range res.Pieces {
		out[i] = e.replacement(c, p, muted)
	}
	e.State.SetReplacements(out)
	styles := make([]code.LineStyle, 0, len(res.Styles))
	sizes := [...]float32{1, 2, 1.5, 1.25, 1.125, 1, 1}
	for _, s := range res.Styles {
		style := code.LineStyle{Line: s.Line, Strike: s.Done}
		if s.Heading > 0 {
			style.Font = ui.Font{Size: e.font().Size * sizes[s.Heading], Weight: 700}
		}
		styles = append(styles, style)
	}
	e.State.SetLineStyles(styles)
	blocks := make([]code.Block, 0, len(res.Blocks))
	for _, b := range res.Blocks {
		blocks = append(blocks, e.block(b))
	}
	e.State.SetBlocks(blocks)
}

// RefreshLive works out what live preview shows again, as what an embed
// names may have come or gone.
func (e *NoteEditor) RefreshLive() {
	e.live.on = false
	clear(e.live.resolved)
}

// embedKind says what an embed's target draws as: as the web app's, an
// image of the vault, a web image if the preference loads them, or a note.
func (e *NoteEditor) embedKind(target string, wikilink bool) livepreview.EmbedKind {
	if e.Rendered == nil || e.Sources == nil || target == "" {
		return livepreview.NoEmbed
	}
	if strings.Contains(target, ":") || strings.HasPrefix(target, "//") {
		if !wikilink && e.Rendered.RemoteImages && (strings.HasPrefix(target, "https://") || strings.HasPrefix(target, "http://")) {
			return livepreview.ImageEmbed
		}
		return livepreview.NoEmbed
	}
	p := e.Sources.Resolve(target)
	switch ext := strings.ToLower(path.Ext(p)); {
	case p == "":
		return livepreview.NoEmbed
	case media.Kind(p) != "":
		return livepreview.ImageEmbed
	case imageExts[ext]:
		return livepreview.ImageEmbed
	case ext == ".md" && wikilink:
		return livepreview.NoteEmbed
	}
	return livepreview.NoEmbed
}

// imageExts are the images MyGo decodes.
var imageExts = map[string]bool{".avif": true, ".png": true, ".jpg": true, ".jpeg": true, ".gif": true, ".bmp": true, ".webp": true, ".svg": true}

// block turns a block of live preview into MyGo's.
func (e *NoteEditor) block(b livepreview.Block) code.Block {
	if b.Kind == livepreview.CodeHeader {
		return e.codeHeader(b)
	}
	return code.Block{Line: b.Line, Height: blockHeight(b), Label: b.Label, Interactive: true,
		Content: func(c *ui.Context, el ui.Element, at code.Block) {
			// Links and folds inside take their own presses; any other
			// shows the source, as a caret on the block does.
			if el.Clicked() {
				e.caretToLine(at.Line)
			}
			el.Padding(4, 0).Children(func() { e.Rendered.View(c, b.Source, b.Width) })
		}}
}

// blockHeight is how high a block is taken to be until it has shown: its
// source's lines, or an image's usual height.
func blockHeight(b livepreview.Block) float32 {
	if b.Label == "Image" {
		return 200
	}
	return float32(strings.Count(b.Source, "\n")+2) * 24
}

func (e *NoteEditor) caretToLine(line int) {
	at := e.b.Buffer().Offset(code.Pos{Line: line})
	e.State.Select(at, at)
	e.focusNext = true
}

// codeHeader draws a fence's language and a button that copies its code.
func (e *NoteEditor) codeHeader(b livepreview.Block) code.Block {
	return code.Block{Line: b.Line, Height: 28, Label: b.Label, Interactive: true,
		Content: func(c *ui.Context, el ui.Element, at code.Block) {
			if el.Clicked() {
				e.caretToLine(at.Line)
			}
			t := c.Theme()
			el.Children(func() {
				ui.Row(c).FillWidth().Height(28).Padding(0, 8).AlignItems(ui.Center).Background(t.Surface).Radius(4).Children(func() {
					ui.Text(c, b.Language).FontSize(12).TextColor(t.TextMuted).Grow(1)
					copyCode := ui.ButtonBase(c).KeepFocus().Cursor(ui.CursorPointer).Label("Copy code").Padding(2, 6)
					copyCode.Children(func() { ui.Text(c, "Copy").FontSize(12).TextColor(t.TextMuted) })
					if copyCode.Clicked() {
						c.WriteClipboard(b.Code)
					}
				})
			})
		}}
}

func (e *NoteEditor) selectedLines(buf *code.Buffer) []lineSpan {
	sels := e.State.Selections()
	out := make([]lineSpan, len(sels))
	for i, s := range sels {
		out[i] = lineSpan{buf.Pos(s.Start()).Line, buf.Pos(s.End()).Line}
	}
	return out
}

func (e *NoteEditor) replacement(c *ui.Context, p livepreview.Piece, muted ui.Color) code.Replacement {
	r := code.Replacement{Start: p.Start, End: p.End}
	switch p.Kind {
	case livepreview.Bullet:
		r.Text, r.Color = "•", muted
	case livepreview.Quote:
		r.Text, r.Color = "▏ ", muted
	case livepreview.Rule:
		r.Text, r.Color = strings.Repeat("─", ruleWidth), muted
	case livepreview.Dim:
		r.Text, r.Color = p.Label, muted
	case livepreview.Math:
		if obj, ok := e.live.math.Inline(p.Label, e.font().Size, c.Theme().Text); ok {
			r.Width = obj.Width
			r.Label = p.Label
			r.Content = func(_ *ui.Context, el ui.Element, _ code.Replacement) {
				el.Draw(func(p *ui.Painter, rect ui.Rect) {
					// Fit tall inline expressions inside the line; display math gets its own block.
					width, height := obj.Width, obj.Height
					if height > rect.H {
						width *= rect.H / height
						height = rect.H
					}
					obj.Draw(p, ui.Rect{X: rect.X, Y: rect.Y + (rect.H-height)/2, W: width, H: height})
				})
			}
		} else {
			r.Text = "$" + p.Label + "$"
		}
	case livepreview.Task:
		return e.taskBox(p)
	case livepreview.Link, livepreview.Tag, livepreview.Footnote:
		return e.object(c, p)
	}
	return r
}

// font is the editor's font, with what MyGo fills in.
func (e *NoteEditor) font() ui.Font {
	f := e.State.Font
	if f.Family == "" {
		f.Family = "monospace"
	}
	if f.Size <= 0 {
		f.Size = 13
	}
	return f
}

// objectWidth measures a label in the editor's font, once for each.
func (e *NoteEditor) objectWidth(c *ui.Context, label string, f ui.Font) float32 {
	l := &e.live
	if l.widths == nil || l.widthsFont != f {
		l.widths, l.widthsFont = map[string]float32{}, f
	}
	w, ok := l.widths[label]
	if !ok {
		w, _ = c.MeasureText(0, ui.Span{Text: label, Font: f.Family, Size: f.Size})
		l.widths[label] = w
	}
	return w + 1
}

// object draws a link, a tag or a footnote as an element a click reaches:
// what the web app's widgets are.
func (e *NoteEditor) object(c *ui.Context, p livepreview.Piece) code.Replacement {
	f := e.font()
	resolved := p.Kind != livepreview.Link || e.resolved(p)
	return code.Replacement{Start: p.Start, End: p.End, Label: p.Label, Interactive: true, Width: e.objectWidth(c, p.Label, f),
		Content: func(c *ui.Context, el ui.Element, at code.Replacement) {
			t := c.Theme()
			color := t.Accent
			if !resolved {
				color = color.Alpha(0.7)
			}
			el.Children(func() {
				b := ui.ButtonBase(c).Fill().Center().KeepFocus().Cursor(ui.CursorPointer).Label(p.Label)
				b.Children(func() { ui.Text(c, p.Label).Font(f.Family).FontSize(f.Size).TextColor(color).SingleLine() })
				if b.Clicked() {
					e.activate(p, at, c.Modifiers())
				}
				if p.Kind == livepreview.Link {
					e.previewOn(b, at, c.Modifiers())
				}
			})
		}}
}

// resolved reports whether a link leads to a file of the vault, or the web.
func (e *NoteEditor) resolved(p livepreview.Piece) bool {
	if e.Sources == nil || p.Target == "" {
		return true
	}
	l := &e.live
	found, ok := l.resolved[p.Target]
	if !ok {
		if l.resolved == nil {
			l.resolved = map[string]bool{}
		}
		found = e.Sources.Resolve(p.Target) != ""
		l.resolved[p.Target] = found
	}
	return found
}

// activate follows what was clicked: Ctrl opens a link in a new tab, and
// Ctrl + Alt to the right.
func (e *NoteEditor) activate(p livepreview.Piece, at code.Replacement, mods ui.Modifiers) {
	switch p.Kind {
	case livepreview.Tag:
		if e.FollowTag != nil {
			e.FollowTag(strings.TrimPrefix(p.Label, "#"))
		}
	case livepreview.Footnote:
		text := e.b.Buffer().String()
		if i := strings.Index(text, "\n[^"+p.Label+"]:"); i >= 0 {
			e.State.Select(i+1, i+1)
			e.focusNext = true
		}
	default:
		where := WhereHere
		switch {
		case mods&ui.Cmd != 0 && mods&ui.Alt != 0:
			where = WhereRight
		case mods&ui.Cmd != 0:
			where = WhereTab
		}
		e.followAt(at.Start, where)
	}
}

// previewOn shows a link's page preview while Ctrl rests on it.
func (e *NoteEditor) previewOn(b ui.Element, at code.Replacement, mods ui.Modifiers) {
	switch hovered := b.Hovered() && mods&ui.Cmd != 0; {
	case hovered && e.live.hovered != at.Start+1:
		e.live.hovered = at.Start + 1
		e.hover(code.HoverRequest{Offset: at.Start})
	case !hovered && e.live.hovered == at.Start+1:
		e.live.hovered = 0
	}
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
