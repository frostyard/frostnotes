package query_test

import (
	"encoding/json/v2"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"testing"

	"github.com/frostyard/frostnotes/api"
	"github.com/frostyard/frostnotes/internal/query"
)

// view wraps a table view's keys in a base.
func view(keys string) string {
	return "views:\n  - type: table\n    order: [file.name]\n" + indent(keys, "    ")
}

func indent(s, prefix string) string {
	var b strings.Builder
	for _, line := range strings.SplitAfter(s, "\n") {
		if line != "" {
			b.WriteString(prefix + line)
		}
	}
	return b.String()
}

func TestSort(t *testing.T) {
	e := fixtureEnv(t)
	missingRating := []string{"Attachments/Doc.pdf", "Attachments/data.txt", "Attachments/pic.png", "Broken.md",
		"Daily/2026-03-05.md", "Daily/2026-03-06.md", "Dashboards/Open.base", "Home.md", "Projects/Delta.md", "Unicode Ünï.md"}
	tests := []struct {
		name string
		base string
		want []string
	}{
		{"ASC is the default: numbers, then text, then the missing, and the path breaks ties", view("sort:\n  - property: note.rating\n"),
			slices.Concat([]string{"Projects/Archive/Old.md", "Areas/Work.md", "Inbox.md", "Misc/Spaced Out.md", "Projects/Beta.md", "Reading/Books.md",
				"Projects/Alpha.md", "Areas/Garden.md", "Reading/Article One.md", "Misc/Many.md", "Projects/Gamma.md"}, missingRating)},
		{"DESC: the missing stay last, the path still ascends", view("sort:\n  - property: rating\n    direction: DESC\n"),
			slices.Concat([]string{"Projects/Gamma.md", "Misc/Many.md", "Areas/Garden.md", "Reading/Article One.md", "Projects/Alpha.md",
				"Projects/Beta.md", "Reading/Books.md", "Areas/Work.md", "Inbox.md", "Misc/Spaced Out.md", "Projects/Archive/Old.md"}, missingRating)},
		{"a lower-case direction", view("sort:\n  - property: rating\n    direction: desc\nlimit: 1\n"), []string{"Projects/Gamma.md"}},
		{"by file.mtime, newest first", view("sort:\n  - property: file.mtime\n    direction: DESC\n"),
			[]string{"Projects/Alpha.md", "Home.md", "Unicode Ünï.md", "Reading/Books.md", "Reading/Article One.md", "Projects/Gamma.md",
				"Projects/Delta.md", "Projects/Beta.md", "Misc/Spaced Out.md", "Misc/Many.md", "Inbox.md", "Dashboards/Open.base",
				"Daily/2026-03-06.md", "Daily/2026-03-05.md", "Broken.md", "Attachments/pic.png", "Attachments/data.txt",
				"Attachments/Doc.pdf", "Areas/Work.md", "Areas/Garden.md", "Projects/Archive/Old.md"}},
		{"by file.name, a case-kept text", view("sort:\n  - property: file.name\nlimit: 4\n"),
			[]string{"Daily/2026-03-05.md", "Daily/2026-03-06.md", "Projects/Alpha.md", "Reading/Article One.md"}},
		{"two keys", "filters: kind\n" + view("sort:\n  - property: kind\n  - property: rating\n    direction: DESC\n"),
			[]string{"Areas/Garden.md", "Areas/Work.md", "Reading/Article One.md", "Reading/Books.md"}},
		{"the first key wins over the second", "filters: kind\n" + view("sort:\n  - property: rating\n    direction: DESC\n  - property: kind\n"),
			[]string{"Areas/Garden.md", "Reading/Article One.md", "Reading/Books.md", "Areas/Work.md"}},
		{"booleans after text, false before true", "filters: file.hasProperty(\"done\")\n" + view("sort:\n  - property: done\n"),
			[]string{"Projects/Gamma.md", "Projects/Alpha.md", "Misc/Many.md", "Projects/Beta.md"}},
		{"lists after the rest, by their text", "filters: file.hasProperty(\"tags\")\n" + view("sort:\n  - property: tags\n"),
			[]string{"Projects/Gamma.md", "Areas/Garden.md", "Areas/Work.md", "Daily/2026-03-05.md", "Daily/2026-03-06.md", "Inbox.md",
				"Projects/Alpha.md", "Projects/Beta.md", "Projects/Archive/Old.md", "Reading/Article One.md", "Reading/Books.md", "Unicode Ünï.md"}},
		{"a null sorts with the missing", "filters: file.hasProperty(\"notes\") || file.hasProperty(\"mood\")\n" + view("sort:\n  - property: notes\n"),
			[]string{"Areas/Garden.md", "Areas/Work.md", "Daily/2026-03-05.md", "Daily/2026-03-06.md", "Misc/Many.md"}},
		{"sort by a mixed-case key", view("sort:\n  - property: Status\nlimit: 1\n"), []string{"Misc/Spaced Out.md"}},
		{"no sort: by path", view(""), allFiles},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			res := e.mustRun(tc.base)
			if got := paths(res); !slices.Equal(got, tc.want) {
				t.Errorf("rows\n got %q\nwant %q", got, tc.want)
			}
		})
	}
}

