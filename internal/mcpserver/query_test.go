package mcpserver

import (
	"encoding/json/v2"
	"fmt"
	"slices"
	"strings"
	"testing"
)

// queryFixture is a vault of made-up notes, an attachment and two bases
// for query_notes (ADR-0037's check 7).
func queryFixture() map[string]string {
	return map[string]string{
		"Garden/Quince.md":  "---\nstatus: open\npriority: 3\ntags: [garden]\n---\nSECRETBODY-quince\n",
		"Garden/Pear.md":    "---\nstatus: done\npriority: 1\ntags: [garden]\n---\nSECRETBODY-pear\n",
		"Projects/Fence.md": "---\nstatus: open\npriority: 5\n---\nSECRETBODY-fence\n",
		"Projects/Gate.md":  "---\nstatus: open\n---\nSECRETBODY-gate\n",
		"Garden/photo.png":  "made-up image bytes",
		"Bases/Open.base": `filters: 'status == "open"'
views:
  - type: table
    name: Open
    order: [file.name, note.status, priority, formula.score]
    sort:
      - property: priority
        direction: DESC
  - type: kanban
    name: Board
  - type: table
    name: Grouped
    groupBy: {property: status, direction: ASC}
    groupOrder: [open]
  - type: table
    name: Here
    filters: 'file.inFolder(this.file.folder)'
`,
		"Bases/Board.base": "views:\n  - {type: kanban, name: Board}\n  - {type: table, name: Table}\n",
	}
}

// queryPaths is the paths of out's files, in order.
func queryPaths(out queryOut) []string {
	paths := make([]string, len(out.Files))
	for i, file := range out.Files {
		paths[i] = file.Path
	}
	return paths
}

// filters, as a statement or an object, reach note.query intact and return
// rows with no note's content, attachments too.
func TestQueryNotesFiltersReturnOnlyMetadata(t *testing.T) {
	h := newHarness(t, queryFixture(), false)
	for _, tc := range []struct {
		name    string
		filters any
		want    []string
	}{
		{"statement", `status == "open"`, []string{"Garden/Quince.md", "Projects/Fence.md", "Projects/Gate.md"}},
		{"nested object", map[string]any{"and": []any{`status == "open"`, map[string]any{"not": []any{`file.inFolder("Projects")`}}}}, []string{"Garden/Quince.md"}},
		{"quotes unicode and comparison", `priority > 2 && status != "dône \"x\""`, []string{"Garden/Quince.md", "Projects/Fence.md"}},
		{"attachment", `file.ext == "png"`, []string{"Garden/photo.png"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			res := h.call("query_notes", map[string]any{"filters": tc.filters})
			text := resultText(t, res)
			if res.IsError {
				t.Fatal(text)
			}
			var out queryOut
			if err := json.Unmarshal([]byte(text), &out); err != nil {
				t.Fatal(err)
			}
			if got := queryPaths(out); !slices.Equal(got, tc.want) || out.Total != len(tc.want) || out.Notice != Notice {
				t.Fatalf("query = %+v, paths %v; want %v and Notice", out, got, tc.want)
			}
			for _, file := range out.Files {
				if file.Properties == nil || len(file.Properties) != 0 {
					t.Errorf("%s properties = %#v, want {}", file.Path, file.Properties)
				}
				if file.Path == "Garden/photo.png" && file.Title != "photo" {
					t.Errorf("attachment title = %q, want photo", file.Title)
				}
			}
			if strings.Contains(text, "SECRETBODY") || strings.Contains(text, `"content"`) {
				t.Errorf("query leaked content: %s", text)
			}
		})
	}
}

// properties come back by the names given, a missing value as null, each
// name once.
func TestQueryNotesPropertiesKeepNamesNullsAndDeduplicate(t *testing.T) {
	files := queryFixture()
	h := newHarness(t, files, false)
	var out queryOut
	h.ok("query_notes", map[string]any{"filters": `status == "open"`, "properties": []string{"status", "note.priority", "file.size", "status", "note.priority"}}, &out)
	if out.Total != 3 || len(out.Files) != 3 {
		t.Fatalf("query = %+v, want 3 files", out)
	}
	priorities := map[string]any{"Garden/Quince.md": float64(3), "Projects/Fence.md": float64(5), "Projects/Gate.md": nil}
	for _, file := range out.Files {
		priority, present := file.Properties["note.priority"]
		if len(file.Properties) != 3 || file.Properties["status"] != "open" || !present || priority != priorities[file.Path] || file.Properties["file.size"] != float64(len(files[file.Path])) {
			t.Errorf("%s properties = %#v", file.Path, file.Properties)
		}
	}
}

