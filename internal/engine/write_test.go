package engine_test

import (
	"bytes"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json/v2"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/frostyard/frostnotes/api"
)

func TestUploadCreatesAFileFromBytes(t *testing.T) {
	av := openVault(t, t.TempDir())
	ctx := t.Context()
	data := []byte{0x89, 'P', 'N', 'G', 0, 0xff, '\n'}
	up := &api.NoteUploadParams{VaultID: av.id, Path: "Assets/Pasted image 20261017093000.png",
		Data: base64.StdEncoding.EncodeToString(data)}
	res, err := av.c.Note().Upload(ctx, up)
	sum := sha256.Sum256(data)
	if err != nil || res.Hash != hex.EncodeToString(sum[:]) {
		t.Fatalf("Upload = %+v, %v", res, err)
	}
	if got, err := os.ReadFile(filepath.Join(av.dir, filepath.FromSlash(up.Path))); err != nil || !bytes.Equal(got, data) {
		t.Fatalf("the file holds %q, %v", got, err)
	}
	vs, err := av.c.Note().Versions(ctx, &api.NoteVersionsParams{VaultID: av.id, Path: up.Path})
	if err != nil || len(vs) != 1 || vs[0].Author != "rpctest" {
		t.Errorf("versions = %+v, %v", vs, err)
	}
	if _, err := av.c.Note().Upload(ctx, up); !errors.Is(err, api.ErrConflict) {
		t.Errorf("an upload onto an existing file = %v, want conflict", err)
	}
	for name, data := range map[string]string{
		"not base64":   "!!!",
		"over 10 MB":   base64.StdEncoding.EncodeToString(make([]byte, 10<<20+1)),
		"its encoding": strings.Repeat("A", base64.StdEncoding.EncodedLen(10<<20)+4),
	} {
		_, err := av.c.Note().Upload(ctx, &api.NoteUploadParams{VaultID: av.id, Path: "big.bin", Data: data})
		if !errors.Is(err, api.ErrInvalidParams) {
			t.Errorf("an upload %s = %v, want invalidParams", name, err)
		}
	}
}

func TestMkdirMakesFoldersTheListingShows(t *testing.T) {
	dir := t.TempDir()
	writeFiles(t, dir, map[string]string{".frostnotes/vault.yaml": "ignore: [\"Skip/\"]\n", "Plan.md": "plan\n"})
	av := openVault(t, dir)
	ctx := t.Context()
	for range 2 { // an existing folder is not an error
		if err := av.c.Note().Mkdir(ctx, &api.NoteMkdirParams{VaultID: av.id, Path: "Projects/Alpha/Beta/"}); err != nil {
			t.Fatal(err)
		}
	}
	files, err := av.c.Note().Files(ctx, &api.NoteFilesParams{VaultID: av.id})
	if err != nil || fmt.Sprint(files.Folders) != "[Projects Projects/Alpha Projects/Alpha/Beta]" {
		t.Errorf("folders = %v, %v", files.Folders, err)
	}
	for _, p := range []string{"", "Plan.md", "Plan.md/x", ".frostnotes/x", ".hidden", "Skip/a", "../x"} {
		if err := av.c.Note().Mkdir(ctx, &api.NoteMkdirParams{VaultID: av.id, Path: p}); !errors.Is(err, api.ErrInvalidParams) {
			t.Errorf("Mkdir(%q) = %v, want invalidParams", p, err)
		}
	}
}

func TestTemplateExpandsItsVariables(t *testing.T) {
	dir := t.TempDir()
	writeFiles(t, dir, map[string]string{
		".frostnotes/vault.yaml": "templates:\n  dateFormat: DD.MM.YYYY\n",
		"Templates/Meeting.md":   "# {{title}}\n{{date}} {{time}} {{date:YYYY}} {{date+1d}}\n",
		"Templates/logo.png":     "png",
	})
	av := openVault(t, dir)
	ctx := t.Context()
	title, at := "Standup", time.Date(2026, 10, 17, 9, 30, 0, 0, time.UTC)
	got, err := av.c.Note().Template(ctx, &api.NoteTemplateParams{VaultID: av.id, Template: "Templates/Meeting.md", Title: &title, At: &at})
	if want := "# Standup\n17.10.2026 09:30 2026 18.10.2026\n"; err != nil || got.Content != want {
		t.Errorf("Template = %+v, %v; want %q", got, err, want)
	}
	for p, want := range map[string]error{"Templates/None.md": api.ErrNotFound, "Templates/logo.png": api.ErrInvalidParams} {
		if _, err := av.c.Note().Template(ctx, &api.NoteTemplateParams{VaultID: av.id, Template: p}); !errors.Is(err, want) {
			t.Errorf("Template(%s) = %v, want %v", p, err, want)
		}
	}
}

