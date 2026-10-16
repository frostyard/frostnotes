package store

// CONTRACT TEST for task card T-0015 (docs/tasks). Do not edit.

import (
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/frostyard/frostnotes/internal/markdown"
)

func TestLinkName(t *testing.T) {
	for target, want := range map[string]string{
		"Plan":                 "plan",
		"Plan.md":              "plan",
		"Orchard/North Block":  "north block",
		"recipes/APPLE BUTTER": "apple butter",
		"./media/Soil-Map.png": "soil-map.png",
		"../Harvest.MD":        "harvest",
		"CAFÉ":                 "café",
		"Plan.md.md":           "plan.md",
		"":                     "",
	} {
		if got := LinkName(target); got != want {
			t.Errorf("LinkName(%q) = %q, want %q", target, got, want)
		}
	}
}

// harvest is a note, and harvestDoc a Doc like the one markdown.Parse
// returns for it, written out so this test does not depend on the parser.
// IndexNote stores the Doc it is given.
const harvest = "---\ntitle: Harvest log\ntags: [harvest]\naliases: [Picking log]\nup: \"[[Plan]]\"\n---\n" +
	"# Harvest\nSee [[Plan#Goals|the plan]], [map](media/map.png) and [[Nowhere]] #orchard\n" +
	"- [ ] pick ^b1\n"

func harvestDoc() markdown.Doc {
	return markdown.Doc{
		Links: []markdown.Link{
			{Kind: markdown.LinkProperty, Target: "Plan", Key: "up", Start: 72, End: 80, Line: 5},
			{Kind: markdown.LinkWiki, Target: "Plan", Subpath: "#Goals", Alias: "the plan", Start: 100, End: 126, Line: 8},
			{Kind: markdown.LinkMarkdown, Target: "media/map.png", Alias: "map", Start: 128, End: 148, Line: 8},
			{Kind: markdown.LinkWiki, Target: "Nowhere", Start: 153, End: 164, Line: 8},
			{Kind: markdown.LinkWiki, Subpath: "#Harvest", Start: 0, End: 0, Line: 9},
		},
		Tags:     []markdown.Tag{{Name: "harvest", Front: true, Line: 3}, {Name: "Orchard", Line: 8}},
		Headings: []markdown.Heading{{Level: 1, Text: "Harvest", Line: 7}},
		Blocks:   []markdown.Block{{ID: "b1", Line: 9}},
		Tasks:    []markdown.Task{{Status: " ", Text: "pick ^b1", Line: 9}},
		Aliases:  []string{"Picking log"},
		Props: map[string]any{
			"title": "Harvest log", "tags": []any{"harvest"}, "aliases": []any{"Picking log"}, "up": "[[Plan]]",
			"n": int64(3), "f": 1.5, "b": true, "z": nil, "m": map[string]any{"k": "v"}, "l": []any{"A", int64(2), []any{"x"}, nil},
		},
		Words: 12,
	}
}

// rows renders a query's rows as strings, one per row.
func rows(t *testing.T, d *DB, q string, args ...any) []string {
	t.Helper()
	rs, err := d.db.QueryContext(t.Context(), q, args...)
	if err != nil {
		t.Fatal(err)
	}
	defer rs.Close()
	cols, _ := rs.Columns()
	var out []string
	for rs.Next() {
		vals := make([]any, len(cols))
		ptrs := make([]any, len(cols))
		for i := range vals {
			ptrs[i] = &vals[i]
		}
		if err := rs.Scan(ptrs...); err != nil {
			t.Fatal(err)
		}
		var parts []string
		for _, v := range vals {
			parts = append(parts, fmt.Sprint(v))
		}
		out = append(out, strings.Join(parts, "|"))
	}
	if err := rs.Err(); err != nil {
		t.Fatal(err)
	}
	return out
}

func checkRows(t *testing.T, what string, got, want []string) {
	t.Helper()
	if strings.Join(got, "\n") != strings.Join(want, "\n") {
		t.Errorf("%s:\n got %q\nwant %q", what, got, want)
	}
}

