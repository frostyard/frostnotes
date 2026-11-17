package engine_test

import (
	"encoding/json/jsontext"
	"encoding/json/v2"
	"slices"
	"strings"
	"testing"

	"github.com/frostyard/frostnotes/api"
)

func queryVault(t *testing.T) *apiVault {
	t.Helper()
	dir := t.TempDir()
	writeFiles(t, dir, map[string]string{
		".obsidian/types.json": `{"types":{"rating":"number","due":"date"}}`,
		"Home.md":              "---\ntags: [hub]\n---\nSee [[Plan]]\n",
		"Plan.md":              "---\nstatus: open\nrating: 4\ndue: 2020-01-02\ntags: [work]\n---\nBack to [[Home]]\n",
		"Work/Task.md":         "---\nstatus: done\nrating: 2\ndue: 2999-01-01\n---\nFor [[Plan]]\n",
		"Work/Other.md":        "---\nstatus: open\n---\nNo links\n",
		"Work/data.csv":        "a,b\n",
		"Boards/Open.base": `filters:
  and:
    - file.inFolder("Work")
    - 'status == "open"'
properties:
  note.status:
    displayName: State
views:
  - type: table
    name: Open
    order: [file.name, note.status]
  - type: kanban
    name: Board
`,
		"Boards/Links.base": "filters: file.hasLink(this)\n",
		"Boards/Bad.base":   "filters: 'file.backlinks.isEmpty()'\n",
		"Boards/Big.base":   "filters: 'true'\n# " + strings.Repeat("x", 1<<20),
	})
	return openVault(t, dir)
}

func str(s string) *string { return &s }

