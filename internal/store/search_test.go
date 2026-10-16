package store

// CONTRACT TEST for task card T-0016 (docs/tasks). Do not edit.

import (
	"fmt"
	"strings"
	"testing"

	"github.com/frostyard/frostnotes/api"
	"github.com/frostyard/frostnotes/internal/markdown"
)

func TestSearchQuery(t *testing.T) {
	for _, tc := range []struct{ text, want string }{
		{"plan", `"plan"*`},
		{"  apple   butter ", `"apple"* "butter"*`},
		{`"apple butter" pie`, `"apple butter" "pie"*`},
		{`say "hi there`, `"say"* "hi there"`},
		{`ab"c d"`, `"ab"* "c d"`},
		{"café 2026-10", `"café"* "2026-10"*`},
		{"NEAR AND OR NOT", `"NEAR"* "AND"* "OR"* "NOT"*`},
		{"title:x col:", `"title:x"* "col:"*`},
		{`-- ** () ""  " "`, ""},
		{"", ""},
	} {
		if got := SearchQuery(tc.text); got != tc.want {
			t.Errorf("SearchQuery(%q) = %s, want %s", tc.text, got, tc.want)
		}
	}
}

// searchVault indexes notes, given as path: content, with Docs holding
// only their aliases, in a new vault, and returns the vault's ID.
func searchVault(t *testing.T, ft *fileTest, notes map[string]string, aliases map[string][]string) int64 {
	t.Helper()
	var vault int64
	ft.tx(func(tx *Tx) {
		v, err := tx.InsertVault(t.Context(), "s", fmt.Sprintf("/s%d", len(notes)))
		if err != nil {
			t.Fatal(err)
		}
		vault = v.ID
		for p, content := range notes {
			f, err := tx.PutFile(t.Context(), File{VaultID: vault, Path: p, Hash: "h"})
			if err != nil {
				t.Fatal(err)
			}
			if err := tx.IndexNote(t.Context(), f, []byte(content), markdown.Doc{Aliases: aliases[p]}); err != nil {
				t.Fatal(err)
			}
		}
	})
	return vault
}

func hitPaths(hits []api.SearchHit) []string {
	out := []string{}
	for _, h := range hits {
		out = append(out, h.Path)
	}
	return out
}

func TestSearch(t *testing.T) {
	ft := newFileTest(t)
	ctx := t.Context()
	vault := searchVault(t, ft, map[string]string{
		"Orchard/Planting.md": "Rows of quince.\n",
		"Garden/Beds.md":      "We plan the beds in March and keep the planting log here.\n",
		"Recipes/Café.md":     "Butter and apples.\n",
		"Logs/Harvest.md":     "---\nseason: autumn\n---\nPicked pears.\n",
		"Logs/Old.md":         "Nothing about it.\n",
		"Logs/Alias.md":       "Empty.\n",
	}, map[string][]string{"Logs/Alias.md": {"Seedling planner"}})
	other := searchVault(t, ft, map[string]string{"Plan.md": "plan plan plan\n"}, nil)

	for _, tc := range []struct {
		name, query string
		want        []string
	}{
		{"prefix, title before alias before body", "plan", []string{"Orchard/Planting.md", "Logs/Alias.md", "Garden/Beds.md"}},
		{"diacritics and case", "CAFE", []string{"Recipes/Café.md"}},
		{"frontmatter text", "autumn", []string{"Logs/Harvest.md"}},
		{"every term", "plan march", []string{"Garden/Beds.md"}},
		{"phrase", `"planting log"`, []string{"Garden/Beds.md"}},
		{"phrase is not a prefix", `"plan the bed"`, []string{}},
		{"no match", "zucchini", []string{}},
		{"nothing to match", "--", []string{}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			hits, err := ft.d.Search(ctx, vault, SearchQuery(tc.query), 20)
			if err != nil {
				t.Fatal(err)
			}
			if got := hitPaths(hits); fmt.Sprint(got) != fmt.Sprint(tc.want) {
				t.Fatalf("Search(%q) = %q, want %q", tc.query, got, tc.want)
			}
		})
	}

	hits, err := ft.d.Search(ctx, vault, SearchQuery("plan"), 2)
	if err != nil || len(hits) != 2 {
		t.Fatalf("limit 2: %d hits, %v", len(hits), err)
	}
	if hits, _ := ft.d.Search(ctx, other, SearchQuery("plan"), 20); fmt.Sprint(hitPaths(hits)) != "[Plan.md]" {
		t.Fatalf("another vault's search = %q", hitPaths(hits))
	}
	if hits, err := ft.d.Search(ctx, vault, "", 20); err != nil || len(hits) != 0 {
		t.Fatalf(`Search("") = %v, %v; want none`, hits, err)
	}
	if hits[0].Title != "Planting" {
		t.Errorf("title = %q, want the file name without .md", hits[0].Title)
	}
}

func TestSearchSnippets(t *testing.T) {
	ft := newFileTest(t)
	long := strings.Repeat("filler words go here ", 20) + "the quince harvest was early " + strings.Repeat("more filler text ", 20)
	vault := searchVault(t, ft, map[string]string{
		"Body.md":   "We plan the beds.\n",
		"Long.md":   long,
		"Quince.md": "Nothing here.\n",
	}, nil)
	snippet := func(query, path string) []api.SnippetPart {
		t.Helper()
		hits, err := ft.d.Search(t.Context(), vault, SearchQuery(query), 20)
		if err != nil {
			t.Fatal(err)
		}
		for _, h := range hits {
			if h.Path == path {
				return h.Snippet
			}
		}
		t.Fatalf("%s did not match %q", path, query)
		return nil
	}
	join := func(parts []api.SnippetPart) (all, matched string) {
		for _, p := range parts {
			if p.Text == "" {
				t.Errorf("empty part in %+v", parts)
			}
			all += p.Text
			if p.Match {
				matched += "[" + p.Text + "]"
			}
		}
		return all, matched
	}

	all, matched := join(snippet("plan", "Body.md"))
	if strings.TrimSpace(all) != "We plan the beds." || matched != "[plan]" {
		t.Errorf("Body.md snippet = %q with matches %s", all, matched)
	}
	all, matched = join(snippet("quince", "Long.md"))
	if matched != "[quince]" || !strings.Contains(all, "the quince harvest") || !strings.Contains(all, "…") || len(strings.Fields(all)) > 20 {
		t.Errorf("Long.md snippet = %q with matches %s; want about 16 words around the match, cut with …", all, matched)
	}
	all, matched = join(snippet("quince", "Quince.md"))
	if all != "Quince" || matched != "[Quince]" {
		t.Errorf("a title match's snippet = %q with matches %s; want the title", all, matched)
	}
}
