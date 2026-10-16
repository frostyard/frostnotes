package markdown

import (
	"bufio"
	"encoding/json/v2"
	"fmt"
	"os"
	"reflect"
	"regexp"
	"slices"
	"strings"
	"testing"
)

// parseFixture is one "### Fnn" fixture from the "Parse fixtures" section
// of docs/specs/markdown.md: a whole note and what Parse returns for it.
type parseFixture struct {
	Name string // the heading, "F01 — Wikilink forms"
	Note []byte // the md block's content, each line ending in "\n"
	Want fixtureDoc
}

// fixtureDoc is a fixture's json block. Omitted fields are empty or zero;
// fixtures check lines, not offsets.
type fixtureDoc struct {
	Links []struct {
		Kind    LinkKind `json:"kind"` // "" means wiki
		Target  string   `json:"target"`
		Subpath string   `json:"subpath"`
		Alias   string   `json:"alias"`
		Embed   bool     `json:"embed"`
		Key     string   `json:"key"`
		Line    int      `json:"line"`
	} `json:"links"`
	Tags []struct {
		Name  string `json:"name"`
		Front bool   `json:"front"`
		Line  int    `json:"line"`
	} `json:"tags"`
	Headings []struct {
		Level int    `json:"level"`
		Text  string `json:"text"`
		Line  int    `json:"line"`
	} `json:"headings"`
	Blocks []struct {
		ID   string `json:"id"`
		Line int    `json:"line"`
	} `json:"blocks"`
	Tasks []struct {
		Status string `json:"status"`
		Text   string `json:"text"`
		Line   int    `json:"line"`
	} `json:"tasks"`
	Aliases   []string       `json:"aliases"`
	Props     map[string]any `json:"props"`
	PropError bool           `json:"propError"`
	Words     int            `json:"words"`
}

var fixtureHeading = regexp.MustCompile(`^### (F\d\d .*)$`)

// specParseFixtures reads every parse fixture from the spec, so the spec
// and the tests cannot drift apart.
func specParseFixtures(t *testing.T) []parseFixture {
	t.Helper()
	f, err := os.Open("../../docs/specs/markdown.md")
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	var out []parseFixture
	var cur *parseFixture
	var fence, lang string
	var block []string
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		line := sc.Text()
		switch {
		case fence != "" && line == fence:
			if err := cur.add(lang, block); err != nil {
				t.Fatalf("%s: %v", cur.Name, err)
			}
			fence, block = "", nil
		case fence != "":
			block = append(block, line)
		case strings.HasPrefix(line, "## "):
			cur = nil
		case fixtureHeading.MatchString(line):
			out = append(out, parseFixture{Name: fixtureHeading.FindStringSubmatch(line)[1]})
			cur = &out[len(out)-1]
		case cur != nil && strings.HasPrefix(line, "```"):
			ticks := strings.TrimRight(line, "abcdefghijklmnopqrstuvwxyz")
			fence, lang = ticks, line[len(ticks):]
		}
	}
	if err := sc.Err(); err != nil {
		t.Fatal(err)
	}
	if len(out) < 14 {
		t.Fatalf("read %d parse fixtures from the spec; the section moved or changed shape", len(out))
	}
	return out
}

// add takes one fenced block of a fixture: the md block, then the json.
func (f *parseFixture) add(lang string, lines []string) error {
	switch {
	case lang == "md" && f.Note == nil:
		f.Note = []byte(strings.Join(lines, "\n") + "\n")
	case lang == "json" && f.Note != nil:
		return json.Unmarshal([]byte(strings.Join(lines, "\n")), &f.Want, json.RejectUnknownMembers(true))
	default:
		return fmt.Errorf("unexpected %q block: a fixture is an md block, then a json block", lang)
	}
	return nil
}

// Parts of a Doc that a fixture test compares.
const (
	partProps         = "props" // Props, PropError and Aliases
	partFrontTags     = "front tags"
	partPropertyLinks = "property links"
	partWikiLinks     = "wiki links"
	partMarkdownLinks = "markdown links"
	partInlineTags    = "inline tags"
	partHeadings      = "headings"
	partBlocks        = "blocks"
	partTasks         = "tasks"
	partWords         = "words"
)

var allParts = []string{
	partProps, partFrontTags, partPropertyLinks, partWikiLinks, partMarkdownLinks,
	partInlineTags, partHeadings, partBlocks, partTasks, partWords,
}

// diffDoc compares the named parts of got with want, ignoring offsets, and
// describes each difference. Links and tags of one kind are compared in
// their order in the whole list.
func diffDoc(want fixtureDoc, got Doc, parts ...string) []string {
	var diffs []string
	for _, p := range parts {
		w, g := project(p, want, got)
		if !reflect.DeepEqual(w, g) {
			diffs = append(diffs, fmt.Sprintf("%s:\n got %v\nwant %v", p, g, w))
		}
	}
	return diffs
}