// groupShape is a result's groups as "value:count".
func groupShape(res *api.QueryResult) []string {
	out := []string{}
	for _, g := range res.Groups {
		out = append(out, string(g.Value)+":"+strconv.FormatInt(g.Count, 10))
	}
	return out
}

func TestGroupBy(t *testing.T) {
	e := fixtureEnv(t)
	tests := []struct {
		name   string
		base   string
		groups []string
		rows   []string // the first rows, when set
		total  int64
	}{
		{"by text, null last", view("groupBy:\n  property: note.status\n  direction: ASC\n"),
			[]string{`"Active":1`, `"active":2`, `"done":2`, `"paused":1`, `null:15`}, []string{"Unicode Ünï.md", "Inbox.md", "Projects/Alpha.md", "Projects/Archive/Old.md", "Projects/Beta.md", "Projects/Gamma.md"}, 21},
		{"DESC, null still last", view("groupBy:\n  property: status\n  direction: DESC\n"),
			[]string{`"paused":1`, `"done":2`, `"active":2`, `"Active":1`, `null:15`}, []string{"Projects/Gamma.md", "Projects/Archive/Old.md", "Projects/Beta.md", "Inbox.md", "Projects/Alpha.md", "Unicode Ünï.md"}, 21},
		{"the direction is ASC by default", view("groupBy:\n  property: status\nlimit: 3\n"),
			[]string{`"Active":1`, `"active":2`}, []string{"Unicode Ünï.md", "Inbox.md", "Projects/Alpha.md"}, 21},
		{"the limit counts rows across groups", view("groupBy:\n  property: status\n"), nil, nil, 21},
		{"by number, 3 and 3.0 are one group", view("groupBy:\n  property: mood\n"),
			[]string{`3:3`, `4:1`, `5:1`, `null:16`}, []string{"Areas/Garden.md", "Daily/2026-03-05.md", "Misc/Many.md", "Areas/Work.md", "Daily/2026-03-06.md"}, 21},
		{"by a property that is only null", view("groupBy:\n  property: notes\n"), []string{`null:21`}, nil, 21},
		{"by file.folder", view("groupBy:\n  property: file.folder\n"),
			[]string{`"":4`, `"Areas":2`, `"Attachments":3`, `"Daily":2`, `"Dashboards":1`, `"Misc":2`, `"Projects":4`, `"Projects/Archive":1`, `"Reading":2`}, nil, 21},
		{"then the sort inside a group", "filters: kind\n" + view("groupBy:\n  property: kind\nsort:\n  - property: rating\n    direction: DESC\n"),
			[]string{`"area":2`, `"article":1`, `"list":1`}, []string{"Areas/Garden.md", "Areas/Work.md", "Reading/Article One.md", "Reading/Books.md"}, 4},
		{"by a list, equal lists are together", "filters: file.hasProperty(\"category\")\n" + view("groupBy:\n  property: category\n"),
			[]string{`["alpha","core"]:1`, `["beta"]:1`, `[]:1`}, []string{"Projects/Alpha.md", "Projects/Beta.md", "Projects/Gamma.md"}, 3},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			res := e.mustRun(tc.base)
			if res.Total != tc.total {
				t.Errorf("total = %d, want %d", res.Total, tc.total)
			}
			if tc.groups != nil {
				if got := groupShape(res); !slices.Equal(got, tc.groups) {
					t.Errorf("groups\n got %q\nwant %q", got, tc.groups)
				}
			}
			n := 0
			for _, g := range res.Groups {
				n += int(g.Count)
			}
			if n != len(res.Rows) {
				t.Errorf("the groups hold %d rows, the result has %d", n, len(res.Rows))
			}
			if tc.rows != nil {
				if got := paths(res)[:len(tc.rows)]; !slices.Equal(got, tc.rows) {
					t.Errorf("first rows\n got %q\nwant %q", got, tc.rows)
				}
			}
		})
	}
	t.Run("without groupBy there are no groups", func(t *testing.T) {
		if res := e.mustRun(view("")); res.Groups != nil {
			t.Errorf("groups = %v, want none", res.Groups)
		}
	})
}

