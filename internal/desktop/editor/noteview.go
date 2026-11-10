package editor

import (
	"strconv"
	"strings"
	"time"

	"github.com/egoist/mygo/ui"
	"github.com/frostyard/frostui"

	"github.com/frostyard/frostnotes/internal/desktop/notes"
)

// ReadableWidth is a note's width under Readable line length.
const ReadableWidth = 700

// NoteView is a tab's note in edit mode: its inline title, its properties
// and its editor (docs/specs/ui.md#editor-and-reading-view).
type NoteView struct {
	Editor     *NoteEditor
	Properties *PropertiesPanel
	// ShowTitle and ShowProperties are the Show inline title and
	// Properties in document preferences.
	ShowTitle, ShowProperties bool
	// Readable limits the note's width (Readable line length).
	Readable bool
	// Rename moves the note: its new name, without ".md".
	Rename func(title string)

	title        string
	titleFor     string
	titleFocused bool
	folded       string // the text whose frontmatter was folded
}

// NewNoteView makes the view of an editor.
func NewNoteView(e *NoteEditor) *NoteView {
	return &NoteView{Editor: e, Properties: NewPropertiesPanel(e.Binding().Document()), ShowTitle: true, ShowProperties: true}
}

func (v *NoteView) doc() *notes.Document { return v.Editor.Binding().Document() }

func noteTitle(p string) string {
	base := p[strings.LastIndexByte(p, '/')+1:]
	return strings.TrimSuffix(base, ".md")
}

// View draws the note.
func (v *NoteView) View(c *ui.Context) ui.Element {
	return ui.Column(c).Fill().AlignItems(ui.Center).Children(func() {
		col := ui.Column(c).Grow(1).MinHeight(0).FillWidth().AlignItems(ui.Stretch)
		if v.Readable {
			col.MaxWidth(ReadableWidth)
		}
		col.Children(func() {
			if v.ShowTitle {
				v.inlineTitle(c)
			}
			if v.ShowProperties {
				v.Properties.View(c)
				v.foldFrontmatter()
			}
			v.Editor.View(c).Grow(1).FillWidth()
		})
	})
}

// foldFrontmatter hides the frontmatter's lines in the editor while the
// panel shows them.
func (v *NoteView) foldFrontmatter() {
	d := v.doc()
	if !d.Loaded() || !strings.HasPrefix(d.Text(), "---\n") || v.folded == d.Path() {
		return
	}
	v.folded = d.Path()
	v.Editor.State.Fold(0)
}

// inlineTitle draws the note's name above it: Enter or leaving the field
// renames the file.
func (v *NoteView) inlineTitle(c *ui.Context) {
	name := noteTitle(v.doc().Path())
	if v.titleFor != name {
		v.title, v.titleFor = name, name
	}
	t := c.Theme()
	in := ui.TextInputBase(c, &v.title).Label("Note title").FontSize(26).Bold().TextColor(t.Text).Padding(frostui.SpaceM, frostui.SpaceL, frostui.SpaceS)
	focused := in.Focused()
	if in.Submitted() || v.titleFocused && !focused {
		v.commitTitle()
	}
	v.titleFocused = focused
}

func (v *NoteView) commitTitle() {
	title := strings.TrimSpace(v.title)
	if title == "" || title == v.titleFor || strings.ContainsAny(title, "/\\:") {
		v.title = v.titleFor
		return
	}
	if v.Rename != nil {
		v.Rename(title)
	}
}

// PropertiesPanel draws and edits a note's frontmatter
// (docs/specs/ui.md#editor-and-reading-view): typed values, editable keys,
// remove and add, each edit rewriting only that key's lines.
type PropertiesPanel struct {
	doc *notes.Document
	// Types are the vault's property types (note.properties).
	Types func() map[string]string
	// Suggestions are what a tags field offers.
	Suggestions func() []string
	Folded      bool

	text   string
	parsed Frontmatter
	drafts map[string]*fieldDraft
	adding string
	isAdd  bool
	err    string
}

