package editor

import (
	"fmt"
	"strings"
	"testing"

	"github.com/egoist/mygo/ui"

	"github.com/frostyard/frostnotes/internal/desktop/desktoptest"
	"github.com/frostyard/frostnotes/internal/desktop/notes"
	"github.com/frostyard/frostnotes/internal/desktop/reading"
)

// longNote is a note of n lines in the shapes of the user's notes:
// headings, lists, tasks, links, tags, code and quotes, as M0's spike's.
func longNote(n int) string {
	shapes := []string{
		"## Section %d",
		"Some prose with a [[Link %d|alias]] and a #tag/%d, **bold** and *italic* words.",
		"- a list item %d with `code`",
		"- [ ] a task %d",
		"> a quote line %d",
		"A plain line of text, number %d, long enough to wrap in a narrow window on a laptop screen.",
	}
	var b strings.Builder
	for i := range n {
		fmt.Fprintf(&b, shapes[i%len(shapes)]+"\n", i)
	}
	return b.String()
}

// BenchmarkTypingInALongNote measures a keystroke in a 10,000-line note:
// MyGo's editor inserts it and draws a frame (on the CPU, in the Tester),
// the binding reports it, the document takes it and, in live preview, the
// marks are worked out again. M3's target is under 16 ms (plan 0009,
// Phases 2 and 5).
func BenchmarkTypingInALongNote(b *testing.B) {
	b.Run("source", func(b *testing.B) { benchTyping(b, false) })
	b.Run("live", func(b *testing.B) { benchTyping(b, true) })
}

func benchTyping(b *testing.B, live bool) {
	n := desktoptest.StartNotesd(b)
	loop := desktoptest.NewLoop()
	blocks := strings.Repeat("> [!note] A callout\n> its text\n\n| a | b |\n|---|---|\n| 1 | 2 |\n\n```go\nx := 1\n```\n\n", 100)
	content := longNote(10000-strings.Count(blocks, "\n")) + blocks
	n.Create("Long.md", content)
	d := notes.NewDocument(loop, func() notes.NoteAPI { return n.AppNotes() }, n.Vault, "Long.md")
	d.Load()
	loop.Until(b, "the note to load", d.Loaded)
	ed := NewNoteEditor(Bind(d))
	ed.Prefs.Live = live
	if live {
		ed.Rendered = reading.NewBlocks(loop, func() reading.Renderer { return n.AppNotes() }, d, n.Blobs)
	}
	tt := ui.NewTester(func(c *ui.Context) { ed.View(c).Fill() }, 1200, 800)
	if err := tt.Click("Note"); err != nil {
		b.Fatal(err)
	}
	mid := len(content) / 2
	ed.State.Select(mid, mid)
	tt.Frame()
	for b.Loop() {
		tt.Type("x")
		tt.Frame()
	}
	b.StopTimer()
	if got, want := len(d.Text()), len(content)+b.N; got != want {
		b.Fatalf("document length = %d, want %d after %d keystrokes", got, want, b.N)
	}
}
