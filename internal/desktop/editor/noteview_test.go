package editor

import (
	"testing"

	"github.com/egoist/mygo/ui"

	"github.com/frostyard/frostnotes/internal/desktop/notes"
)

func openView(t *testing.T, content string) (*editorFixture, *NoteView, *ui.Tester) {
	t.Helper()
	f := openEditor(t, "Plan.md", content)
	v := NewNoteView(f.ed)
	tt := ui.NewTester(func(c *ui.Context) { v.View(c) }, 800, 600)
	return f, v, tt
}

func TestTheInlineTitle(t *testing.T) {
	_, v, tt := openView(t, "text\n")
	var renamed string
	v.Rename = func(s string) { renamed = s }
	if err := tt.Click("Note title"); err != nil {
		t.Fatal(err)
	}
	tt.Key(ui.Cmd, ui.KeyA)
	tt.Type("Better plan")
	tt.Key(0, ui.KeyEnter)
	if renamed != "Better plan" {
		t.Fatalf("renamed to %q", renamed)
	}
}

func TestThePropertiesPanel(t *testing.T) {
	f, v, tt := openView(t, "---\nstatus: draft\ndone: false\ntags:\n  - a\n---\nbody\n")
	tt.Frame()
	if !tt.HasText("Properties") {
		t.Fatalf("no panel: %q", tt.Texts())
	}
	// A text value, committed with Enter, rewrites only its line.
	if err := tt.Click("status"); err != nil {
		t.Fatal(err)
	}
	tt.Key(ui.Cmd, ui.KeyA)
	tt.Type("final")
	tt.Key(0, ui.KeyEnter)
	if f.doc.Text() != "---\nstatus: final\ndone: false\ntags:\n  - a\n---\nbody\n" {
		t.Fatalf("after editing: %q", f.doc.Text())
	}
	if err := tt.Click("done"); err != nil {
		t.Fatal(err)
	}
	if f.doc.Text() != "---\nstatus: final\ndone: true\ntags:\n  - a\n---\nbody\n" {
		t.Fatalf("after checking: %q", f.doc.Text())
	}
	if err := tt.Click("Remove status"); err != nil {
		t.Fatal(err)
	}
	if f.doc.Text() != "---\ndone: true\ntags:\n  - a\n---\nbody\n" {
		t.Fatalf("after removing: %q", f.doc.Text())
	}
	// Ctrl + ; adds a row; undo takes the last edit back.
	v.Properties.Add()
	tt.Frame()
	tt.Type("owner")
	tt.Key(0, ui.KeyEnter)
	if f.doc.Text() != "---\ndone: true\ntags:\n  - a\nowner: \"\"\n---\nbody\n" {
		t.Fatalf("after adding: %q", f.doc.Text())
	}
	if !v.Editor.Undo() {
		t.Fatal("no undo")
	}
	f.loop.Advance(notes.SaveDelay)
	f.loop.Until(t, "the save", func() bool { return f.doc.Status() == notes.StatusClean })
	if got := f.Read("Plan.md").Content; got != "---\ndone: true\ntags:\n  - a\n---\nbody\n" {
		t.Fatalf("file holds %q", got)
	}
}

func TestANewTabStartsAfterTheFrontmatter(t *testing.T) {
	content := "---\ntags: [a]\n---\nbody\n"
	_, v, tt := openView(t, content)
	v.Editor.State.Select(0, 0)
	v.placed = ""
	tt.Frame()
	tt.Frame()
	if got, want := v.Editor.State.Cursor(), len("---\ntags: [a]\n---\n"); got != want {
		t.Fatalf("the caret is at %d, want %d", got, want)
	}
	rs := v.Editor.State.Replacements()
	if len(rs) != 1 || rs[0].Start != 0 || rs[0].End != len("---\ntags: [a]\n---\n") {
		t.Fatalf("replacements %+v", rs)
	}
}

func TestANoteWithoutProperties(t *testing.T) {
	_, _, tt := openView(t, "just text\n")
	tt.Frame()
	if tt.HasText("Properties") {
		t.Fatalf("a panel for a note without frontmatter: %q", tt.Texts())
	}
}
