package main

import (
	"encoding/json"
	"encoding/json/jsontext"
	"reflect"
	"strings"
	"testing"

	"github.com/frostyard/frostnotes/api"
)

func TestQuery(t *testing.T) {
	result := api.QueryResult{
		View:  api.QueryView{Name: "Open", Type: "table"},
		Views: []api.QueryView{{Name: "Open", Type: "table"}},
		Columns: []api.QueryColumn{{Key: "file.name", DisplayName: "Name"}, {Key: "note.status", DisplayName: "Status"},
			{Key: "tags", DisplayName: "tags"}, {Key: "formula.x", DisplayName: "x"}},
		Rows: []api.QueryRow{
			{Path: "A.md", Title: "A", Kind: api.FileKindNote, Values: toValues(`"A.md"`, `"open"`, `["a","b",1]`, `null`)},
			{Path: "B.md", Title: "B", Kind: api.FileKindNote, Values: toValues(`"B.md"`, `null`, `[]`, `null`)},
			{Path: "C\td.pdf", Title: "C", Kind: api.FileKindAttachment, Values: toValues(`"C\td.pdf"`, `4.5`, `true`, `null`)},
		},
		Total:   7,
		Unshown: []string{"formula.x"},
	}
	fake := &fakeNotes{query: result}
	srv := notesServer(t, fake, "garden")

	want := "Name\tStatus\ttags\tx\n" +
		"A.md\topen\ta, b, 1\t\n" +
		"B.md\t\t\t\n" +
		"C d.pdf\t4.5\ttrue\t\n" +
		"3 of 7 rows\n"
	if out := notes(t, srv, "query", "Boards/Open.base"); out != want {
		t.Errorf("query = %q, want %q", out, want)
	}
	checkCall(t, fake.only(t), &api.NoteQueryParams{VaultID: 1, Path: ptr("Boards/Open.base")})

	notes(t, srv, "query", "Boards/Open.base", "--view", "Open", "--limit", "5", "--from", "Plan.md")
	checkCall(t, fake.only(t), &api.NoteQueryParams{VaultID: 1, Path: ptr("Boards/Open.base"), View: ptr("Open"), From: ptr("Plan.md"), Limit: ptr(int64(5))})

	out, err := notesIn(t, srv, "filters: 'true'\n", "query", "-")
	if err != nil || out != want {
		t.Fatalf("query - = %q, %v", out, err)
	}
	checkCall(t, fake.only(t), &api.NoteQueryParams{VaultID: 1, Source: ptr("filters: 'true'\n")})

	var got api.QueryResult
	if err := json.Unmarshal([]byte(notes(t, srv, "--json", "query", "x.base")), &got); err != nil || len(got.Rows) != 3 || got.Rows[2].Path != "C\td.pdf" || len(got.Rows[0].Values) != 4 || got.Total != 7 || !reflect.DeepEqual(got.Unshown, result.Unshown) {
		t.Errorf("query --json = %+v, %v", got, err)
	}
	fake.only(t)

	// Groups: a line "## VALUE" before each, the null group's empty.
	fake.query.Groups = []api.QueryGroup{{Value: jsontext.Value(`"open"`), Count: 1}, {Value: jsontext.Value(`null`), Count: 2}}
	want = "Name\tStatus\ttags\tx\n## open\nA.md\topen\ta, b, 1\t\n##\nB.md\t\t\t\nC d.pdf\t4.5\ttrue\t\n3 of 7 rows\n"
	if out := notes(t, srv, "query", "x.base"); out != want {
		t.Errorf("grouped query = %q, want %q", out, want)
	}
	fake.only(t)

	// A notice exits 0.
	fake.query = api.QueryResult{Notice: ptr("views[1]: view type kanban is outside the subset")}
	if out := notes(t, srv, "query", "x.base"); out != "notice: views[1]: view type kanban is outside the subset\n" {
		t.Errorf("a notice = %q", out)
	}
	fake.only(t)

	// A refused base is an error.
	fake.err = &api.Error{Code: api.CodeInvalidParams, Message: `filters: "formula.x": formula.x is outside the subset`}
	if err := notesErr(t, srv, "query", "x.base"); !strings.Contains(err.Error(), "formula.x is outside the subset") {
		t.Errorf("a refused base: %v", err)
	}
	fake.err = nil
	fake.only(t)

	if err := notesErr(t, srv, "query"); err == nil {
		t.Error("query without a path")
	}
}

func toValues(vs ...string) []jsontext.Value {
	out := make([]jsontext.Value, len(vs))
	for i, v := range vs {
		out[i] = jsontext.Value(v)
	}
	return out
}
