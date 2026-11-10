package livepreview

import (
	"strings"
	"testing"
)

// show draws text as live preview would: hidden ranges go, and stand-ins
// are written as • ▏ ☐ ☑ and ──.
func show(text string, ps []Piece) string {
	var b strings.Builder
	at := 0
	for _, p := range ps {
		b.WriteString(text[at:p.Start])
		switch p.Kind {
		case Bullet:
			b.WriteString("•")
		case Quote:
			b.WriteString("▏")
		case Task:
			if p.Done {
				b.WriteString("☑")
			} else {
				b.WriteString("☐")
			}
		case Rule:
			b.WriteString("──")
		}
		at = p.End
	}
	b.WriteString(text[at:])
	return b.String()
}

func TestWhatShows(t *testing.T) {
	for _, tc := range []struct{ in, want string }{
		{"# Title", "Title"},
		{"###### Six", "Six"},
		{"####### seven", "####### seven"},
		{"#notatitle", "#notatitle"},
		{"Some **bold** text", "Some bold text"},
		{"Some *it* and _it_ too", "Some it and it too"},
		{"***both***", "*both*"},
		{"a ~~gone~~ b ==hi== c", "a gone b hi c"},
		{"2 * 3 * 4", "2 * 3 * 4"},
		{"snake_case_name", "snake_case_name"},
		{"\\*not\\*", "\\*not\\*"},
		{"`code **x**` y", "code **x** y"},
		{"``a`b`` c", "a`b c"},
		{"[text](https://e.com/a_(b)) after", "text after"},
		{"[**bold** link](x)", "bold link"},
		{"[[Plan]] and [[Plan#Goals|the plan]]", "Plan and the plan"},
		{"![[a.png]] stays", "![[a.png]] stays"},
		{"![alt](a.png) stays", "![alt](a.png) stays"},
		{"$x_1 * y_2$ stays", "$x_1 * y_2$ stays"},
		{"costs $5 and *more*", "costs $5 and more"},
		{"see[^1] here", "see[1] here"},
		{"text %%hidden%% more", "text  more"},
		{"A paragraph ^block-1", "A paragraph"},
		{"- item", "• item"},
		{"  * nested *it*", "  • nested it"},
		{"1. ordered", "1. ordered"},
		{"- [ ] open", "☐ open"},
		{"- [x] done", "☑ done"},
		{"1. [x] ordered", "1. ☑ ordered"},
		{"> quoted **b**", "▏quoted b"},
		{"> # title", "▏title"},
		{"---", "──"},
		{"* * *", "──"},
		{"no marks", "no marks"},
	} {
		var s Scanner
		if got := show(tc.in, s.Pieces(tc.in, nil, false)); got != tc.want {
			t.Errorf("%q shows %q, want %q", tc.in, got, tc.want)
		}
	}
}

func TestBlocksKeepTheirSource(t *testing.T) {
	for _, tc := range []struct{ name, in, want string }{
		{"fence", "```go\n**x**\n```\n*y*", "```go\n**x**\n```\ny"},
		{"tilde fence", "~~~\n# x\n~~~", "~~~\n# x\n~~~"},
		{"longer fence", "````\n```\n*x*\n````\n*y*", "````\n```\n*x*\n````\ny"},
		{"math", "$$\n*x*\n$$\n*y*", "$$\n*x*\n$$\ny"},
		{"indented code", "para\n\n    **x**", "para\n\n    **x**"},
		{"setext", "Title\n---", "Title\n---"},
		{"rule after a gap", "Title\n\n---", "Title\n\n──"},
		{"frontmatter", "---\na: *b*\n---\n*c*", "---\na: *b*\n---\nc"},
		{"comment", "a\n%%\nsecret\n%%\nb", "a\nb"},
		{"comment that runs on", "%%\nnever closed *x*", "%%\nnever closed *x*"},
	} {
		var s Scanner
		if got := show(tc.in, s.Pieces(tc.in, nil, false)); got != tc.want {
			t.Errorf("%s: %q shows %q, want %q", tc.name, tc.in, got, tc.want)
		}
	}
}