func TestLimit(t *testing.T) {
	e := fixtureEnv(t)
	tests := []struct {
		name string
		base string
		call int
		want int
	}{
		{"no limit", view(""), 1000, 21},
		{"the param", view(""), 4, 4},
		{"the view's limit is smaller", view("limit: 3\n"), 10, 3},
		{"the param is smaller", view("limit: 50\n"), 2, 2},
		{"the view's limit is larger than 1000", view("limit: 5000\n"), 1000, 21},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			res := e.mustRun(tc.base, withLimit(tc.call))
			if len(res.Rows) != tc.want || res.Total != 21 {
				t.Errorf("rows = %d, total = %d; want %d rows of 21", len(res.Rows), res.Total, tc.want)
			}
		})
	}
}

func TestColumns(t *testing.T) {
	e := fixtureEnv(t)
	alpha, err := os.ReadFile(filepath.Join(e.dir, "Projects", "Alpha.md"))
	if err != nil {
		t.Fatal(err)
	}
	base := `filters: 'file.path == "Projects/Alpha.md" || file.path == "Attachments/pic.png"'
properties:
  note.status:
    displayName: Status
  rating:
    displayName: Rating
  file.name:
    displayName: Name
  formula.total:
    displayName: Total
  due: {}
  note.owner:
    displayName:
views:
  - type: table
    order:
      - file.name
      - note.status
      - rating
      - due
      - formula.total
      - file.ctime
      - this.x
      - formula.total
      - file.tags
      - file.links
      - file.folder
      - file.size
      - file.mtime
      - tags
      - notes
      - items
      - file.ext
      - file.basename
      - file.path
      - owner
`
	res := e.mustRun(base)
	wantKeys := []string{"file.name", "note.status", "rating", "due", "formula.total", "file.ctime", "this.x", "formula.total", "file.tags",
		"file.links", "file.folder", "file.size", "file.mtime", "tags", "notes", "items", "file.ext", "file.basename", "file.path", "owner"}
	wantNames := []string{"Name", "Status", "Rating", "due", "Total", "file.ctime", "this.x", "Total", "file.tags",
		"file.links", "file.folder", "file.size", "file.mtime", "tags", "notes", "items", "file.ext", "file.basename", "file.path", "owner"}
	for i, c := range res.Columns {
		if c.Key != wantKeys[i] || c.DisplayName != wantNames[i] {
			t.Errorf("column %d = %+v, want %q %q", i, c, wantKeys[i], wantNames[i])
		}
	}
	if len(res.Columns) != len(wantKeys) {
		t.Fatalf("%d columns, want %d", len(res.Columns), len(wantKeys))
	}
	if want := []string{"formula.total", "file.ctime", "this.x"}; !slices.Equal(res.Unshown, want) {
		t.Errorf("unshown = %q, want %q", res.Unshown, want)
	}
	if got, want := paths(res), []string{"Attachments/pic.png", "Projects/Alpha.md"}; !slices.Equal(got, want) {
		t.Fatalf("rows = %q, want %q", got, want)
	}
	values := func(row api.QueryRow) []string {
		out := make([]string, len(row.Values))
		for i, v := range row.Values {
			out[i] = string(v)
		}
		return out
	}
	size := strconv.Itoa(len(alpha))
	wantAlpha := []string{`"Alpha.md"`, `"active"`, `4.5`, `"2026-03-01"`, `null`, `null`, `null`, `null`,
		`["project","area/work","inline","Area/Home"]`, `["Beta","Gamma","Unwritten"]`, `"Projects"`, size,
		`"2026-03-04T12:00:00.123Z"`, `["project","area/work"]`, `null`, `null`, `"md"`, `"Alpha"`, `"Projects/Alpha.md"`, `"Ada"`}
	if got := values(res.Rows[1]); !slices.Equal(got, wantAlpha) {
		t.Errorf("Alpha's values\n got %q\nwant %q", got, wantAlpha)
	}
	if res.Rows[1].Title != "Alpha" || res.Rows[1].Kind != api.FileKindNote {
		t.Errorf("Alpha = %+v", res.Rows[1])
	}
	// An attachment has no note properties and its title is file.basename.
	pic := res.Rows[0]
	wantPic := []string{`"pic.png"`, `null`, `null`, `null`, `null`, `null`, `null`, `null`, `[]`, `[]`, `"Attachments"`, `null`, `null`,
		`null`, `null`, `null`, `"png"`, `"pic"`, `"Attachments/pic.png"`, `null`}
	got := values(pic)
	got[11], got[12] = "null", "null" // the size and the time: checked below
	if !slices.Equal(got, wantPic) {
		t.Errorf("pic's values\n got %q\nwant %q", got, wantPic)
	}
	if string(pic.Values[11]) == "null" || !strings.HasPrefix(string(pic.Values[12]), `"2026-03-01T`) {
		t.Errorf("pic's size and mtime = %s %s", pic.Values[11], pic.Values[12])
	}
	if pic.Title != "pic" || pic.Kind != api.FileKindAttachment {
		t.Errorf("pic = %+v", pic)
	}
}

