//go:build linux && integration

package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"
)

// The typing gate on NSGo (plan 0011, Phase 2; ADR-0043): a keystroke in a
// 10,000-line note costs under localTypingBudget on a developer machine and
// under ciTypingBudget on CI, in source mode and in live preview. They are
// the MyGo client's gate's values (internal/desktop/editor,
// typing_bench_test.go), which ADR-0043 explains, and the gate's here until
// that one goes.
const (
	localTypingBudget = 16 * time.Millisecond
	ciTypingBudget    = 25 * time.Millisecond
)

// typingBudget is the gate for the machine the benchmark runs on: CI's when
// strconv.ParseBool reads the CI variable as true, else the local one.
func typingBudget(getenv func(string) string) time.Duration {
	if ci, err := strconv.ParseBool(getenv("CI")); err == nil && ci {
		return ciTypingBudget
	}
	return localTypingBudget
}

// TestTypingBudgets holds the gate's 2 values apart, as the MyGo client's
// test does: 16 ms locally, a looser one on CI, chosen only by a true CI.
func TestTypingBudgets(t *testing.T) {
	if localTypingBudget != 16*time.Millisecond || ciTypingBudget <= localTypingBudget {
		t.Errorf("the budgets are %v locally and %v on CI", localTypingBudget, ciTypingBudget)
	}
	for ci, want := range map[string]time.Duration{"": localTypingBudget, "false": localTypingBudget, "yes": localTypingBudget, "true": ciTypingBudget, "1": ciTypingBudget} {
		if got := typingBudget(func(string) string { return ci }); got != want {
			t.Errorf("typingBudget with CI=%q = %v, want %v", ci, got, want)
		}
	}
}

// longNote is a note of n lines in the shapes of the MyGo client's typing
// gate: headings, lists, tasks, links, tags, code and quotes.
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
		s := shapes[i%len(shapes)]
		args := make([]any, strings.Count(s, "%d"))
		for j := range args {
			args[j] = i
		}
		fmt.Fprintf(&b, s+"\n", args...)
	}
	return b.String()
}

// BenchmarkTypingInALongNote measures a keystroke in a 10,000-line note in
// the NSGo client, on a real notesd, headless (make nsgo-bench). A keystroke
// is 1 step of NSGo's test host typing 1 character: the key and the edit
// reach the focused field, the client does its work (in source mode NSGo's
// editor inserts the character, the client reports it to the document and
// the folds are worked out again; in live preview the line's field reports
// it, the document takes it and the lines are scanned again), and the host
// runs what was posted and commits the views that changed to its model of
// the window, until nothing is left. It draws no pixels: GTK's layout and
// painting are not in it, and the editor's highlighting runs off the UI
// goroutine. A keystroke over typingBudget fails the run.
func BenchmarkTypingInALongNote(b *testing.B) {
	b.Run("source", func(b *testing.B) { benchTyping(b, false) })
	b.Run("live", func(b *testing.B) { benchTyping(b, true) })
}

func benchTyping(b *testing.B, live bool) {
	blocks := strings.Repeat("> [!note] A callout\n> its text\n\n| a | b |\n|---|---|\n| 1 | 2 |\n\n```go\nx := 1\n```\n\n", 100)
	content := longNote(10000-strings.Count(blocks, "\n")) + blocks
	app, vault := startVault(b, map[string]string{"Long.md": content}, 1150, 760)
	app.WaitFor("label:Long", 10*time.Second)
	app.Press("label:Long")
	app.WaitFor("text:Long.md · Saved", 10*time.Second)
	field := source
	if live {
		// The field of the note's first line.
		app.Press("role:tab Live preview")
		app.WaitFor("label:Edit line 1", 10*time.Second)
		app.Press("label:Edit line 1")
		field = "label:Markdown line"
		app.WaitFor(field, 5*time.Second)
		app.Focus(field)
	} else {
		// The caret at the note's end, the most lines before it.
		app.Focus(field)
		app.Key("mod+end")
	}
	app.Settle()
	for b.Loop() {
		app.Type(field, "x")
	}
	b.StopTimer()
	per := b.Elapsed() / time.Duration(b.N)
	// Every keystroke reached the note.
	app.Key("mod+s")
	path := filepath.Join(vault, "Long.md")
	deadline := time.Now().Add(20 * time.Second)
	for {
		got, err := os.ReadFile(path)
		if err == nil && len(got) == len(content)+b.N {
			break
		}
		if time.Now().After(deadline) {
			b.Fatalf("the note on disk has %d bytes, want %d after %d keystrokes", len(got), len(content)+b.N, b.N)
		}
		app.Advance(100 * time.Millisecond)
	}
	budget := typingBudget(os.Getenv)
	if per > budget {
		b.Fatalf("a keystroke took %v, over the typing gate of %v (local %v, CI %v)", per, budget, localTypingBudget, ciTypingBudget)
	}
}
