package history

import (
	"testing"

	"github.com/egoist/mygo/ui"

	"github.com/frostyard/frostnotes/internal/desktop/desktoptest"
	"github.com/frostyard/frostnotes/internal/desktop/editor"
	"github.com/frostyard/frostnotes/internal/desktop/notes"
)

type fixture struct {
	*desktoptest.Notesd
	loop *desktoptest.Loop
	doc  *notes.Document
}

func open(t *testing.T) *fixture {
	t.Helper()
	f := &fixture{Notesd: desktoptest.StartNotesd(t), loop: desktoptest.NewLoop()}
	base := f.Create("Plan.md", "first\nline two\n")
	f.Write("Plan.md", base, "second\nline two\n")
	f.doc = notes.NewDocument(f.loop, f.notes, f.Vault, "Plan.md")
	f.doc.Load()
	f.loop.Until(t, "the note to load", f.doc.Loaded)
	return f
}

func (f *fixture) notes() notes.NoteAPI {
	if c := f.AppNotes(); c != nil {
		return c
	}
	return nil
}

func (f *fixture) history() API {
	if c := f.AppNotes(); c != nil {
		return c
	}
	return nil
}

func TestTheHistory(t *testing.T) {
	f := open(t)
	h := New(f.loop, f.history, nil, f.doc)
	f.loop.Until(t, "the versions", func() bool { return len(h.Entries()) >= 2 && h.Content() != "" })
	if h.Content() != "second\nline two\n" {
		t.Fatalf("the newest version holds %q", h.Content())
	}
	tt := ui.NewTester(func(c *ui.Context) { h.View(c) }, 900, 500)
	h.Select(1)
	f.loop.Until(t, "the older version", func() bool { return h.Content() == "first\nline two\n" })
	tt.Frame()
	if !tt.HasText("Restore this version") {
		t.Fatalf("no restore button: %q", tt.Texts())
	}
	if err := tt.Click("Show changes"); err != nil {
		t.Fatal(err)
	}
	if !tt.HasText("Hide changes") {
		t.Fatalf("changes not shown: %q", tt.Texts())
	}
	if err := tt.Click("Restore this version"); err != nil {
		t.Fatal(err)
	}
	f.loop.Until(t, "the restore", func() bool { return f.Read("Plan.md").Content == "first\nline two\n" })
	f.loop.Until(t, "the document to follow", func() bool { return f.doc.Text() == "first\nline two\n" })
}

func TestTheMergeView(t *testing.T) {
	f := open(t)
	b := editor.Bind(f.doc)
	m := NewMerge(b, "second\nLINE TWO\n", "Comparing with a conflict")
	tt := ui.NewTester(func(c *ui.Context) { m.View(c) }, 900, 500)
	tt.Frame()
	if n := len(m.State.Hunks()); n != 1 {
		t.Fatalf("%d hunks", n)
	}
	if !m.TakeBack(0) {
		t.Fatal("the chunk was not taken back")
	}
	if f.doc.Text() != "second\nLINE TWO\n" || !f.doc.Unsaved() {
		t.Fatalf("the note holds %q, unsaved %v", f.doc.Text(), f.doc.Unsaved())
	}
	f.loop.Advance(notes.SaveDelay)
	f.loop.Until(t, "the save", func() bool { return f.doc.Status() == notes.StatusClean })
	if got := f.Read("Plan.md").Content; got != "second\nLINE TWO\n" {
		t.Fatalf("file holds %q", got)
	}
	tt.Key(ui.Cmd, ui.KeyZ)
	if f.doc.Text() != "second\nline two\n" {
		t.Fatalf("Ctrl+Z in the merge view made %q", f.doc.Text())
	}
	done := false
	m.Done = func() { done = true }
	if err := tt.Click("Done"); err != nil || !done {
		t.Fatalf("Done: %v %v", err, done)
	}
}

func TestRestoringTwiceKeepsTheSelection(t *testing.T) {
	f := open(t)
	f.Write("Plan.md", f.Read("Plan.md").Hash, "third\nline two\n")
	f.doc.Reload()
	f.loop.Until(t, "the document", func() bool { return f.doc.Text() == "third\nline two\n" })
	h := New(f.loop, f.history, nil, f.doc)
	f.loop.Until(t, "the versions", func() bool { return len(h.Entries()) == 3 && h.Content() != "" })
	h.Select(2)
	f.loop.Until(t, "the oldest", func() bool { return h.Content() == "first\nline two\n" })
	h.Restore()
	f.loop.Until(t, "the new version", func() bool { return len(h.Entries()) == 4 })
	if got := h.Entries()[h.selected].Version.ID; got != h.Entries()[3].Version.ID {
		t.Fatalf("the selection moved to version %d", got)
	}
	f.Write("Plan.md", f.Read("Plan.md").Hash, "fourth\nline two\n")
	f.doc.Reload()
	f.loop.Until(t, "the document", func() bool { return f.doc.Text() == "fourth\nline two\n" })
	h.Restore()
	f.loop.Until(t, "the second restore", func() bool { return f.Read("Plan.md").Content == "first\nline two\n" })
}
