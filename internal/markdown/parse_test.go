package markdown

import (
	"fmt"
	"testing"
)

func TestSplitWikilink(t *testing.T) {
	for _, tc := range []struct {
		inner                  string
		target, subpath, alias string
		ok                     bool
	}{
		{"Plan", "Plan", "", "", true},
		{" Orchard/Plan | the plan ", "Orchard/Plan", "", "the plan", true},
		{"Plan#Goals#Sub", "Plan", "#Goals#Sub", "", true},
		{"Soil# ^ph-test |pH", "Soil", "#^ph-test", "pH", true},
		{"#Tasks", "", "#Tasks", "", true},
		{"Recipes/Apple Butter\\|Butter", "Recipes/Apple Butter", "", "Butter", true},
		{"a|b|c", "a", "", "b|c", true},
		{"Note #nope", "Note", "#nope", "", true},
		{"|x", "", "", "x", true},
		{" ", "", "", "", false},
		{"|", "", "", "", false},
	} {
		target, subpath, alias, ok := splitWikilink(tc.inner)
		if target != tc.target || subpath != tc.subpath || alias != tc.alias || ok != tc.ok {
			t.Errorf("splitWikilink(%q) = %q, %q, %q, %t; want %q, %q, %q, %t",
				tc.inner, target, subpath, alias, ok, tc.target, tc.subpath, tc.alias, tc.ok)
		}
	}
}

func TestWikilinkPattern(t *testing.T) {
	got := fmt.Sprintf("%q", wikilinkPattern.FindAllString("[[a]] ![[b|c]] [[]] [[d\ne]] [[[f]]] [[g]h]]", -1))
	if want := `["[[a]]" "![[b|c]]" "[[f]]"]`; got != want {
		t.Fatalf("matches = %s, want %s", got, want)
	}
}

func TestSourceLines(t *testing.T) {
	note := []byte("a\nbc\r\n\nd")
	s := newSource(note, SplitFrontmatter(note))
	for off, want := range []int{1, 1, 2, 2, 2, 2, 3, 4, 4} {
		if got := s.line(off); got != want {
			t.Errorf("line(%d) = %d, want %d", off, got, want)
		}
	}
}

func TestSourceBody(t *testing.T) {
	for _, tc := range []struct {
		note, body string
		base       int
	}{
		{"# x", "# x", 0},
		{"\xef\xbb\xbf# x", "# x", 3},
		{"---\na: 1\n---\nbody", "body", 13},
		{"\xef\xbb\xbf---\r\na: 1\r\n---\r\nbody", "body", 19},
	} {
		s := newSource([]byte(tc.note), SplitFrontmatter([]byte(tc.note)))
		if string(s.body) != tc.body || s.base != tc.base || string(s.note[s.base:]) != tc.body {
			t.Errorf("newSource(%q): body %q at %d, want %q at %d", tc.note, s.body, s.base, tc.body, tc.base)
		}
	}
}

func TestSourceInComment(t *testing.T) {
	s := &source{comments: [][2]int{{2, 4}, {7, 8}}}
	for off, want := range []bool{false, false, true, true, false, false, false, true, false} {
		if got := s.inComment(off); got != want {
			t.Errorf("inComment(%d) = %t, want %t", off, got, want)
		}
	}
}

func TestFinish(t *testing.T) {
	d := Doc{
		Links:   []Link{{Target: "b", Start: 9}, {Target: "a\xff", Start: 2}, {Target: "c", Start: 9}},
		Tags:    []Tag{{Name: "Harvest", Front: true}, {Name: "x"}, {Name: "harvest"}, {Name: "HARVEST"}, {Name: "X"}},
		Aliases: []string{"\xffa"},
	}
	finish(&d)
	if got := fmt.Sprint(d.Links); got != fmt.Sprint([]Link{{Target: "a�", Start: 2}, {Target: "b", Start: 9}, {Target: "c", Start: 9}}) {
		t.Errorf("links = %s", got)
	}
	if got := fmt.Sprint(d.Tags); got != fmt.Sprint([]Tag{{Name: "Harvest", Front: true}, {Name: "x"}}) {
		t.Errorf("tags = %s", got)
	}
	if d.Aliases[0] != "�a" {
		t.Errorf("aliases = %q", d.Aliases)
	}
}
