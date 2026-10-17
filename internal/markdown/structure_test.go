package markdown

// CONTRACT TEST for task card T-0012 (docs/tasks). Do not edit.

import (
	"fmt"
	"strings"
	"testing"
)

// TestParseSpecFixtures runs every parse fixture in the spec against the
// whole of Parse's output.
func TestParseSpecFixtures(t *testing.T) {
	for _, f := range specParseFixtures(t) {
		t.Run(f.Name[:3], func(t *testing.T) {
			for _, d := range diffDoc(f.Want, Parse(f.Note), allParts...) {
				t.Error(d)
			}
		})
	}
}

func TestParseMarkdownLinks(t *testing.T) {
	for _, tc := range []struct {
		name, note string
		want       []Link // Kind, Line and End are checked separately
		covers     []string
	}{
		{"inline", "See [the plan](Plan.md).", []Link{{Target: "Plan.md", Alias: "the plan", Start: 4}}, []string{"[the plan](Plan.md)"}},
		{"image", "![map](./media/soil-map.png)", []Link{{Target: "./media/soil-map.png", Alias: "map", Embed: true, Start: 0}},
			[]string{"![map](./media/soil-map.png)"}},
		{"percent-decoded with subpath", "[s](Orchard/North%20Block/Soil.md#pH%20test)",
			[]Link{{Target: "Orchard/North Block/Soil.md", Subpath: "#pH test", Alias: "s", Start: 0}},
			[]string{"[s](Orchard/North%20Block/Soil.md#pH%20test)"}},
		{"angle brackets and a title", "[a](<North Block/Plan.md> \"Title\")", []Link{{Target: "North Block/Plan.md", Alias: "a", Start: 0}},
			[]string{"[a](<North Block/Plan.md> \"Title\")"}},
		{"self heading", "[here](#Tasks)", []Link{{Subpath: "#Tasks", Alias: "here", Start: 0}}, []string{"[here](#Tasks)"}},
		{"backslash escape and entity", "[x](a\\_b&amp;c.md)", []Link{{Target: "a_b&c.md", Alias: "x", Start: 0}}, []string{"[x](a\\_b&amp;c.md)"}},
		{"invalid percent escape kept", "[x](100%25%zz.md)", []Link{{Target: "100%25%zz.md", Alias: "x", Start: 0}}, []string{"[x](100%25%zz.md)"}},
		{"plain-text alias", "[**bold** `code`\ntext](x.md)", []Link{{Target: "x.md", Alias: "bold code text", Start: 0}}, []string{"[**bold** `code`\ntext](x.md)"}},
		{"reference forms", "[full][r] [collapsed][] [r]\n\n[r]: Apple%20Butter.md\n[collapsed]: c.md",
			[]Link{{Target: "Apple Butter.md", Alias: "full", Start: 0}, {Target: "c.md", Alias: "collapsed", Start: 10},
				{Target: "Apple Butter.md", Alias: "r", Start: 24}},
			[]string{"[full][r]", "[collapsed][]", "[r]"}},
		{"external links are not indexed", "[a](https://x.test/a.md) [b](mailto:w@x.test) [c](//host/c.md) <https://x.test> https://x.test/d.md [e](C:x.md)", nil, nil},
		{"an empty destination is not a link", "[a]() [b](<>)", nil, nil},
		{"not in code, but in comments", "`[a](a.md)` %%[b](b.md)%% [c](c.md)",
			[]Link{{Target: "b.md", Alias: "b", Start: 14}, {Target: "c.md", Alias: "c", Start: 26}}, []string{"[b](b.md)", "[c](c.md)"}},
		{"after frontmatter", "---\nk: v\n---\n[a](a.md)", []Link{{Target: "a.md", Alias: "a", Start: 13}}, []string{"[a](a.md)"}},
		{"a wikilink is not a markdown link", "[[a.md]] ![[b.png]]", nil, nil},
	} {
		t.Run(tc.name, func(t *testing.T) {
			d := Parse([]byte(tc.note))
			var got []Link
			var covers []string
			for _, l := range d.Links {
				if l.Kind != LinkMarkdown {
					continue
				}
				if want := strings.Count(tc.note[:l.Start], "\n") + 1; l.Line != want {
					t.Errorf("%+v: line %d, want %d", l, l.Line, want)
				}
				covers = append(covers, tc.note[l.Start:l.End])
				l.Kind, l.Line, l.End = "", 0, 0
				got = append(got, l)
			}
			if fmt.Sprint(got) != fmt.Sprint(tc.want) {
				t.Errorf("links =\n%+v\nwant\n%+v", got, tc.want)
			}
			if fmt.Sprintf("%q", covers) != fmt.Sprintf("%q", tc.covers) {
				t.Errorf("links cover %q, want %q", covers, tc.covers)
			}
		})
	}
}

