package store

// CONTRACT TEST for task card T-0017 (docs/tasks). Do not edit.

import (
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/frostyard/frostnotes/api"
	"github.com/frostyard/frostnotes/internal/markdown"
)

// queryVault indexes notes with the given Docs; a path without a Doc is
// an attachment.
func queryVault(t *testing.T, ft *fileTest, docs map[string]*markdown.Doc, paths ...string) map[string]File {
	t.Helper()
	files := map[string]File{}
	ft.tx(func(tx *Tx) {
		for i, p := range paths {
			f, err := tx.PutFile(t.Context(), File{VaultID: ft.vault, Path: p, Size: int64(10 + i),
				ModTime: time.Unix(1_760_000_000+int64(i), 5), Hash: fmt.Sprintf("hash%d", i)})
			if err != nil {
				t.Fatal(err)
			}
			files[p] = f
		}
		for _, p := range paths {
			if doc := docs[p]; doc != nil {
				if err := tx.IndexNote(t.Context(), files[p], []byte("x"), *doc); err != nil {
					t.Fatal(err)
				}
			}
		}
	})
	return files
}

func entryPaths(entries []api.NoteEntry) []string {
	out := []string{}
	for _, e := range entries {
		out = append(out, e.Path)
	}
	return out
}

func TestListNotes(t *testing.T) {
	ft := newFileTest(t)
	ctx := t.Context()
	tags := func(names ...string) []markdown.Tag {
		var out []markdown.Tag
		for _, n := range names {
			out = append(out, markdown.Tag{Name: n, Line: 1})
		}
		return out
	}
	docs := map[string]*markdown.Doc{
		"Inbox.md":                     {Tags: tags("Area/Work")},
		"Orchard/Plan.md":              {Tags: tags("area"), Props: map[string]any{"status": "Done", "owner": []any{"Wren", "Ilsa"}}},
		"Orchard/North Block/Soil.md":  {Tags: tags("areas"), Props: map[string]any{"status": map[string]any{"k": "v"}}},
		"Orchard Notes/Plan.md":        {Tags: tags("work"), Props: map[string]any{"status": "done", "owner": []any{}}},
		"Members/Wren/2026-09-14.md":   {Props: map[string]any{"status": nil, "count": int64(3)}},
		"Orchard/North Block/media.md": {},
	}
	files := queryVault(t, ft, docs, "Inbox.md", "Orchard/Plan.md", "Orchard/North Block/Soil.md", "Orchard Notes/Plan.md",
		"Members/Wren/2026-09-14.md", "Orchard/North Block/media.md", "Orchard/map.png")

	all, err := ft.d.ListNotes(ctx, ft.vault, NoteFilter{})
	if err != nil {
		t.Fatal(err)
	}
	if got, want := entryPaths(all), []string{"Inbox.md", "Members/Wren/2026-09-14.md", "Orchard Notes/Plan.md",
		"Orchard/North Block/Soil.md", "Orchard/North Block/media.md", "Orchard/Plan.md"}; fmt.Sprint(got) != fmt.Sprint(want) {
		t.Fatalf("every note = %q, want %q (notes only, by path)", got, want)
	}
	f := files["Orchard/Plan.md"]
	for _, e := range all {
		if e.Path == f.Path && (e.Title != "Plan" || e.Hash != f.Hash || e.Size != f.Size || !e.ModifiedAt.Equal(f.ModTime)) {
			t.Errorf("entry %+v does not match its file %+v", e, f)
		}
	}

	for _, tc := range []struct {
		name string
		f    NoteFilter
		want []string
	}{
		{"folder and below", NoteFilter{Folder: "Orchard"}, []string{"Orchard/North Block/Soil.md", "Orchard/North Block/media.md", "Orchard/Plan.md"}},
		{"folder with slashes", NoteFilter{Folder: "/Orchard/North Block/"}, []string{"Orchard/North Block/Soil.md", "Orchard/North Block/media.md"}},
		{"folder case matters", NoteFilter{Folder: "orchard"}, []string{}},
		{"tag and nested tags, ignoring case", NoteFilter{Tag: "AREA"}, []string{"Inbox.md", "Orchard/Plan.md"}},
		{"tag with #", NoteFilter{Tag: "#area/work"}, []string{"Inbox.md"}},
		{"has a property, any value", NoteFilter{Property: "status"}, []string{"Members/Wren/2026-09-14.md", "Orchard Notes/Plan.md",
			"Orchard/North Block/Soil.md", "Orchard/Plan.md"}},
		{"has a property with an empty list", NoteFilter{Property: "owner"}, []string{"Orchard Notes/Plan.md", "Orchard/Plan.md"}},
		{"property value, ignoring case", NoteFilter{Property: "status", Value: "DONE"}, []string{"Orchard Notes/Plan.md", "Orchard/Plan.md"}},
		{"list item", NoteFilter{Property: "owner", Value: "ilsa"}, []string{"Orchard/Plan.md"}},
		{"number", NoteFilter{Property: "count", Value: "3"}, []string{"Members/Wren/2026-09-14.md"}},
		{"keys are exact", NoteFilter{Property: "Status"}, []string{}},
		{"filters combine", NoteFilter{Folder: "Orchard", Tag: "area", Property: "status", Value: "done"}, []string{"Orchard/Plan.md"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got, err := ft.d.ListNotes(ctx, ft.vault, tc.f)
			if err != nil {
				t.Fatal(err)
			}
			if fmt.Sprint(entryPaths(got)) != fmt.Sprint(tc.want) {
				t.Fatalf("ListNotes(%+v) = %q, want %q", tc.f, entryPaths(got), tc.want)
			}
		})
	}
	if got, err := ft.d.ListNotes(ctx, ft.vault+1, NoteFilter{}); err != nil || got == nil || len(got) != 0 {
		t.Fatalf("another vault = %#v, %v; want an empty slice", got, err)
	}
}

