package editor

import (
	"fmt"
	"os"
	"strconv"
	"strings"
	"testing"
	"time"

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

// localTypingBudget is the typing gate on a developer machine: a keystroke
// in a 10,000-line note costs under 16 ms, one frame at 60 Hz, in source
// mode and in live preview (plan 0009, Phases 2 and 5; ADR-0043).
const localTypingBudget = 16 * time.Millisecond

// ciTypingBudget is the typing gate on CI. GitHub's runners do not have one
// CPU: in 19 runs of `main` and its branches from 2026-11-11 to 2026-11-12
// (#193), live preview measured 10.4 to 13.3 ms on an AMD EPYC 9V45, 15.1
// to 15.6 ms on an EPYC 9V74, 17.1 to 21.1 ms on an EPYC 7763 and 18.8 to
// 20.8 ms on Intel Xeon Platinum 8370C and 8573C. The slowest CPU is about twice
// the fastest, so 16 ms passed or failed by the runner a job drew. 25 ms is
// the highest figure seen, 21.1 ms, plus a margin of about a fifth for the
// spread of runs on one CPU (up to 8%) and a CPU not seen yet. On the
// slowest CPU seen, a keystroke can grow by about a fifth before CI fails; on
// the fastest, by nearly double, so the local gate stays the strict one. It
// is a separate value and never replaces localTypingBudget (ADR-0043).
const ciTypingBudget = 25 * time.Millisecond

// typingBudget is the gate for the machine the benchmark runs on: CI's when
// strconv.ParseBool reads the CI variable as true (GitHub Actions sets
// CI=true), else the local one.
func typingBudget(getenv func(string) string) time.Duration {
	if ci, err := strconv.ParseBool(getenv("CI")); err == nil && ci {
		return ciTypingBudget
	}
	return localTypingBudget
}

// BenchmarkTypingInALongNote measures a keystroke in a 10,000-line note:
// MyGo's editor inserts it and draws a frame (on the CPU, in the Tester),
// the binding reports it, the document takes it and, in live preview, the
// marks are worked out again. A keystroke over typingBudget fails the run
// (`make desktop-bench`).
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
	budget := typingBudget(os.Getenv)
	if per := b.Elapsed() / time.Duration(b.N); per > budget {
		b.Fatalf("a keystroke took %v, over the typing gate of %v (local %v, CI %v)",
			per, budget, localTypingBudget, ciTypingBudget)
	}
}
