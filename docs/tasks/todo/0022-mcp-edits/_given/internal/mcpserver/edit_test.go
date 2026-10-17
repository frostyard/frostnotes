package mcpserver

// CONTRACT TEST for task card T-0022 (docs/tasks). Do not edit.

import (
	"strings"
	"testing"
)

func TestReplaceOnce(t *testing.T) {
	for _, tc := range []struct {
		content, old, new string
		want, err         string
	}{
		{"a b c", "b", "B", "a B c", ""},
		{"one\r\ntwo\r\n", "one\r\n", "", "two\r\n", ""},
		{"caf\uFFFD au lait", "au lait", "noir", "caf\uFFFD noir", ""},
		{"a b b", "b", "x", "", "2 times"},
		{"abc", "z", "x", "", "does not occur"},
		{"abc", "", "x", "", "empty"},
	} {
		got, err := replaceOnce(tc.content, tc.old, tc.new)
		switch {
		case tc.err == "" && (err != nil || got != tc.want):
			t.Errorf("replaceOnce(%q, %q, %q) = %q, %v; want %q", tc.content, tc.old, tc.new, got, err, tc.want)
		case tc.err != "" && (err == nil || !strings.Contains(err.Error(), tc.err)):
			t.Errorf("replaceOnce(%q, %q, %q) = %q, %v; want an error with %q", tc.content, tc.old, tc.new, got, err, tc.err)
		}
	}
}

func TestAppendText(t *testing.T) {
	for _, tc := range []struct{ content, text, want string }{
		{"", "x", "x\n"},
		{"a", "x", "a\nx\n"},
		{"a\n", "x\n", "a\nx\n"},
		{"a\n\n", "x", "a\n\nx\n"},
		{"a\r\n", "- one\n- two", "a\r\n- one\n- two\n"},
	} {
		if got := appendText(tc.content, tc.text); got != tc.want {
			t.Errorf("appendText(%q, %q) = %q, want %q", tc.content, tc.text, got, tc.want)
		}
	}
}

func TestPrependText(t *testing.T) {
	for _, tc := range []struct{ content, text, want string }{
		{"", "x", "x\n"},
		{"body\n", "x", "x\nbody\n"},
		{"---\ntitle: t\n---\nbody", "x", "---\ntitle: t\n---\nx\nbody"},
		{"---\r\ntitle: t\r\n---\r\nbody", "x\n", "---\r\ntitle: t\r\n---\r\nx\nbody"},
		{"\ufeff# Heading\n", "x", "\ufeffx\n# Heading\n"},
		{"---\nnot closed\n", "x", "x\n---\nnot closed\n"},
	} {
		if got := prependText(tc.content, tc.text); got != tc.want {
			t.Errorf("prependText(%q, %q) = %q, want %q", tc.content, tc.text, got, tc.want)
		}
	}
}

func TestSetProperties(t *testing.T) {
	note := "---\n" +
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
	for _, tc := range []struct {
		name    string
		content string
		props   map[string]any
		want    string
	}{
		{"replace in place", note, map[string]any{"status": "done", "tags": []any{"x"}},
			strings.Replace(strings.Replace(note, "status: draft\n", "status: done\n", 1), "tags:\n  - a\n  - b\n", "tags:\n  - x\n", 1)},
		{"remove", note, map[string]any{"other": nil, "tags": nil},
			strings.Replace(strings.Replace(note, "other: 1\n", "", 1), "tags:\n  - a\n  - b\n", "", 1)},
		{"a quoted key, by its name", note, map[string]any{"due date": "2026-11-01"},
			strings.Replace(note, "\"due date\": 2026-10-01\n", "due date: \"2026-11-01\"\n", 1)},
		{"add at the end", note, map[string]any{"up": "[[Plan]]", "count": float64(3)},
			strings.Replace(note, "other: 1\n---\n", "other: 1\ncount: 3\nup: '[[Plan]]'\n---\n", 1)},
		{"nothing to remove", note, map[string]any{"missing": nil}, note},
		{"no frontmatter yet", "# Plan\n", map[string]any{"status": "done"}, "---\nstatus: done\n---\n# Plan\n"},
		{"empty frontmatter", "---\n---\nbody", map[string]any{"a": true}, "---\na: true\n---\nbody"},
		{"CRLF", "---\r\ntitle: t\r\n---\r\nbody\r\n", map[string]any{"title": "u", "new": "v"},
			"---\r\ntitle: u\r\nnew: v\r\n---\r\nbody\r\n"},
		{"a mapping value", "---\na: 1\n---\n", map[string]any{"meta": map[string]any{"by": "wren"}}, "---\na: 1\nmeta:\n  by: wren\n---\n"},
		{"after a BOM", "\ufeff# Plan\n", map[string]any{"a": "b"}, "\ufeff---\na: b\n---\n# Plan\n"},
	} {
		got, err := setProperties(tc.content, tc.props)
		if err != nil || got != tc.want {
			t.Errorf("%s: setProperties = %q, %v\nwant %q", tc.name, got, err, tc.want)
		}
	}
	for name, content := range map[string]string{
		"a list":         "---\n- a\n- b\n---\nbody",
		"invalid YAML":   "---\na: [\n---\nbody",
		"a repeated key": "---\na: 1\na: 2\n---\nbody",
	} {
		if got, err := setProperties(content, map[string]any{"k": "v"}); err == nil {
			t.Errorf("setProperties on frontmatter that is %s = %q, want an error", name, got)
		}
	}
}

func TestApplyEdit(t *testing.T) {
	const note = "---\nstatus: draft\n---\none\ntwo\n"
	for _, tc := range []struct {
		in   editIn
		want string
	}{
		{editIn{OldText: "two", NewText: "2"}, "---\nstatus: draft\n---\none\n2\n"},
		{editIn{OldText: "two\n"}, "---\nstatus: draft\n---\none\n"},
		{editIn{Append: "three"}, note + "three\n"},
		{editIn{Prepend: "zero"}, "---\nstatus: draft\n---\nzero\none\ntwo\n"},
		{editIn{Properties: map[string]any{"status": "done"}}, "---\nstatus: done\n---\none\ntwo\n"},
	} {
		got, err := applyEdit(note, tc.in)
		if err != nil || got != tc.want {
			t.Errorf("applyEdit(%+v) = %q, %v; want %q", tc.in, got, err, tc.want)
		}
	}
	for _, in := range []editIn{
		{},
		{Append: "x", Prepend: "y"},
		{OldText: "one", NewText: "1", Append: "x"},
		{NewText: "orphan"},
		{Properties: map[string]any{}},
		{OldText: "missing", NewText: "x"},
	} {
		if got, err := applyEdit(note, in); err == nil {
			t.Errorf("applyEdit(%+v) = %q, want an error", in, got)
		}
	}
}
