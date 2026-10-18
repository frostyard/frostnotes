package store

// CONTRACT TEST for task card T-0043 (docs/tasks). Do not edit.

import (
	"reflect"
	"testing"
	"time"

	"github.com/frostyard/frostnotes/api"
	"github.com/frostyard/frostnotes/internal/markdown"
)

func countTags(names ...string) []markdown.Tag {
	var out []markdown.Tag
	for _, n := range names {
		out = append(out, markdown.Tag{Name: n, Line: 1})
	}
	return out
}

// otherVault indexes one note with doc in a second vault, which counts
// must leave out.
func otherVault(t *testing.T, ft *fileTest, doc markdown.Doc) {
	t.Helper()
	ft.tx(func(tx *Tx) {
		v, err := tx.InsertVault(t.Context(), "other", "/other")
		if err != nil {
			t.Fatal(err)
		}
		f, err := tx.PutFile(t.Context(), File{VaultID: v.ID, Path: "x.md", Hash: "h", ModTime: time.Unix(1, 0)})
		if err != nil {
			t.Fatal(err)
		}
		if err := tx.IndexNote(t.Context(), f, []byte("x"), doc); err != nil {
			t.Fatal(err)
		}
	})
}

func TestTagCounts(t *testing.T) {
	ft := newFileTest(t)
	ctx := t.Context()
	if got, err := ft.d.TagCounts(ctx, ft.vault); err != nil || got == nil || len(got) != 0 {
		t.Fatalf("an empty vault's tags = %#v, %v; want an empty slice", got, err)
	}
	queryVault(t, ft, map[string]*markdown.Doc{
		"a.md": {Tags: countTags("garden", "Area/Work")},
		"b.md": {Tags: countTags("Garden", "area")},
		"c.md": {Tags: countTags("garden/beds", "zinnia")},
		"d.md": {},
	}, "a.md", "b.md", "c.md", "d.md", "map.png")
	otherVault(t, ft, markdown.Doc{Tags: countTags("garden", "elsewhere")})

	got, err := ft.d.TagCounts(ctx, ft.vault)
	if err != nil {
		t.Fatal(err)
	}
	want := []api.TagCount{
		{Tag: "area", Count: 1},
		{Tag: "Area/Work", Count: 1},
		{Tag: "Garden", Count: 2},
		{Tag: "garden/beds", Count: 1},
		{Tag: "zinnia", Count: 1},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("TagCounts = %+v\nwant %+v\n(one per tag ignoring case, by folded name; nested tags whole; the spelling that sorts first)", got, want)
	}
}

func TestPropertyCounts(t *testing.T) {
	ft := newFileTest(t)
	ctx := t.Context()
	if got, err := ft.d.PropertyCounts(ctx, ft.vault, nil); err != nil || got == nil || len(got) != 0 {
		t.Fatalf("an empty vault's properties = %#v, %v; want an empty slice", got, err)
	}
	queryVault(t, ft, map[string]*markdown.Doc{
		"a.md": {Props: map[string]any{"status": "done", "Tags": []any{"x"}, "due": "2026-10-17", "été": "chaud"}},
		"b.md": {Props: map[string]any{"Status": nil, "owner": []any{}, "Été": 1}},
		"c.md": {Props: map[string]any{"status": map[string]any{"k": "v"}, "count": int64(3)}},
		"d.md": {},
	}, "a.md", "b.md", "c.md", "d.md", "map.png")
	otherVault(t, ft, markdown.Doc{Props: map[string]any{"status": "x", "elsewhere": true}})

	types := map[string]string{"due": "date", "tags": "tags", "count": "number", "unused": "text"}
	got, err := ft.d.PropertyCounts(ctx, ft.vault, types)
	if err != nil {
		t.Fatal(err)
	}
	want := []api.PropertyCount{
		{Key: "count", Type: "number", Count: 1},
		{Key: "due", Type: "date", Count: 1},
		{Key: "owner", Type: "", Count: 1},
		{Key: "Status", Type: "", Count: 3},
		{Key: "Tags", Type: "tags", Count: 1},
		{Key: "Été", Type: "", Count: 2},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("PropertyCounts = %+v\nwant %+v\n(one per key ignoring case as strings.ToLower folds it, by lower-cased key; null and empty values count; types by lower-cased key)", got, want)
	}
	if got, err := ft.d.PropertyCounts(ctx, ft.vault, nil); err != nil || len(got) != 6 || got[1].Type != "" {
		t.Fatalf("without types: %+v, %v; want the same keys, each of type \"\"", got, err)
	}
}
