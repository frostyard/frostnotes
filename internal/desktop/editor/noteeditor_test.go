package editor

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/egoist/mygo/ui"

	"github.com/frostyard/frostnotes/internal/desktop/desktoptest"
	"github.com/frostyard/frostnotes/internal/desktop/notes"
)

type editorFixture struct {
	*desktoptest.Notesd
	t    *testing.T
	loop *desktoptest.Loop
	doc  *notes.Document
	ed   *NoteEditor
	tt   *ui.Tester
}

// openEditor opens path in an editor drawn by a headless tester, with its
// caret at the end of the note.
func openEditor(t *testing.T, path, content string) *editorFixture {
	t.Helper()
	f := &editorFixture{Notesd: desktoptest.StartNotesd(t), t: t, loop: desktoptest.NewLoop()}
	f.Create(path, content)
	f.doc = notes.NewDocument(f.loop, func() notes.NoteAPI {
		if c := f.AppNotes(); c != nil {
			return c
		}
		return nil
	}, f.Vault, path)
	f.doc.Load()
	f.loop.Until(t, "the note to load", f.doc.Loaded)
	f.ed = NewNoteEditor(Bind(f.doc))
	f.tt = ui.NewTester(func(c *ui.Context) { f.ed.View(c).Fill() }, 640, 400)
	if err := f.tt.Click("Note"); err != nil {
		t.Fatal(err)
	}
	f.caret(len(content))
	return f
}

func (f *editorFixture) caret(pos int) {
	f.ed.State.Select(pos, pos)
	f.tt.Frame()
}

func (f *editorFixture) text() string { return f.ed.Binding().Buffer().String() }

// save runs the save the document's debounce would, and waits for it.
func (f *editorFixture) save() {
	f.t.Helper()
	f.tt.Frame()
	f.loop.Advance(notes.SaveDelay)
	f.loop.Until(f.t, "the save", func() bool { return f.doc.Status() == notes.StatusClean })
	f.tt.Frame()
}

func TestTypingInTheEditorReachesTheFile(t *testing.T) {
	f := openEditor(t, "a.md", "hello\n")
	f.caret(5)
	f.tt.Type(" world")
	f.save()
	if got := f.Read("a.md").Content; got != "hello world\n" {
		t.Fatalf("file holds %q", got)
	}
}

func TestUndoInTheEditorSkipsAnotherWritersChange(t *testing.T) {
	f := openEditor(t, "a.md", "one\ntwo\n")
	f.caret(3)
	f.tt.Type(" 1")
	f.save()
	hash := f.Write("a.md", f.doc.Base(), "zero\none 1\ntwo\n")
	f.doc.Changed(hash)
	f.loop.Until(t, "the reload", func() bool { return f.doc.Base() == hash })
	f.tt.Frame()
	if f.text() != "zero\none 1\ntwo\n" {
		t.Fatalf("the editor shows %q", f.text())
	}
	f.tt.Key(ui.Cmd, ui.KeyZ)
	if f.text() != "zero\none\ntwo\n" {
		t.Fatalf("Ctrl+Z made %q", f.text())
	}
	f.tt.Key(ui.Cmd, ui.KeyZ)
	if f.text() != "zero\none\ntwo\n" {
		t.Fatalf("a second Ctrl+Z took back the other writer's line: %q", f.text())
	}
	f.tt.Key(ui.Cmd|ui.Shift, ui.KeyZ)
	if f.text() != "zero\none 1\ntwo\n" {
		t.Fatalf("Ctrl+Shift+Z made %q", f.text())
	}
	f.save()
	if got := f.Read("a.md").Content; got != "zero\none 1\ntwo\n" {
		t.Fatalf("file holds %q", got)
	}
}

