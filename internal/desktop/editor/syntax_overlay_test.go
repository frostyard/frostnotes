package editor

import (
	"os"
	"slices"
	"strings"
	"testing"

	code "github.com/egoist/mygo/ui/editor"
)

// overlaySlow is overlay without its fast path: every pattern on every
// line, as it was. The tests hold overlay to it.
func overlaySlow(line string, kinds []code.TokenKind) (open bool) {
	free := func(s, e int) bool {
		for i := s; i < e; i++ {
			if kinds[i] == code.TokenCode || kinds[i] == code.TokenComment {
				return false
			}
		}
		return true
	}
	set := func(s, e int, k code.TokenKind) {
		for i := s; i < e; i++ {
			kinds[i] = k
		}
	}
	if m := calloutRe.FindStringSubmatchIndex(line); m != nil {
		set(m[2], m[3], code.TokenKeyword)
	}
	for _, m := range wikiRe.FindAllStringIndex(line, -1) {
		if free(m[0], m[1]) {
			set(m[0], m[1], code.TokenLink)
		}
	}
	for _, m := range footnoteRe.FindAllStringIndex(line, -1) {
		if free(m[0], m[1]) {
			set(m[0], m[1], code.TokenLink)
		}
	}
	for _, m := range highRe.FindAllStringIndex(line, -1) {
		if free(m[0], m[1]) {
			set(m[0], m[1], code.TokenInserted)
		}
	}
	for _, m := range mathRe.FindAllStringIndex(line, -1) {
		if free(m[0], m[1]) {
			set(m[0], m[1], code.TokenString)
		}
	}
	for _, t := range tagRanges(line) {
		if free(t[0], t[1]) && kinds[t[0]] != code.TokenLink {
			set(t[0], t[1], code.TokenType)
		}
	}
	if m := blockIDRe.FindStringSubmatchIndex(line); m != nil && free(m[2], m[3]) {
		set(m[2], m[3], code.TokenComment)
	}
	for i := 0; i < len(line); {
		s := strings.Index(line[i:], "%%")
		if s < 0 {
			break
		}
		s += i
		if !free(s, s+2) {
			i = s + 2
			continue
		}
		e := strings.Index(line[s+2:], "%%")
		if e < 0 {
			set(s, len(line), code.TokenComment)
			return true
		}
		e += s + 4
		set(s, e, code.TokenComment)
		i = e
	}
	return false
}

// mdKinds are the kinds MyGo's Markdown lexer gives each byte of line, as
// inline lays them out for overlay.
func mdKinds(line string) []code.TokenKind {
	sub, _ := code.LanguageNamed("Markdown").Tokenizer.Tokenize(line, 0, nil)
	kinds := make([]code.TokenKind, len(line))
	pos := 0
	for _, t := range sub {
		for ; pos < t.End && pos < len(kinds); pos++ {
			kinds[pos] = t.Kind
		}
	}
	return kinds
}

// TestOverlayKeepsTheTokens compares overlay against the same rules without
// its fast path: a line that a pattern's literal mark keeps it from trying
// must not match.
func TestOverlayKeepsTheTokens(t *testing.T) {
	lines := []string{
		// Each mark of the fast path.
		"> [!note] A callout",
		">> [!warning]- foldable",
		"See [[Other note|alias]] and ![[a.png]].",
		"See [^1] and [^note] here.",
		"An ==highlight== and ==two words== here.",
		"Math $x^2$ and $a+b$ here.",
		"Text %%hidden%% shown ^block-1",
		"A #tag and #nested/tag here.",
		// None of them.
		"A plain line of text.",
		"# Title",
		"- a list item with `code`",
		"| a | b |",
		"",
		// Edge cases: a mark, but no match.
		"a $ with no closing",
		"==",
		"== ==",
		"text [!note] without a quote",
		"> [!]",
		"^ not at the end ^here",
		"x^2 is not a block ID",
		"[[[three brackets",
		"[[unclosed",
		"[^",
		"[^ spaces are no footnote]",
		"$5 and $6 cost",
		"$$",
		"$$x$$",
		"a ==b",
		"%%open",
		"closed %% after",
		"`[[not a link]]` and `==not a highlight==`",
		`\[[escaped]] and \$x\$`,
		"#tag, not#[^1] one",
		"\\^block-1 escaped",
	}
	for _, text := range []string{longNote(600), specLines(t)} {
		for line := range strings.SplitSeq(text, "\n") {
			lines = append(lines, line)
		}
	}
	for _, line := range lines {
		wantKinds := mdKinds(line)
		gotKinds := slices.Clone(wantKinds)
		wantOpen := overlaySlow(line, wantKinds)
		gotOpen := overlay(line, gotKinds)
		if gotOpen != wantOpen {
			t.Errorf("overlay(%q) left a comment open: %v, want %v", line, gotOpen, wantOpen)
		}
		if i := firstDiff(gotKinds, wantKinds); i >= 0 {
			t.Errorf("overlay(%q) makes byte %d kind %d, want %d", line, i, gotKinds[i], wantKinds[i])
		}
	}
}

// firstDiff is where two kind slices first differ, -1 when they do not.
func firstDiff(a, b []code.TokenKind) int {
	for i := range max(len(a), len(b)) {
		if i >= len(a) || i >= len(b) || a[i] != b[i] {
			return i
		}
	}
	return -1
}

// specLines are the lines of the Markdown spec, whose examples are every
// shape the overlay reads.
func specLines(t *testing.T) string {
	t.Helper()
	b, err := os.ReadFile("../../../docs/specs/markdown.md")
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}
