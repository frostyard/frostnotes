package main

// CONTRACT TEST for task card T-0019 (docs/tasks). Do not edit.

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/frostyard/frostnotes/api"
	"github.com/frostyard/frostnotes/internal/rpctest"
)

// fakeNotes serves the note domain from its fields and records the params
// of every call, so notesctl's tests need no engine.
type fakeNotes struct {
	mu    sync.Mutex
	calls []any
	err   error // every method returns it when set

	entries   []api.NoteEntry
	note      api.Note
	hits      []api.SearchHit
	backlinks []api.Backlink
	write     api.WriteResult
	move      api.MoveResult
	versions  []api.Version
}

func (f *fakeNotes) record(p any) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.calls = append(f.calls, p)
	return f.err
}

// only returns the one call made since the last reset, and resets.
func (f *fakeNotes) only(t *testing.T) any {
	t.Helper()
	f.mu.Lock()
	defer f.mu.Unlock()
	if len(f.calls) != 1 {
		t.Fatalf("notesctl made %d note calls, want 1: %+v", len(f.calls), f.calls)
	}
	c := f.calls[0]
	f.calls = nil
	return c
}

func (f *fakeNotes) List(_ context.Context, p *api.NoteListParams) ([]api.NoteEntry, error) {
	return f.entries, f.record(p)
}
func (f *fakeNotes) Read(_ context.Context, p *api.NoteReadParams) (*api.Note, error) {
	return &f.note, f.record(p)
}
func (f *fakeNotes) Search(_ context.Context, p *api.NoteSearchParams) ([]api.SearchHit, error) {
	return f.hits, f.record(p)
}
func (f *fakeNotes) Backlinks(_ context.Context, p *api.NoteBacklinksParams) ([]api.Backlink, error) {
	return f.backlinks, f.record(p)
}
func (f *fakeNotes) Links(_ context.Context, p *api.NoteLinksParams) ([]api.Link, error) {
	return nil, f.record(p)
}
func (f *fakeNotes) Create(_ context.Context, p *api.NoteCreateParams) (*api.WriteResult, error) {
	return &f.write, f.record(p)
}
func (f *fakeNotes) Write(_ context.Context, p *api.NoteWriteParams) (*api.WriteResult, error) {
	return &f.write, f.record(p)
}
func (f *fakeNotes) Move(_ context.Context, p *api.NoteMoveParams) (*api.MoveResult, error) {
	return &f.move, f.record(p)
}
func (f *fakeNotes) Delete(_ context.Context, p *api.NoteDeleteParams) error { return f.record(p) }
func (f *fakeNotes) Versions(_ context.Context, p *api.NoteVersionsParams) ([]api.Version, error) {
	return f.versions, f.record(p)
}
func (f *fakeNotes) Revert(_ context.Context, p *api.NoteRevertParams) (*api.WriteResult, error) {
	return &f.write, f.record(p)
}
func (f *fakeNotes) Daily(_ context.Context, p *api.NoteDailyParams) (*api.DailyNote, error) {
	return nil, f.record(p)
}

func (f *fakeNotes) Files(_ context.Context, p *api.NoteFilesParams) (*api.VaultFiles, error) {
	return nil, f.record(p)
}

func (f *fakeNotes) Resolve(_ context.Context, p *api.NoteResolveParams) ([]api.Resolution, error) {
	return nil, f.record(p)
}

func (f *fakeNotes) Render(_ context.Context, p *api.NoteRenderParams) (*api.RenderedNote, error) {
	return nil, f.record(p)
}

func (f *fakeNotes) Upload(_ context.Context, p *api.NoteUploadParams) (*api.WriteResult, error) {
	return nil, f.record(p)
}

func (f *fakeNotes) ReadVersion(_ context.Context, p *api.NoteReadVersionParams) (*api.VersionContent, error) {
	return nil, f.record(p)
}

func (f *fakeNotes) Tags(_ context.Context, p *api.NoteTagsParams) ([]api.TagCount, error) {
	return nil, f.record(p)
}

func (f *fakeNotes) Properties(_ context.Context, p *api.NotePropertiesParams) ([]api.PropertyCount, error) {
	return nil, f.record(p)
}

func (f *fakeNotes) Template(_ context.Context, p *api.NoteTemplateParams) (*api.ExpandedTemplate, error) {
	return nil, f.record(p)
}

func (f *fakeNotes) MoveFolder(_ context.Context, p *api.NoteMoveFolderParams) (*api.FolderMoveResult, error) {
	return nil, f.record(p)
}

func (f *fakeNotes) DeleteFolder(_ context.Context, p *api.NoteDeleteFolderParams) (*api.FolderDeleteResult, error) {
	return nil, f.record(p)
}

func (f *fakeNotes) Mkdir(_ context.Context, p *api.NoteMkdirParams) error {
	return f.record(p)
}

