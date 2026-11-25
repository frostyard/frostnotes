package markup

import (
	"fmt"
	"slices"
	"strings"
	"testing"
)

// kindsIn is the kind of each byte of text's lines, through Highlight of
// lines from to to.
func kindsIn(text string, from, to int) [][]Kind {
	lines := strings.Split(text, "\n")
	toks := Highlight(lines, from, to)
	out := make([][]Kind, len(toks))
	for i, ts := range toks {
		out[i] = make([]Kind, len(lines[from+i]))
		for _, t := range ts {
			fill(out[i], t.Start, t.End, t.Kind)
		}
	}
	return out
}

// kindAt is the kind of the first byte of sub in line i of text.
func kindAt(t *testing.T, text string, kinds [][]Kind, i int, sub string) Kind {
	t.Helper()
	line := strings.Split(text, "\n")[i]
	at := strings.Index(line, sub)
	if at < 0 {
		t.Fatalf("%q not in line %d", sub, i)
	}
	return kinds[i][at]
}

// obsidianNote holds each piece of syntax source mode colors, as the MyGo
// client's TestObsidianSyntax does, and Markdown's own.
var obsidianNote = strings.Join([]string{
	"---",                                 // 0
	"tags: [a, b]",                        // 1
	"---",                                 // 2
	"# Title with #tag",                   // 3
	"See [[Other note|alias]] and #tag.",  // 4
	"An ==highlight== and $x^2$ math.",    // 5
	"> [!note] A callout",                 // 6
	"Text %%hidden%% shown ^block-1",      // 7
	"%% open",                             // 8
	"still hidden",                        // 9
	"closed %% after",                     // 10
	"```go",                               // 11
	"x := [[not a link]] // #nottag",      // 12
	"```",                                 // 13
	"$$",                                  // 14
	"\\int x",                             // 15
	"$$",                                  // 16
	"KindFootnote[^1] and ![[image.png]]", // 17
	"Issue #123 is not a tag",             // 18
	"Some **bold**, *italic* and ~~gone~~ text.",        // 19
	"A `code #span` and [a link](https://example.com).", // 20
	"- [ ] a task",                     // 21
	"[^1]: the note",                   // 22
	"***",                              // 23
	"snake_case_name and _under_",      // 24
	"![alt](pic.png) at <https://a.b>", // 25
}, "\n")

func TestHighlightObsidianSyntax(t *testing.T) {
	text := obsidianNote
	k := kindsIn(text, 0, strings.Count(text, "\n")+1)
	cases := []struct {
		line int
		sub  string
		want Kind
	}{
		{0, "---", KindFrontmatter},
		{1, "tags", KindProperty},
		{1, "[a", KindValue},
		{2, "---", KindFrontmatter},
		{3, "# Title", KindHeading},
		{3, "#tag", KindTag},
		{4, "[[Other", KindWikilink},
		{4, "alias]]", KindWikilink},
		{4, "#tag", KindTag},
		{4, "See", KindNone},
		{5, "==high", KindHighlight},
		{5, "$x", KindMath},
		{6, ">", KindQuote},
		{6, "[!note]", KindCallout},
		{7, "%%hidden", KindComment},
		{7, "shown", KindNone},
		{7, "^block-1", KindBlockID},
		{8, "open", KindComment},
		{9, "still", KindComment},
		{10, "closed", KindComment},
		{10, "after", KindNone},
		{11, "```go", KindCode},
		{12, "x :=", KindCode},
		{12, "[[not", KindCode},
		{12, "#nottag", KindCode},
		{13, "```", KindCode},
		{14, "$$", KindMath},
		{15, "int", KindMath},
		{16, "$$", KindMath},
		{17, "[^1]", KindFootnote},
		{17, "![[image", KindEmbed},
		{18, "#123", KindNone},
		{19, "**", KindMarks},
		{19, "bold", KindStrong},
		{19, "*italic", KindMarks},
		{19, "italic", KindEmphasis},
		{19, "~~", KindMarks},
		{19, "gone", KindStrike},
		{20, "`code", KindCode},
		{20, "#span", KindCode},
		{20, "[a link]", KindLink},
		{20, "(https", KindURL},
		{21, "- [ ]", KindList},
		{21, "a task", KindNone},
		{22, "[^1]", KindFootnote},
		{22, "the note", KindNone},
		{23, "***", KindRule},
		{24, "case", KindNone},
		{24, "under", KindEmphasis},
		{25, "![alt]", KindEmbed},
		{25, "<https", KindURL},
	}
	for _, c := range cases {
		if got := kindAt(t, text, k, c.line, c.sub); got != c.want {
			t.Errorf("line %d %q: kind %v, want %v", c.line, c.sub, got, c.want)
		}
	}
}

// Highlight reads what is open from the lines before from: a job for the
// lines in view gives them what a job from the start does.
func TestHighlightFromAnyLine(t *testing.T) {
	text := obsidianNote
	n := strings.Count(text, "\n") + 1
	whole := kindsIn(text, 0, n)
	for from := range n {
		part := kindsIn(text, from, min(from+3, n))
		for i, ks := range part {
			if !slices.Equal(ks, whole[from+i]) {
				t.Errorf("line %d from %d: %v, from 0: %v", from+i, from, ks, whole[from+i])
			}
		}
	}
	if got := Highlight(strings.Split(text, "\n"), n-1, n+5); len(got) != 1 {
		t.Errorf("a job past the end gives %d lines, want 1", len(got))
	}
	if got := Highlight(nil, 0, 3); got != nil {
		t.Errorf("no lines give %v", got)
	}
}