func TestDefaultColumn(t *testing.T) {
	e := fixtureEnv(t)
	res := e.mustRun("filters: 'file.name == \"Home.md\"'\nviews:\n  - type: table\n")
	if len(res.Columns) != 1 || res.Columns[0].Key != "file.name" || res.Columns[0].DisplayName != "file.name" ||
		len(res.Rows) != 1 || string(res.Rows[0].Values[0]) != `"Home.md"` {
		t.Errorf("result = %+v", res)
	}
	res = e.mustRun("filters: 'file.name == \"Home.md\"'\nviews:\n  - type: table\n    order: []\n")
	if len(res.Columns) != 0 || len(res.Rows) != 1 || len(res.Rows[0].Values) != 0 {
		t.Errorf("an empty order = %+v", res)
	}
	// A base with no views has one table view with no name.
	res = e.mustRun("")
	if res.View != (api.QueryView{Type: "table"}) || len(res.Views) != 1 || res.Total != 21 {
		t.Errorf("the empty base = %+v", res)
	}
}

func TestCardsImage(t *testing.T) {
	e := fixtureEnv(t)
	base := `filters: 'file.path == "Projects/Archive/Old.md" || file.path == "Home.md"'
views:
  - type: cards
    name: Gallery
    image: note.status
    cardSize: 200
    order: [file.name]
  - type: table
    name: Plain
    image: note.status
`
	res := e.mustRun(base)
	if res.Image == nil || *res.Image != "note.status" {
		t.Fatalf("image = %v", res.Image)
	}
	if string(res.Rows[0].Image) != `null` || string(res.Rows[1].Image) != `"done"` {
		t.Errorf("row images = %s, %s", res.Rows[0].Image, res.Rows[1].Image)
	}
	if res.View != (api.QueryView{Name: "Gallery", Type: "cards"}) {
		t.Errorf("view = %+v", res.View)
	}
	res = e.mustRun(base, withView("Plain"))
	if res.Image != nil || res.Rows[0].Image != nil {
		t.Errorf("a table view has no image: %v %s", res.Image, res.Rows[0].Image)
	}
	// An image that is a file property, a formula, and a cards view without one.
	res = e.mustRun(strings.Replace(base, "note.status", "file.folder", 1))
	if string(res.Rows[0].Image) != `""` || string(res.Rows[1].Image) != `"Projects/Archive"` {
		t.Errorf("file.folder images = %s, %s", res.Rows[0].Image, res.Rows[1].Image)
	}
	res = e.mustRun(strings.Replace(base, "note.status", "formula.x", 1))
	if string(res.Rows[1].Image) != `null` {
		t.Errorf("formula image = %s", res.Rows[1].Image)
	}
	res = e.mustRun(strings.Replace(base, "    image: note.status\n    cardSize", "    cardSize", 1))
	if res.Image != nil || res.Rows[0].Image != nil {
		t.Errorf("no image key: %v", res.Image)
	}
}

