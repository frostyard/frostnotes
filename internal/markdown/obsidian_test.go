package markdown

// CONTRACT TEST for task card T-0011 (docs/tasks). Do not edit.

import (
	"fmt"
	"strings"
	"testing"

	"github.com/yuin/goldmark/text"
)

func TestParseObsidianFixtures(t *testing.T) {
	for _, f := range specParseFixtures(t) {
		t.Run(f.Name[:3], func(t *testing.T) {
			for _, d := range diffDoc(f.Want, Parse(f.Note), partWikiLinks, partInlineTags) {
				t.Error(d)
			}
		})
	}
}

// wikiLinks are d's links of kind wiki, each checked for its kind, its
// line and the text it covers.
func wikiLinks(t *testing.T, note string, d Doc) []Link {
	t.Helper()
	var out []Link
	for _, l := range d.Links {
		if l.Kind != LinkWiki {
			continue
		}
		if want := strings.Count(note[:l.Start], "\n") + 1; l.Line != want {
			t.Errorf("%+v: line %d, want %d", l, l.Line, want)
		}
		s := note[l.Start:l.End]
		if !strings.HasPrefix(strings.TrimPrefix(s, "!"), "[[") || !strings.HasSuffix(s, "]]") || l.Embed != strings.HasPrefix(s, "!") {
			t.Errorf("%+v covers %q", l, s)
		}
		l.Kind, l.Line = "", 0
		out = append(out, l)
	}
	return out
}

func tagNames(d Doc) []string {
	var out []string
	for _, tag := range d.Tags {
		if !tag.Front {
			out = append(out, tag.Name)
		}
	}
	return out
}

// TestParseWikilinks holds T-0002's cases, now over Parse.
func TestParseWikilinks(t *testing.T) {
	for _, tc := range []struct {
		name, body string
		want       []Link
	}{
		{"none", "plain text [not a link] [[\n]]\n", nil},
		{"simple", "see [[Note]].", []Link{{Target: "Note", Start: 4, End: 12}}},
		{"folder path", "[[Projects/Plan]]", []Link{{Target: "Projects/Plan", Start: 0, End: 17}}},
		{"alias", "[[Note|the note]]", []Link{{Target: "Note", Alias: "the note", Start: 0, End: 17}}},
		{"heading", "[[Note#Goals]]", []Link{{Target: "Note", Subpath: "#Goals", Start: 0, End: 14}}},
		{"block with alias", "[[Note#^abc123|that]]", []Link{{Target: "Note", Subpath: "#^abc123", Alias: "that", Start: 0, End: 21}}},
		{"same note heading", "[[#Later]]", []Link{{Subpath: "#Later", Start: 0, End: 10}}},
		{"embed", "![[diagram.png]]", []Link{{Target: "diagram.png", Embed: true, Start: 0, End: 16}}},
		{"spaces trimmed", "[[ Note | alias ]]", []Link{{Target: "Note", Alias: "alias", Start: 0, End: 18}}},
		{"pipe in alias", "[[Note|a|b]]", []Link{{Target: "Note", Alias: "a|b", Start: 0, End: 12}}},
		{"two on a line", "[[A]] and ![[B]]", []Link{{Target: "A", Start: 0, End: 5}, {Target: "B", Embed: true, Start: 10, End: 16}}},
		{"unicode", "[[Café ☕]]", []Link{{Target: "Café ☕", Start: 0, End: 13}}},
		{"empty", "[[]] and [[|x]]", []Link{{Alias: "x", Start: 9, End: 15}}},
		{"inline code", "`[[No]]` but [[Yes]]", []Link{{Target: "Yes", Start: 13, End: 20}}},
		{"double backtick code", "``a ` [[No]]`` [[Yes]]", []Link{{Target: "Yes", Start: 15, End: 22}}},
		{"unmatched backtick", "a ` b [[Yes]]", []Link{{Target: "Yes", Start: 6, End: 13}}},
		{"fenced code", "```\n[[No]]\n```\n[[Yes]]\n", []Link{{Target: "Yes", Start: 15, End: 22}}},
		{"tilde fence with info", "~~~md\n[[No]]\n~~~\n[[Yes]]", []Link{{Target: "Yes", Start: 17, End: 24}}},
		{"longer closing fence", "````\n```\n[[No]]\n`````\n[[Yes]]", []Link{{Target: "Yes", Start: 22, End: 29}}},
		{"unclosed fence", "```\n[[No]]\n", nil},
		{"indented fence", "   ```\n[[No]]\n   ```\n", nil},
		// Beyond T-0002.
		{"escaped backticks are not code", "Escaped \\`[[Gamma]]\\` is a link.", []Link{{Target: "Gamma", Start: 10, End: 19}}},
		{"escaped bracket", "\\[[No]] and [[Yes]]", []Link{{Target: "Yes", Start: 12, End: 19}}},
		{"escaped bang is not an embed", "\\![[Yes]]", []Link{{Target: "Yes", Start: 2, End: 9}}},
		{"table pipe", "| a |\n| - |\n| [[Recipes/Apple Butter\\|Butter]] |\n",
			[]Link{{Target: "Recipes/Apple Butter", Alias: "Butter", Start: 14, End: 46}}},
		{"indented code", "    [[No]]\n\n[[Yes]]", []Link{{Target: "Yes", Start: 12, End: 19}}},
		{"html block", "<div>\n[[No]]\n</div>\n\n[[Yes]]", []Link{{Target: "Yes", Start: 21, End: 28}}},
		{"inline html", "<span title=\"[[No]]\">[[Yes]]</span>", []Link{{Target: "Yes", Start: 21, End: 28}}},
		{"in a quote and a list", "> [[A]]\n- [[B]]", []Link{{Target: "A", Start: 2, End: 7}, {Target: "B", Start: 10, End: 15}}},
		{"third bracket", "[[[A]]]", []Link{{Target: "A", Start: 1, End: 6}}},
		{"crlf", "x\r\n[[A]]\r\n", []Link{{Target: "A", Start: 3, End: 8}}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got := wikiLinks(t, tc.body, Parse([]byte(tc.body)))
			if fmt.Sprint(got) != fmt.Sprint(tc.want) {
				t.Fatalf("links in %q =\n%+v\nwant\n%+v", tc.body, got, tc.want)
			}
		})
	}
}