// sort orders the files, a missing value last.
func TestQueryNotesSortPutsMissingValuesLast(t *testing.T) {
	h := newHarness(t, queryFixture(), false)
	var out queryOut
	h.ok("query_notes", map[string]any{
		"filters": `file.hasTag("garden") || file.inFolder("Projects")`,
		"sort":    []map[string]any{{"property": "priority", "direction": "DESC"}},
	}, &out)
	want := []string{"Projects/Fence.md", "Garden/Quince.md", "Garden/Pear.md", "Projects/Gate.md"}
	if got := queryPaths(out); !slices.Equal(got, want) {
		t.Fatalf("sorted paths = %v, want %v", got, want)
	}
}

// limit is 1 to 1000, and 0 is the default, as in list_notes.
func TestQueryNotesLimitBoundaries(t *testing.T) {
	h := newHarness(t, queryFixture(), false)
	for _, tc := range []struct {
		name  string
		limit *int
		count int
	}{
		{"absent", nil, 3}, {"zero", new(0), 3}, {"one", new(1), 1}, {"maximum", new(1000), 3},
	} {
		t.Run(tc.name, func(t *testing.T) {
			args := map[string]any{"filters": `status == "open"`}
			if tc.limit != nil {
				args["limit"] = *tc.limit
			}
			var out queryOut
			h.ok("query_notes", args, &out)
			if len(out.Files) != tc.count || out.Total != 3 {
				t.Errorf("query = %+v, want %d files and total 3", out, tc.count)
			}
		})
	}
	for _, limit := range []int{-1, 1001} {
		t.Run(fmt.Sprint(limit), func(t *testing.T) {
			if text := h.fails("query_notes", map[string]any{"filters": `status == "open"`, "limit": limit}); !strings.Contains(text, "limit must be between 1 and 1000") {
				t.Fatal(text)
			}
		})
	}
}

// With path, the base's first view applies, and properties name its
// columns, KEY and note.KEY alike.
func TestQueryNotesPathUsesFirstViewAndMatchesNoteColumns(t *testing.T) {
	h := newHarness(t, queryFixture(), false)
	for _, properties := range [][]string{nil, {"status"}, {"note.status"}} {
		var out queryOut
		args := map[string]any{"path": "Bases/Open.base"}
		if properties != nil {
			args["properties"] = properties
		}
		h.ok("query_notes", args, &out)
		want := []string{"Projects/Fence.md", "Garden/Quince.md", "Projects/Gate.md"}
		if got := queryPaths(out); !slices.Equal(got, want) || out.Total != 3 {
			t.Fatalf("paths = %v, total %d; want %v", got, out.Total, want)
		}
		for _, file := range out.Files {
			if len(file.Properties) != len(properties) {
				t.Errorf("properties = %#v, want names %v", file.Properties, properties)
			}
			for _, property := range properties {
				if file.Properties[property] != "open" {
					t.Errorf("%s properties = %#v", file.Path, file.Properties)
				}
			}
		}
	}
	text := h.fails("query_notes", map[string]any{"path": "Bases/Open.base", "properties": []string{"missing"}})
	for _, want := range []string{`is not a column of view "Open"`, "file.name, note.status, priority, formula.score"} {
		if !strings.Contains(text, want) {
			t.Errorf("error = %s, want %q", text, want)
		}
	}
	text = h.fails("query_notes", map[string]any{"path": "Bases/Open.base", "properties": []string{"formula.score"}})
	if !strings.Contains(text, `property "formula.score" cannot be returned`) {
		t.Fatal(text)
	}
}

