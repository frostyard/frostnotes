package markdown

// CONTRACT TEST for task card T-0010 (docs/tasks). Do not edit.

import (
	"fmt"
	"strings"
	"testing"
)

func TestParseFrontFixtures(t *testing.T) {
	for _, f := range specParseFixtures(t) {
		t.Run(f.Name[:3], func(t *testing.T) {
			for _, d := range diffDoc(f.Want, Parse(f.Note), partProps, partFrontTags, partPropertyLinks) {
				t.Error(d)
			}
		})
	}
}

func TestParseFrontmatterProps(t *testing.T) {
	for _, tc := range []struct {
		name, note string
		props      string // propsJSON of Props; "" for nil
		err        bool
	}{
		{"no frontmatter", "# Title\n", "", false},
		{"unclosed is not frontmatter", "---\ntitle: x\n", "", false},
		{"empty", "---\n---\nbody\n", "{}", false},
		{"only a comment", "---\n# draft\n---\n", "{}", false},
		{"scalars", "---\ns: text\nq: \"2026-10-16\"\nn: 3\nf: 1.5\nb: true\nz: null\nd: 2026-10-16\n---\n",
			`{"b":true,"d":"2026-10-16","f":1.5,"n":3,"q":"2026-10-16","s":"text","z":null}`, false},
		{"yaml 1.2 words stay strings", "---\na: yes\nb: off\n---\n", `{"a":"yes","b":"off"}`, false},
		{"infinity stays as written", "---\nx: .inf\ny: .nan\n---\n", `{"x":".inf","y":".nan"}`, false},
		{"lists and maps", "---\nl: [1, two]\nm:\n  k: v\n  2: w\n---\n", `{"l":[1,"two"],"m":{"2":"w","k":"v"}}`, false},
		{"anchors", "---\na: &x [1]\nb: *x\n---\n", `{"a":[1],"b":[1]}`, false},
		{"crlf and bom", "\xef\xbb\xbf---\r\ntitle: Plan\r\n---\r\nbody\r\n", `{"title":"Plan"}`, false},
		{"not a mapping", "---\n- a\n- b\n---\nbody\n", "", true},
		{"a scalar", "---\njust text\n---\n", "", true},
		{"does not parse", "---\ntags: [unclosed\n---\n", "", true},
		{"repeated key", "---\na: 1\na: 2\n---\n", "", true},
		{"invalid utf-8", "---\na: \xff\n---\n", "", true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			d := Parse([]byte(tc.note))
			if (d.PropError != nil) != tc.err {
				t.Fatalf("PropError = %v, want error %t", d.PropError, tc.err)
			}
			if tc.props == "" {
				if d.Props != nil {
					t.Fatalf("Props = %v, want nil", d.Props)
				}
				return
			}
			if d.Props == nil {
				t.Fatal("Props = nil, want a mapping")
			}
			if got := propsJSON(d.Props); got != tc.props {
				t.Fatalf("Props = %s, want %s", got, tc.props)
			}
		})
	}
}

func TestParseFrontmatterErrorKeepsNothing(t *testing.T) {
	d := Parse([]byte("---\ntags: [a]\naliases: [b]\nup: \"[[c]]\"\ntags: [d]\n---\n"))
	if d.PropError == nil || !strings.Contains(d.PropError.Error(), "tags") {
		t.Fatalf("PropError = %v, want one naming the repeated key", d.PropError)
	}
	if d.Props != nil || d.Aliases != nil || len(d.Tags) > 0 || len(d.Links) > 0 {
		t.Fatalf("a property error left %+v", d)
	}
}

func TestParseFrontmatterTags(t *testing.T) {
	for _, tc := range []struct {
		name, front string
		want        []string
	}{
		{"flow list", "tags: [harvest, \"#orchard/north\"]", []string{"harvest", "orchard/north"}},
		{"block list", "tags:\n  - a\n  - '#b'", []string{"a", "b"}},
		{"string at commas and spaces", "tags: \"a, b c,,d\t#e\"", []string{"a", "b", "c", "d", "e"}},
		{"unquoted # after a space is a yaml comment", "tags: a #b", []string{"a"}},
		{"one tag", "tags: '#solo'", []string{"solo"}},
		{"numbers as written", "tags: [2026, 1.50]", []string{"2026", "1.50"}},
		{"list items are not split", "tags: [\"a b\"]", []string{"a b"}},
		{"null and nested items skipped", "tags: [~, [x], {k: v}, ok]", []string{"ok"}},
		{"empty", "tags:", nil},
		{"empty strings", "tags: [\"\", \"#\"]", nil},
		{"once each, first spelling", "tags: [Idea, idea, IDEA, other]", []string{"Idea", "other"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			d := Parse([]byte("---\ntitle: t\n" + tc.front + "\n---\n"))
			if d.PropError != nil {
				t.Fatal(d.PropError)
			}
			var got []string
			for _, tag := range d.Tags {
				if !tag.Front || tag.Line != 3 {
					t.Errorf("tag %+v: want Front and line 3, the tags key's", tag)
				}
				got = append(got, tag.Name)
			}
			if fmt.Sprintf("%q", got) != fmt.Sprintf("%q", tc.want) {
				t.Fatalf("tags = %q, want %q", got, tc.want)
			}
		})
	}
}