// A folder moves with every file in it, each keeping its versions, and
// every link of the generated vault still leads to the same file.
func TestMoveFolderKeepsEveryLink(t *testing.T) {
	av, m := generatedVault(t)
	ctx := t.Context()
	from, to := "Orchard/North Block", "Fields/North"
	resolved := func(p string) []string {
		links, err := av.c.Note().Links(ctx, &api.NoteLinksParams{VaultID: av.id, Path: p})
		if err != nil {
			t.Fatalf("Links(%s): %v", p, err)
		}
		var out []string
		for _, l := range links {
			dest := ""
			if l.Resolved != nil {
				dest = *l.Resolved
			}
			out = append(out, fmt.Sprintf("%d %s", l.Line, dest))
		}
		slices.Sort(out)
		return out
	}
	moved := func(p string) string {
		if rest, ok := strings.CutPrefix(p, from+"/"); ok {
			return to + "/" + rest
		}
		return p
	}
	before := map[string][]string{}
	var wantMoved []string
	for _, p := range m.Notes {
		before[p] = resolved(p)
	}
	for _, p := range append(slices.Clone(m.Notes), m.Attachments...) {
		if moved(p) != p {
			wantMoved = append(wantMoved, moved(p))
		}
	}
	slices.Sort(wantMoved)
	versions, err := av.c.Note().Versions(ctx, &api.NoteVersionsParams{VaultID: av.id, Path: from + "/Plan.md"})
	if err != nil || len(versions) == 0 {
		t.Fatalf("versions = %+v, %v", versions, err)
	}

	yes := true
	res, err := av.c.Note().MoveFolder(ctx, &api.NoteMoveFolderParams{VaultID: av.id, Path: from, To: to, UpdateLinks: &yes})
	if err != nil {
		t.Fatal(err)
	}
	if res.Path != to || !slices.Equal(res.Moved, wantMoved) || len(res.Rewritten) == 0 {
		t.Fatalf("MoveFolder = %+v\nwant moved %v and some notes rewritten", res, wantMoved)
	}
	for p, links := range before {
		got := resolved(moved(p))
		var want []string
		for _, l := range links {
			line, dest, _ := strings.Cut(l, " ")
			want = append(want, line+" "+moved(dest))
		}
		slices.Sort(want)
		if !slices.Equal(got, want) {
			t.Errorf("%s's links after the move:\n%v\nwant %v", moved(p), got, want)
		}
	}
	after, err := av.c.Note().Versions(ctx, &api.NoteVersionsParams{VaultID: av.id, Path: to + "/Plan.md"})
	if err != nil || len(after) < len(versions) || after[len(after)-1].ID != versions[len(versions)-1].ID {
		t.Errorf("the moved note's versions = %+v, %v; want its own %+v", after, err, versions)
	}
	files, err := av.c.Note().Files(ctx, &api.NoteFilesParams{VaultID: av.id})
	if err != nil || slices.Contains(files.Folders, from) || !slices.Contains(files.Folders, to+"/Seed Beds") {
		t.Errorf("folders after the move = %v, %v", files.Folders, err)
	}

	for _, tc := range []struct {
		from, to string
		want     error
	}{
		{"Garden", "Orchard", api.ErrConflict},
		{"Garden", "Garden/Inner", api.ErrInvalidParams},
		{"Garden", "Garden", api.ErrInvalidParams},
		{"Nowhere", "Elsewhere", api.ErrNotFound},
		{".frostnotes", "Settings", api.ErrInvalidParams},
		{"Garden", "Archive/Garden", api.ErrInvalidParams}, // ignored
	} {
		_, err := av.c.Note().MoveFolder(ctx, &api.NoteMoveFolderParams{VaultID: av.id, Path: tc.from, To: tc.to})
		if !errors.Is(err, tc.want) {
			t.Errorf("MoveFolder(%s, %s) = %v, want %v", tc.from, tc.to, err, tc.want)
		}
	}
}