func TestHighlightFrontmatterOnlyOnTheFirstLine(t *testing.T) {
	text := "Intro\n---\nkey: value"
	if got := kindAt(t, text, kindsIn(text, 0, 3), 2, "key"); got == KindProperty {
		t.Fatal("a rule after the first line opened frontmatter")
	}
}

func TestHighlightIndentedCode(t *testing.T) {
	text := "Text\n\n    code #notatag\n- item\n    continued #tag"
	k := kindsIn(text, 0, 5)
	if got := kindAt(t, text, k, 2, "code"); got != KindCode {
		t.Errorf("indented code after a blank line is %v", got)
	}
	if got := kindAt(t, text, k, 4, "#tag"); got != KindTag {
		t.Errorf("a list item's indented line is %v", got)
	}
}

func TestHighlightTagsIncludeCombiningMarks(t *testing.T) {
	text := "#café/note!"
	kinds := kindsIn(text, 0, 1)[0]
	for i := 0; i < len(text)-1; i++ {
		if kinds[i] != KindTag {
			t.Fatalf("byte %d has kind %v", i, kinds[i])
		}
	}
	if kinds[len(text)-1] == KindTag {
		t.Fatal("punctuation included in tag")
	}
}

func TestFolds(t *testing.T) {
	text := strings.Join([]string{
		"---",        // 0
		"a: 1",       // 1
		"---",        // 2
		"# One",      // 3
		"text",       // 4
		"## Two",     // 5
		"more",       // 6
		"",           // 7
		"# Three",    // 8
		"- item",     // 9
		"  - nested", // 10
		"  - nested", // 11
		"- item",     // 12
		"```",        // 13
		"# not",      // 14
		"```",        // 15
		"",           // 16
	}, "\n")
	want := []Fold{
		{0, 2},  // the frontmatter
		{3, 6},  // up to the next level-1 heading, without the blank line
		{5, 6},  // a subsection
		{8, 15}, // to the end, without the blank line
		{9, 11}, // a list item's nested items
	}
	if got := Folds(text); !slices.Equal(got, want) {
		t.Errorf("Folds = %v, want %v", got, want)
	}
	if got := Folds(""); len(got) != 0 {
		t.Errorf("an empty note folds %v", got)
	}
}

func TestOffsetAndLineCol(t *testing.T) {
	text := "ab\n🌱x\n\nend"
	cases := []struct{ line, col, off int }{
		{0, 0, 0}, {0, 2, 2}, {1, 0, 3}, {1, 2, 7}, {1, 3, 8}, {2, 0, 9}, {3, 3, 13},
	}
	for _, c := range cases {
		if got := Offset(text, c.line, c.col); got != c.off {
			t.Errorf("Offset(%d, %d) = %d, want %d", c.line, c.col, got, c.off)
		}
		if l, col := LineCol(text, c.off); l != c.line || col != c.col {
			t.Errorf("LineCol(%d) = %d, %d; want %d, %d", c.off, l, col, c.line, c.col)
		}
	}
	// Clamped: past a line's end, past the last line, inside a pair.
	if got := Offset(text, 0, 9); got != 2 {
		t.Errorf("a column past the line's end is at %d", got)
	}
	if got := Offset(text, 9, 0); got != 10 {
		t.Errorf("a line past the end is at %d", got)
	}
	if got := Offset(text, 1, 1); got != 3 {
		t.Errorf("a column inside a surrogate pair is at %d", got)
	}
	if l, col := LineCol(text, 99); l != 3 || col != 3 {
		t.Errorf("LineCol past the end = %d, %d", l, col)
	}
}

// BenchmarkHighlightView measures a job for the lines in view at the end
// of a 10,000-line note, the lines before it read for what is open.
func BenchmarkHighlightView(b *testing.B) {
	lines := strings.Split(longNote(10000), "\n")
	for b.Loop() {
		Highlight(lines, len(lines)-60, len(lines))
	}
}

// BenchmarkFolds measures the folds of a 10,000-line note.
func BenchmarkFolds(b *testing.B) {
	text := longNote(10000)
	for b.Loop() {
		Folds(text)
	}
}

// longNote is a note of n lines in the shapes of the typing gate's note
// (internal/desktop/editor/typing_bench_test.go).
func longNote(n int) string {
	shapes := []string{
		"## Section %d",
		"Some prose with a [[KindLink %d|alias]] and a #tag/%d, **bold** and *italic* words.",
		"- a list item %d with `code`",
		"- [ ] a task %d",
		"> a quote line %d",
		"A plain line of text, number %d, long enough to wrap in a narrow window on a laptop screen.",
	}
	var b strings.Builder
	for i := range n {
		s := shapes[i%len(shapes)]
		fmt.Fprintf(&b, s+"\n", slices.Repeat([]any{i}, strings.Count(s, "%d"))...)
	}
	return b.String()
}