// notesServer runs notesd with fake as its note domain and registers one
// vault per name, in order, so their IDs are 1, 2, ….
func notesServer(t *testing.T, fake *fakeNotes, vaults ...string) *rpctest.Server {
	t.Helper()
	srv := rpctest.StartWith(t, rpctest.Options{Note: fake})
	for _, name := range vaults {
		dir := filepath.Join(t.TempDir(), name)
		if err := os.Mkdir(dir, 0o755); err != nil {
			t.Fatal(err)
		}
		if _, err := runNotesctl(t, "--socket", srv.Socket, "vault", "add", dir); err != nil {
			t.Fatal(err)
		}
	}
	return srv
}

// notes runs notesctl against srv and fails the test if it fails.
func notes(t *testing.T, srv *rpctest.Server, args ...string) string {
	t.Helper()
	out, err := runNotesctl(t, append([]string{"--socket", srv.Socket}, args...)...)
	if err != nil {
		t.Fatalf("notesctl %q: %v", args, err)
	}
	return out
}

// notesErr runs notesctl against srv and returns its error.
func notesErr(t *testing.T, srv *rpctest.Server, args ...string) error {
	t.Helper()
	_, err := runNotesctl(t, append([]string{"--socket", srv.Socket}, args...)...)
	if err == nil {
		t.Fatalf("notesctl %q succeeded, want an error", args)
	}
	return err
}

func ptr[T any](v T) *T { return &v }

func checkCall(t *testing.T, got, want any) {
	t.Helper()
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("params = %s, want %s", show(got), show(want))
	}
}

func show(v any) string {
	b, _ := json.Marshal(v)
	return fmt.Sprintf("%T%s", v, b)
}

func TestNoteVaultChoice(t *testing.T) {
	fake := &fakeNotes{}
	one := notesServer(t, fake, "garden")
	notes(t, one, "ls")
	checkCall(t, fake.only(t), &api.NoteListParams{VaultID: 1})

	two := notesServer(t, fake, "garden", "work")
	for _, tc := range []struct {
		vault string
		want  int64
	}{{"work", 2}, {"WORK", 2}, {"1", 1}} {
		notes(t, two, "--vault", tc.vault, "ls")
		checkCall(t, fake.only(t), &api.NoteListParams{VaultID: tc.want})
	}
	for _, tc := range []struct {
		srv  *rpctest.Server
		args []string
		want string
	}{
		{two, []string{"ls"}, "--vault"},
		{two, []string{"--vault", "nope", "ls"}, "nope"},
		{two, []string{"--vault", "9", "ls"}, "9"},
		{notesServer(t, fake), []string{"ls"}, "notesctl vault add"},
	} {
		if err := notesErr(t, tc.srv, tc.args...); !strings.Contains(err.Error(), tc.want) {
			t.Errorf("notesctl %q: %v, want an error mentioning %q", tc.args, err, tc.want)
		}
	}
	if len(fake.calls) != 0 {
		t.Errorf("a failed vault choice still called the note domain: %+v", fake.calls)
	}
}

func TestNoteList(t *testing.T) {
	fake := &fakeNotes{entries: []api.NoteEntry{
		{Path: "Inbox.md", Title: "Inbox", Hash: "h1", Size: 3, ModifiedAt: time.Date(2026, 10, 16, 12, 0, 0, 0, time.UTC),
			Aliases: []string{}},
		{Path: "Orchard/Plan.md", Title: "Plan", Hash: "h2", Size: 9, ModifiedAt: time.Date(2026, 10, 15, 8, 0, 0, 0, time.UTC),
			Aliases: []string{"Roadmap"}},
	}}
	srv := notesServer(t, fake, "garden")
	if out := notes(t, srv, "ls"); out != "Inbox.md\nOrchard/Plan.md\n" {
		t.Errorf("ls = %q", out)
	}
	checkCall(t, fake.only(t), &api.NoteListParams{VaultID: 1})
	notes(t, srv, "ls", "Orchard", "--tag", "area/work", "--prop", "status=In progress")
	checkCall(t, fake.only(t), &api.NoteListParams{VaultID: 1, Folder: ptr("Orchard"), Tag: ptr("area/work"),
		Property: ptr("status"), Value: ptr("In progress")})
	notes(t, srv, "ls", "--prop", "status")
	checkCall(t, fake.only(t), &api.NoteListParams{VaultID: 1, Property: ptr("status")})

	var got []api.NoteEntry
	if err := json.Unmarshal([]byte(notes(t, srv, "--json", "ls")), &got); err != nil || !reflect.DeepEqual(got, fake.entries) {
		t.Errorf("ls --json = %+v, %v", got, err)
	}
	fake.only(t)
	fake.entries = nil
	if out := notes(t, srv, "ls"); out != "" {
		t.Errorf("ls of nothing = %q, want no output", out)
	}
}