// A folder delete deletes what the caller listed and has not changed since,
// then the folders left empty.
func TestDeleteFolderKeepsWhatChanged(t *testing.T) {
	dir := t.TempDir()
	writeFiles(t, dir, map[string]string{"Old/a.md": "a\n", "Old/b.md": "b\n", "Old/Deep/c.png": "c", "Keep.md": "[[a]]\n"})
	if err := os.MkdirAll(filepath.Join(dir, "Old", "Empty"), 0o755); err != nil {
		t.Fatal(err)
	}
	av := openVault(t, dir)
	ctx := t.Context()
	list := func() []api.FileBase {
		files, err := av.c.Note().Files(ctx, &api.NoteFilesParams{VaultID: av.id})
		if err != nil {
			t.Fatal(err)
		}
		var out []api.FileBase
		for _, f := range files.Files {
			if strings.HasPrefix(f.Path, "Old/") {
				out = append(out, api.FileBase{Path: f.Path, Base: f.Hash})
			}
		}
		return out
	}
	listed := list()
	b, err := av.c.Note().Read(ctx, &api.NoteReadParams{VaultID: av.id, Path: "Old/b.md"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := av.c.Note().Write(ctx, &api.NoteWriteParams{VaultID: av.id, Path: "Old/b.md", Base: b.Hash, Content: "b, edited\n"}); err != nil {
		t.Fatal(err)
	}
	if _, err := av.c.Note().Create(ctx, &api.NoteCreateParams{VaultID: av.id, Path: "Old/new.md", Content: "new\n"}); err != nil {
		t.Fatal(err)
	}
	res, err := av.c.Note().DeleteFolder(ctx, &api.NoteDeleteFolderParams{VaultID: av.id, Path: "Old", Files: listed})
	if err != nil || fmt.Sprint(res.Deleted) != "[Old/Deep/c.png Old/a.md]" || fmt.Sprint(res.Kept) != "[Old/b.md Old/new.md]" || res.Removed {
		t.Fatalf("DeleteFolder = %+v, %v", res, err)
	}
	for p, want := range map[string]bool{"Old/Deep": false, "Old/Empty": false, "Old/b.md": true, "Old/new.md": true} {
		if _, err := os.Lstat(filepath.Join(dir, filepath.FromSlash(p))); (err == nil) != want {
			t.Errorf("%s exists: %v, want %v", p, err == nil, want)
		}
	}
	vs, err := av.c.Note().Versions(ctx, &api.NoteVersionsParams{VaultID: av.id, Path: "Old/a.md"})
	if err != nil || len(vs) != 2 || !vs[0].Deleted {
		t.Errorf("a deleted file's versions = %+v, %v", vs, err)
	}

	res, err = av.c.Note().DeleteFolder(ctx, &api.NoteDeleteFolderParams{VaultID: av.id, Path: "Old", Files: list()})
	if err != nil || len(res.Deleted) != 2 || len(res.Kept) != 0 || !res.Removed {
		t.Fatalf("DeleteFolder again = %+v, %v", res, err)
	}
	if _, err := os.Lstat(filepath.Join(dir, "Old")); !errors.Is(err, os.ErrNotExist) {
		t.Errorf("Old is still there: %v", err)
	}
	for _, tc := range []struct {
		folder string
		files  []api.FileBase
		want   error
	}{
		{"Old", nil, api.ErrNotFound},
		{"Keep", []api.FileBase{{Path: "Keep.md", Base: "x"}}, api.ErrInvalidParams},
		{".frostnotes", nil, api.ErrInvalidParams},
	} {
		_, err := av.c.Note().DeleteFolder(ctx, &api.NoteDeleteFolderParams{VaultID: av.id, Path: tc.folder, Files: tc.files})
		if !errors.Is(err, tc.want) {
			t.Errorf("DeleteFolder(%s) = %v, want %v", tc.folder, err, tc.want)
		}
	}
}

// vault.configure writes vault.yaml keeping what it does not change, and
// vault.configured follows it and a hand edit of the file.
func TestConfigureWritesVaultYAML(t *testing.T) {
	dir := t.TempDir()
	writeFiles(t, dir, map[string]string{
		".frostnotes/vault.yaml": "# my settings\ndaily:\n  folder: Journal # where days go\nextra: kept\n",
		".obsidian/app.json":     `{"alwaysUpdateLinks":true}`,
		"Plan.md":                "plan\n",
	})
	av := openVault(t, dir)
	ctx := t.Context()
	configured := func(env api.EventEnvelope) bool {
		var ev api.VaultConfigured
		return env.Event == "vault.configured" && json.Unmarshal(env.Data, &ev) == nil && ev.ID == av.id
	}
	format, location, inbox, attachments, wikilinks := "YYYY/MM/DD", api.NewNoteLocationFolder, "/Inbox/", " ./assets ", false
	s, err := av.c.Vault().Configure(ctx, &api.VaultConfigureParams{ID: av.id, DailyFormat: &format,
		NewNoteLocation: &location, NewNoteFolder: &inbox, AttachmentFolder: &attachments, Wikilinks: &wikilinks,
		Ignore: []string{"Archive/"}})
	if err != nil {
		t.Fatal(err)
	}
	if s.DailyFolder != "Journal" || s.DailyFormat != format || s.NewNoteLocation != location || s.NewNoteFolder != "Inbox" ||
		s.AttachmentFolder != "./assets" || s.Wikilinks || !s.UpdateLinks || fmt.Sprint(s.Ignore) != "[Archive/]" ||
		s.Sources["wikilinks"] != api.SettingSourceFrostnotes || s.Sources["updateLinks"] != api.SettingSourceObsidian {
		t.Errorf("Configure = %+v", s)
	}
	av.waitFor("vault.configured", configured)
	data, err := os.ReadFile(filepath.Join(dir, ".frostnotes", "vault.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"# my settings", "folder: Journal # where days go", "extra: kept", "format: YYYY/MM/DD",
		"location: folder\n  folder: Inbox", "folder: ./assets", "wikilinks: false", "ignore: [Archive/]"} {
		if !strings.Contains(string(data), want) {
			t.Errorf("vault.yaml lacks %q:\n%s", want, data)
		}
	}
	again, err := av.c.Vault().Settings(ctx, &api.VaultSettingsParams{ID: av.id})
	if err != nil || fmt.Sprintf("%+v", *again) != fmt.Sprintf("%+v", *s) {
		t.Errorf("Settings = %+v, %v; want what Configure returned", again, err)
	}

	bad := []*api.VaultConfigureParams{
		{ID: av.id, NewNoteFolder: new(".frostnotes")},
		{ID: av.id, DailyFolder: new("../up")},
		{ID: av.id, DailyFormat: new("../../YYYY")}, // Journal/../../2026.md
		{ID: av.id, DailyTemplate: new(".hidden/t")},
		{ID: av.id, AttachmentFolder: new("./../x")},
		{ID: av.id, Ignore: []string{"a//b"}},
	}
	for _, p := range bad {
		if _, err := av.c.Vault().Configure(ctx, p); !errors.Is(err, api.ErrInvalidParams) {
			t.Errorf("Configure(%+v) = %v, want invalidParams", p, err)
		}
	}
	if now, _ := os.ReadFile(filepath.Join(dir, ".frostnotes", "vault.yaml")); !bytes.Equal(now, data) {
		t.Errorf("a refused configure changed vault.yaml:\n%s", now)
	}

	// A hand edit is a change of settings too.
	writeFiles(t, dir, map[string]string{".frostnotes/vault.yaml": "daily:\n  folder: Days\n"})
	av.waitFor("vault.configured after a hand edit", configured)
	if s, err := av.c.Vault().Settings(ctx, &api.VaultSettingsParams{ID: av.id}); err != nil || s.DailyFolder != "Days" {
		t.Errorf("Settings after a hand edit = %+v, %v", s, err)
	}
	if _, err := av.c.Vault().Configure(ctx, &api.VaultConfigureParams{ID: 999}); !errors.Is(err, api.ErrNotFound) {
		t.Errorf("Configure of an unknown vault = %v, want notFound", err)
	}
}

// A vault with no vault.yaml gets one, in a new .frostnotes folder.
func TestConfigureMakesTheSettingsFolder(t *testing.T) {
	av := openVault(t, t.TempDir())
	update := true
	if _, err := av.c.Vault().Configure(t.Context(), &api.VaultConfigureParams{ID: av.id, UpdateLinks: &update}); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(filepath.Join(av.dir, ".frostnotes", "vault.yaml"))
	if err != nil || string(data) != "links:\n  update: true\n" {
		t.Errorf("vault.yaml = %q, %v", data, err)
	}
}