func TestParseLinksInOrder(t *testing.T) {
	note := "---\nup: \"[[P]]\"\n---\n[m](m.md) [[w]] ![i](i.png) ![[e]]\n"
	var got []string
	for _, l := range Parse([]byte(note)).Links {
		got = append(got, fmt.Sprintf("%s:%s", l.Kind, l.Target))
	}
	want := []string{"property:P", "markdown:m.md", "wiki:w", "markdown:i.png", "wiki:e"}
	if fmt.Sprint(got) != fmt.Sprint(want) {
		t.Fatalf("links = %v, want %v", got, want)
	}
}

func TestParseHeadings(t *testing.T) {
	for _, tc := range []struct {
		name, note string
		want       []Heading
	}{
		{"atx levels", "# 1\n## 2\n### 3\n#### 4\n##### 5\n###### 6\n####### 7",
			[]Heading{{1, "1", 1}, {2, "2", 2}, {3, "3", 3}, {4, "4", 4}, {5, "5", 5}, {6, "6", 6}}},
		{"closing sequence and spaces", "##   Goals   ##  \n# Not # closed", []Heading{{2, "Goals", 1}, {1, "Not # closed", 2}}},
		{"setext", "Title\n=====\nSub\n---", []Heading{{1, "Title", 1}, {2, "Sub", 3}}},
		{"multi-line setext", "One\ntwo\n===", []Heading{{1, "One two", 1}}},
		{"empty atx", "#\n## ##", []Heading{{1, "", 1}, {2, "", 2}}},
		{"markup as written", "## A [[Link]] and #tag *em*", []Heading{{2, "A [[Link]] and #tag *em*", 1}}},
		{"not a heading", "#NotAHeading\n    # indented code", nil},
		{"in a quote and a list", "> # Q\n- ## L", []Heading{{1, "Q", 1}, {2, "L", 2}}},
		{"crlf", "# A\r\nB\r\n---\r\n", []Heading{{1, "A", 1}, {2, "B", 2}}},
		{"after frontmatter", "---\nk: v\n---\n# A", []Heading{{1, "A", 4}}},
		{"not in code, but in comments", "```\n# code\n```\n%%\n# comment\n%%\n# yes", []Heading{{1, "comment", 5}, {1, "yes", 7}}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := Parse([]byte(tc.note)).Headings; fmt.Sprint(got) != fmt.Sprint(tc.want) {
				t.Fatalf("headings = %+v, want %+v", got, tc.want)
			}
		})
	}
}