func TestNoteInfo(t *testing.T) {
	ft := newFileTest(t)
	ctx := t.Context()
	files := queryVault(t, ft, map[string]*markdown.Doc{
		"A.md": {
			Props:   map[string]any{"tags": []any{"x"}, "n": int64(2)},
			Aliases: []string{"Alpha", "First"},
			Tags:    []markdown.Tag{{Name: "x", Front: true, Line: 2}, {Name: "Later", Line: 5}},
			Words:   7,
		},
		"B.md": {PropError: errors.New("frontmatter: not a mapping"), Words: 1},
	}, "A.md", "B.md", "c.png")
	info, err := ft.d.NoteInfo(ctx, files["A.md"].ID)
	if err != nil {
		t.Fatal(err)
	}
	if info.Title != "A" || string(info.Props) != `{"n":2,"tags":["x"]}` || info.PropError != "" ||
		fmt.Sprint(info.Aliases) != "[Alpha First]" || fmt.Sprint(info.Tags) != "[x Later]" || info.Words != 7 {
		t.Errorf("NoteInfo(A.md) = %+v (props %s)", info, info.Props)
	}
	info, err = ft.d.NoteInfo(ctx, files["B.md"].ID)
	if err != nil || string(info.Props) != "{}" || info.PropError != "frontmatter: not a mapping" || info.Aliases == nil || info.Tags == nil {
		t.Errorf("NoteInfo(B.md) = %+v, %v; want {} props, the error, and empty non-nil lists", info, err)
	}
	for _, id := range []int64{files["c.png"].ID, 999} {
		if _, err := ft.d.NoteInfo(ctx, id); !errors.Is(err, ErrNotFound) {
			t.Errorf("NoteInfo(%d) = %v, want ErrNotFound", id, err)
		}
	}
}

func linkStrings(links []api.Link) []string {
	out := []string{}
	for _, l := range links {
		resolved := "-"
		if l.Resolved != nil {
			resolved = *l.Resolved
		}
		out = append(out, fmt.Sprintf("%s %q%s|%s embed=%t key=%s line=%d -> %s", l.Kind, l.Target, l.Subpath, l.Alias, l.Embed, l.Key, l.Line, resolved))
	}
	return out
}