func TestViewsAndNotices(t *testing.T) {
	e := fixtureEnv(t)
	base := `filters: file.ext == "md"
formulas:
  total: 'rating * 2'
summaries: &anchor
  x: 1
views:
  - type: table
    name: A
    order: [file.name]
    columnSize: {file.name: 100}
    rowHeight: tall
    limit: 2
  - type: kanban
    name: B
    filters: 'formula.nonsense >>> 1'
  - type: table
    name: C
    groupOrder: [done, open]
    groupBy:
      property: status
  - type: list
    name: D
  - type: table
    name: A
    filters: 'formula.x > 1'
  - type: map
  - name: untyped
`
	res := e.mustRun(base)
	wantViews := []api.QueryView{{Name: "A", Type: "table"}, {Name: "B", Type: "kanban"}, {Name: "C", Type: "table"},
		{Name: "D", Type: "list"}, {Name: "A", Type: "table"}, {Type: "map"}, {Name: "untyped"}}
	if !slices.Equal(res.Views, wantViews) {
		t.Errorf("views = %+v, want %+v", res.Views, wantViews)
	}
	if res.View != wantViews[0] || len(res.Rows) != 2 || res.Total != 17 || res.Notice != nil {
		t.Errorf("view A = %+v", res)
	}
	for _, tc := range []struct{ view, notice string }{
		{"B", "views[1]: view type kanban is outside the subset"},
		{"C", "views[2]: groupOrder is outside the subset"},
		{"D", "views[3]: view type list is outside the subset"},
		{"", "views[5]: view type map is outside the subset"},
		{"untyped", "views[6]: a view with no type is outside the subset"},
	} {
		t.Run("notice "+tc.view, func(t *testing.T) {
			res := e.mustRun(base, withView(tc.view))
			if tc.view == "" { // the first view named "" is the map
				if res.View.Type != "map" {
					t.Fatalf("view = %+v", res.View)
				}
			}
			if res.Notice == nil || *res.Notice != tc.notice {
				t.Fatalf("notice = %v, want %q", res.Notice, tc.notice)
			}
			if res.Rows == nil || len(res.Rows) != 0 || res.Total != 0 || res.Columns == nil || len(res.Columns) != 0 ||
				len(res.Groups) != 0 || res.Unshown == nil || len(res.Views) != len(wantViews) {
				t.Errorf("a notice has no rows: %+v", res)
			}
		})
	}
	// The first view of that name wins; the other views' filters are not checked.
	if res := e.mustRun(base, withView("A")); res.Total != 17 {
		t.Errorf("the first view named A: %+v", res)
	}
	// A notice never fails on the filters.
	bad := "filters: 'formula.x'\nviews:\n  - type: kanban\n"
	if res := e.mustRun(bad); res.Notice == nil {
		t.Errorf("a kanban view with a bad global filter: %+v", res)
	}
	if _, err := e.run("views:\n  - type: table\n    name: A\n", withView("Z")); err == nil || !strings.Contains(err.Error(), `no view is named "Z"`) {
		t.Errorf("an unknown view: %v", err)
	}
}