func TestTheFrontmatterHidesForThePanel(t *testing.T) {
	var s Scanner
	in := "---\na: b\n---\nbody"
	if got := show(in, s.Pieces(in, nil, true)); got != "body" {
		t.Fatalf("shows %q", got)
	}
	bom := "\uFEFF" + in
	ps := s.Pieces(bom, nil, true)
	if got := show(bom, ps); got != "body" {
		t.Fatalf("with a byte order mark, shows %q", got)
	}
	if got := show(in, s.Pieces(in, func(l int) bool { return l == 1 }, true)); got != in {
		t.Fatalf("a caret in the frontmatter shows %q", got)
	}
}

func TestASelectionsLinesShowTheirSource(t *testing.T) {
	var s Scanner
	in := "# One\n**two**\n- three\n"
	got := show(in, s.Pieces(in, func(l int) bool { return l == 1 }, false))
	if got != "One\n**two**\n• three\n" {
		t.Fatalf("shows %q", got)
	}
	in = "a\n%%\nb\n%%\nc"
	if got := show(in, s.Pieces(in, func(l int) bool { return l == 2 }, false)); got != in {
		t.Fatalf("a caret in a comment shows %q", got)
	}
}

func TestPiecesAreSortedAndApart(t *testing.T) {
	var s Scanner
	for _, in := range []string{
		"**a *b* c** [x **y**](z) `q` ==h== ~~s~~ [[l|m]] ^id",
		"***a*** **b* c** *",
		"[[a]]**[[b]]**[c]([[d]])",
		"> - [ ] **a** ^b\n> ---\n\r\n# c\r\n",
		"* * * * *a",
		"[[[[]]]] ]] [ ( ) ] (",
		"\\", "`", "$", "[", "*", "=", "~", "%%", "![", "[[", "[^",
	} {
		end := -1
		for _, p := range s.Pieces(in, nil, true) {
			if p.Start < end || p.End < p.Start || p.End > len(in) {
				t.Errorf("%q: %+v after %d", in, p, end)
			}
			end = p.End
		}
	}
}

func TestLineEndingsAreLeftOut(t *testing.T) {
	var s Scanner
	in := "# a\r\n**b**\r\n"
	if got := show(in, s.Pieces(in, nil, false)); got != "a\r\nb\r\n" {
		t.Fatalf("shows %q", got)
	}
}

func FuzzPieces(f *testing.F) {
	for _, in := range []string{"# a **b** [c](d) [[e|f]] `g`", "---\na\n---\n%%\n```\n$$\n", "- [x] *", "> *", "\uFEFF---\n"} {
		f.Add(in)
	}
	f.Fuzz(func(t *testing.T, in string) {
		var s Scanner
		end := -1
		for _, p := range s.Pieces(in, nil, true) {
			if p.Start < end || p.End < p.Start || p.End > len(in) {
				t.Fatalf("%q: %+v after %d", in, p, end)
			}
			end = p.End
		}
	})
}

func BenchmarkAKeystrokeInALongNote(b *testing.B) {
	var lines []string
	for i := range 10000 {
		switch i % 5 {
		case 0:
			lines = append(lines, "## Heading number one")
		case 1:
			lines = append(lines, "Some **bold** and *it* text with a [[Link|alias]] and `code`.")
		case 2:
			lines = append(lines, "- [ ] a task with [a link](https://example.com/x)")
		case 3:
			lines = append(lines, "> a quote ==marked==")
		default:
			lines = append(lines, "")
		}
	}
	text := strings.Join(lines, "\n")
	var s Scanner
	s.Pieces(text, nil, false)
	i := 0
	for b.Loop() {
		i++
		changed := text[:len(text)/2] + string(rune('a'+i%26)) + text[len(text)/2:]
		s.Pieces(changed, func(l int) bool { return l == 5000 }, false)
	}
}