func TestParseWikilinkOffsetsInTheNote(t *testing.T) {
	for _, tc := range []struct{ name, note, prefix string }{
		{"after frontmatter", "---\na: 1\n---\nSee [[Plan]]\n", "---\na: 1\n---\nSee "},
		{"after a bom", "\xef\xbb\xbfSee [[Plan]]\n", "\xef\xbb\xbfSee "},
		{"after a bom and crlf frontmatter", "\xef\xbb\xbf---\r\na: 1\r\n---\r\nSee [[Plan]]\r\n", "\xef\xbb\xbf---\r\na: 1\r\n---\r\nSee "},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got := wikiLinks(t, tc.note, Parse([]byte(tc.note)))
			if len(got) != 1 || got[0].Start != len(tc.prefix) || got[0].End != len(tc.prefix)+len("[[Plan]]") {
				t.Fatalf("links = %+v, want [[Plan]] at %d", got, len(tc.prefix))
			}
		})
	}
}

// TestParseInlineTags holds T-0002's cases, now over Parse.
func TestParseInlineTags(t *testing.T) {
	for _, tc := range []struct {
		name, body string
		want       []string
	}{
		{"none", "no tags here\n", nil},
		{"start of body", "#idea", []string{"idea"}},
		{"after space", "an #idea and #plan.", []string{"idea", "plan"}},
		{"nested", "#area/work-life", []string{"area/work-life"}},
		{"unicode and underscore", "#café_notes #日本", []string{"café_notes", "日本"}},
		{"heading is not a tag", "# Heading\n## Sub\n", nil},
		{"no space before", "a#b http://x.test/#frag", nil},
		{"digits only", "#123 #2026-10 #v2", []string{"2026-10", "v2"}},
		{"repeats once, first spelling", "#Idea then #idea and #IDEA", []string{"Idea"}},
		{"stops at punctuation", "(#todo), #done!", []string{"done"}},
		{"after a newline and a tab", "x\n#one\t#two", []string{"one", "two"}},
		{"not in code", "`#no` #yes\n```\n#no\n```\n", []string{"yes"}},
		{"not in a wikilink", "[[Note #nope]] #yes", []string{"yes"}},
		{"bare hash", "# and #", nil},
		// Beyond T-0002.
		{"combining mark", "#café x", []string{"café"}},
		{"after a bom", "\xef\xbb\xbf#first", []string{"first"}},
		{"in a heading, quote, list and table", "## H #h\n> #q\n- #l\n\n| #t |\n| - |\n| #c |\n", []string{"h", "q", "l", "t", "c"}},
		{"at the start of a block's text", ">#q\n\n|#t|x|\n|-|-|\n|#c|y|\n", []string{"q", "t", "c"}},
		{"not after other punctuation", "x.#no ##no", nil},
		{"escaped", "\\#no #yes", []string{"yes"}},
		{"not in link text", "[a #no](x.md) [#no2](https://x.test) #yes", []string{"yes"}},
		{"not in a url", "<https://x.test/#no> https://x.test/a#no #yes", []string{"yes"}},
		{"not in math", "$x #no$ #yes", []string{"yes"}},
		{"frontmatter is not body", "---\nk: #no\n---\n#yes", []string{"yes"}},
		{"atx heading needs a space", "#NotAHeading", []string{"NotAHeading"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			d := Parse([]byte(tc.body))
			if got := tagNames(d); fmt.Sprintf("%q", got) != fmt.Sprintf("%q", tc.want) {
				t.Fatalf("tags in %q = %q, want %q", tc.body, got, tc.want)
			}
		})
	}
}