func TestFilterLists(t *testing.T) {
	e := fixtureEnv(t)
	tests := []struct {
		name string
		base string
		want []string
	}{
		{"an empty and is true", "filters:\n  and: []\n", allFiles},
		{"an empty or is false", "filters:\n  or: []\n", nil},
		{"an empty not is true", "filters:\n  not: []\n", allFiles},
		{"not means none is true", "filters:\n  not:\n    - status == \"done\"\n    - status == \"paused\"\n",
			except("Projects/Archive/Old.md", "Projects/Beta.md", "Projects/Gamma.md")},
		{"nesting", `filters:
  and:
    - file.inFolder("Projects")
    - or:
        - status == "active"
        - and:
            - status == "done"
            - not:
                - file.inFolder("Projects/Archive")
`, []string{"Projects/Alpha.md", "Projects/Beta.md"}},
		{"global and view filters join with and", `filters: file.inFolder("Projects")
views:
  - type: table
    filters:
      or:
        - status == "done"
        - file.name == "Delta.md"
`, []string{"Projects/Archive/Old.md", "Projects/Beta.md", "Projects/Delta.md"}},
		{"an empty view filter is true", "filters: 'file.name == \"Home.md\"'\nviews:\n  - type: table\n    filters:\n      and: []\n", []string{"Home.md"}},
		{"null filters are absent", "filters:\nviews:\n  - type: table\n    filters:\n", allFiles},
		{"a block scalar statement", "filters: |\n  status ==\n  \"active\"\n", []string{"Inbox.md", "Projects/Alpha.md"}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			res := e.mustRun(tc.base)
			if got := paths(res); !slices.Equal(got, nonNil(tc.want)) {
				t.Errorf("rows\n got %q\nwant %q", got, tc.want)
			}
		})
	}
}

func TestSpecExample(t *testing.T) {
	e := fixtureEnv(t)
	source, err := os.ReadFile("testdata/vault/Dashboards/Open.base")
	if err != nil {
		t.Fatal(err)
	}
	res := e.mustRun(string(source))
	if got, want := paths(res), []string{"Projects/Alpha.md", "Projects/Gamma.md", "Projects/Delta.md"}; !slices.Equal(got, want) {
		t.Errorf("rows = %q, want %q", got, want)
	}
	wantCols := []api.QueryColumn{{Key: "file.name", DisplayName: "file.name"}, {Key: "note.status", DisplayName: "Status"}, {Key: "note.due", DisplayName: "due"}}
	if !slices.Equal(res.Columns, wantCols) {
		t.Errorf("columns = %+v", res.Columns)
	}
	out, err := json.Marshal(res.Rows[2].Values)
	if err != nil || string(out) != `["Delta.md",null,null]` {
		t.Errorf("Delta's values = %s, %v", out, err)
	}
	if res.View != (api.QueryView{Name: "Open", Type: "table"}) || res.Total != 3 {
		t.Errorf("view = %+v, total %d", res.View, res.Total)
	}
}