func TestNoteRowsFromADoc(t *testing.T) {
	ft := newFileTest(t)
	ctx := t.Context()
	var note File
	ft.tx(func(tx *Tx) {
		ft.put(tx, "Plan.md")
		ft.put(tx, "Logs/media/map.png")
		var err error
		note, err = tx.PutFile(ctx, File{VaultID: ft.vault, Path: "Logs/Harvest.md", Size: int64(len(harvest)), ModTime: time.Unix(5, 0), Hash: "h"})
		if err != nil {
			t.Fatal(err)
		}
		if err := tx.IndexNote(ctx, note, []byte(harvest), harvestDoc()); err != nil {
			t.Fatal(err)
		}
	})
	id := note.ID
	checkRows(t, "notes", rows(t, ft.d, `SELECT title, props, prop_error, aliases, words FROM notes WHERE file_id = ?`, id), []string{
		`Harvest|{"aliases":["Picking log"],"b":true,"f":1.5,"l":["A",2,["x"],null],"m":{"k":"v"},"n":3,"tags":["harvest"],"title":"Harvest log","up":"[[Plan]]","z":null}||["Picking log"]|12`,
	})
	checkRows(t, "props", rows(t, ft.d, `SELECT key, ord, value, folded, num FROM props WHERE file_id = ? ORDER BY key, ord`, id), []string{
		"aliases|0|Picking log|picking log|<nil>",
		"b|0|true|true|<nil>",
		"f|0|1.5|1.5|1.5",
		"l|0|A|a|<nil>",
		"l|1|2|2|2",
		"l|3|||<nil>",
		"n|0|3|3|3",
		"tags|0|harvest|harvest|<nil>",
		"title|0|Harvest log|harvest log|<nil>",
		"up|0|[[Plan]]|[[plan]]|<nil>",
		"z|0|||<nil>",
	})
	checkRows(t, "tags", rows(t, ft.d, `SELECT ord, folded, name, front, line FROM tags WHERE file_id = ? ORDER BY ord`, id), []string{
		"0|harvest|harvest|1|3", "1|orchard|Orchard|0|8",
	})
	checkRows(t, "links", rows(t, ft.d, `SELECT l.ord, l.kind, l.target, l.subpath, l.alias, l.embed, l.key, l.start, l.stop, l.line, l.name, coalesce(f.path, '-')
		FROM links l LEFT JOIN files f ON f.id = l.dest_id WHERE l.file_id = ? ORDER BY l.ord`, id), []string{
		"0|property|Plan|||0|up|72|80|5|plan|Plan.md",
		"1|wiki|Plan|#Goals|the plan|0||100|126|8|plan|Plan.md",
		"2|markdown|media/map.png||map|0||128|148|8|map.png|Logs/media/map.png",
		"3|wiki|Nowhere|||0||153|164|8|nowhere|-",
		"4|wiki||#Harvest||0||0|0|9||Logs/Harvest.md",
	})
	checkRows(t, "headings", rows(t, ft.d, `SELECT ord, level, text, line FROM headings WHERE file_id = ?`, id), []string{"0|1|Harvest|7"})
	checkRows(t, "blocks", rows(t, ft.d, `SELECT ord, block_id, line FROM blocks WHERE file_id = ?`, id), []string{"0|b1|9"})
	checkRows(t, "tasks", rows(t, ft.d, `SELECT ord, status, text, line FROM tasks WHERE file_id = ?`, id), []string{"0| |pick ^b1|9"})
	body := strings.SplitN(harvest, "---\n", 3)[2]
	front := "title: Harvest log\ntags: [harvest]\naliases: [Picking log]\nup: \"[[Plan]]\"\n"
	checkRows(t, "notes_fts", rows(t, ft.d, `SELECT rowid, title, aliases, body, props FROM notes_fts`), []string{
		fmt.Sprintf("%d|Harvest|Picking log|%s|%s", id, body, front),
	})
}

func TestReindexReplacesANotesRows(t *testing.T) {
	ft := newFileTest(t)
	ctx := t.Context()
	var note File
	ft.tx(func(tx *Tx) {
		note = ft.put(tx, "Harvest.md")
		if err := tx.IndexNote(ctx, note, []byte(harvest), harvestDoc()); err != nil {
			t.Fatal(err)
		}
		content := "Just [[Other]] and #new\n"
		doc := markdown.Doc{
			Links:   []markdown.Link{{Kind: markdown.LinkWiki, Target: "Other", Start: 5, End: 14, Line: 1}},
			Tags:    []markdown.Tag{{Name: "new", Line: 1}},
			Aliases: []string{"A", "B"},
			Words:   4,
		}
		if err := tx.IndexNote(ctx, note, []byte(content), doc); err != nil {
			t.Fatal(err)
		}
	})
	for table, want := range map[string]int{"notes": 1, "props": 0, "tags": 1, "links": 1, "headings": 0, "blocks": 0, "tasks": 0, "notes_fts": 1} {
		if got := rows(t, ft.d, `SELECT count(*) FROM `+table); got[0] != fmt.Sprint(want) {
			t.Errorf("%s has %s rows after re-indexing, want %d", table, got[0], want)
		}
	}
	checkRows(t, "notes", rows(t, ft.d, `SELECT props, prop_error, aliases, words FROM notes`), []string{`{}||["A","B"]|4`})
	checkRows(t, "notes_fts", rows(t, ft.d, `SELECT title, aliases, body, props FROM notes_fts`), []string{
		"Harvest|A\nB|Just [[Other]] and #new\n|",
	})
}

func TestNoteRowsRecordAPropError(t *testing.T) {
	ft := newFileTest(t)
	ft.tx(func(tx *Tx) {
		note := ft.put(tx, "Bad.md")
		doc := markdown.Doc{PropError: errors.New("frontmatter: yaml: line 1: did not find expected node content"), Words: 1}
		if err := tx.IndexNote(t.Context(), note, []byte("---\ntags: [x\n---\nbody\xff\n"), doc); err != nil {
			t.Fatal(err)
		}
	})
	checkRows(t, "notes", rows(t, ft.d, `SELECT props, prop_error, aliases FROM notes`), []string{
		`{}|frontmatter: yaml: line 1: did not find expected node content|[]`,
	})
	checkRows(t, "notes_fts", rows(t, ft.d, `SELECT body, props FROM notes_fts`), []string{"body\uFFFD\n|tags: [x\n"})
	if got := rows(t, ft.d, `SELECT count(*) FROM props`); got[0] != "0" {
		t.Errorf("props rows = %s, want 0", got[0])
	}
}

func TestNoteLinksResolveLater(t *testing.T) {
	ft := newFileTest(t)
	var note File
	ft.tx(func(tx *Tx) {
		note = ft.put(tx, "Inbox.md")
		doc := markdown.Doc{Links: []markdown.Link{{Kind: markdown.LinkWiki, Target: "Plan", Start: 0, End: 8, Line: 1}}}
		if err := tx.IndexNote(t.Context(), note, []byte("[[Plan]]"), doc); err != nil {
			t.Fatal(err)
		}
	})
	if got := ft.dest(note, 0); got != "" {
		t.Fatalf("before Plan.md exists: %q", got)
	}
	ft.tx(func(tx *Tx) { ft.put(tx, "Garden/Plan.md") })
	if got := ft.dest(note, 0); got != "Garden/Plan.md" {
		t.Fatalf("after Garden/Plan.md is put: %q", got)
	}
}
