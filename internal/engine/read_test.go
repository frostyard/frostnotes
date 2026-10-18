package engine_test

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json/v2"
	"errors"
	"fmt"
	"html"
	"maps"
	"os"
	"path"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/frostyard/frostnotes/api"
	"github.com/frostyard/frostnotes/internal/vaultgen"
)

// generatedVault is a generated vault with an empty folder, a hidden one,
// an ignored one and Obsidian's property types, registered with a test
// notesd.
func generatedVault(t *testing.T) (*apiVault, vaultgen.Manifest) {
	t.Helper()
	dir := filepath.Join(t.TempDir(), "vault")
	m, err := vaultgen.Generate(dir, vaultgen.Options{Seed: 5, Notes: 60})
	if err != nil {
		t.Fatal(err)
	}
	writeFiles(t, dir, map[string]string{
		".frostnotes/vault.yaml": "ignore: [\"Archive/\"]\n",
		".obsidian/types.json":   `{"types":{"created":"date","Rating":"number"}}`,
		".hidden/Secret.md":      "hidden\n",
		"Archive/Old.md":         "ignored\n",
	})
	if err := os.MkdirAll(filepath.Join(dir, "Empty", "Deeper"), 0o755); err != nil {
		t.Fatal(err)
	}
	return openVault(t, dir), m
}

func TestFilesListsTheIndexAndEveryFolder(t *testing.T) {
	av, m := generatedVault(t)
	got, err := av.c.Note().Files(t.Context(), &api.NoteFilesParams{VaultID: av.id})
	if err != nil {
		t.Fatal(err)
	}
	want := slices.Sorted(slices.Values(append(slices.Clone(m.Notes), m.Attachments...)))
	var paths []string
	for _, f := range got.Files {
		paths = append(paths, f.Path)
		data, err := os.ReadFile(filepath.Join(av.dir, filepath.FromSlash(f.Path)))
		if err != nil {
			t.Fatal(err)
		}
		sum := sha256.Sum256(data)
		kind := api.FileKindAttachment
		if strings.HasSuffix(f.Path, ".md") {
			kind = api.FileKindNote
		}
		if f.Hash != hex.EncodeToString(sum[:]) || f.Size != int64(len(data)) || f.Kind != kind || f.ModifiedAt.IsZero() {
			t.Errorf("%s: %+v, want hash %x, %d bytes, kind %s", f.Path, f, sum, len(data), kind)
		}
	}
	if !slices.Equal(paths, want) {
		t.Errorf("files = %v\nwant %v", paths, want)
	}
	folders := map[string]bool{"Empty": true, "Empty/Deeper": true}
	for _, p := range want {
		for dir := path.Dir(p); dir != "."; dir = path.Dir(dir) {
			folders[dir] = true
		}
	}
	if w := slices.Sorted(maps.Keys(folders)); !slices.Equal(got.Folders, w) {
		t.Errorf("folders = %v\nwant %v (no hidden, settings or ignored folder)", got.Folders, w)
	}
	if _, err := av.c.Note().Files(t.Context(), &api.NoteFilesParams{VaultID: 999}); !errors.Is(err, api.ErrNotFound) {
		t.Errorf("Files of an unknown vault = %v, want notFound", err)
	}
}

