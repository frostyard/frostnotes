package editor

import (
	"reflect"
	"strings"
	"testing"
)

const propNote = "---\n" +
	"title: Plan # the name\n" +
	"status: draft\n" +
	"tags:\n" +
	"  - a\n" +
	"  - b\n" +
	"\n" +
	"# owners follow\n" +
	"\"due date\": 2026-10-01\n" +
	"other: 1\n" +
	"---\n" +
	"body\n"

func setOK(t *testing.T, text string, props map[string]any, want string) {
	t.Helper()
	got, err := SetProperties(text, props)
	if err != nil {
		t.Fatalf("SetProperties(%q): %v", text, err)
	}
	if got != want {
		t.Errorf("SetProperties:\n%q\nwant\n%q", got, want)
	}
}

// The cases of app/src/editor/properties.test.ts.
func TestSetProperties(t *testing.T) {
	setOK(t, propNote, map[string]any{"status": "done", "tags": []any{"x"}},
		strings.NewReplacer("status: draft\n", "status: done\n", "tags:\n  - a\n  - b\n", "tags:\n  - x\n").Replace(propNote))
	setOK(t, propNote, map[string]any{"other": nil, "tags": nil},
		strings.NewReplacer("other: 1\n", "", "tags:\n  - a\n  - b\n", "").Replace(propNote))
	setOK(t, propNote, map[string]any{"missing": nil}, propNote)
	setOK(t, propNote, map[string]any{"due date": "2026-11-01"},
		strings.Replace(propNote, "\"due date\": 2026-10-01\n", "due date: 2026-11-01\n", 1))
	setOK(t, propNote, map[string]any{"up": "[[Plan]]", "count": int64(3)},
		strings.Replace(propNote, "other: 1\n---\n", "other: 1\ncount: 3\nup: \"[[Plan]]\"\n---\n", 1))
	setOK(t, "# Plan\n", map[string]any{"status": "done"}, "---\nstatus: done\n---\n# Plan\n")
	setOK(t, "# Plan\n", map[string]any{"status": nil}, "# Plan\n")
	setOK(t, "---\n---\nbody", map[string]any{"a": true}, "---\na: true\n---\nbody")
	setOK(t, "---\r\ntitle: t\r\n---\r\nbody\r\n", map[string]any{"title": "u", "new": "v"}, "---\r\ntitle: u\r\nnew: v\r\n---\r\nbody\r\n")
	setOK(t, "---\na: 1\n---\n", map[string]any{"meta": map[string]any{"by": "wren"}, "list": []any{"x", "y"}},
		"---\na: 1\nlist:\n  - x\n  - y\nmeta:\n  by: wren\n---\n")
	setOK(t, "\uFEFF# Plan\n", map[string]any{"a": "b"}, "\uFEFF---\na: b\n---\n# Plan\n")
	setOK(t, "\uFEFF---\na: 1\n---\n", map[string]any{"a": int64(2)}, "\uFEFF---\na: 2\n---\n")
	for _, note := range []string{"---\n- a\n- b\n---\nbody", "---\na: [\n---\nbody", "---\na: 1\na: 2\n---\nbody", "---\n{a: 1, b: 2}\n---\nbody"} {
		if _, err := SetProperties(note, map[string]any{"k": "v"}); err == nil {
			t.Errorf("%q took a property", note)
		}
	}
}

func TestReadFrontmatter(t *testing.T) {
	want := []Property{{"title", "Plan"}, {"status", "draft"}, {"tags", []any{"a", "b"}}, {"due date", "2026-10-01"}, {"other", int64(1)}}
	if got := ReadFrontmatter(propNote); !reflect.DeepEqual(got.Properties, want) || got.Error != "" {
		t.Errorf("%+v", got)
	}
	if got := ReadFrontmatter("# Plan\n"); got.Exists {
		t.Errorf("%+v", got)
	}
	if got := ReadFrontmatter("---\n---\nbody"); !got.Exists || len(got.Properties) != 0 || got.Error != "" {
		t.Errorf("%+v", got)
	}
	if ReadFrontmatter("---\n- a\n---\n").Error == "" || ReadFrontmatter("---\na: [\n---\n").Error == "" {
		t.Error("a broken frontmatter reads")
	}
}

func TestRenameProperty(t *testing.T) {
	ok := func(text, from, to, want string) {
		t.Helper()
		got, err := RenameProperty(text, from, to)
		if err != nil || got != want {
			t.Errorf("rename %s to %s: %q, %v; want %q", from, to, got, err, want)
		}
	}
	ok(propNote, "status", "state", strings.Replace(propNote, "status: draft\n", "state: draft\n", 1))
	ok(propNote, "tags", "labels", strings.Replace(propNote, "tags:\n", "labels:\n", 1))
	ok("---\r\na: 1\r\nb: 2\r\n---\r\n", "a", "c", "---\r\nc: 1\r\nb: 2\r\n---\r\n")
	ok(propNote, "status", "status", propNote)
	if _, err := RenameProperty(propNote, "missing", "x"); err == nil {
		t.Error("a missing key renamed")
	}
	if _, err := RenameProperty(propNote, "status", "other"); err == nil {
		t.Error("a name taken")
	}
}

func TestPropertyType(t *testing.T) {
	cases := []struct {
		key   string
		value any
		vault string
		want  string
	}{
		{"due", "2026-10-01", "text", TypeText}, {"x", "1", "number", TypeNumber}, {"x", "a", "unknown", TypeText},
		{"tags", []any{"a"}, "", TypeTags}, {"Aliases", "a", "", TypeAliases},
		{"l", []any{"a", "b"}, "", TypeMultitext}, {"c", true, "", TypeCheckbox}, {"n", 3.5, "", TypeNumber},
		{"d", "2026-10-18", "", TypeDate}, {"t", "2026-10-18T09:41", "", TypeDatetime}, {"s", "hello", "", TypeText}, {"e", nil, "", TypeText},
	}
	for _, c := range cases {
		if got := PropertyType(c.key, c.value, c.vault); got != c.want {
			t.Errorf("%s %v %q: %s, want %s", c.key, c.value, c.vault, got, c.want)
		}
	}
}

func TestRenameKeepsNaNValues(t *testing.T) {
	for _, value := range []string{".nan", "[.nan, 1]", "{nested: [.nan, .inf]}"} {
		before := "---\na: " + value + " # keep\n---\nBody\n"
		got, err := RenameProperty(before, "a", "b")
		want := "---\nb: " + value + " # keep\n---\nBody\n"
		if err != nil || got != want {
			t.Fatalf("rename %s: %q, %v", value, got, err)
		}
	}
}