func TestObsidianKeysInTheEditor(t *testing.T) {
	f := openEditor(t, "a.md", "word\n")
	f.ed.State.Select(0, 4)
	f.tt.Frame()
	f.tt.Key(ui.Cmd, ui.KeyB)
	if f.text() != "**word**\n" {
		t.Fatalf("Ctrl+B made %q", f.text())
	}
	f.tt.Key(ui.Cmd, ui.KeyZ)
	if f.text() != "word\n" {
		t.Fatalf("undo of bold made %q", f.text())
	}
	f.caret(4)
	f.tt.Key(ui.Cmd, ui.KeyL)
	if f.text() != "- [ ] word\n" {
		t.Fatalf("Ctrl+L made %q", f.text())
	}
	f.caret(10)
	f.tt.Key(0, ui.KeyEnter)
	f.tt.Type("next")
	if f.text() != "- [ ] word\n- [ ] next\n" {
		t.Fatalf("Enter in a task made %q", f.text())
	}
	f.tt.Key(ui.Cmd, ui.KeyD)
	if f.text() != "- [ ] word\n" {
		t.Fatalf("Ctrl+D made %q", f.text())
	}
	f.save()
	if got := f.Read("a.md").Content; got != "- [ ] word\n" {
		t.Fatalf("file holds %q", got)
	}
}

func TestFollowingALink(t *testing.T) {
	f := openEditor(t, "a.md", "see [[Plan#Goals|the plan]] and https://example.com/x\n")
	var got []Link
	var where []Where
	f.ed.Follow = func(l Link, w Where) {
		got = append(got, l)
		where = append(where, w)
	}
	f.caret(8)
	f.tt.Key(ui.Alt, ui.KeyEnter)
	f.caret(40)
	f.tt.Key(ui.Cmd, ui.KeyEnter)
	if len(got) != 2 {
		t.Fatalf("followed %+v", got)
	}
	if got[0].Target != "Plan" || got[0].Subpath != "#Goals" || where[0] != WhereHere {
		t.Errorf("wikilink followed as %+v %v", got[0], where[0])
	}
	if got[1].URL != "https://example.com/x" || where[1] != WhereTab {
		t.Errorf("web link followed as %+v %v", got[1], where[1])
	}
}

type fakeSources struct{ targets []Target }

func (s fakeSources) LinkTargets(query string, limit int) []Target {
	var out []Target
	for _, t := range s.targets {
		if fuzzyMatch(strings.ToLower(t.Label), strings.ToLower(query)) {
			out = append(out, t)
		}
	}
	return out
}

func (fakeSources) Tags() []string { return []string{"project", "garden"} }

func (fakeSources) Subpaths(_ string, blocks bool, done func([]Target)) {
	if blocks {
		done([]Target{{Insert: "abc", Label: "abc", Detail: "a line"}})
		return
	}
	done([]Target{{Insert: "Goals", Label: "Goals"}})
}

func TestCompletingALink(t *testing.T) {
	f := openEditor(t, "a.md", "")
	f.ed.Sources = fakeSources{targets: []Target{{Insert: "Garden plan", Label: "Garden plan"}, {Insert: "Journal/2026-11-10", Label: "2026-11-10", Detail: "Journal"}}}
	for _, k := range []string{"[", "[", "g", "a", "r", "d"} {
		f.tt.Type(k) // a key at a time, as a person types
	}
	f.tt.Frame()
	if !f.tt.HasText("Garden plan") {
		t.Fatalf("no completion of [[: %q", f.tt.Texts())
	}
	f.tt.Key(0, ui.KeyEnter)
	if f.text() != "[[Garden plan]]" {
		t.Fatalf("the completion made %q", f.text())
	}
}

func TestAMixedLineEndingNoteIsReadOnly(t *testing.T) {
	f := openEditor(t, "a.md", "one\r\ntwo\n")
	f.tt.Type("typed")
	f.tt.Frame()
	if strings.Contains(f.text(), "typed") || f.doc.Unsaved() {
		t.Fatalf("typed into a read-only note: %q", f.text())
	}
	if raw, _ := os.ReadFile(filepath.Join(f.Dir, "a.md")); string(raw) != "one\r\ntwo\n" {
		t.Fatalf("the file changed: %q", raw)
	}
}

func TestCompletingAHeading(t *testing.T) {
	f := openEditor(t, "a.md", "")
	f.ed.Sources = fakeSources{}
	for _, k := range []string{"[", "[", "P", "#"} {
		f.tt.Type(k)
	}
	f.tt.Frame()
	if !f.tt.HasText("Goals") {
		t.Fatalf("no completion of a heading: %q", f.tt.Texts())
	}
	f.tt.Key(0, ui.KeyEnter)
	if f.text() != "[[P#Goals]]" {
		t.Fatalf("the completion made %q", f.text())
	}
}