func TestResolveFollowsTheManifest(t *testing.T) {
	av, m := generatedVault(t)
	bySource := map[string][]vaultgen.Link{}
	for _, l := range m.Links {
		bySource[l.Source] = append(bySource[l.Source], l)
	}
	for source, links := range bySource {
		var targets []string
		for _, l := range links {
			targets = append(targets, l.Target)
		}
		got, err := av.c.Note().Resolve(t.Context(), &api.NoteResolveParams{VaultID: av.id, From: source, Targets: targets})
		if err != nil || len(got) != len(links) {
			t.Fatalf("Resolve(%s) = %+v, %v", source, got, err)
		}
		for i, l := range links {
			dest := ""
			if got[i].Path != nil {
				dest = *got[i].Path
			}
			if got[i].Target != l.Target || dest != l.Want {
				t.Errorf("%s: %q resolves to %q, want %q", source, l.Target, dest, l.Want)
			}
		}
	}
	// From a note not yet written: by name, wherever it would be.
	got, err := av.c.Note().Resolve(t.Context(), &api.NoteResolveParams{VaultID: av.id, From: "New/Unsaved.md",
		Targets: []string{"Overview", "", "No such note"}})
	if err != nil || len(got) != 3 || got[0].Path == nil || *got[0].Path != "Garden/Overview.md" ||
		got[1].Path == nil || *got[1].Path != "New/Unsaved.md" || got[2].Path != nil {
		t.Errorf("Resolve from an unsaved note = %+v, %v", got, err)
	}
	for _, p := range []*api.NoteResolveParams{
		{VaultID: av.id, From: "../x.md"},
		{VaultID: av.id, From: "a.md", Targets: make([]string, 1001)},
	} {
		if _, err := av.c.Note().Resolve(t.Context(), p); !errors.Is(err, api.ErrInvalidParams) {
			t.Errorf("Resolve(from %q, %d targets) = %v, want invalidParams", p.From, len(p.Targets), err)
		}
	}
}

func TestRenderDrawsEveryLinkOfTheManifest(t *testing.T) {
	av, m := generatedVault(t)
	ctx := t.Context()
	files, err := av.c.Note().Files(ctx, &api.NoteFilesParams{VaultID: av.id})
	if err != nil {
		t.Fatal(err)
	}
	hashes := map[string]string{}
	for _, f := range files.Files {
		hashes[f.Path] = f.Hash
	}
	rendered := map[string]string{}
	for _, p := range m.Notes {
		r, err := av.c.Note().Render(ctx, &api.NoteRenderParams{VaultID: av.id, Path: p})
		if err != nil {
			t.Fatalf("Render(%s): %v", p, err)
		}
		rendered[p] = r.HTML
	}
	for _, l := range m.Links {
		if l.Want == "" {
			continue
		}
		h := rendered[l.Source]
		if !strings.Contains(h, `data-path="`+html.EscapeString(l.Want)+`"`) {
			t.Errorf("%s: %s draws no link to %s", l.Source, l.Text, l.Want)
		}
		if strings.HasPrefix(l.Text, "![[") && !strings.Contains(h, `data-blob="`+hashes[l.Want]+path.Ext(l.Want)+`"`) {
			t.Errorf("%s: %s names no blob for %s", l.Source, l.Text, l.Want)
		}
	}

	// What an editor holds renders as if it were the note, sanitized.
	content := "Unsaved [[Overview#Goals]] <script>alert(1)</script><img src=x onerror=alert(1)>\n"
	r, err := av.c.Note().Render(ctx, &api.NoteRenderParams{VaultID: av.id, Path: "New/Unsaved.md", Content: &content})
	if err != nil || !strings.Contains(r.HTML, `data-path="Garden/Overview.md">Overview &gt; Goals</a>`) ||
		strings.Contains(r.HTML, "script") || strings.Contains(r.HTML, "onerror") {
		t.Errorf("Render(content) = %+v, %v", r, err)
	}
	for _, tc := range []struct {
		p    string
		want error
	}{
		{"No such note.md", api.ErrNotFound},
		{m.Attachments[0], api.ErrInvalidParams},
		{".frostnotes/vault.yaml", api.ErrInvalidParams},
		{"../out.md", api.ErrInvalidParams},
	} {
		if _, err := av.c.Note().Render(ctx, &api.NoteRenderParams{VaultID: av.id, Path: tc.p}); !errors.Is(err, tc.want) {
			t.Errorf("Render(%s) = %v, want %v", tc.p, err, tc.want)
		}
	}
}