// project renders one part of each side in a comparable form.
func project(part string, want fixtureDoc, got Doc) (w, g []string) {
	link := func(kind LinkKind, target, subpath, alias string, embed bool, key string, line int) string {
		return fmt.Sprintf("{%s %q %q %q embed=%t key=%q line %d}", kind, target, subpath, alias, embed, key, line)
	}
	links := func(kind LinkKind) {
		for _, l := range want.Links {
			if cmp := cmpOr(l.Kind, LinkWiki); cmp == kind {
				w = append(w, link(cmp, l.Target, l.Subpath, l.Alias, l.Embed, l.Key, l.Line))
			}
		}
		for _, l := range got.Links {
			if l.Kind == kind {
				g = append(g, link(l.Kind, l.Target, l.Subpath, l.Alias, l.Embed, l.Key, l.Line))
			}
		}
	}
	tags := func(front bool) {
		for _, t := range want.Tags {
			if t.Front == front {
				w = append(w, fmt.Sprintf("{%q line %d}", t.Name, t.Line))
			}
		}
		for _, t := range got.Tags {
			if t.Front == front {
				g = append(g, fmt.Sprintf("{%q line %d}", t.Name, t.Line))
			}
		}
	}
	switch part {
	case partProps:
		w = []string{propsJSON(want.Props), fmt.Sprint(want.PropError), fmt.Sprintf("%q", want.Aliases)}
		g = []string{propsJSON(got.Props), fmt.Sprint(got.PropError != nil), fmt.Sprintf("%q", got.Aliases)}
		if len(want.Aliases) == 0 && len(got.Aliases) == 0 {
			w[2], g[2] = "[]", "[]"
		}
	case partFrontTags, partInlineTags:
		tags(part == partFrontTags)
	case partPropertyLinks:
		links(LinkProperty)
	case partWikiLinks:
		links(LinkWiki)
	case partMarkdownLinks:
		links(LinkMarkdown)
	case partHeadings:
		for _, h := range want.Headings {
			w = append(w, fmt.Sprintf("{h%d %q line %d}", h.Level, h.Text, h.Line))
		}
		for _, h := range got.Headings {
			g = append(g, fmt.Sprintf("{h%d %q line %d}", h.Level, h.Text, h.Line))
		}
	case partBlocks:
		for _, b := range want.Blocks {
			w = append(w, fmt.Sprintf("{%q line %d}", b.ID, b.Line))
		}
		for _, b := range got.Blocks {
			g = append(g, fmt.Sprintf("{%q line %d}", b.ID, b.Line))
		}
	case partTasks:
		for _, t := range want.Tasks {
			w = append(w, fmt.Sprintf("{%q %q line %d}", t.Status, t.Text, t.Line))
		}
		for _, t := range got.Tasks {
			g = append(g, fmt.Sprintf("{%q %q line %d}", t.Status, t.Text, t.Line))
		}
	case partWords:
		w, g = []string{fmt.Sprint(want.Words)}, []string{fmt.Sprint(got.Words)}
	default:
		panic("unknown part " + part)
	}
	return w, g
}

func cmpOr(k, def LinkKind) LinkKind {
	if k == "" {
		return def
	}
	return k
}

// propsJSON renders props as JSON with sorted keys; an empty mapping and
// none are both "{}". A value JSON cannot hold is reported as such.
func propsJSON(props map[string]any) string {
	if len(props) == 0 {
		return "{}"
	}
	b, err := json.Marshal(props, json.Deterministic(true))
	if err != nil {
		return fmt.Sprintf("not JSON: %v", err)
	}
	var v any
	if err := json.Unmarshal(b, &v); err != nil {
		return fmt.Sprintf("not JSON: %v", err)
	}
	b, _ = json.Marshal(v, json.Deterministic(true))
	return string(b)
}

// docFrom is the Doc a fixture describes, with no offsets.
func docFrom(f fixtureDoc) Doc {
	d := Doc{Props: f.Props, Aliases: f.Aliases, Words: f.Words}
	if f.PropError {
		d.PropError = fmt.Errorf("fixture")
	}
	for _, l := range f.Links {
		d.Links = append(d.Links, Link{Kind: cmpOr(l.Kind, LinkWiki), Target: l.Target, Subpath: l.Subpath,
			Alias: l.Alias, Embed: l.Embed, Key: l.Key, Line: l.Line})
	}
	for _, t := range f.Tags {
		d.Tags = append(d.Tags, Tag(t))
	}
	for _, h := range f.Headings {
		d.Headings = append(d.Headings, Heading(h))
	}
	for _, b := range f.Blocks {
		d.Blocks = append(d.Blocks, Block(b))
	}
	for _, t := range f.Tasks {
		d.Tasks = append(d.Tasks, Task(t))
	}
	return d
}

// TestParseFixturesRead checks the fixture reader and the comparison the
// parser cards' tests use: every fixture reads, in order, and agrees with
// itself.
func TestParseFixturesRead(t *testing.T) {
	fixtures := specParseFixtures(t)
	for i, f := range fixtures {
		if want := fmt.Sprintf("F%02d ", i+1); !strings.HasPrefix(f.Name, want) {
			t.Errorf("fixture %d is %q, want it to start with %q", i+1, f.Name, want)
		}
		if len(f.Note) < 2 {
			t.Errorf("%s: empty note", f.Name)
		}
		if diffs := diffDoc(f.Want, docFrom(f.Want), allParts...); len(diffs) > 0 {
			t.Errorf("%s disagrees with itself: %s", f.Name, diffs)
		}
	}
	f := fixtures[0]
	if diffs := diffDoc(f.Want, Doc{}, partWikiLinks, partWords); len(diffs) != 2 {
		t.Errorf("an empty Doc against %s: %d differences, want 2: %s", f.Name, len(diffs), diffs)
	}
	if !slices.ContainsFunc(fixtures, func(f parseFixture) bool {
		return strings.Contains(string(f.Note), "````") || strings.Contains(string(f.Note), "```")
	}) {
		t.Error("no fixture holds a fence; the reader no longer sees F02's inner fence")
	}
}
