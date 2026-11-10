package editor

import (
	"slices"
	"strings"
	"testing"
)

// at reads a test's text, as format.test.ts does: "|" is a caret and
// "{…}" a selection.
func at(s string) (string, []Sel) {
	var doc strings.Builder
	var sels []Sel
	open := -1
	for _, r := range s {
		switch r {
		case '|':
			sels = append(sels, caret(doc.Len()))
		case '{':
			open = doc.Len()
		case '}':
			sels = append(sels, Sel{open, doc.Len()})
		default:
			doc.WriteRune(r)
		}
	}
	if len(sels) == 0 {
		sels = []Sel{caret(0)}
	}
	return doc.String(), sels
}

// show writes a text and its selections as a test's text.
func show(doc string, sels []Sel) string {
	type mark struct {
		pos int
		m   string
	}
	var marks []mark
	for _, s := range sels {
		if s.Empty() {
			marks = append(marks, mark{s.Head, "|"})
		} else {
			marks = append(marks, mark{s.From(), "{"}, mark{s.To(), "}"})
		}
	}
	slices.SortStableFunc(marks, func(a, b mark) int {
		if a.pos != b.pos {
			return a.pos - b.pos
		}
		if a.m == "}" {
			return -1
		}
		if b.m == "}" {
			return 1
		}
		return 0
	})
	var out strings.Builder
	pos := 0
	for _, m := range marks {
		out.WriteString(doc[pos:m.pos] + m.m)
		pos = m.pos
	}
	return out.String() + doc[pos:]
}

func run(t *testing.T, cmd func(string, []Sel) Edit, in string) string {
	t.Helper()
	doc, sels := at(in)
	e := cmd(doc, sels)
	out, err := e.Changes.Apply(doc)
	if err != nil {
		t.Fatalf("%q: %v", in, err)
	}
	return show(out, e.Sels)
}

func mark(m string) func(string, []Sel) Edit {
	return func(doc string, sels []Sel) Edit { return ToggleMark(doc, sels, m) }
}

func heading(n int) func(string, []Sel) Edit {
	return func(doc string, sels []Sel) Edit { return SetHeading(doc, sels, n) }
}

func pair(before, after string) func(string, []Sel) Edit {
	return func(doc string, sels []Sel) Edit { return InsertPair(doc, sels, before, after) }
}