func TestReadVersionGivesEachVersionsContent(t *testing.T) {
	dir := t.TempDir()
	writeFiles(t, dir, map[string]string{"Plan.md": "first\n", "bin.dat": "a\xffb"})
	av := openVault(t, dir)
	ctx := t.Context()
	note := av.c.Note()
	n, err := note.Read(ctx, &api.NoteReadParams{VaultID: av.id, Path: "Plan.md"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := note.Write(ctx, &api.NoteWriteParams{VaultID: av.id, Path: "Plan.md", Base: n.Hash, Content: "second\n"}); err != nil {
		t.Fatal(err)
	}
	cur, _ := note.Read(ctx, &api.NoteReadParams{VaultID: av.id, Path: "Plan.md"})
	if err := note.Delete(ctx, &api.NoteDeleteParams{VaultID: av.id, Path: "Plan.md", Base: cur.Hash}); err != nil {
		t.Fatal(err)
	}
	vs, err := note.Versions(ctx, &api.NoteVersionsParams{VaultID: av.id, Path: "Plan.md"})
	if err != nil || len(vs) != 3 {
		t.Fatalf("Versions = %+v, %v", vs, err)
	}
	for i, want := range []string{"", "second\n", "first\n"} {
		got, err := note.ReadVersion(ctx, &api.NoteReadVersionParams{VaultID: av.id, Version: vs[i].ID})
		if err != nil || got.Content != want || got.Version != vs[i] {
			t.Errorf("ReadVersion(%d) = %+v, %v; want %q", vs[i].ID, got, err, want)
		}
	}
	bin, _ := note.Versions(ctx, &api.NoteVersionsParams{VaultID: av.id, Path: "bin.dat"})
	if got, err := note.ReadVersion(ctx, &api.NoteReadVersionParams{VaultID: av.id, Version: bin[0].ID}); err != nil || got.Content != "a�b" {
		t.Errorf("ReadVersion of bytes that are not UTF-8 = %+v, %v", got, err)
	}

	// Another vault's version does not exist in this one.
	other := t.TempDir()
	writeFiles(t, other, map[string]string{"Else.md": "else\n"})
	v2, err := av.c.Vault().Add(ctx, &api.VaultAddParams{Path: other})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := note.ReadVersion(ctx, &api.NoteReadVersionParams{VaultID: v2.ID, Version: vs[0].ID}); !errors.Is(err, api.ErrNotFound) {
		t.Errorf("ReadVersion of another vault's version = %v, want notFound", err)
	}
	if _, err := note.ReadVersion(ctx, &api.NoteReadVersionParams{VaultID: av.id, Version: 99999}); !errors.Is(err, api.ErrNotFound) {
		t.Errorf("ReadVersion(99999) = %v, want notFound", err)
	}
}

func TestTagsPropertiesAndFiltersMatchTheNotes(t *testing.T) {
	av, m := generatedVault(t)
	ctx := t.Context()
	tags, props := map[string]int64{}, map[string]int64{}
	orchard := []string{}
	for _, p := range m.Notes {
		n, err := av.c.Note().Read(ctx, &api.NoteReadParams{VaultID: av.id, Path: p})
		if err != nil {
			t.Fatal(err)
		}
		seen := map[string]bool{}
		nested := false
		for _, tag := range n.Tags {
			tag = strings.ToLower(tag)
			if !seen[tag] {
				seen[tag] = true
				tags[tag]++
			}
			nested = nested || tag == "orchard" || strings.HasPrefix(tag, "orchard/")
		}
		if nested {
			orchard = append(orchard, p)
		}
		keys := map[string]bool{}
		for _, k := range propKeys(t, n.Props) {
			keys[strings.ToLower(k)] = true
		}
		for k := range keys {
			props[k]++
		}
	}

	gotTags, err := av.c.Note().Tags(ctx, &api.NoteTagsParams{VaultID: av.id})
	if err != nil {
		t.Fatal(err)
	}
	got := map[string]int64{}
	for _, tc := range gotTags {
		got[strings.ToLower(tc.Tag)] = tc.Count
	}
	if !maps.Equal(got, tags) || len(tags) < 3 {
		t.Errorf("Tags = %v, want %v", got, tags)
	}

	gotProps, err := av.c.Note().Properties(ctx, &api.NotePropertiesParams{VaultID: av.id})
	if err != nil {
		t.Fatal(err)
	}
	got = map[string]int64{}
	types := map[string]string{}
	for _, pc := range gotProps {
		got[strings.ToLower(pc.Key)] = pc.Count
		types[strings.ToLower(pc.Key)] = pc.Type
	}
	if !maps.Equal(got, props) {
		t.Errorf("Properties = %v, want %v", got, props)
	}
	if types["created"] != "date" || types["tags"] != "tags" || types["title"] != "" {
		t.Errorf("property types = %v; want created from types.json, tags built in, title untyped", types)
	}

	limit := int64(100)
	hits, err := av.c.Note().Search(ctx, &api.NoteSearchParams{VaultID: av.id, Query: "tag:#Orchard", Limit: &limit})
	var paths []string
	for _, h := range hits {
		paths = append(paths, h.Path)
	}
	if err != nil || !slices.Equal(paths, orchard) || len(orchard) == 0 {
		t.Errorf("Search(tag:#Orchard) = %v, %v\nwant %v", paths, err, orchard)
	}
	hits, err = av.c.Note().Search(ctx, &api.NoteSearchParams{VaultID: av.id, Query: "quince path:garden/"})
	if err != nil || len(hits) == 0 {
		t.Fatalf("Search(quince path:garden/) = %v, %v", hits, err)
	}
	for _, h := range hits {
		if !strings.HasPrefix(h.Path, "Garden/") || len(h.Snippet) == 0 {
			t.Errorf("Search(quince path:garden/) found %+v", h)
		}
	}
}

// propKeys returns the keys of a note's props, a JSON object.
func propKeys(t *testing.T, props []byte) []string {
	t.Helper()
	var obj map[string]any
	if err := json.Unmarshal(props, &obj); err != nil {
		t.Fatal(err)
	}
	return slices.Collect(maps.Keys(obj))
}

func TestVaultSettingsSayWhereEachComesFrom(t *testing.T) {
	dir := t.TempDir()
	writeFiles(t, dir, map[string]string{
		".frostnotes/vault.yaml":     "daily:\n  folder: Journal/\nlinks:\n  update: true\n",
		".obsidian/daily-notes.json": `{"folder":"Daily","format":"YYYY/MM/DD","template":""}`,
		".obsidian/app.json":         `{"newFileLocation":"folder","newFileFolderPath":"Inbox/","useMarkdownLinks":true,"alwaysUpdateLinks":false,"newLinkFormat":"sideways"}`,
	})
	av := openVault(t, dir)
	s, err := av.c.Vault().Settings(t.Context(), &api.VaultSettingsParams{ID: av.id})
	if err != nil {
		t.Fatal(err)
	}
	want := api.VaultSettings{
		DailyFolder: "Journal", DailyFormat: "YYYY/MM/DD", TemplateDateFormat: "YYYY-MM-DD", TemplateTimeFormat: "HH:mm",
		NewNoteLocation: api.NewNoteLocationFolder, NewNoteFolder: "Inbox", AttachmentFolder: "/",
		LinkFormat: api.LinkFormatShortest, Wikilinks: false, UpdateLinks: true, Ignore: []string{},
		Sources: map[string]api.SettingSource{
			"dailyFolder": "frostnotes", "dailyFormat": "obsidian", "dailyTemplate": "default",
			"templateFolder": "default", "templateDateFormat": "default", "templateTimeFormat": "default",
			"newNoteLocation": "obsidian", "newNoteFolder": "obsidian", "attachmentFolder": "default",
			"linkFormat": "default", "wikilinks": "obsidian", "updateLinks": "frostnotes", "ignore": "default",
		},
	}
	if fmt.Sprintf("%+v", *s) != fmt.Sprintf("%+v", want) {
		t.Errorf("Settings =\n%+v\nwant\n%+v", *s, want)
	}

	bare := openVault(t, t.TempDir())
	s, err = bare.c.Vault().Settings(t.Context(), &api.VaultSettingsParams{ID: bare.id})
	if err != nil || s.NewNoteLocation != api.NewNoteLocationRoot || !s.Wikilinks || s.UpdateLinks ||
		s.AttachmentFolder != "/" || s.DailyFormat != "YYYY-MM-DD" || len(s.Sources) != 13 {
		t.Errorf("a bare vault's settings = %+v, %v; want Obsidian's defaults", s, err)
	}
	for k, src := range s.Sources {
		if src != api.SettingSourceDefault {
			t.Errorf("a bare vault's %s comes from %s", k, src)
		}
	}
	if _, err := bare.c.Vault().Settings(t.Context(), &api.VaultSettingsParams{ID: 999}); !errors.Is(err, api.ErrNotFound) {
		t.Errorf("Settings of an unknown vault = %v, want notFound", err)
	}
}
