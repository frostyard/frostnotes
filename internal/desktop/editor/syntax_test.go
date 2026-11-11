package editor

import (
	"strings"
	"testing"

	code "github.com/egoist/mygo/ui/editor"
)

// kindsOf tokenizes text line by line and returns, for each line, the
// kind at each byte.
func kindsOf(text string) [][]code.TokenKind {
	tok := Language().Tokenizer
	var state uint64
	var out [][]code.TokenKind
	for line := range strings.SplitSeq(text, "\n") {
		toks, next := tok.Tokenize(line, state, nil)
		state = next
		kinds := make([]code.TokenKind, len(line))
		pos := 0
		for _, t := range toks {
			for ; pos < t.End && pos < len(kinds); pos++ {
				kinds[pos] = t.Kind
			}
		}
		out = append(out, kinds)
	}
	return out
}

// kindAt is the kind of the first byte of sub in line i.
func kindAt(t *testing.T, text string, kinds [][]code.TokenKind, i int, sub string) code.TokenKind {
	t.Helper()
	line := strings.Split(text, "\n")[i]
	at := strings.Index(line, sub)
	if at < 0 {
		t.Fatalf("%q not in line %d", sub, i)
	}
	return kinds[i][at]
}

func TestObsidianSyntax(t *testing.T) {
	text := strings.Join([]string{
		"---",                                // 0
		"tags: [a, b]",                       // 1
		"---",                                // 2
		"# Title",                            // 3
		"See [[Other note|alias]] and #tag.", // 4
		"An ==highlight== and $x^2$ math.",   // 5
		"> [!note] A callout",                // 6
		"Text %%hidden%% shown ^block-1",     // 7
		"%% open",                            // 8
		"still hidden",                       // 9
		"closed %% after",                    // 10
		"```go",                              // 11
		"x := [[not a link]] // #nottag",     // 12
		"```",                                // 13
		"$$",                                 // 14
		"\\int x",                            // 15
		"$$",                                 // 16
		"Footnote[^1] and ![[image.png]]",    // 17
		"Issue #123 is not a tag",            // 18
	}, "\n")
	k := kindsOf(text)
	cases := []struct {
		line int
		sub  string
		want code.TokenKind
	}{
		{0, "---", code.TokenPunctuation},
		{1, "tags", code.TokenProperty},
		{1, "[a", code.TokenString},
		{3, "Title", code.TokenHeading},
		{4, "[[Other", code.TokenLink},
		{4, "alias]]", code.TokenLink},
		{4, "#tag", code.TokenType},
		{5, "==high", code.TokenInserted},
		{5, "$x", code.TokenString},
		{6, "[!note]", code.TokenKeyword},
		{7, "%%hidden", code.TokenComment},
		{7, "shown", code.TokenText},
		{7, "^block-1", code.TokenComment},
		{8, "open", code.TokenComment},
		{9, "still", code.TokenComment},
		{10, "closed", code.TokenComment},
		{10, "after", code.TokenText},
		{15, "int", code.TokenString},
		{17, "[^1]", code.TokenLink},
		{17, "![[image", code.TokenLink},
		{18, "#123", code.TokenText},
	}
	for _, c := range cases {
		if got := kindAt(t, text, k, c.line, c.sub); got != c.want {
			t.Errorf("line %d %q: kind %v, want %v", c.line, c.sub, got, c.want)
		}
	}
	// Inside a fenced block nothing is Obsidian's.
	for _, sub := range []string{"[[not", "#nottag"} {
		if got := kindAt(t, text, k, 12, sub); got == code.TokenLink || got == code.TokenType {
			t.Errorf("fenced %q highlighted as %v", sub, got)
		}
	}
}

func TestFrontmatterOnlyOnTheFirstLine(t *testing.T) {
	text := "Intro\n---\nkey: value"
	k := kindsOf(text)
	if got := kindAt(t, text, k, 2, "key"); got == code.TokenProperty {
		t.Fatal("a rule after the first line opened frontmatter")
	}
}

func TestFolding(t *testing.T) {
	text := strings.Join([]string{
		"# One",      // 0
		"text",       // 1
		"## Two",     // 2
		"more",       // 3
		"",           // 4
		"# Three",    // 5
		"- item",     // 6
		"  - nested", // 7
		"  - nested", // 8
		"- item",     // 9
		"```",        // 10
		"# not",      // 11
		"```",        // 12
	}, "\n")
	buf := code.NewBuffer(text)
	h := Language().Highlighter(buf.Snapshot(), func() {}).(*highlighter)
	h.Highlight(buf.LineCount(), 1e9)
	cases := []struct {
		line, end int
		ok        bool
	}{
		{0, 3, true},  // up to the next level-1 heading, without the blank line
		{2, 3, true},  // a subsection
		{5, 12, true}, // to the end
		{6, 8, true},  // a list item's nested items
		{7, 0, false},
		{11, 0, false}, // a '#' in a fenced block is no heading
	}
	for _, c := range cases {
		end, ok := h.FoldEnd(c.line)
		if ok != c.ok || ok && end != c.end {
			t.Errorf("FoldEnd(%d) = %d, %v; want %d, %v", c.line, end, ok, c.end, c.ok)
		}
	}
}

func TestTagsIncludeCombiningMarks(t *testing.T) {
	text := "#cafe\u0301/note!"
	kinds := kindsOf(text)[0]
	for i := 0; i < len(text)-1; i++ {
		if kinds[i] != code.TokenType {
			t.Fatalf("byte %d has kind %v", i, kinds[i])
		}
	}
	if kinds[len(text)-1] == code.TokenType {
		t.Fatal("punctuation included in tag")
	}
}