func TestThis(t *testing.T) {
	e := fixtureEnv(t)
	tests := []struct {
		name string
		this string
		stmt string
		want []string
	}{
		{"hasLink(this)", "Projects/Alpha.md", `file.hasLink(this)`, []string{"Areas/Garden.md", "Broken.md", "Home.md", "Inbox.md", "Projects/Beta.md", "Projects/Delta.md", "Projects/Gamma.md"}},
		{"hasLink(this.file)", "Projects/Alpha.md", `file.hasLink(this.file)`, []string{"Areas/Garden.md", "Broken.md", "Home.md", "Inbox.md", "Projects/Beta.md", "Projects/Delta.md", "Projects/Gamma.md"}},
		{"hasLink(this) of another note", "Projects/Beta.md", `file.hasLink(this)`, []string{"Projects/Alpha.md", "Projects/Archive/Old.md"}},
		{"this.file.name", "Projects/Alpha.md", `file.name == this.file.name`, []string{"Projects/Alpha.md"}},
		{"this.file.name on the left", "Projects/Alpha.md", `this.file.name == file.name`, []string{"Projects/Alpha.md"}},
		{"this.file.folder", "Projects/Alpha.md", `file.folder == this.file.folder`, []string{"Projects/Alpha.md", "Projects/Beta.md", "Projects/Delta.md", "Projects/Gamma.md"}},
		{"this.file.path folds", "Projects/Alpha.md", `this.file.path == "Projects/Alpha.md"`, allFiles},
		{"this.file.basename folds", "Projects/Alpha.md", `this.file.basename == "Alpha" && this.file.ext == "md" && this.file.folder == "Projects"`, allFiles},
		{"this.file.name != folds", "Projects/Alpha.md", `this.file.name != "Alpha.md"`, nil},
		{"hasTag(this.file.tags)", "Projects/Alpha.md", `file.hasTag(this.file.tags)`,
			[]string{"Areas/Garden.md", "Areas/Work.md", "Projects/Alpha.md", "Projects/Archive/Old.md", "Projects/Beta.md", "Projects/Delta.md", "Projects/Gamma.md", "Reading/Article One.md"}},
		{"containsAny(this.file.tags)", "Projects/Alpha.md", `file.tags.containsAny(this.file.tags)`,
			[]string{"Areas/Work.md", "Projects/Alpha.md", "Projects/Beta.md", "Projects/Gamma.md"}},
		{"this.file.mtime", "Projects/Alpha.md", `file.mtime >= this.file.mtime`, []string{"Projects/Alpha.md"}},
		{"this.file.mtime with milliseconds", "Projects/Alpha.md", `file.mtime == this.file.mtime`, []string{"Projects/Alpha.md"}},
		{"this.file.size", "Attachments/Doc.pdf", `file.size == this.file.size`, []string{"Attachments/Doc.pdf"}},
		{"a base's this, an attachment with no tags", "Dashboards/Open.base", `file.hasTag(this.file.tags)`, nil},
		{"a base's this, empty containsAny", "Dashboards/Open.base", `file.tags.containsAny(this.file.tags)`, nil},
		{"a base's this, its extension", "Dashboards/Open.base", `this.file.ext == "base"`, allFiles},
		{"hasLink of a name, from this", "Reading/Article One.md", `file.hasLink("Books")`, []string{"Inbox.md", "Projects/Gamma.md", "Reading/Article One.md"}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			res := e.mustRun(filterBase(tc.stmt), withThis(tc.this, e))
			if got := paths(res); !slices.Equal(got, nonNil(tc.want)) {
				t.Errorf("%s with this = %s\n got %q\nwant %q", tc.stmt, tc.this, got, tc.want)
			}
		})
	}
	t.Run("a bare source has no this", func(t *testing.T) {
		for _, stmt := range []string{`file.hasLink(this)`, `this.file.name == "x"`, `file.hasLink(this.file)`, `file.name == this.file.name`} {
			_, err := e.run(filterBase(stmt))
			if err == nil || !strings.Contains(err.Error(), "this is not set") || !strings.Contains(err.Error(), fmt.Sprintf("filters: %q", stmt)) {
				t.Errorf("%s: %v", stmt, err)
			}
		}
	})
	t.Run("this is not a Go-visible dependency of the options", func(t *testing.T) {
		var o query.Options
		if o.This != nil {
			t.Fatal("This is nil by default")
		}
	})
}