func TestParseFrontmatterAliases(t *testing.T) {
	for _, tc := range []struct {
		name, front string
		want        []string
	}{
		{"list", "aliases: [Picking log, Harvest]", []string{"Picking log", "Harvest"}},
		{"block list", "aliases:\n  - One\n  - 2", []string{"One", "2"}},
		{"a string is one alias", "aliases: Picking log, autumn", []string{"Picking log, autumn"}},
		{"null", "aliases:", nil},
		{"empty and nested items skipped", "aliases: [\"\", [x], ok]", []string{"ok"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			d := Parse([]byte("---\n" + tc.front + "\n---\n"))
			if fmt.Sprintf("%q", d.Aliases) != fmt.Sprintf("%q", tc.want) {
				t.Fatalf("Aliases = %q, want %q", d.Aliases, tc.want)
			}
		})
	}
}

func TestParsePropertyLinks(t *testing.T) {
	note := "---\n" +
		"title: \"[[Title link]]\"\n" + // title is an ordinary property
		"tags: [\"[[no]]\"]\n" +
		"aliases: [\"[[no]]\"]\n" +
		"up: \"[[Orchard/Plan#Goals|the plan]]\"\n" +
		"related:\n" +
		"  - \"[[A]] and ![[map.png]]\"\n" +
		"  - plain\n" +
		"  - '[[B]]'\n" +
		"nested:\n" +
		"  k: \"[[no]]\"\n" +
		"n: 3\n" +
		"empty: \"[[]] and [[ ]]\"\n" +
		"---\n" +
		"body\n"
	d := Parse([]byte(note))
	if d.PropError != nil {
		t.Fatal(d.PropError)
	}
	want := []Link{
		{Target: "Title link", Key: "title", Line: 2},
		{Target: "Orchard/Plan", Subpath: "#Goals", Alias: "the plan", Key: "up", Line: 5},
		{Target: "A", Key: "related", Line: 6},
		{Target: "map.png", Embed: true, Key: "related", Line: 6},
		{Target: "B", Key: "related", Line: 6},
	}
	var got []Link
	for _, l := range d.Links {
		if l.Kind == LinkProperty {
			got = append(got, l)
		}
	}
	if len(got) != len(want) {
		t.Fatalf("property links = %+v, want %d", got, len(want))
	}
	for i, l := range got {
		w := want[i]
		w.Kind, w.Start, w.End = LinkProperty, l.Start, l.End
		if l != w {
			t.Errorf("link %d = %+v, want %+v", i, l, w)
		}
		text := note[l.Start:l.End]
		if !strings.HasSuffix(text, "]]") || !strings.HasPrefix(strings.TrimPrefix(text, "!"), "[[") ||
			!strings.Contains(text, w.Target) || (l.Embed != strings.HasPrefix(text, "!")) {
			t.Errorf("link %d covers %q", i, text)
		}
	}
	if got[2].Start >= got[3].Start || got[3].Start >= got[4].Start {
		t.Errorf("links of one key are out of order: %+v", got[2:])
	}
}

func TestParsePropertyLinkOffsets(t *testing.T) {
	for _, tc := range []struct {
		name, note, covered string
		line                int
	}{
		{"plain scalar", "---\nup: see [[Plan]]\n---\n", "[[Plan]]", 2},
		{"after a multibyte key", "---\ncafé: \"[[Plan]]\"\n---\n", "[[Plan]]", 2},
		{"bom and crlf", "\xef\xbb\xbf---\r\na: 1\r\nup: \"[[Plan]]\"\r\n---\r\n", "[[Plan]]", 3},
		{"second item", "---\nup:\n  - x\n  - \"[[Plan]]\"\n---\n", "[[Plan]]", 2},
	} {
		t.Run(tc.name, func(t *testing.T) {
			d := Parse([]byte(tc.note))
			if len(d.Links) != 1 {
				t.Fatalf("links = %+v, want one", d.Links)
			}
			l := d.Links[0]
			if got := tc.note[l.Start:l.End]; got != tc.covered || l.Line != tc.line {
				t.Fatalf("link covers %q on line %d, want %q on line %d", got, l.Line, tc.covered, tc.line)
			}
		})
	}
}
