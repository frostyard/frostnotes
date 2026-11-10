package notes

import (
	"testing"
	"time"

	"github.com/frostyard/frostnotes/internal/desktop/desktoptest"
)

func TestHistoryJoinsTyping(t *testing.T) {
	var h History
	text := ""
	at := desktoptest.Epoch
	typeAt := func(pos int, s string, after time.Duration) {
		t.Helper()
		cs := mustChanges(t, len(text), Change{From: pos, To: pos, Insert: s})
		at = at.Add(after)
		h.Record(cs, text, at)
		text = mustApply(t, cs, text)
	}
	typeAt(0, "h", 0)
	typeAt(1, "i", 100*time.Millisecond)
	typeAt(2, "!", 100*time.Millisecond)
	typeAt(3, " again", time.Second) // a pause: a new step
	undo, ok := h.Undo(text)
	if !ok {
		t.Fatal("nothing to undo")
	}
	text = mustApply(t, undo, text)
	if text != "hi!" {
		t.Fatalf("first undo made %q", text)
	}
	undo, _ = h.Undo(text)
	text = mustApply(t, undo, text)
	if text != "" || h.CanUndo() {
		t.Fatalf("second undo made %q", text)
	}
	redo, _ := h.Redo(text)
	if text = mustApply(t, redo, text); text != "hi!" {
		t.Fatalf("redo made %q", text)
	}
}

func TestHistoryMovesOverRemoteChanges(t *testing.T) {
	var h History
	text := "abc"
	cs := mustChanges(t, 3, Change{From: 3, To: 3, Insert: "X"})
	h.Record(cs, text, desktoptest.Epoch)
	text = mustApply(t, cs, text) // abcX
	remote := mustChanges(t, 4, Change{From: 0, To: 0, Insert: "123"})
	h.Remote(remote)
	text = mustApply(t, remote, text) // 123abcX
	undo, ok := h.Undo(text)
	if !ok {
		t.Fatal("nothing to undo")
	}
	if text = mustApply(t, undo, text); text != "123abc" {
		t.Fatalf("undo made %q", text)
	}
	// A new edit clears redo.
	cs = mustChanges(t, len(text), Change{From: 0, To: 1})
	h.Record(cs, text, desktoptest.Epoch.Add(time.Hour))
	if h.CanRedo() {
		t.Fatal("redo survived a new edit")
	}
}