func TestLinksAndBacklinks(t *testing.T) {
	ft := newFileTest(t)
	ctx := t.Context()
	wiki := func(target string, line, start int) markdown.Link {
		return markdown.Link{Kind: markdown.LinkWiki, Target: target, Line: line, Start: start, End: start + 1}
	}
	files := queryVault(t, ft, map[string]*markdown.Doc{
		"Inbox.md": {Links: []markdown.Link{
			{Kind: markdown.LinkProperty, Target: "Plan", Key: "up", Line: 2, Start: 5, End: 13},
			wiki("Plan", 4, 20),
			{Kind: markdown.LinkMarkdown, Target: "media/map.png", Alias: "map", Embed: true, Line: 4, Start: 40, End: 60},
			wiki("Nowhere", 5, 70),
			{Kind: markdown.LinkWiki, Subpath: "#Top", Line: 6, Start: 80, End: 90},
		}},
		"Archive/Old.md": {Links: []markdown.Link{wiki("Plan", 3, 9), wiki("Plan", 1, 50), wiki("Plan", 3, 2)}},
		"Plan.md":        {Links: []markdown.Link{wiki("Plan", 1, 0)}},
	}, "Inbox.md", "Archive/Old.md", "Plan.md", "media/map.png")

	links, err := ft.d.Links(ctx, files["Inbox.md"].ID)
	if err != nil {
		t.Fatal(err)
	}
	want := []string{
		`property "Plan"| embed=false key=up line=2 -> Plan.md`,
		`wiki "Plan"| embed=false key= line=4 -> Plan.md`,
		`markdown "media/map.png"|map embed=true key= line=4 -> media/map.png`,
		`wiki "Nowhere"| embed=false key= line=5 -> -`,
		`wiki ""#Top| embed=false key= line=6 -> Inbox.md`,
	}
	if got := linkStrings(links); strings.Join(got, "\n") != strings.Join(want, "\n") {
		t.Errorf("Links(Inbox.md) =\n%s\nwant\n%s", strings.Join(got, "\n"), strings.Join(want, "\n"))
	}
	if links, err := ft.d.Links(ctx, files["media/map.png"].ID); err != nil || links == nil || len(links) != 0 {
		t.Errorf("Links of an attachment = %#v, %v; want an empty slice", links, err)
	}

	back, err := ft.d.Backlinks(ctx, files["Plan.md"].ID)
	if err != nil {
		t.Fatal(err)
	}
	var got []string
	for _, b := range back {
		got = append(got, fmt.Sprintf("%s:%d:%s", b.Source, b.Link.Line, linkStrings([]api.Link{b.Link})[0]))
	}
	wantBack := []string{
		`Archive/Old.md:1:wiki "Plan"| embed=false key= line=1 -> Plan.md`,
		`Archive/Old.md:3:wiki "Plan"| embed=false key= line=3 -> Plan.md`,
		`Archive/Old.md:3:wiki "Plan"| embed=false key= line=3 -> Plan.md`,
		`Inbox.md:2:property "Plan"| embed=false key=up line=2 -> Plan.md`,
		`Inbox.md:4:wiki "Plan"| embed=false key= line=4 -> Plan.md`,
	}
	if strings.Join(got, "\n") != strings.Join(wantBack, "\n") {
		t.Errorf("Backlinks(Plan.md), without its link to itself =\n%s\nwant\n%s", strings.Join(got, "\n"), strings.Join(wantBack, "\n"))
	}
	if back, err := ft.d.Backlinks(ctx, files["media/map.png"].ID); err != nil || len(back) != 1 || back[0].Source != "Inbox.md" || !back[0].Link.Embed {
		t.Errorf("Backlinks(media/map.png) = %+v, %v", back, err)
	}
	if back, err := ft.d.Backlinks(ctx, files["Inbox.md"].ID); err != nil || back == nil || len(back) != 0 {
		t.Errorf("Backlinks(Inbox.md) = %#v, %v; want an empty slice", back, err)
	}
}