// With path, this is the base file.
func TestQueryNotesPathSetsThisToTheBase(t *testing.T) {
	files := queryFixture()
	// This note satisfies both the base's open filter and Here's folder filter.
	files["Bases/Local.md"] = "---\nstatus: open\n---\nSECRETBODY-local\n"
	h := newHarness(t, files, false)
	var out queryOut
	h.ok("query_notes", map[string]any{"path": "Bases/Open.base", "view": "Here"}, &out)
	if got := queryPaths(out); !slices.Equal(got, []string{"Bases/Local.md"}) || out.Total != 1 {
		t.Fatalf("Here = %+v", out)
	}
}

// A view that shows a notice in the app is a tool error that names the
// view, never empty files (ADR-0037, check 7).
func TestQueryNotesUnsupportedViewsAreErrorsWithoutFiles(t *testing.T) {
	h := newHarness(t, queryFixture(), false)
	for _, tc := range []struct {
		name, path, view, want string
	}{
		{"kanban", "Bases/Open.base", "Board", `views[1]: view type kanban is outside the subset`},
		{"groupOrder", "Bases/Open.base", "Grouped", `views[2]: groupOrder is outside the subset`},
		{"first view kanban", "Bases/Board.base", "", `views[0]: view type kanban is outside the subset`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			res := h.call("query_notes", map[string]any{"path": tc.path, "view": tc.view})
			text := resultText(t, res)
			name := tc.view
			if name == "" {
				name = "Board"
			}
			if !res.IsError || res.StructuredContent != nil || strings.Contains(text, `"files"`) || !strings.Contains(text, fmt.Sprintf("view %q", name)) || !strings.Contains(text, tc.want) {
				t.Fatalf("unsupported view: %+v, text %s", res, text)
			}
		})
	}
}

// The source keeps U+0085, U+2028, U+2029 and an emoji, which YAML would
// otherwise fold or break, in literals and property names.
func TestQueryNotesPreservesUnicodeLineBreaksAndEmoji(t *testing.T) {
	files := map[string]string{
		"Labels/Space.md": "---\nlabel: \"a b\"\n---\nspace\n",
		"Labels/Emoji.md": "---\nlabel: \"🍐 pear\"\n---\nemoji\n",
	}
	for _, r := range []rune{'\u0085', '\u2028', '\u2029'} {
		files[fmt.Sprintf("Labels/U%04X.md", r)] = fmt.Sprintf("---\nlabel: \"a\\u%04xb\"\n\"p\\u%04xq\": preserved\n---\nlabel\n", r, r)
	}
	h := newHarness(t, files, false)
	for _, r := range []rune{'\u0085', '\u2028', '\u2029'} {
		t.Run(fmt.Sprintf("U%04X", r), func(t *testing.T) {
			property := "p" + string(r) + "q"
			var out queryOut
			h.ok("query_notes", map[string]any{
				"filters":    `label == "a` + string(r) + `b"`,
				"properties": []string{property},
			}, &out)
			want := fmt.Sprintf("Labels/U%04X.md", r)
			if got := queryPaths(out); !slices.Equal(got, []string{want}) || out.Total != 1 {
				t.Fatalf("paths = %v, total %d; want only %s", got, out.Total, want)
			}
			if properties := out.Files[0].Properties; len(properties) != 1 || properties[property] != "preserved" {
				t.Errorf("properties = %#v, want %q preserved", properties, property)
			}
		})
	}
	var out queryOut
	h.ok("query_notes", map[string]any{"filters": `label == "🍐 pear"`}, &out)
	if got := queryPaths(out); !slices.Equal(got, []string{"Labels/Emoji.md"}) || out.Total != 1 {
		t.Fatalf("emoji paths = %v, total %d", got, out.Total)
	}
}

// An unnamed view with a notice is an error that says so.
func TestQueryNotesUnnamedViewNoticeIsAnErrorWithoutFiles(t *testing.T) {
	h := newHarness(t, map[string]string{"Bases/Unnamed.base": "views:\n  - {type: kanban}\n"}, false)
	res := h.call("query_notes", map[string]any{"path": "Bases/Unnamed.base"})
	text := resultText(t, res)
	if !res.IsError || res.StructuredContent != nil || strings.Contains(text, `"files"`) || !strings.Contains(text, "an unnamed view: views[0]: view type kanban is outside the subset") {
		t.Fatalf("unnamed view = %+v, text %s", res, text)
	}
}