func TestNoteCat(t *testing.T) {
	fake := &fakeNotes{note: api.Note{Path: "Plan.md", Title: "Plan", Hash: "h", Content: "---\na: 1\n---\r\nno final newline",
		Props: []byte(`{"a":1}`), Aliases: []string{}, Tags: []string{"x"}}}
	srv := notesServer(t, fake, "garden")
	if out := notes(t, srv, "cat", "Plan.md"); out != fake.note.Content {
		t.Errorf("cat = %q, want the content byte for byte", out)
	}
	checkCall(t, fake.only(t), &api.NoteReadParams{VaultID: 1, Path: "Plan.md"})
	var got struct {
		Path, Hash, Content string
		Tags                []string
	}
	if err := json.Unmarshal([]byte(notes(t, srv, "--json", "cat", "Plan.md")), &got); err != nil || got.Hash != "h" || got.Content != fake.note.Content {
		t.Errorf("cat --json = %+v, %v", got, err)
	}
	fake.only(t)
	fake.err = api.NotFound("note Missing.md does not exist")
	if err := notesErr(t, srv, "cat", "Missing.md"); !strings.Contains(err.Error(), "note Missing.md does not exist") {
		t.Errorf("cat of a missing note: %v", err)
	}
	if err := notesErr(t, srv, "cat"); !strings.Contains(err.Error(), "arg") {
		t.Errorf("cat without a path: %v", err)
	}
}

func TestNoteSearch(t *testing.T) {
	fake := &fakeNotes{hits: []api.SearchHit{
		{Path: "Garden/Beds.md", Title: "Beds", Snippet: []api.SnippetPart{{Text: "We "}, {Text: "plan", Match: true}, {Text: " the\nbeds…"}}},
		{Path: "Planting.md", Title: "Planting", Snippet: []api.SnippetPart{{Text: "Planting", Match: true}}},
	}}
	srv := notesServer(t, fake, "garden")
	out := notes(t, srv, "search", "plan", "the beds", "--limit", "5")
	if want := "Garden/Beds.md\tWe **plan** the beds…\nPlanting.md\t**Planting**\n"; out != want {
		t.Errorf("search =\n%q\nwant\n%q", out, want)
	}
	checkCall(t, fake.only(t), &api.NoteSearchParams{VaultID: 1, Query: "plan the beds", Limit: ptr[int64](5)})
	notes(t, srv, "search", "x")
	checkCall(t, fake.only(t), &api.NoteSearchParams{VaultID: 1, Query: "x"})
	var got []api.SearchHit
	if err := json.Unmarshal([]byte(notes(t, srv, "--json", "search", "plan")), &got); err != nil || !reflect.DeepEqual(got, fake.hits) {
		t.Errorf("search --json = %+v, %v", got, err)
	}
	fake.only(t)
	if err := notesErr(t, srv, "search"); !strings.Contains(err.Error(), "arg") {
		t.Errorf("search without a query: %v", err)
	}
}

func TestNoteBacklinks(t *testing.T) {
	link := func(kind api.LinkKind, target, subpath, alias, key string, embed bool, line int64) api.Link {
		return api.Link{Kind: kind, Target: target, Subpath: subpath, Alias: alias, Key: key, Embed: embed, Line: line, Resolved: ptr("Plan.md")}
	}
	fake := &fakeNotes{backlinks: []api.Backlink{
		{Source: "A.md", Link: link(api.LinkKindWiki, "Plan", "#Goals", "the plan", "", false, 3)},
		{Source: "A.md", Link: link(api.LinkKindWiki, "Plan.md", "", "", "", true, 4)},
		{Source: "B.md", Link: link(api.LinkKindMarkdown, "Plan.md", "#Goals", "the plan", "", false, 1)},
		{Source: "B.md", Link: link(api.LinkKindMarkdown, "Plan.md", "", "", "", true, 2)},
		{Source: "C.md", Link: link(api.LinkKindProperty, "Plan", "", "", "up", false, 2)},
		{Source: "Plan.md", Link: link(api.LinkKindWiki, "", "#Top", "", "", false, 9)},
	}}
	// notesd sends each link's line; the CLI's JSON passes it through.
	for i := range fake.backlinks {
		fake.backlinks[i].Context = []api.SnippetPart{}
	}
	fake.backlinks[0].Context = []api.SnippetPart{{Text: "see "}, {Text: "[[Plan#Goals|the plan]]", Match: true}}
	srv := notesServer(t, fake, "garden")
	want := "A.md:3\t[[Plan#Goals|the plan]]\n" +
		"A.md:4\t![[Plan.md]]\n" +
		"B.md:1\t[the plan](Plan.md#Goals)\n" +
		"B.md:2\t![](Plan.md)\n" +
		"C.md:2\tup: [[Plan]]\n" +
		"Plan.md:9\t[[#Top]]\n"
	if out := notes(t, srv, "backlinks", "Plan.md"); out != want {
		t.Errorf("backlinks =\n%s\nwant\n%s", out, want)
	}
	checkCall(t, fake.only(t), &api.NoteBacklinksParams{VaultID: 1, Path: "Plan.md"})
	var got []api.Backlink
	if err := json.Unmarshal([]byte(notes(t, srv, "--json", "backlinks", "Plan.md")), &got); err != nil || !reflect.DeepEqual(got, fake.backlinks) {
		t.Errorf("backlinks --json = %+v, %v", got, err)
	}
}