// fieldDraft is what a row's fields hold while they are edited.
type fieldDraft struct {
	key, value           string
	keyFocus, valueFocus bool
	list                 []string
	check                bool
	when                 time.Time
	seen                 any
}

// NewPropertiesPanel makes the panel of doc.
func NewPropertiesPanel(doc *notes.Document) *PropertiesPanel {
	return &PropertiesPanel{doc: doc, drafts: map[string]*fieldDraft{}}
}

// Add opens a new row (Ctrl + ;).
func (p *PropertiesPanel) Add() { p.isAdd, p.adding, p.Folded = true, "", false }

func (p *PropertiesPanel) apply(next string, err error) {
	if err != nil {
		p.err = err.Error()
		return
	}
	p.err = ""
	p.doc.Edit(notes.Diff(p.doc.Text(), next))
}

func (p *PropertiesPanel) set(key string, value any) {
	next, err := SetProperties(p.doc.Text(), map[string]any{key: value})
	p.apply(next, err)
}

func (p *PropertiesPanel) rename(from, to string) {
	next, err := RenameProperty(p.doc.Text(), from, to)
	p.apply(next, err)
	delete(p.drafts, from)
}

// View draws the panel; it draws nothing without properties.
func (p *PropertiesPanel) View(c *ui.Context) ui.Element {
	if text := p.doc.Text(); text != p.text || p.parsed.Properties == nil {
		p.text, p.parsed = text, ReadFrontmatter(text)
	}
	if !p.doc.Loaded() || !p.parsed.Exists && !p.isAdd {
		return ui.Box(c).Height(0)
	}
	t := c.Theme()
	return ui.Column(c).Gap(frostui.SpaceXS).Padding(frostui.SpaceS, frostui.SpaceL).AlignItems(ui.Stretch).Children(func() {
		head := ui.Row(c).Gap(frostui.SpaceXS).AlignItems(ui.Center).Role(ui.RoleButton).Focusable().Cursor(ui.CursorPointer).Label("Properties").Expanded(!p.Folded)
		if head.Clicked() {
			p.Folded = !p.Folded
		}
		head.Children(func() {
			mark := "▾"
			if p.Folded {
				mark = "▸"
			}
			frostui.Muted(c, mark)
			frostui.Strong(c, "Properties")
		})
		if p.Folded {
			return
		}
		if p.parsed.Error != "" {
			ui.Text(c, "This note's frontmatter cannot be read: "+p.parsed.Error).TextColor(t.Danger).Wrap()
			return
		}
		for _, prop := range p.parsed.Properties {
			p.row(c.Key(prop.Key), prop)
		}
		p.addRow(c)
		if p.err != "" {
			ui.Text(c, p.err).TextColor(t.Danger).Wrap()
		}
	})
}

func (p *PropertiesPanel) draft(prop Property) *fieldDraft {
	d := p.drafts[prop.Key]
	if d == nil {
		d = &fieldDraft{key: prop.Key}
		p.drafts[prop.Key] = d
	}
	return d
}

func (p *PropertiesPanel) row(c *ui.Context, prop Property) {
	d := p.draft(prop)
	vault := ""
	if p.Types != nil {
		vault = p.Types()[prop.Key]
	}
	kind := PropertyType(prop.Key, prop.Value, vault)
	ui.Row(c).Gap(frostui.SpaceS).AlignItems(ui.Center).Children(func() {
		key := ui.TextInputBase(c, &d.key).Label("Property name").Width(160)
		focused := key.Focused()
		if (key.Submitted() || d.keyFocus && !focused) && d.key != prop.Key {
			if strings.TrimSpace(d.key) == "" {
				d.key = prop.Key
			} else {
				p.rename(prop.Key, strings.TrimSpace(d.key))
			}
		}
		d.keyFocus = focused
		ui.Row(c).Grow(1).AlignItems(ui.Center).Children(func() { p.value(c, prop, kind, d) })
		if frostui.IconButton(c, "window-close", "Remove "+prop.Key).Clicked() {
			p.set(prop.Key, nil)
		}
	})
}

