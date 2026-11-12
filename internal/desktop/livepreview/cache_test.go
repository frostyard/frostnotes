package livepreview

import (
	"fmt"
	"strings"
	"testing"
)

// mixedNote is a note of n lines in the shapes of the user's notes, as
// longNote is in the editor's typing benchmark: headings, prose with links
// and tags, list items, tasks and quotes.
func mixedNote(n int) string {
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

// maxScanAllocs bounds what a Scan of a cached note may allocate: 3, the
// result's Pieces, Styles and Code, measured on a note of 2,000 lines. The
// bound leaves room for a result slice of another kind, and stays far below
// the thousands a line of its own would cost.
const maxScanAllocs = 8

// TestASecondScanOfAnEditAllocatesLittle guards the line cache: a keystroke
// scans the whole note again, and must cost the lines it changed, not a
// piece of every line. Without the cache this test fails by an allocation
// for each line.
func TestASecondScanOfAnEditAllocatesLittle(t *testing.T) {
	note := mixedNote(2000)
	var s Scanner
	if res := s.Scan(note, Options{}); len(res.Pieces) == 0 {
		t.Fatal("the note showed nothing")
	}
	at := len(note) / 2
	edited := note[:at] + "x" + note[at:] // one keystroke, in the middle
	if res := s.Scan(edited, Options{}); len(res.Pieces) == 0 {
		t.Fatal("the edited note showed nothing")
	}
	allocs := testing.AllocsPerRun(5, func() { s.Scan(edited, Options{}) })
	if allocs > maxScanAllocs {
		t.Fatalf("a scan of a cached note of %d lines allocates %.0f times, want at most %d: the line cache or the result's sizing is not working",
			strings.Count(edited, "\n"), allocs, maxScanAllocs)
	}
}