func TestNoteQuery(t *testing.T) {
	av := queryVault(t)
	ctx := t.Context()
	run := func(c *api.Client, p api.NoteQueryParams) (*api.QueryResult, error) {
		p.VaultID = av.id
		return c.Note().Query(ctx, &p)
	}
	paths := func(r *api.QueryResult) []string {
		out := []string{}
		for _, row := range r.Rows {
			out = append(out, row.Path)
		}
		return out
	}
	agent := av.srv.DialAs(t, api.Identity{Caller: api.CallerAgent, Token: "claude", Scope: &api.Scope{Vault: av.id}})
	session := av.srv.DialAs(t, api.Identity{Caller: api.CallerSession, Login: "ana@example.com", Node: "phone"})

	t.Run("a base file, through every caller kind that may read", func(t *testing.T) {
		for name, c := range map[string]*api.Client{"local": av.c, "agent": agent, "session": session} {
			res, err := run(c, api.NoteQueryParams{Path: str("Boards/Open.base")})
			if err != nil {
				t.Fatalf("%s: %v", name, err)
			}
			if got, want := paths(res), []string{"Work/Other.md"}; !slices.Equal(got, want) || res.Total != 1 {
				t.Errorf("%s: rows %q total %d, want %q", name, got, res.Total, want)
			}
			if res.View.Name != "Open" || len(res.Views) != 2 || res.Columns[1].DisplayName != "State" ||
				string(res.Rows[0].Values[1]) != `"open"` {
				t.Errorf("%s: result = %+v", name, res)
			}
		}
	})
	t.Run("source", func(t *testing.T) {
		res, err := run(av.c, api.NoteQueryParams{Source: str("filters: 'rating >= 3 && due < today()'\n")})
		if err != nil || !slices.Equal(paths(res), []string{"Plan.md"}) {
			t.Errorf("types from the vault's settings: %v, %v", res, err)
		}
	})
	t.Run("a view and a limit", func(t *testing.T) {
		res, err := run(av.c, api.NoteQueryParams{Path: str("Boards/Open.base"), View: str("Board")})
		if err != nil || res.Notice == nil || *res.Notice != "views[1]: view type kanban is outside the subset" || res.Total != 0 {
			t.Errorf("a notice: %+v, %v", res, err)
		}
		limit := int64(2)
		res, err = run(av.c, api.NoteQueryParams{Source: str("filters: 'true'\n"), Limit: &limit})
		if err != nil {
			t.Fatal(err)
		}
		if len(res.Rows) != 2 || res.Total != 9 {
			t.Errorf("limit 2: %d rows of %d", len(res.Rows), res.Total)
		}
	})
	t.Run("this is the base file for a path, and from for an embed", func(t *testing.T) {
		res, err := run(av.c, api.NoteQueryParams{Path: str("Boards/Links.base")})
		if err != nil || len(res.Rows) != 0 {
			t.Errorf("notes that link to the base: %v, %v", paths(res), err)
		}
		for from, want := range map[string][]string{
			"Plan.md": {"Home.md", "Work/Task.md"},
			"Home.md": {"Plan.md"},
		} {
			res, err = run(av.c, api.NoteQueryParams{Path: str("Boards/Links.base"), From: &from})
			if err != nil || !slices.Equal(paths(res), want) {
				t.Errorf("from %s: %v, %v; want %v", from, paths(res), err, want)
			}
			res, err = run(av.c, api.NoteQueryParams{Source: str("filters: file.hasLink(this)\n"), From: &from})
			if err != nil || !slices.Equal(paths(res), want) {
				t.Errorf("source from %s: %v, %v; want %v", from, paths(res), err, want)
			}
		}
		res, err = run(av.c, api.NoteQueryParams{Source: str("filters: file.hasTag(this.file.tags)\n"), From: str("Plan.md")})
		if err != nil || !slices.Equal(paths(res), []string{"Plan.md"}) {
			t.Errorf("this.file.tags: %v, %v", paths(res), err)
		}
	})
	t.Run("errors", func(t *testing.T) {
		big := int64(1001)
		zero := int64(0)
		for _, tc := range []struct {
			name string
			p    api.NoteQueryParams
			code api.ErrorCode
			has  string
		}{
			{"neither", api.NoteQueryParams{}, api.CodeInvalidParams, "exactly one of path and source"},
			{"both", api.NoteQueryParams{Path: str("Boards/Open.base"), Source: str("filters: 'true'")}, api.CodeInvalidParams, "exactly one of path and source"},
			{"a refused base", api.NoteQueryParams{Path: str("Boards/Bad.base")}, api.CodeInvalidParams, `filters: "file.backlinks.isEmpty()": file.backlinks is outside the subset`},
			{"yaml", api.NoteQueryParams{Source: str("filters: [")}, api.CodeInvalidParams, "base: not valid YAML"},
			{"a bare source has no this", api.NoteQueryParams{Source: str("filters: file.hasLink(this)")}, api.CodeInvalidParams, "this is not set"},
			{"an unknown view", api.NoteQueryParams{Path: str("Boards/Open.base"), View: str("Nope")}, api.CodeInvalidParams, `no view is named "Nope"`},
			{"limit 0", api.NoteQueryParams{Source: str("filters: 'true'"), Limit: &zero}, api.CodeInvalidParams, "limit 0"},
			{"limit 1001", api.NoteQueryParams{Source: str("filters: 'true'"), Limit: &big}, api.CodeInvalidParams, "limit 1001"},
			{"a bad path", api.NoteQueryParams{Path: str("../x.base")}, api.CodeInvalidParams, ""},
			{"a bad from", api.NoteQueryParams{Source: str("filters: 'true'"), From: str("/etc/passwd")}, api.CodeInvalidParams, ""},
			{"a missing base", api.NoteQueryParams{Path: str("Boards/None.base")}, api.CodeNotFound, ""},
			{"a missing from", api.NoteQueryParams{Source: str("filters: 'true'"), From: str("None.md")}, api.CodeNotFound, ""},
			{"a base over 1 MiB", api.NoteQueryParams{Path: str("Boards/Big.base")}, api.CodeInvalidParams, "a base is at most 1048576"},
			{"a source over 1 MiB", api.NoteQueryParams{Source: str("filters: 'true'\n# " + strings.Repeat("x", 1<<20))}, api.CodeInvalidParams, "a base is at most 1048576"},
		} {
			_, err := run(av.c, tc.p)
			if code(err) != tc.code || !strings.Contains(err.Error(), tc.has) {
				t.Errorf("%s: %v, want code %d holding %q", tc.name, err, tc.code, tc.has)
			}
		}
		if _, err := av.c.Note().Query(ctx, &api.NoteQueryParams{VaultID: av.id + 99, Source: str("filters: 'true'")}); code(err) != api.CodeNotFound {
			t.Errorf("an unknown vault: %v", err)
		}
	})
	t.Run("the wire form", func(t *testing.T) {
		var raw jsontext.Value
		err := av.c.Call(ctx, "note.query", map[string]any{"vaultId": av.id, "source": "views:\n  - type: cards\n    image: file.name\n    order: [file.name]\n    groupBy:\n      property: file.folder\nfilters: 'file.ext == \"csv\"'\n"}, &raw)
		if err != nil {
			t.Fatal(err)
		}
		const want = `{"view":{"name":"","type":"cards"},"views":[{"name":"","type":"cards"}],"columns":[{"key":"file.name","displayName":"file.name"}],` +
			`"rows":[{"path":"Work/data.csv","title":"data","kind":"attachment","values":["data.csv"],"image":"data.csv"}],` +
			`"groups":[{"value":"Work","count":1}],"total":1,"unshown":[],"image":"file.name"}`
		var a, b any
		if json.Unmarshal(raw, &a) != nil || json.Unmarshal([]byte(want), &b) != nil || !jsonEqual(a, b) {
			t.Errorf("wire form = %s\nwant        %s", raw, want)
		}
	})
}

func jsonEqual(a, b any) bool {
	x, _ := json.Marshal(a, json.Deterministic(true))
	y, _ := json.Marshal(b, json.Deterministic(true))
	return string(x) == string(y)
}