// No input adds a key or a view to the base the tool builds.
func TestQueryNotesInputCannotChangeTheBaseShape(t *testing.T) {
	h := newHarness(t, queryFixture(), false)
	t.Run("filter cannot inject a view", func(t *testing.T) {
		text := h.fails("query_notes", map[string]any{"filters": `x"}],"views":[{"type":"kanban"}]`})
		if !strings.Contains(text, "invalidParams") || !strings.Contains(text, "filters:") || strings.Contains(text, "kanban is outside the subset") {
			t.Fatal(text)
		}
	})
	t.Run("quoted and multiline property names", func(t *testing.T) {
		names := []string{`a"b`, "c\nd"}
		var out queryOut
		h.ok("query_notes", map[string]any{"filters": `file.ext == "md"`, "properties": names}, &out)
		if len(out.Files) != 4 || out.Total != 4 {
			t.Fatalf("query = %+v, want 4 notes", out)
		}
		for _, file := range out.Files {
			if len(file.Properties) != len(names) {
				t.Errorf("%s properties = %#v", file.Path, file.Properties)
			}
			for _, name := range names {
				if value, ok := file.Properties[name]; !ok || value != nil {
					t.Errorf("%s property %q = %#v, present %t; want null", file.Path, name, value, ok)
				}
			}
		}
	})
	t.Run("filter mapping has one key", func(t *testing.T) {
		text := h.fails("query_notes", map[string]any{"filters": map[string]any{"and": []string{"true"}, "or": []string{"true"}}})
		if !strings.Contains(text, "invalidParams") || !strings.Contains(text, "a filter mapping has exactly one key") {
			t.Fatal(text)
		}
	})
}

// The default limit is 200, absent or 0, not note.query's 1000.
func TestQueryNotesDefaultLimitReturns200Of201Matches(t *testing.T) {
	files := make(map[string]string)
	for i := range 201 {
		files[fmt.Sprintf("Many/Note-%03d.md", i)] = "made-up note\n"
	}
	h := newHarness(t, files, false)
	var previous []string
	for _, tc := range []struct {
		name  string
		limit *int
	}{{"absent", nil}, {"zero", new(0)}} {
		t.Run(tc.name, func(t *testing.T) {
			args := map[string]any{"filters": `file.inFolder("Many")`}
			if tc.limit != nil {
				args["limit"] = *tc.limit
			}
			var out queryOut
			h.ok("query_notes", args, &out)
			if len(out.Files) != 200 || out.Total != len(files) {
				t.Fatalf("query returned %d files, total %d; want 200, 201", len(out.Files), out.Total)
			}
			paths := queryPaths(out)
			if previous != nil && !slices.Equal(paths, previous) {
				t.Errorf("zero limit paths differ from absent limit")
			}
			previous = paths
		})
	}
}

// Bad input is a tool error, and notesd's refusals pass through.
func TestQueryNotesRefusesInvalidInputsAndPreservesNotesdErrors(t *testing.T) {
	h := newHarness(t, queryFixture(), false)
	for _, tc := range []struct {
		name string
		args map[string]any
		want []string
	}{
		{"no source", map[string]any{}, []string{"give filters or path"}},
		{"two sources", map[string]any{"filters": "true", "path": "Bases/Open.base"}, []string{"give filters or path, not both"}},
		{"view without path", map[string]any{"filters": "true", "view": "Open"}, []string{"view needs path"}},
		{"non-base path", map[string]any{"path": "Garden/Quince.md"}, []string{"path must be a .base file"}},
		{"sort with path", map[string]any{"path": "Bases/Open.base", "sort": []map[string]any{{"property": "priority"}}}, []string{"sort applies only with filters: a base's view sets its own order"}},
		{"empty property", map[string]any{"filters": "true", "properties": []string{"status", ""}}, []string{"a property name is empty"}},
		{"this without path", map[string]any{"filters": `this.file.name == "x"`}, []string{"this is not set"}},
		{"unsupported filter", map[string]any{"filters": `file.backlinks.isEmpty()`}, []string{"invalidParams", "filters:", "outside the subset"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			text := h.fails("query_notes", tc.args)
			for _, want := range tc.want {
				if !strings.Contains(text, want) {
					t.Errorf("error = %s, want %q", text, want)
				}
			}
		})
	}
}