func TestParseTasks(t *testing.T) {
	for _, tc := range []struct {
		name, note string
		want       []Task
	}{
		{"statuses", "- [ ] open\n- [x] done\n* [X] too\n+ [/] half\n1. [-] gone\n2) [>] later",
			[]Task{{" ", "open", 1}, {"x", "done", 2}, {"X", "too", 3}, {"/", "half", 4}, {"-", "gone", 5}, {">", "later", 6}}},
		{"nested", "- [ ] a\n    - [x] b\n        - [ ] c", []Task{{" ", "a", 1}, {"x", "b", 2}, {" ", "c", 3}}},
		{"text is the first line, trimmed", "- [ ]   spaced  \n  more\n- [x]\ttab", []Task{{" ", "spaced", 1}, {"x", "tab", 3}}},
		{"multibyte status", "- [é] accent", []Task{{"é", "accent", 1}}},
		{"not tasks", "- [] a\n- [xx] b\n- [x]c\n- [ ]\n- x [ ] d\nText [ ] e\n- `[ ] f`", nil},
		{"in a quote", "> - [ ] q", []Task{{" ", "q", 1}}},
		{"crlf", "- [ ] a\r\n- [x] b\r\n", []Task{{" ", "a", 1}, {"x", "b", 2}}},
		{"after frontmatter", "---\nk: v\n---\n- [ ] a", []Task{{" ", "a", 4}}},
		{"not in code, but in comments", "```\n- [ ] code\n```\n%%\n- [ ] comment\n%%\n- [ ] yes", []Task{{" ", "comment", 5}, {" ", "yes", 7}}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := Parse([]byte(tc.note)).Tasks; fmt.Sprint(got) != fmt.Sprint(tc.want) {
				t.Fatalf("tasks = %+v, want %+v", got, tc.want)
			}
		})
	}
}

func TestParseBlockIDs(t *testing.T) {
	for _, tc := range []struct {
		name, note string
		want       []Block
	}{
		{"end of a paragraph", "Text. ^para-1", []Block{{"para-1", 1}}},
		{"end of its last line", "one\ntwo ^b2\n", []Block{{"b2", 2}}},
		{"alone on a paragraph's last line", "one\n^b3", []Block{{"b3", 2}}},
		{"list item", "- a ^i1\n- b\n  ^i2", []Block{{"i1", 1}, {"i2", 3}}},
		{"after a list, quote and table", "- a\n\n^l1\n\n> q\n\n^q1\n\n| a |\n| - |\n| 1 |\n\n\n^t1",
			[]Block{{"l1", 3}, {"q1", 7}, {"t1", 14}}},
		{"alone after a paragraph is not an id", "para\n\n^no", nil},
		{"trailing spaces", "Text ^tr  \n", []Block{{"tr", 1}}},
		{"not ids", "a ^not here\nb^no\nc ^a_b\nd `^code`\n", nil},
		{"after frontmatter", "---\nk: v\n---\nx ^f", []Block{{"f", 4}}},
		{"not in code, but in comments", "```\nx ^c\n```\n\n%%\n\nx ^m\n\n%%\n\ny ^yes", []Block{{"m", 7}, {"yes", 11}}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := Parse([]byte(tc.note)).Blocks; fmt.Sprint(got) != fmt.Sprint(tc.want) {
				t.Fatalf("blocks = %+v, want %+v", got, tc.want)
			}
		})
	}
}

func TestParseWords(t *testing.T) {
	for _, tc := range []struct {
		name, note string
		want       int
	}{
		{"empty", "", 0},
		{"plain", "one two  three\n", 3},
		{"markup counts as written", "[[Apple Butter]] - [ ] #tag **x**", 4},
		{"punctuation alone is not a word", "- * --- | -> 1. ...", 1},
		{"unicode", "café 日本語 ½ x", 3},
		{"frontmatter is not counted", "---\ntitle: a b c\n---\nbody", 1},
		{"comments are not counted", "a %%b c%% d %%\ne\n%%", 2},
		{"a comment splits a word", "a%%x%%b", 2},
		{"unclosed comment", "a %% b c", 1},
		{"code counts", "`x y` ```\nz\n```", 3},
		{"invalid utf-8", "a\xffb \xff", 1},
		{"bom", "\xef\xbb\xbfword", 1},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := Parse([]byte(tc.note)).Words; got != tc.want {
				t.Fatalf("words in %q = %d, want %d", tc.note, got, tc.want)
			}
		})
	}
}
