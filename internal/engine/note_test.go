package engine_test

import (
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
	"github.com/frostyard/frostnotes/internal/rpctest"
	"github.com/frostyard/frostnotes/internal/vault"
	"github.com/frostyard/frostnotes/internal/vaultgen"
)

// apiVault is a vault registered with a test notesd, and a client
// subscribed to its events.
type apiVault struct {
	t   *testing.T
	srv *rpctest.Server
	c   *api.Client
	id  int64
	dir string
}

// openVault registers dir and waits for its first scan to end.
func openVault(t *testing.T, dir string) *apiVault {
	t.Helper()
	srv := rpctest.StartWith(t, rpctest.Options{Vault: vault.Config{Debounce: 20 * time.Millisecond}})
	c := srv.Dial(t)
	if _, err := c.Events().Subscribe(t.Context(), &api.EventsSubscribeParams{}); err != nil {
		t.Fatal(err)
	}
	v, err := c.Vault().Add(t.Context(), &api.VaultAddParams{Path: dir})
	if err != nil {
		t.Fatal(err)
	}
	av := &apiVault{t: t, srv: srv, c: c, id: v.ID, dir: dir}
	av.waitScan()
	return av
}

func writeFiles(t *testing.T, dir string, files map[string]string) {
	t.Helper()
	for p, content := range files {
		full := filepath.Join(dir, filepath.FromSlash(p))
		if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(full, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
}

// waitFor reads events until one matches, failing after a minute: a
// scan under the race detector on a slow runner takes seconds.
func (av *apiVault) waitFor(what string, match func(api.EventEnvelope) bool) {
	av.t.Helper()
	timeout := time.After(time.Minute)
	for {
		select {
		case env := <-av.c.Notifications():
			if match(env) {
				return
			}
		case <-timeout:
			av.t.Fatalf("no %s within a minute", what)
		}
	}
}

// waitScan waits for the vault's running scan to end.
func (av *apiVault) waitScan() {
	av.t.Helper()
	av.waitFor("finished scan", func(env api.EventEnvelope) bool {
		var ev api.VaultScan
		if env.Event != "vault.scan" || json.Unmarshal(env.Data, &ev) != nil || ev.Status.VaultID != av.id {
			return false
		}
		if ev.Status.State == api.ScanStateFailed {
			av.t.Fatalf("scan failed: %v", *ev.Status.Error)
		}
		return ev.Status.State == api.ScanStateIdle
	})
}

func TestNoteAPI(t *testing.T) {
	dir := t.TempDir()
	writeFiles(t, dir, map[string]string{
		"Inbox.md":          "---\ntags: [work]\n---\nplant the [[Orchard/Quince]]\n",
		"Orchard/Quince.md": "# Quince\nNeeds pruning.\n",
		"Orchard/map.png":   "\x89PNG",
	})
	av := openVault(t, dir)
	ctx := t.Context()
	note := av.c.Note()

	for _, folder := range []string{"/", "", "."} {
		list, err := note.List(ctx, &api.NoteListParams{VaultID: av.id, Folder: &folder})
		if err != nil || len(list) != 2 {
			t.Errorf("List(folder %q) = %d notes, %v; want the whole vault's 2", folder, len(list), err)
		}
	}
	bad := "../up"
	if _, err := note.List(ctx, &api.NoteListParams{VaultID: av.id, Folder: &bad}); !errors.Is(err, api.ErrInvalidParams) {
		t.Errorf("List(folder ../up) = %v, want invalidParams", err)
	}
	if _, err := note.List(ctx, &api.NoteListParams{VaultID: 999}); !errors.Is(err, api.ErrNotFound) {
		t.Errorf("List of an unknown vault = %v, want notFound", err)
	}

	n, err := note.Read(ctx, &api.NoteReadParams{VaultID: av.id, Path: "Inbox.md"})
	if err != nil || fmt.Sprint(n.Tags) != "[work]" || n.Title != "Inbox" || string(n.Props) != `{"tags":["work"]}` {
		t.Fatalf("Read = %+v, %v", n, err)
	}
	links, err := note.Links(ctx, &api.NoteLinksParams{VaultID: av.id, Path: "Inbox.md"})
	if err != nil || len(links) != 1 || links[0].Resolved == nil || *links[0].Resolved != "Orchard/Quince.md" {
		t.Fatalf("Links = %+v, %v", links, err)
	}
	back, err := note.Backlinks(ctx, &api.NoteBacklinksParams{VaultID: av.id, Path: "Orchard/Quince.md"})
	if err != nil || len(back) != 1 || back[0].Source != "Inbox.md" {
		t.Fatalf("Backlinks = %+v, %v", back, err)
	}

	// A write's author defaults to the client name given to rpc.hello.
	res, err := note.Write(ctx, &api.NoteWriteParams{VaultID: av.id, Path: "Inbox.md", Base: n.Hash,
		Content: n.Content + "and the medlar\n"})
	if err != nil {
		t.Fatal(err)
	}
	agent := api.VersionSourceAgent
	name := "claude-code"
	if _, err := note.Write(ctx, &api.NoteWriteParams{VaultID: av.id, Path: "Inbox.md", Base: res.Hash,
		Content: "replaced\n", Source: &agent, Author: &name}); err != nil {
		t.Fatal(err)
	}
	versions, err := note.Versions(ctx, &api.NoteVersionsParams{VaultID: av.id, Path: "Inbox.md"})
	if err != nil || len(versions) != 3 {
		t.Fatalf("Versions = %+v, %v", versions, err)
	}
	if v := versions[0]; v.Source != api.VersionSourceAgent || v.Author != "claude-code" {
		t.Errorf("the agent's version = %+v", v)
	}
	if v := versions[1]; v.Source != api.VersionSourceEditor || v.Author != "rpctest" {
		t.Errorf("the client's version = %+v, want author rpctest", v)
	}
	outside := api.VersionSourceOutside
	if _, err := note.Write(ctx, &api.NoteWriteParams{VaultID: av.id, Path: "Inbox.md", Base: res.Hash,
		Content: "x", Source: &outside}); !errors.Is(err, api.ErrInvalidParams) {
		t.Errorf("a write claiming to be outside = %v, want invalidParams", err)
	}

	hits, err := note.Search(ctx, &api.NoteSearchParams{VaultID: av.id, Query: "prun"})
	if err != nil || len(hits) != 1 || hits[0].Path != "Orchard/Quince.md" {
		t.Errorf("Search(prun) = %+v, %v", hits, err)
	}
	s, err := av.c.Vault().Status(ctx, &api.VaultStatusParams{ID: av.id})
	if err != nil || s.Notes != 2 || s.Attachments != 1 || s.State != api.ScanStateIdle {
		t.Errorf("Status = %+v, %v", s, err)
	}
	if _, err := av.c.Vault().Reindex(ctx, &api.VaultReindexParams{ID: av.id}); err != nil {
		t.Fatal(err)
	}
	av.waitScan()

	// A deleted file's versions stay reachable by its path.
	cur, _ := note.Read(ctx, &api.NoteReadParams{VaultID: av.id, Path: "Inbox.md"})
	if err := note.Delete(ctx, &api.NoteDeleteParams{VaultID: av.id, Path: "Inbox.md", Base: cur.Hash}); err != nil {
		t.Fatal(err)
	}
	versions, err = note.Versions(ctx, &api.NoteVersionsParams{VaultID: av.id, Path: "Inbox.md"})
	if err != nil || len(versions) != 4 || !versions[0].Deleted || versions[0].Author != "rpctest" {
		t.Fatalf("Versions after a delete = %+v, %v", versions, err)
	}
	if _, err := note.Revert(ctx, &api.NoteRevertParams{VaultID: av.id, Path: "Inbox.md", Version: versions[2].ID}); err != nil {
		t.Fatal(err)
	}
	if n, err := note.Read(ctx, &api.NoteReadParams{VaultID: av.id, Path: "Inbox.md"}); err != nil || !strings.HasSuffix(n.Content, "and the medlar\n") {
		t.Fatalf("Read after the revert = %+v, %v", n, err)
	}
}

// An edit made with another program reaches the index through the watcher.
func TestWatcherIndexesOutsideEdits(t *testing.T) {
	dir := t.TempDir()
	writeFiles(t, dir, map[string]string{"Plan.md": "first\n", "Old/Note.md": "moving\n"})
	av := openVault(t, dir)
	ctx := t.Context()
	changed := func(p string) func(api.EventEnvelope) bool {
		return func(env api.EventEnvelope) bool {
			var ev api.NoteChanged
			return env.Event == "note.changed" && json.Unmarshal(env.Data, &ev) == nil && ev.Path == p &&
				ev.Source == api.VersionSourceOutside
		}
	}
	// As vim saves: a temporary file renamed over the note.
	tmp := filepath.Join(dir, ".Plan.md.swp")
	if err := os.WriteFile(tmp, []byte("first\nsecond, about medlars\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Rename(tmp, filepath.Join(dir, "Plan.md")); err != nil {
		t.Fatal(err)
	}
	av.waitFor("note.changed for Plan.md", changed("Plan.md"))
	hits, err := av.c.Note().Search(ctx, &api.NoteSearchParams{VaultID: av.id, Query: "medlar"})
	if err != nil || len(hits) != 1 {
		t.Fatalf("Search after the edit = %+v, %v", hits, err)
	}

	writeFiles(t, dir, map[string]string{"New/Idea.md": "an idea\n"})
	av.waitFor("note.changed for New/Idea.md", changed("New/Idea.md"))

	if err := os.Rename(filepath.Join(dir, "Old/Note.md"), filepath.Join(dir, "Old/Renamed.md")); err != nil {
		t.Fatal(err)
	}
	av.waitFor("note.moved", func(env api.EventEnvelope) bool {
		var ev api.NoteMoved
		return env.Event == "note.moved" && json.Unmarshal(env.Data, &ev) == nil && ev.To == "Old/Renamed.md"
	})
	if err := os.Remove(filepath.Join(dir, "Plan.md")); err != nil {
		t.Fatal(err)
	}
	av.waitFor("note.removed", func(env api.EventEnvelope) bool { return env.Event == "note.removed" })
	list, err := av.c.Note().List(ctx, &api.NoteListParams{VaultID: av.id})
	var paths []string
	for _, e := range list {
		paths = append(paths, e.Path)
	}
	if err != nil || fmt.Sprint(paths) != "[New/Idea.md Old/Renamed.md]" {
		t.Errorf("List = %v, %v", paths, err)
	}
}

// A generated vault's index holds exactly its manifest's links, each
// resolved to the manifest's file, and none of its decoys in code.
func TestGeneratedVaultIndexesItsManifest(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "vault")
	const notes = 150
	m, err := vaultgen.Generate(dir, vaultgen.Options{Seed: 11, Notes: notes})
	if err != nil {
		t.Fatal(err)
	}
	av := openVault(t, dir)
	ctx := t.Context()
	var want, got []string
	line := map[string]func(int) int{}
	for _, l := range m.Links {
		if line[l.Source] == nil {
			data, err := os.ReadFile(filepath.Join(dir, filepath.FromSlash(l.Source)))
			if err != nil {
				t.Fatal(err)
			}
			line[l.Source] = func(off int) int { return 1 + strings.Count(string(data[:off]), "\n") }
		}
		want = append(want, fmt.Sprintf("%s:%d %s %s -> %s", l.Source, line[l.Source](l.Start), l.Kind, l.Target, l.Want))
	}
	decoys := map[string]bool{}
	for _, d := range m.Decoys {
		decoys[d.Target] = true
	}
	for _, p := range m.Notes {
		links, err := av.c.Note().Links(ctx, &api.NoteLinksParams{VaultID: av.id, Path: p})
		if err != nil {
			t.Fatalf("Links(%s): %v", p, err)
		}
		for _, l := range links {
			if decoys[l.Target] {
				t.Errorf("%s:%d: the decoy %q, inside code, is indexed as a link", p, l.Line, l.Target)
			}
			dest := ""
			if l.Resolved != nil {
				dest = *l.Resolved
			}
			got = append(got, fmt.Sprintf("%s:%d %s %s -> %s", p, l.Line, l.Kind, l.Target, dest))
		}
	}
	slices.Sort(want)
	slices.Sort(got)
	if !slices.Equal(got, want) {
		for _, w := range want {
			if !slices.Contains(got, w) {
				t.Errorf("missing or different: %s", w)
			}
		}
		for _, g := range got {
			if !slices.Contains(want, g) {
				t.Errorf("not in the manifest: %s", g)
			}
		}
	}
	if len(want) < notes {
		t.Errorf("only %d links in the manifest", len(want))
	}
}

// A daily note an agent's call creates is the agent's version.
func TestDailyRecordsItsCreator(t *testing.T) {
	av := openVault(t, t.TempDir())
	agent, name := api.VersionSourceAgent, "claude-code"
	date := "2026-10-16"
	d, err := av.c.Note().Daily(t.Context(), &api.NoteDailyParams{VaultID: av.id, Date: &date, Source: &agent, Author: &name})
	if err != nil || !d.Created || d.Note.Path != "2026-10-16.md" {
		t.Fatalf("Daily = %+v, %v", d, err)
	}
	vs, err := av.c.Note().Versions(t.Context(), &api.NoteVersionsParams{VaultID: av.id, Path: d.Note.Path})
	if err != nil || len(vs) != 1 || vs[0].Source != agent || vs[0].Author != name {
		t.Fatalf("versions = %+v, %v; want one by %s", vs, err, name)
	}
	outside := api.VersionSourceOutside
	if _, err := av.c.Note().Daily(t.Context(), &api.NoteDailyParams{VaultID: av.id, Source: &outside}); !errors.Is(err, api.ErrInvalidParams) {
		t.Errorf("a daily note claimed by another program = %v, want invalidParams", err)
	}
}