func (p *PropertiesPanel) value(c *ui.Context, prop Property, kind string, d *fieldDraft) {
	changed := !same(d.seen, prop.Value)
	d.seen = prop.Value
	label := prop.Key
	switch kind {
	case TypeCheckbox:
		b, _ := prop.Value.(bool)
		if changed {
			d.check = b
		}
		if ui.Checkbox(c, &d.check, "").Label(label).Changed() {
			p.set(prop.Key, d.check)
		}
	case TypeMultitext, TypeAliases, TypeTags:
		if changed {
			d.list = listItems(prop.Value)
		}
		var suggestions []string
		if kind == TypeTags && p.Suggestions != nil {
			suggestions = p.Suggestions()
		}
		if ui.TokenField(c, &d.list, suggestions).Label(label).Grow(1).Changed() {
			out := make([]any, len(d.list))
			for i, s := range d.list {
				out[i] = s
			}
			p.set(prop.Key, out)
		}
	case TypeDate:
		if changed {
			d.when, _ = time.Parse("2006-01-02", valueText(prop.Value))
		}
		if ui.DateInput(c, &d.when).Label(label).Changed() {
			p.set(prop.Key, d.when.Format("2006-01-02"))
		}
	case TypeDatetime:
		if changed {
			d.when = parseDateTime(valueText(prop.Value))
		}
		a := ui.DateInput(c, &d.when).Label(label).Changed()
		b := ui.TimeInput(c, &d.when).Label(label + " time").Changed()
		if a || b {
			p.set(prop.Key, d.when.Format("2006-01-02T15:04"))
		}
	default:
		if changed {
			d.value = valueText(prop.Value)
		}
		in := ui.TextInputBase(c, &d.value).Label(label).Grow(1)
		focused := in.Focused()
		if (in.Submitted() || d.valueFocus && !focused) && d.value != valueText(prop.Value) {
			p.set(prop.Key, typedText(d.value, kind))
		}
		d.valueFocus = focused
	}
}

func parseDateTime(s string) time.Time {
	for _, layout := range []string{"2006-01-02T15:04:05", "2006-01-02T15:04"} {
		if t, err := time.Parse(layout, s); err == nil {
			return t
		}
	}
	return time.Time{}
}

// typedText is a text field's value as its type stores it.
func typedText(s, kind string) any {
	if kind == TypeNumber {
		if n, err := strconv.ParseInt(s, 10, 64); err == nil {
			return n
		}
		if f, err := strconv.ParseFloat(s, 64); err == nil {
			return f
		}
	}
	return s
}

func same(a, b any) bool {
	if a == nil || b == nil {
		return a == nil && b == nil
	}
	x, xs := a.([]any)
	y, ys := b.([]any)
	if xs != ys {
		return false
	}
	if xs {
		if len(x) != len(y) {
			return false
		}
		for i := range x {
			if !same(x[i], y[i]) {
				return false
			}
		}
		return true
	}
	if _, m := a.(map[string]any); m {
		return false
	}
	return a == b
}

// addRow is the new row's field, and the button that opens it.
func (p *PropertiesPanel) addRow(c *ui.Context) {
	if p.isAdd {
		in := ui.TextInputBase(c, &p.adding).Placeholder("Property name").Label("New property name").AutoFocus()
		if in.Submitted() {
			key := strings.TrimSpace(p.adding)
			p.isAdd, p.adding = false, ""
			if key != "" {
				p.set(key, "")
			}
		}
		return
	}
	if frostui.LabeledButton(c, "list-add", "Add property", false).Clicked() {
		p.Add()
	}
}
