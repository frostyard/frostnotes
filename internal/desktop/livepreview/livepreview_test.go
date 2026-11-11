package livepreview

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"
	"unsafe"

	"github.com/frostyard/frostnotes/internal/vaultgen"
)

// show draws text as live preview would: hidden ranges go, and stand-ins
// are written as • ▏ ☐ ☑ and ──.
func show(text string, ps []Piece) string {
	var b strings.Builder
	at := 0
	for _, p := range ps {
		b.WriteString(text[at:p.Start])
		switch p.Kind {
		case Link, Tag, Footnote, Dim:
			b.WriteString(p.Label)
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
		{"see[^1] here", "see1 here"},
		{"[^1]: a definition", "[^1]: a definition"},
		{"a #tag and #nested/tag, not#one or #123", "a #tag and #nested/tag, not#one or #123"},
		{"- [x] done **well**", "☑ done well"},
		{"- [x] done [[Plan]]", "☑ done Plan"},
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
	for _, in := range []string{"# a **b** [c](d) [[e|f]] `g`", "---\na\n---\n%%\n```\n$$\n", "- [x] *", "> *", "\uFEFF---\n", "> [!a]\n> b\n", "|a|\n|-|\n|b|", "```go\nx", "![[a.png|3]] ![a](b.png) #t [^n]"} {
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
		checkScan(t, in, s.Scan(in, Options{Blocks: true, Embed: embedAll, HideFront: true}))
	})
}

// checkScan holds a scan to what MyGo is given: pieces sorted, apart and
// in range, and blocks on lines of the text.
func checkScan(t *testing.T, in string, r Result) {
	t.Helper()
	end := -1
	for _, p := range r.Pieces {
		if p.Start < end || p.End < p.Start || p.End > len(in) {
			t.Fatalf("%q: %+v after %d", in, p, end)
		}
		end = p.End
	}
	lines := strings.Count(in, "\n")
	for _, b := range r.Blocks {
		if b.Line < 0 || b.Line > lines {
			t.Fatalf("%q: %+v of %d lines", in, b, lines)
		}
	}
}

func TestEveryNoteOfAGeneratedVaultScans(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "vault")
	m, err := vaultgen.Generate(dir, vaultgen.Options{Seed: 9, Notes: 400})
	if err != nil {
		t.Fatal(err)
	}
	var s Scanner
	blocks := 0
	for _, n := range m.Notes {
		data, err := os.ReadFile(filepath.Join(dir, n))
		if err != nil {
			t.Fatal(err)
		}
		r := s.Scan(string(data), Options{Blocks: true, HideFront: true, Embed: embedAll})
		checkScan(t, string(data), r)
		blocks += len(r.Blocks)
	}
	if blocks == 0 {
		t.Fatal("the vault drew no blocks")
	}
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

func embedAll(target string, wikilink bool) EmbedKind {
	switch {
	case target == "":
		return NoEmbed
	case strings.HasSuffix(target, ".png"), strings.HasSuffix(target, ".jpg"):
		return ImageEmbed
	case wikilink && target != "Missing":
		return NoteEmbed
	}
	return NoEmbed
}

func scanBlocks(text string, revealed func(int) bool) Result {
	var s Scanner
	return s.Scan(text, Options{Revealed: revealed, Blocks: true, Embed: embedAll})
}

func TestWholeBlocksDrawInPlaceOfTheirLines(t *testing.T) {
	for _, tc := range []struct {
		name, in, shown, source, label string
		line                           int
	}{
		{"callout", "a\n> [!note] Title\n> text\nb", "a\nb", "> [!note] Title\n> text", "Callout", 1},
		{"callout at the end", "> [!tip]\n> text", "", "> [!tip]\n> text", "Callout", 0},
		{"table", "| a | b |\n|---|---|\n| 1 | 2 |\n\nafter", "\nafter", "| a | b |\n|---|---|\n| 1 | 2 |", "Table", 0},
		{"table without outer pipes", "a | b\n--- | ---\n1 | 2\n", "", "a | b\n--- | ---\n1 | 2", "Table", 0},
		{"note embed", "x\n![[Plan]]\ny", "x\ny", "![[Plan]]", "Embedded note", 1},
		{"image alone", "![[a.png]]\ny", "y", "![[a.png]]", "Image", 0},
		{"markdown image alone", "![alt](dir/a.png)\ny", "y", "![alt](dir/a.png)", "Image", 0},
	} {
		r := scanBlocks(tc.in, nil)
		if got := show(tc.in, r.Pieces); got != tc.shown {
			t.Errorf("%s: shows %q, want %q", tc.name, got, tc.shown)
		}
		if len(r.Blocks) != 1 || r.Blocks[0].Source != tc.source || r.Blocks[0].Line != tc.line || r.Blocks[0].Label != tc.label {
			t.Errorf("%s: blocks %+v", tc.name, r.Blocks)
		}
	}
}

func TestAWholeBlockShowsItsSourceWithASelection(t *testing.T) {
	for _, tc := range []struct {
		name, in string
		line     int
	}{
		{"callout", "> [!note]\n> text\n", 1},
		{"table", "| a |\n|---|\n| 1 |\n", 2},
		{"fence", "```go\nx\n```\n", 1},
		{"embed", "![[Plan]]\n", 0},
	} {
		r := scanBlocks(tc.in, func(l int) bool { return l == tc.line })
		if len(r.Blocks) != 0 {
			t.Errorf("%s: blocks %+v", tc.name, r.Blocks)
		}
	}
}

func TestWhatIsNotAWholeBlock(t *testing.T) {
	for _, tc := range []struct{ name, in string }{
		{"a plain quote", "> quote\n"},
		{"pipes without a delimiter", "a | b\nc | d\n"},
		{"a delimiter of the wrong width", "a | b\n---\n"},
		{"an unresolved embed", "![[Missing]]\n"},
		{"a note embed within a line", "see ![[Plan]] here\n"},
		{"indented code", "x\n\n    > [!note]\n"},
		{"a callout in a fence", "~~~\n> [!note]\n~~~\n"},
	} {
		r := scanBlocks(tc.in, func(l int) bool { return tc.name == "a callout in a fence" })
		if len(r.Blocks) != 0 {
			t.Errorf("%s: blocks %+v", tc.name, r.Blocks)
		}
	}
}

func TestAnImageWithinALineGoesUnderIt(t *testing.T) {
	in := "before ![[a.png|300x200]] after\nnext"
	r := scanBlocks(in, nil)
	if got := show(in, r.Pieces); got != "before  after\nnext" {
		t.Fatalf("shows %q", got)
	}
	if len(r.Blocks) != 1 || r.Blocks[0].Line != 0 || r.Blocks[0].Width != 300 || r.Blocks[0].Source != "![[a.png|300x200]]" {
		t.Fatalf("blocks %+v", r.Blocks)
	}
}

func TestAFenceGetsAHeader(t *testing.T) {
	in := "a\n```go title\nx := 1\ny\n```\nb"
	r := scanBlocks(in, nil)
	if got := show(in, r.Pieces); got != "a\nx := 1\ny\nb" {
		t.Fatalf("shows %q", got)
	}
	want := Block{Kind: CodeHeader, Line: 1, Label: "Code block", Language: "go", Code: "x := 1\ny"}
	if len(r.Blocks) != 1 || r.Blocks[0] != want {
		t.Fatalf("blocks %+v", r.Blocks)
	}
	in = "```\nnever closed\n"
	r = scanBlocks(in, nil)
	if got := show(in, r.Pieces); got != "never closed\n" {
		t.Fatalf("an open fence shows %q", got)
	}
	if len(r.Blocks) != 1 || r.Blocks[0].Code != "never closed" {
		t.Fatalf("an open fence's blocks %+v", r.Blocks)
	}
}

func TestParseEmbed(t *testing.T) {
	for _, tc := range []struct {
		in, target string
		wikilink   bool
		width      int
	}{
		{"![[a.png]]", "a.png", true, 0},
		{"![[a.png|300]]", "a.png", true, 300},
		{"![[Plan#Goals|the plan]]", "Plan", true, 0},
		{"![alt](a%20b.png)", "a b.png", false, 0},
		{"![alt|120](<a b.png> \"t\")", "a b.png", false, 120},
		{"![x](https://e.test/a.png#f)", "https://e.test/a.png#f", false, 0},
	} {
		target, wikilink, width := parseEmbed(tc.in)
		if target != tc.target || wikilink != tc.wikilink || width != tc.width {
			t.Errorf("%s: %q %v %d", tc.in, target, wikilink, width)
		}
	}
}

func TestLinksTagsAndFootnotesAreObjects(t *testing.T) {
	in := "[[Plan#Goals]] [site](https://e.test) #tag see[^n]"
	var s Scanner
	var got []Piece
	for _, p := range s.Pieces(in, nil, false) {
		got = append(got, Piece{Kind: p.Kind, Label: p.Label})
	}
	want := []Piece{{Kind: Link, Label: "Plan > Goals"}, {Kind: Link, Label: "site"}, {Kind: Tag, Label: "#tag"}, {Kind: Footnote, Label: "n"}}
	if !slices.Equal(got, want) {
		t.Fatalf("%+v", got)
	}
	long := "[[" + strings.Repeat("x", maxObject+1) + "]]"
	for _, p := range s.Pieces(long, nil, false) {
		if p.Kind != Hide {
			t.Fatalf("a long link drew as an object: %+v", p)
		}
	}
}

func TestNestedLinkTextIsLinear(t *testing.T) {
	line := strings.Repeat("[a ", 40) + "b" + strings.Repeat("](c)", 40)
	done := make(chan struct{})
	go func() {
		var s Scanner
		s.Pieces(line, nil, false)
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(10 * time.Second):
		t.Fatal("nested links took exponential time")
	}
}

func TestTheCacheHoldsNoSubstringOfTheNote(t *testing.T) {
	note := "# One\n[[Plan]] and #tag\n"
	var s Scanner
	s.Pieces(note, nil, false)
	start := uintptr(unsafe.Pointer(unsafe.StringData(note)))
	inside := func(x string) bool {
		if x == "" {
			return false
		}
		p := uintptr(unsafe.Pointer(unsafe.StringData(x)))
		return p >= start && p < start+uintptr(len(note))
	}
	for key, pieces := range s.cache {
		if inside(key.text) {
			t.Errorf("the key %q shares the note's memory", key.text)
		}
		for _, p := range pieces {
			if inside(p.Label) || inside(p.Target) {
				t.Errorf("a piece of %q shares the note's memory", key.text)
			}
		}
	}
}

func TestACommentOpenedAfterText(t *testing.T) {
	in := "text %%start\nhidden\nend%%\n\nafter *x*\n"
	var s Scanner
	if got, want := show(in, s.Pieces(in, nil, false)), "text \nafter x\n"; got != want {
		t.Fatalf("%q, want %q", got, want)
	}
	code := "a `%%` b\n\nafter *x*\n"
	if got, want := show(code, s.Pieces(code, nil, false)), "a %% b\n\nafter x\n"; got != want {
		t.Fatalf("%q, want %q", got, want)
	}
}
