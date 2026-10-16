package markdown

// CONTRACT TEST for task card T-0002 (docs/tasks). Do not edit.

import (
	"fmt"
	"strings"
	"testing"
)

func TestWikilinks(t *testing.T) {
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
	} {
		t.Run(tc.name, func(t *testing.T) {
			got := Wikilinks([]byte(tc.body))
			if fmt.Sprint(got) != fmt.Sprint(tc.want) {
				t.Fatalf("Wikilinks(%q) =\n%+v\nwant\n%+v", tc.body, got, tc.want)
			}
			for _, l := range got {
				if s := tc.body[l.Start:l.End]; !strings.HasPrefix(s, "[[") && !strings.HasPrefix(s, "![[") || !strings.HasSuffix(s, "]]") {
					t.Fatalf("Start:End covers %q", s)
				}
			}
		})
	}
}

func TestInlineTags(t *testing.T) {
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
	} {
		t.Run(tc.name, func(t *testing.T) {
			got := InlineTags([]byte(tc.body))
			if fmt.Sprint(got) != fmt.Sprint(tc.want) {
				t.Fatalf("InlineTags(%q) = %q, want %q", tc.body, got, tc.want)
			}
		})
	}
}
