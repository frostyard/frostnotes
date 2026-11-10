package editor

import (
	"testing"

	"github.com/egoist/mygo/ui"

	"github.com/frostyard/frostnotes/api"
	"github.com/frostyard/frostnotes/internal/desktop/notes"
)

func TestTheConflictNotice(t *testing.T) {
	f := openEditor(t, "a.md", "line\n")
	f.Write("a.md", f.doc.Base(), "theirs\n")
	f.caret(0)
	f.ed.Run(func(text string, sels []Sel) Edit {
		cs, _ := notes.ChangesOf(len(text), notes.Change{From: 0, To: 4, Insert: "mine"})
		return Edit{Changes: cs, Sels: sels}
	})
	f.save()
	var compared int64
	tt := ui.NewTester(func(c *ui.Context) {
		Notice(c, f.doc, NoticeActions{CompareVersion: func(v int64) { compared = v }})
	}, 640, 120)
	if !tt.HasText("Compare") {
		t.Fatalf("no conflict notice: %q", tt.Texts())
	}
	if err := tt.Click("Compare"); err != nil || compared == 0 {
		t.Fatalf("Compare: %v, version %d", err, compared)
	}
	if err := tt.Click("Dismiss"); err != nil {
		t.Fatal(err)
	}
	if f.doc.Notice() != nil || tt.HasText("Compare") {
		t.Fatal("Dismiss left the notice")
	}
}

func TestTheDeletedNotice(t *testing.T) {
	f := openEditor(t, "a.md", "kept\n")
	if err := f.Other.Note().Delete(t.Context(), &api.NoteDeleteParams{VaultID: f.Vault, Path: "a.md", Base: f.doc.Base()}); err != nil {
		t.Fatal(err)
	}
	f.doc.Removed()
	closed := false
	tt := ui.NewTester(func(c *ui.Context) {
		Notice(c, f.doc, NoticeActions{Close: func() { closed = true }})
	}, 640, 120)
	if !tt.HasText("This note was deleted.") {
		t.Fatalf("no deleted notice: %q", tt.Texts())
	}
	if err := tt.Click("Restore"); err != nil {
		t.Fatal(err)
	}
	f.loop.Until(t, "the restore", func() bool { return f.doc.Notice() == nil })
	if f.Read("a.md").Content != "kept\n" || closed {
		t.Fatal("the note did not come back")
	}
}

func TestNoNoticeDrawsNothing(t *testing.T) {
	f := openEditor(t, "a.md", "fine\n")
	tt := ui.NewTester(func(c *ui.Context) { Notice(c, f.doc, NoticeActions{}) }, 640, 120)
	if len(tt.Texts()) != 0 {
		t.Fatalf("a quiet note draws %q", tt.Texts())
	}
}
