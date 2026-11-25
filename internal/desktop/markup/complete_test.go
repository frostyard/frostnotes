package markup

import (
	"slices"
	"testing"
)

func TestCompletionAtReadsWhatIsTyped(t *testing.T) {
	for _, tc := range []struct {
		in   string
		want Completing
		ok   bool
	}{
		{"See [[Gar|", Completing{Kind: CompleteNote, Query: "Gar", From: 6}, true},
		{"See [[Garden pl|]] after", Completing{Kind: CompleteNote, Query: "Garden pl", From: 6, Closed: true}, true},
		{"![[|]]", Completing{Kind: CompleteNote, Query: "", From: 3, Closed: true}, true},
		{"[[Garden#Be|]]", Completing{Kind: CompleteHeading, Target: "Garden", Query: "Be", From: 9, Closed: true}, true},
		{"[[#|", Completing{Kind: CompleteHeading, Target: "", Query: "", From: 3}, true},
		{"[[Garden#^b1|", Completing{Kind: CompleteBlock, Target: "Garden", Query: "b1", From: 10}, true},
		{"a #pro|", Completing{Kind: CompleteTag, Query: "pro", From: 3}, true},
		{"#|", Completing{Kind: CompleteTag, Query: "", From: 1}, true},
		{"#parent/ch|", Completing{Kind: CompleteTag, Query: "parent/ch", From: 1}, true},
		// A heading, a word with # inside it, a closed link, the line before.
		{"# |", Completing{}, false},
		{"a#b|", Completing{}, false},
		{"[[Garden]] |", Completing{}, false},
		{"[[Gar\n|", Completing{}, false},
		{"plain|", Completing{}, false},
	} {
		text, sels := at(tc.in)
		got, ok := CompletionAt(text, sels[0].Head)
		if ok != tc.ok || got != tc.want {
			t.Errorf("%q: %+v, %v; want %+v, %v", tc.in, got, ok, tc.want, tc.ok)
		}
	}
}

func TestFuzzyMatchKeepsTheOrder(t *testing.T) {
	for _, tc := range []struct {
		s, q string
		want bool
	}{{"project/alpha", "pal", true}, {"Project", "pro", true}, {"project", "jp", false}, {"x", "", true}} {
		if got := FuzzyMatch(tc.s, tc.q); got != tc.want {
			t.Errorf("FuzzyMatch(%q, %q) = %v", tc.s, tc.q, got)
		}
	}
}

func TestCommandsAreObsidiansAndEdit(t *testing.T) {
	cmds := Commands()
	var ids []string
	for _, c := range cmds {
		if c.ID == "" || c.Title == "" || c.Edit == nil {
			t.Errorf("command %+v is not whole", c)
		}
		ids = append(ids, c.ID)
	}
	slices.Sort(ids)
	if len(slices.Compact(ids)) != len(cmds) {
		t.Errorf("a command's ID is there twice: %v", ids)
	}
	edit := func(id, in string) string {
		for _, c := range cmds {
			if c.ID == id {
				text, sels := at(in)
				e := c.Edit(text, sels)
				out, err := e.Changes.Apply(text)
				if err != nil {
					t.Fatal(err)
				}
				return show(out, e.Sels)
			}
		}
		t.Fatalf("no command %s", id)
		return ""
	}
	for _, tc := range []struct{ id, in, want string }{
		{"editor:toggle-bold", "a {word} b", "a **{word}** b"},
		{"editor:toggle-highlight", "{x}", "=={x}=="},
		{"editor:insert-embed", "{a}", "![[a|]]"},
		{"editor:set-heading-3", "Ti|tle", "### Ti|tle"},
		{"editor:set-heading-0", "## Ti|tle", "Ti|tle"},
		{"editor:toggle-numbered-list", "{a\nb}", "1. {a\n2. b}"},
	} {
		if got := edit(tc.id, tc.in); got != tc.want {
			t.Errorf("%s on %q made %q, want %q", tc.id, tc.in, got, tc.want)
		}
	}
}