func TestParseTagLines(t *testing.T) {
	d := Parse([]byte("---\na: 1\n---\nx\n\n#one\r\ny #two\n"))
	if fmt.Sprint(d.Tags) != fmt.Sprint([]Tag{{Name: "one", Line: 6}, {Name: "two", Line: 7}}) {
		t.Fatalf("tags = %+v", d.Tags)
	}
}

func TestParseHiddenSyntax(t *testing.T) {
	for _, tc := range []struct {
		name, body string
		links      []string // targets
		tags       []string
	}{
		{"inline math", "a $[[No]] #no$ b [[Yes]] #yes", []string{"Yes"}, []string{"yes"}},
		{"display math inline", "a $$[[No]]$$ [[Yes]]", []string{"Yes"}, nil},
		{"dollar then space is not math", "$ [[A]] $", []string{"A"}, nil},
		{"space before the closer is not math", "costs $5 and [[A]] $6", []string{"A"}, nil},
		{"escaped dollar", "\\$[[A]]$", []string{"A"}, nil},
		{"unclosed math", "$[[A]]", []string{"A"}, nil},
		{"math block", "$$\n[[No]] #no\n$$\n[[Yes]]", []string{"Yes"}, nil},
		{"one-line math block", "$$[[No]]$$\n[[Yes]]", []string{"Yes"}, nil},
		{"math block ends at its closer's line", "$$\nx\ny $$\n[[Yes]]", []string{"Yes"}, nil},
		{"math block interrupts a paragraph", "text\n$$\n[[No]]\n$$\n", nil, nil},
		{"unclosed math block runs to the end of its container", "> $$\n> [[No]]\n\n[[Yes]]", []string{"Yes"}, nil},
		{"math block in a list", "- $$\n  [[No]]\n  $$\n- [[Yes]]", []string{"Yes"}, nil},
		// Obsidian indexes what comments hide (ADR-0015).
		{"an inline comment's link and tag", "Hidden %%[[In]] #in%% inline [[Out]]", []string{"In", "Out"}, []string{"in"}},
		{"a comment block's link", "%%\nA [[In]]\n%%\n[[Out]]", []string{"In", "Out"}, nil},
		{"a comment across blocks", "a %% b\n\n- [[In]]\n\n> [[In2]] %% [[Out]] #out", []string{"In", "In2", "Out"}, []string{"out"}},
		{"an unclosed comment", "[[Out]] %% [[In]]\n\n# H\n[[In2]]", []string{"Out", "In", "In2"}, nil},
		{"comment marks in code are not comments", "`%%` [[A]] `%%`", []string{"A"}, nil},
		{"comment marks in a fence", "```\n%%\n```\n[[A]]\n```\n%%\n```", []string{"A"}, nil},
		{"comment marks in math", "$%%$ [[A]] $%%$", []string{"A"}, nil},
		{"links in a callout", "> [!important] Watering\n> Check [[Plan]] #water", []string{"Plan"}, []string{"water"}},
		{"highlight", "==[[A]] #a==", []string{"A"}, []string{"a"}},
		{"footnote", "Text[^1] [[A]]\n\n[^1]: Note [[B]]", []string{"A", "B"}, nil},
	} {
		t.Run(tc.name, func(t *testing.T) {
			d := Parse([]byte(tc.body))
			var targets []string
			for _, l := range wikiLinks(t, tc.body, d) {
				targets = append(targets, l.Target)
			}
			if fmt.Sprintf("%q", targets) != fmt.Sprintf("%q", tc.links) {
				t.Errorf("links in %q = %q, want %q", tc.body, targets, tc.links)
			}
			if got := tagNames(d); fmt.Sprintf("%q", got) != fmt.Sprintf("%q", tc.tags) {
				t.Errorf("tags in %q = %q, want %q", tc.body, got, tc.tags)
			}
		})
	}
}

func TestFindComments(t *testing.T) {
	for _, tc := range []struct {
		name, note string
		want       []string
	}{
		{"none", "a % b %", nil},
		{"inline", "a %%x%% b %%y%%", []string{"%%x%%", "%%y%%"}},
		{"across lines", "a %%\nx\n%% b", []string{"%%\nx\n%%"}},
		{"unclosed", "a %% b\n", []string{"%% b\n"}},
		{"after frontmatter", "---\nk: '%%'\n---\nx %%y%%", []string{"%%y%%"}},
		{"three percent signs", "%%%x%%", []string{"%%%x%%"}},
		{"not in code", "`%%` %%y%% ```%%```", []string{"%%y%%"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			note := []byte(tc.note)
			s := newSource(note, SplitFrontmatter(note))
			root := newParser(s).Parse(text.NewReader(s.body))
			var got []string
			for _, c := range findComments(root, s) {
				got = append(got, tc.note[c[0]:c[1]])
			}
			if fmt.Sprintf("%q", got) != fmt.Sprintf("%q", tc.want) {
				t.Fatalf("comments in %q = %q, want %q", tc.note, got, tc.want)
			}
		})
	}
}
