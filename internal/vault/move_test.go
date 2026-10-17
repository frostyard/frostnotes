package vault

import (
	"testing"

	"github.com/frostyard/frostnotes/internal/markdown"
)

func TestRetargetText(t *testing.T) {
	for _, tc := range []struct {
		raw  string
		kind markdown.LinkKind
		to   string
		want string
	}{
		{"[[Plan]]", markdown.LinkWiki, "Plan 2", "[[Plan 2]]"},
		{"![[Plan#Goals|the goals]]", markdown.LinkWiki, "Archive/Plan", "![[Archive/Plan#Goals|the goals]]"},
		{`[[Plan\|alias]]`, markdown.LinkWiki, "Plan 2", `[[Plan 2\|alias]]`},
		{"[[ Plan ]]", markdown.LinkWiki, "Plan 2", "[[Plan 2]]"},
		{"[[Plan]]", markdown.LinkProperty, "New", "[[New]]"},
		{"[the plan](Notes/Plan.md)", markdown.LinkMarkdown, "Plan 2.md", "[the plan](Plan%202.md)"},
		{"![map](media/map.png#x)", markdown.LinkMarkdown, "maps/map.png", "![map](maps/map.png#x)"},
		{`[p](<Notes/My Plan.md> "title")`, markdown.LinkMarkdown, "My Plan 2.md", `[p](<My Plan 2.md> "title")`},
		{`[p](Notes/Plan.md "title")`, markdown.LinkMarkdown, "Plan (old).md", `[p](Plan%20%28old%29.md "title")`},
	} {
		got, ok := retargetText(tc.raw, tc.kind, tc.to)
		if !ok || got != tc.want {
			t.Errorf("retargetText(%q, %s, %q) = %q, %v; want %q", tc.raw, tc.kind, tc.to, got, ok, tc.want)
			continue
		}
		// What the rewrite writes must parse back to the new target.
		doc := markdown.Parse([]byte(got))
		if tc.kind != markdown.LinkProperty && (len(doc.Links) != 1 || doc.Links[0].Target != tc.to) {
			t.Errorf("%q parses to %+v, want target %q", got, doc.Links, tc.to)
		}
	}
	for _, tc := range []struct {
		raw  string
		kind markdown.LinkKind
		to   string
	}{
		{"[p][ref]", markdown.LinkMarkdown, "x.md"}, // a reference link: its definition names the file
		{"[[Plan]]", markdown.LinkWiki, "a|b"},
		{"[p](<a.md>)", markdown.LinkMarkdown, "b>.md"},
	} {
		if got, ok := retargetText(tc.raw, tc.kind, tc.to); ok {
			t.Errorf("retargetText(%q, %q) = %q, want no rewrite", tc.raw, tc.to, got)
		}
	}
}

func TestShortestTarget(t *testing.T) {
	files := map[string][]string{
		"plan.md":  {"Archive/Plan.md", "Plan.md"},
		"other.md": {"Deep/Folder/Other.md"},
		"map.png":  {"media/map.png"},
	}
	lookup := func(name string) []string { return files[name] }
	for _, tc := range []struct {
		source, want string
		kind         markdown.LinkKind
		target       string
	}{
		{"Inbox.md", "Deep/Folder/Other.md", markdown.LinkWiki, "Other"},
		{"Inbox.md", "Deep/Folder/Other.md", markdown.LinkMarkdown, "Other.md"},
		{"Inbox.md", "Archive/Plan.md", markdown.LinkWiki, "Archive/Plan"},
		{"Archive/Note.md", "Archive/Plan.md", markdown.LinkWiki, "Plan"},
		{"Archive/Note.md", "Plan.md", markdown.LinkWiki, "/Plan"},
		{"Inbox.md", "media/map.png", markdown.LinkWiki, "map.png"},
	} {
		got, ok := shortestTarget(tc.source, tc.want, tc.kind, lookup)
		if !ok || got != tc.target {
			t.Errorf("shortestTarget(%s → %s, %s) = %q, %v; want %q", tc.source, tc.want, tc.kind, got, ok, tc.target)
		}
	}
}