// The cases of app/src/editor/format.test.ts, which the web app passes.
func TestFormatting(t *testing.T) {
	cases := []struct {
		name    string
		cmd     func(string, []Sel) Edit
		in, out string
	}{
		{"bold wraps", mark("**"), "a {b c} d", "a **{b c}** d"},
		{"bold unwraps", mark("**"), "a **{b c}** d", "a {b c} d"},
		{"bold unwraps inside", mark("**"), "a {**b c**} d", "a {b c} d"},
		{"bold of bold italics", mark("**"), "a ***{b}*** c", "a *{b}* c"},
		{"bold of italics", mark("**"), "a *{b}* c", "a ***{b}*** c"},
		{"bold word", mark("**"), "one wo|rd two", "one **wo|rd** two"},
		{"bold word off", mark("**"), "one **wo|rd** two", "one wo|rd two"},
		{"bold word end", mark("**"), "one word| two", "one **word|** two"},
		{"bold pair", mark("**"), "a | b", "a **|** b"},
		{"bold pair off", mark("**"), "a **|** b", "a | b"},
		{"bold empty", mark("**"), "|", "**|**"},
		{"bold every range", mark("**"), "{a} and {b} and c|", "**{a}** and **{b}** and **c|**"},
		{"italics wraps", mark("*"), "a {b} c", "a *{b}* c"},
		{"italics unwraps", mark("*"), "a *{b}* c", "a {b} c"},
		{"italics inside", mark("*"), "a {*b*} c", "a {b} c"},
		{"italics word", mark("*"), "x wo|rd", "x *wo|rd*"},
		{"italics of bold", mark("*"), "a **{b}** c", "a ***{b}*** c"},
		{"italics of both", mark("*"), "a ***{b}*** c", "a **{b}** c"},
		{"italics in bold", mark("*"), "**wo|rd**", "***wo|rd***"},
		{"strike", mark("~~"), "{x}", "~~{x}~~"},
		{"strike off", mark("~~"), "~~{x}~~", "{x}"},
		{"strike pair", mark("~~"), "a |", "a ~~|~~"},
		{"highlight", mark("=="), "{x y}", "=={x y}=="},
		{"highlight off", mark("=="), "==x|==", "x|"},
		{"code", mark("`"), "use {npm}", "use `{npm}`"},
		{"code off", mark("`"), "use `{npm}`", "use {npm}"},
		{"code pair", mark("`"), "use |", "use `|`"},
		{"comment wraps", ToggleComment, "a {b} c", "a %%{b}%% c"},
		{"comment unwraps", ToggleComment, "a %%{b}%% c", "a {b} c"},
		{"comment line", ToggleComment, "x\n  some |text\ny", "x\n  %%some |text%%\ny"},
		{"comment line off", ToggleComment, "x\n  %%some |text%%\ny", "x\n  some |text\ny"},
		{"comment empty", ToggleComment, "|", "%%|%%"},
		{"link of text", InsertLink, "see {the plan} now", "see [the plan](|) now"},
		{"link of url", InsertLink, "{https://example.com/x?y=1}", "[|](https://example.com/x?y=1)"},
		{"empty link", InsertLink, "a |b", "a [|]()b"},
		{"check", ToggleChecklist, "- [ ] task|", "- [x] task|"},
		{"uncheck", ToggleChecklist, "* [x] done|", "* [ ] done|"},
		{"other status", ToggleChecklist, "  1. [/] doing|", "  1. [ ] doing|"},
		{"check bare box", ToggleChecklist, "- [ ]|", "- [x]|"},
		{"uncheck bare box", ToggleChecklist, "- [x]|\n", "- [ ]|\n"},
		{"item to task", ToggleChecklist, "- item|", "- [ ] item|"},
		{"numbered to task", ToggleChecklist, "1) item|", "1) [ ] item|"},
		{"bare dash to task", ToggleChecklist, "-|", "- [ ] |"},
		{"empty to task", ToggleChecklist, "|", "- [ ] |"},
		{"indented to task", ToggleChecklist, "  te|xt", "  - [ ] te|xt"},
		{"every line", ToggleChecklist, "{a\n- b\n- [ ] c}", "- [ ] {a\n- [ ] b\n- [x] c}"},
		{"a line once", ToggleChecklist, "- [ ] a| b|", "- [x] a| b|"},
		{"delete line", DeleteParagraph, "one\ntw|o\nthree", "one\n|three"},
		{"delete last line", DeleteParagraph, "one\ntwo\nthr|ee", "one\ntwo|"},
		{"delete only line", DeleteParagraph, "on|ly", "|"},
		{"delete lines", DeleteParagraph, "o{ne\ntw}o\nthree", "|three"},
		{"delete touched", DeleteParagraph, "a|b|c\nd\ne|", "|d|"},
		{"heading 2", heading(2), "Title|", "## Title|"},
		{"heading 1 of 3", heading(1), "### Title|", "# Title|"},
		{"heading off", heading(0), "## Title|", "Title|"},
		{"heading off plain", heading(0), "Plain|", "Plain|"},
		{"heading same", heading(2), "## Same|", "## Same|"},
		{"heading of tag", heading(1), "#tag text|", "# #tag text|"},
		{"heading of #", heading(3), "#|", "### |"},
		{"heading lines", heading(3), "{a\n# b}", "### {a\n### b}"},
		{"heading before caret", heading(2), "|x", "## |x"},
		{"quote", ToggleBlockquote, "a|", "> a|"},
		{"quote lines", ToggleBlockquote, "{a\n\nb}", "> {a\n\n> b}"},
		{"unquote", ToggleBlockquote, "{> a\n>b}", "{a\nb}"},
		{"bullets", ToggleBulletList, "{a\nb}", "- {a\n- b}"},
		{"bullets off", ToggleBulletList, "{- a\n- b}", "{a\nb}"},
		{"numbered to bullet", ToggleBulletList, "1. a|", "- a|"},
		{"numbered", ToggleNumberedList, "{a\nb\nc}", "1. {a\n2. b\n3. c}"},
		{"numbered off", ToggleNumberedList, "{1. a\n2. b}", "{a\nb}"},
		{"bullet to numbered", ToggleNumberedList, "- a|", "1. a|"},
		{"indented bullet", ToggleBulletList, "  a|", "  - a|"},
		{"wikilink", pair("[[", "]]"), "see |", "see [[|]]"},
		{"wikilink of text", pair("[[", "]]"), "see {Plan}", "see [[Plan|]]"},
		{"embed", pair("![[", "]]"), "|", "![[|]]"},
		{"tag", pair("#", ""), "a |", "a #|"},
	}
	for _, c := range cases {
		if got := run(t, c.cmd, c.in); got != c.out {
			t.Errorf("%s: %q gives %q, want %q", c.name, c.in, got, c.out)
		}
	}
}

func TestSmartLists(t *testing.T) {
	cases := []struct{ in, out string }{
		{"- one|", "- one\n- |"},
		{"  * one|", "  * one\n  * |"},
		{"3. three|", "3. three\n4. |"},
		{"- [x] done|", "- [x] done\n- [ ] |"},
		{"> quoted|", "> quoted\n> |"},
		{"> - in a quote|", "> - in a quote\n> - |"},
		{"- |", "|"},
		{"> |", "|"},
	}
	for _, c := range cases {
		doc, sels := at(c.in)
		e, ok := ContinueMarkup(doc, sels)
		if !ok {
			t.Errorf("%q: not continued", c.in)
			continue
		}
		out, _ := e.Changes.Apply(doc)
		if got := show(out, e.Sels); got != c.out {
			t.Errorf("%q gives %q, want %q", c.in, got, c.out)
		}
	}
	if _, ok := ContinueMarkup(at("plain|")); ok {
		t.Error("a plain line continued")
	}
	if _, ok := ContinueMarkup(at("-| item")); ok {
		t.Error("Enter inside the marker continued")
	}
	doc, sels := at("- [ ] |")
	e, ok := DeleteMarkupBackward(doc, sels)
	if out, _ := e.Changes.Apply(doc); !ok || show(out, e.Sels) != "|" {
		t.Errorf("backspace after a marker: %v", ok)
	}
	doc, sels = at("a {b} c")
	if e, ok := WrapInMark(doc, sels, "*"); !ok {
		t.Error("a mark typed over a selection did not wrap it")
	} else if out, _ := e.Changes.Apply(doc); show(out, e.Sels) != "a *{b}* c" {
		t.Errorf("wrap made %q", show(out, e.Sels))
	}
	if _, ok := WrapInMark("ab", []Sel{caret(1)}, "*"); ok {
		t.Error("a mark typed at a caret wrapped")
	}
}
