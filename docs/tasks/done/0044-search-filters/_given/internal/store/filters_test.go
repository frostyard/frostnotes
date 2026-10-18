package store

// CONTRACT TEST for task card T-0044 (docs/tasks). Do not edit.

import (
	"reflect"
	"slices"
	"testing"
	"time"

	"github.com/frostyard/frostnotes/internal/markdown"
)

func TestParseQuery(t *testing.T) {
	for _, tc := range []struct {
		text string
		want Query
	}{
		{"plan", Query{Match: `"plan"*`}},
		{"", Query{}},
		{"path:Garden plan", Query{Match: `"plan"*`, Paths: []string{"Garden"}}},
		{`PATH:"Daily notes" File:2026 "planting log"`, Query{Match: `"planting log"`, Paths: []string{"Daily notes"}, Files: []string{"2026"}}},
		{"tag:#Area/Work tag:garden", Query{Tags: []string{"area/work", "garden"}}},
		{"path:a  path:b", Query{Paths: []string{"a", "b"}}},
		{`path: file:"" tag:# plan`, Query{Match: `"plan"*`}},
		{`path:"Unclosed folder`, Query{Paths: []string{"Unclosed folder"}}},
		{"foo:bar paths:x", Query{Match: `"foo:bar"* "paths:x"*`}},
		{`plan "path:x"`, Query{Match: `"plan"* "path:x"`}},
	} {
		if got := ParseQuery(tc.text); !reflect.DeepEqual(got, tc.want) {
			t.Errorf("ParseQuery(%q) = %#v, want %#v", tc.text, got, tc.want)
		}
	}
}

func TestSearchNotes(t *testing.T) {
	ft := newFileTest(t)
	ctx := t.Context()
	notes := []struct {
		path, text string
		tags       []string
	}{
		{"Garden/Beds.md", "We plan the beds.\n", []string{"garden"}},
		{"Garden/Archive/Old plan.md", "Plans of 2020.\n", []string{"Garden/Old"}},
		{"Journal/2026-10-17.md", "Plan the day.\n", []string{"journal"}},
		{"Projects/Café plan.md", "Costs for the café.\n", []string{"Work"}},
		{"Projects/Zinnias.md", "Nothing here.\n", []string{"work/flowers"}},
	}
	ft.tx(func(tx *Tx) {
		for i, n := range notes {
			f, err := tx.PutFile(ctx, File{VaultID: ft.vault, Path: n.path, Hash: "h", ModTime: time.Unix(int64(i), 0)})
			if err != nil {
				t.Fatal(err)
			}
			if err := tx.IndexNote(ctx, f, []byte(n.text), markdown.Doc{Tags: countTags(n.tags...)}); err != nil {
				t.Fatal(err)
			}
		}
		if _, err := tx.PutFile(ctx, File{VaultID: ft.vault, Path: "Garden/plan.png", Hash: "p", ModTime: time.Unix(9, 0)}); err != nil {
			t.Fatal(err)
		}
	})
	other := searchVault(t, ft, map[string]string{"Garden/Plan.md": "plan\n"}, nil)

	search := func(text string, limit int) []string {
		t.Helper()
		hits, err := ft.d.SearchNotes(ctx, ft.vault, ParseQuery(text), limit)
		if err != nil {
			t.Fatal(err)
		}
		if hits == nil {
			t.Fatalf("SearchNotes(%q) = nil, want a slice", text)
		}
		return hitPaths(hits)
	}
	sorted := func(s []string) []string { return slices.Sorted(slices.Values(s)) }

	for _, tc := range []struct {
		name, text string
		want       []string
	}{
		{"words alone, as Search", "plan", []string{"Garden/Archive/Old plan.md", "Garden/Beds.md", "Journal/2026-10-17.md", "Projects/Café plan.md"}},
		{"path narrows, ignoring case", "plan path:garden", []string{"Garden/Archive/Old plan.md", "Garden/Beds.md"}},
		{"file narrows by the name, Unicode case ignored", "plan file:CAFÉ", []string{"Projects/Café plan.md"}},
		{"file is the last segment only", "plan file:garden", []string{}},
		{"tag narrows, nested tags too", "plan tag:#GARDEN", []string{"Garden/Archive/Old plan.md", "Garden/Beds.md"}},
		{"filters combine", "plan path:garden file:old", []string{"Garden/Archive/Old plan.md"}},
		{"no note admitted", "plan path:nowhere", []string{}},
	} {
		if got := sorted(search(tc.text, 20)); !slices.Equal(got, tc.want) {
			t.Errorf("%s: SearchNotes(%q) = %q, want %q", tc.name, tc.text, got, tc.want)
		}
	}

	// Filters alone list the notes they admit, by path, without snippets.
	if got, want := search("tag:work", 20), []string{"Projects/Café plan.md", "Projects/Zinnias.md"}; !reflect.DeepEqual(got, want) {
		t.Errorf("tag:work = %q, want %q", got, want)
	}
	if got, want := search("path:garden", 20), []string{"Garden/Archive/Old plan.md", "Garden/Beds.md"}; !reflect.DeepEqual(got, want) {
		t.Errorf("path:garden = %q, want %q (notes only, by path)", got, want)
	}
	if got, want := search("path:garden", 1), []string{"Garden/Archive/Old plan.md"}; !reflect.DeepEqual(got, want) {
		t.Errorf("path:garden, limit 1 = %q, want %q", got, want)
	}
	hits, err := ft.d.SearchNotes(ctx, ft.vault, ParseQuery("tag:journal"), 20)
	if err != nil || len(hits) != 1 || hits[0].Title != "2026-10-17" || hits[0].Snippet == nil || len(hits[0].Snippet) != 0 {
		t.Errorf("tag:journal = %#v, %v; want one hit titled 2026-10-17 with an empty snippet", hits, err)
	}

	// Ranked matches keep their snippets, and the limit counts admitted notes.
	hits, err = ft.d.SearchNotes(ctx, ft.vault, ParseQuery("plan path:garden"), 1)
	if err != nil || len(hits) != 1 || len(hits[0].Snippet) == 0 {
		t.Errorf("plan path:garden, limit 1 = %#v, %v; want one hit with a snippet", hits, err)
	}
	if got := search("", 20); len(got) != 0 {
		t.Errorf(`SearchNotes("") = %q, want none`, got)
	}
	if hits, _ := ft.d.SearchNotes(ctx, other, ParseQuery("path:garden"), 20); !reflect.DeepEqual(hitPaths(hits), []string{"Garden/Plan.md"}) {
		t.Errorf("another vault's path:garden = %q", hitPaths(hits))
	}
}