func TestTotalWithTheLimit(t *testing.T) {
	e := fixtureEnv(t)
	// total counts every match, with 1 statement, whatever the limit and
	// even with no row.
	for _, tc := range []struct {
		name       string
		base       string
		limit      int
		rows, want int
	}{
		{"a limit below the matches", filterBase(`file.ext == "md"`), 3, 3, 17},
		{"a limit of 1", filterBase(`file.ext == "md"`), 1, 1, 17},
		{"a limit above the matches", filterBase(`file.ext == "md"`), 1000, 17, 17},
		{"no match", filterBase(`false`), 5, 0, 0},
		{"no match with groups", "filters: 'false'\n" + view("groupBy:\n  property: status\n"), 5, 0, 0},
		{"1 match", filterBase(`file.name == "Home.md"`), 5, 1, 1},
	} {
		res := e.mustRun(tc.base, withLimit(tc.limit))
		if len(res.Rows) != tc.rows || int(res.Total) != tc.want {
			t.Errorf("%s: %d rows, total %d; want %d rows, total %d", tc.name, len(res.Rows), res.Total, tc.rows, tc.want)
		}
	}
}

// TestGroupByLargeIntegers: 2 whole numbers that float64 cannot tell apart
// are 2 groups, and 1 and 1.0 are 1.
func TestGroupByLargeIntegers(t *testing.T) {
	dir := t.TempDir()
	for name, n := range map[string]string{"a.md": "9007199254740992", "b.md": "9007199254740993", "c.md": "9007199254740993", "d.md": "1", "e.md": "1.0"} {
		if err := os.WriteFile(filepath.Join(dir, name), []byte("---\nn: "+n+"\n---\n"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	e := indexedEnv(t, dir)
	res := e.mustRun(view("groupBy:\n  property: n\n"))
	want := []string{`1:2`, `9007199254740992:1`, `9007199254740993:2`}
	if got := groupShape(res); !slices.Equal(got, want) {
		t.Errorf("groups = %q, want %q", got, want)
	}
}

// TestReadsOnlyWhatItNeeds: a base is checked where the query reads it:
// the global filters, the evaluated view, and displayName in properties.
func TestReadsOnlyWhatItNeeds(t *testing.T) {
	e := fixtureEnv(t)
	other := "x: &a status\nviews:\n  - type: table\n    name: A\n    order: [file.name]\n  - type: table\n    name: B\n    filters: *a\n    sort: *a\n"
	if res := e.mustRun(other); res.Total != 21 {
		t.Errorf("an alias in another view's filters: %+v", res)
	}
	_, err := e.run(other, withView("B"))
	if err == nil || !strings.HasPrefix(err.Error(), "views[1].filters: YAML anchors and aliases are outside the subset") {
		t.Errorf("the evaluated view with an alias: %v", err)
	}
	// A bad filter of another view, and bad keys of its sort, order and limit.
	bad := "views:\n  - type: table\n  - type: table\n    name: B\n    filters: [x]\n    order: 5\n    sort: x\n    groupBy: 1\n    limit: no\n    image: [x]\n"
	if _, err := e.run(bad); err != nil {
		t.Errorf("another view's keys are not read: %v", err)
	}
	// The global filters are not read for a view that gives a notice.
	global := "x: &a status\nfilters: *a\nviews:\n  - type: kanban\n  - type: table\n    name: T\n"
	if res := e.mustRun(global); res.Notice == nil {
		t.Errorf("a notice with an alias in the global filters: %+v", res)
	}
	if _, err := e.run(global, withView("T")); err == nil || !strings.HasPrefix(err.Error(), "filters: YAML anchors and aliases are outside the subset") {
		t.Errorf("the global filters with an alias: %v", err)
	}
	// Property metadata other than displayName is not read.
	props := "x: &a 5\nproperties:\n  note.x:\n    width: *a\n    nested: {k: *a}\n    displayName: X\nviews:\n  - type: table\n    order: [note.x]\n"
	res := e.mustRun(props)
	if len(res.Columns) != 1 || res.Columns[0].DisplayName != "X" {
		t.Errorf("columns = %+v", res.Columns)
	}
	// groupOrder is a notice whatever it holds.
	if res := e.mustRun("x: &a 1\nviews:\n  - type: table\n    groupOrder: *a\n"); res.Notice == nil {
		t.Errorf("groupOrder with an alias: %+v", res)
	}
}
