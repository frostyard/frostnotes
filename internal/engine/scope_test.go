package engine_test

import (
	"errors"
	"fmt"
	"slices"
	"strings"
	"testing"

	"github.com/frostyard/frostnotes/api"
)

// noteParams are params for every note method, naming vault id: enough
// to get past params decoding. A note method without an entry fails
// TestAgentScope*, so every new one is called both ways.
func noteParams(id int64) map[string]map[string]any {
	zero := strings.Repeat("0", 64)
	return map[string]map[string]any{
		"note.list":         {"vaultId": id},
		"note.read":         {"vaultId": id, "path": "Plan.md"},
		"note.search":       {"vaultId": id, "query": "plan"},
		"note.backlinks":    {"vaultId": id, "path": "Plan.md"},
		"note.links":        {"vaultId": id, "path": "Plan.md"},
		"note.create":       {"vaultId": id, "path": "New.md", "content": "new\n"},
		"note.write":        {"vaultId": id, "path": "Plan.md", "base": zero, "content": "plan\n"},
		"note.move":         {"vaultId": id, "path": "Plan.md", "to": "Moved.md"},
		"note.delete":       {"vaultId": id, "path": "Plan.md", "base": zero},
		"note.versions":     {"vaultId": id, "path": "Plan.md"},
		"note.revert":       {"vaultId": id, "path": "Plan.md", "version": 1, "base": zero},
		"note.daily":        {"vaultId": id},
		"note.appendDaily":  {"vaultId": id, "text": "x"},
		"note.files":        {"vaultId": id},
		"note.resolve":      {"vaultId": id, "from": "Plan.md", "targets": []string{"Plan"}},
		"note.render":       {"vaultId": id, "path": "Plan.md"},
		"note.upload":       {"vaultId": id, "path": "a.bin", "data": "AA=="},
		"note.readVersion":  {"vaultId": id, "version": 1},
		"note.tags":         {"vaultId": id},
		"note.properties":   {"vaultId": id},
		"note.template":     {"vaultId": id, "template": "Plan.md"},
		"note.mkdir":        {"vaultId": id, "path": "Folder"},
		"note.moveFolder":   {"vaultId": id, "path": "Folder", "to": "Other"},
		"note.deleteFolder": {"vaultId": id, "path": "Folder", "files": []any{}},
	}
}

// noteWrites are the note methods specs/lodge-app.md marks writes.
var noteWrites = []string{"note.create", "note.write", "note.move", "note.delete", "note.revert",
	"note.daily", "note.appendDaily", "note.upload", "note.mkdir", "note.moveFolder", "note.deleteFolder"}

func noteMethods(t *testing.T) []string {
	t.Helper()
	var out []string
	for _, m := range api.Methods {
		if strings.HasPrefix(m, "note.") {
			out = append(out, m)
		}
	}
	if len(out) == 0 {
		t.Fatal("no note methods")
	}
	return out
}

// twoVaults opens a vault with Plan.md and registers a second one.
func twoVaults(t *testing.T) (*apiVault, int64) {
	t.Helper()
	dir := t.TempDir()
	writeFiles(t, dir, map[string]string{"Plan.md": "plan\n"})
	av := openVault(t, dir)
	other, err := av.c.Vault().Add(t.Context(), &api.VaultAddParams{Path: t.TempDir()})
	if err != nil {
		t.Fatal(err)
	}
	return av, other.ID
}

func code(err error) api.ErrorCode {
	var ae *api.Error
	if errors.As(err, &ae) {
		return ae.Code
	}
	return 0
}

// An agent's call that names another vault answers as for a vault that
// does not exist, under a read scope and a write scope alike.
func TestAgentScopeHidesOtherVaults(t *testing.T) {
	av, other := twoVaults(t)
	params := noteParams(other)
	for _, write := range []bool{false, true} {
		agent := av.srv.DialAs(t, api.Identity{Caller: api.CallerAgent, Token: "claude",
			Scope: &api.Scope{Vault: av.id, Write: write}})
		for _, m := range noteMethods(t) {
			p, ok := params[m]
			if !ok {
				t.Fatalf("noteParams lacks %s", m)
			}
			err := agent.Call(t.Context(), m, p, nil)
			if slices.Contains(noteWrites, m) && !write {
				if code(err) != api.CodeMethodNotFound {
					t.Errorf("%s under a read scope = %v, want methodNotFound", m, err)
				}
				continue
			}
			if code(err) != api.CodeNotFound || err.Error() != fmt.Sprintf("notFound: vault %d does not exist", other) {
				t.Errorf("%s on another vault (write %t) = %v, want notFound as for no vault", m, write, err)
			}
		}
	}
}

// A read scope refuses exactly the methods marked writes, and admits the
// rest on its own vault.
func TestAgentScopeRefusesWritesUnderARead(t *testing.T) {
	av, _ := twoVaults(t)
	params := noteParams(av.id)
	agent := av.srv.DialAs(t, api.Identity{Caller: api.CallerAgent, Token: "claude", Scope: &api.Scope{Vault: av.id}})
	for _, m := range noteMethods(t) {
		err := agent.Call(t.Context(), m, params[m], nil)
		switch {
		case slices.Contains(noteWrites, m):
			if code(err) != api.CodeMethodNotFound || !strings.Contains(err.Error(), "does not exist") {
				t.Errorf("%s under a read scope = %v, want methodNotFound as for no method", m, err)
			}
		case code(err) == api.CodeMethodNotFound:
			t.Errorf("%s under a read scope = %v, want it admitted", m, err)
		case err != nil && strings.Contains(err.Error(), fmt.Sprintf("vault %d does not exist", av.id)):
			t.Errorf("%s on the scope's vault = %v", m, err)
		}
	}
	if _, err := agent.Note().Read(t.Context(), &api.NoteReadParams{VaultID: av.id, Path: "Plan.md"}); err != nil {
		t.Errorf("read under a read scope = %v", err)
	}
	for _, m := range []string{"vault.list", "hub.status", "sync.hubs", "events.subscribe", "log.hub"} {
		if err := agent.Call(t.Context(), m, map[string]any{}, nil); code(err) != api.CodeMethodNotFound {
			t.Errorf("%s as an agent = %v, want methodNotFound", m, err)
		}
	}
}

// An agent connection without a scope reaches no vault and writes
// nothing: the scope fails closed.
func TestAgentWithoutAScopeReachesNothing(t *testing.T) {
	av, _ := twoVaults(t)
	params := noteParams(av.id)
	agent := av.srv.DialAs(t, api.Identity{Caller: api.CallerAgent, Token: "claude"})
	for _, m := range noteMethods(t) {
		err := agent.Call(t.Context(), m, params[m], nil)
		if c := code(err); c != api.CodeNotFound && c != api.CodeMethodNotFound {
			t.Errorf("%s without a scope = %v, want notFound or methodNotFound", m, err)
		}
	}
}

// A session's and an agent's writes carry the connection's source and
// author, whatever the params say; a local connection keeps the params.
func TestConnectionsNameTheirWrites(t *testing.T) {
	av, _ := twoVaults(t)
	ctx := t.Context()
	agentSrc, editor := api.VersionSourceAgent, api.VersionSourceEditor
	mallory := "mallory"
	cases := []struct {
		name   string
		c      *api.Client
		source api.VersionSource
		author string
	}{
		{"session", av.srv.DialAs(t, api.Identity{Caller: api.CallerSession, Login: "ana@example.com", Node: "phone"}),
			api.VersionSourceEditor, "phone (web)"},
		{"agent", av.srv.DialAs(t, api.Identity{Caller: api.CallerAgent, Token: "claude", Scope: &api.Scope{Vault: av.id, Write: true}}),
			api.VersionSourceAgent, "claude"},
		{"local", av.c, api.VersionSourceAgent, "mallory"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			dir := tc.name + "/"
			latest := func(p string) api.Version {
				t.Helper()
				vs, err := tc.c.Note().Versions(ctx, &api.NoteVersionsParams{VaultID: av.id, Path: p})
				if err != nil || len(vs) == 0 {
					t.Fatalf("versions of %s = %+v, %v", p, vs, err)
				}
				return vs[0]
			}
			check := func(what, p string) {
				t.Helper()
				v := latest(p)
				want := tc.source
				if tc.name == "local" && (what == "move" || what == "delete" || what == "revert" || what == "moveFolder" || what == "deleteFolder") {
					// These take no source or author: a local writer is
					// source editor, named by its client.
					want = api.VersionSourceEditor
					if v.Source != want || v.Author != "rpctest" {
						t.Errorf("%s: version by %s/%q, want %s/rpctest", what, v.Source, v.Author, want)
					}
					return
				}
				if v.Source != want || v.Author != tc.author {
					t.Errorf("%s: version by %s/%q, want %s/%q", what, v.Source, v.Author, want, tc.author)
				}
			}
			// Params that name another writer, which only local keeps.
			src := &agentSrc
			if tc.name == "agent" {
				src = &editor
			}
			created, err := tc.c.Note().Create(ctx, &api.NoteCreateParams{VaultID: av.id, Path: dir + "A.md", Content: "a\n", Source: src, Author: &mallory})
			if err != nil {
				t.Fatal(err)
			}
			check("create", dir+"A.md")
			written, err := tc.c.Note().Write(ctx, &api.NoteWriteParams{VaultID: av.id, Path: dir + "A.md", Base: created.Hash, Content: "b\n", Source: src, Author: &mallory})
			if err != nil {
				t.Fatal(err)
			}
			check("write", dir+"A.md")
			if _, err := tc.c.Note().Upload(ctx, &api.NoteUploadParams{VaultID: av.id, Path: dir + "a.bin", Data: "AA==", Source: src, Author: &mallory}); err != nil {
				t.Fatal(err)
			}
			check("upload", dir+"a.bin")
			// A move records no version of the file it moves; the links it
			// rewrites are writes, by the mover.
			out := tc.name + "-links.md"
			if _, err := av.c.Note().Create(ctx, &api.NoteCreateParams{VaultID: av.id, Path: out,
				Content: "[[" + dir + "A]] ![[" + dir + "a.bin]]\n"}); err != nil {
				t.Fatal(err)
			}
			yes := true
			if _, err := tc.c.Note().Move(ctx, &api.NoteMoveParams{VaultID: av.id, Path: dir + "A.md", To: dir + "B.md", UpdateLinks: &yes}); err != nil {
				t.Fatal(err)
			}
			check("move", out)
			count := func(p string) int {
				t.Helper()
				vs, err := tc.c.Note().Versions(ctx, &api.NoteVersionsParams{VaultID: av.id, Path: p})
				if err != nil {
					t.Fatal(err)
				}
				return len(vs)
			}
			if n := count(out); n != 2 {
				t.Errorf("the move made %d versions of the linking note, want its rewrite", n-1)
			}
			vs, err := tc.c.Note().Versions(ctx, &api.NoteVersionsParams{VaultID: av.id, Path: dir + "B.md"})
			if err != nil || len(vs) < 2 {
				t.Fatalf("versions = %+v, %v", vs, err)
			}
			if _, err := tc.c.Note().Revert(ctx, &api.NoteRevertParams{VaultID: av.id, Path: dir + "B.md", Version: vs[len(vs)-1].ID, Base: written.Hash}); err != nil {
				t.Fatalf("revert: %v", err)
			}
			check("revert", dir+"B.md")
			now := latest(dir + "B.md")
			if err := tc.c.Note().Delete(ctx, &api.NoteDeleteParams{VaultID: av.id, Path: dir + "B.md", Base: now.Hash}); err != nil {
				t.Fatal(err)
			}
			check("delete", dir+"B.md")
			if _, err := tc.c.Note().MoveFolder(ctx, &api.NoteMoveFolderParams{VaultID: av.id, Path: strings.TrimSuffix(dir, "/"), To: tc.name + "-moved", UpdateLinks: &yes}); err != nil {
				t.Fatal(err)
			}
			check("moveFolder", out)
			if n := count(out); n != 3 {
				t.Errorf("after the folder's move the linking note has %d versions, want 3", n)
			}
			files, err := tc.c.Note().Files(ctx, &api.NoteFilesParams{VaultID: av.id})
			if err != nil {
				t.Fatal(err)
			}
			var bases []api.FileBase
			for _, f := range files.Files {
				if strings.HasPrefix(f.Path, tc.name+"-moved/") {
					bases = append(bases, api.FileBase{Path: f.Path, Base: f.Hash})
				}
			}
			if _, err := tc.c.Note().DeleteFolder(ctx, &api.NoteDeleteFolderParams{VaultID: av.id, Path: tc.name + "-moved", Files: bases}); err != nil {
				t.Fatal(err)
			}
			check("deleteFolder", tc.name+"-moved/a.bin")
		})
	}
	// The daily note, made once, by the first writer to ask.
	session := cases[0].c
	daily, err := session.Note().Daily(ctx, &api.NoteDailyParams{VaultID: av.id, Source: &agentSrc, Author: &mallory})
	if err != nil {
		t.Fatal(err)
	}
	vs, err := session.Note().Versions(ctx, &api.NoteVersionsParams{VaultID: av.id, Path: daily.Note.Path})
	if err != nil || len(vs) != 1 || vs[0].Source != api.VersionSourceEditor || vs[0].Author != "phone (web)" {
		t.Errorf("the daily note's versions = %+v, %v; want one by editor/phone (web)", vs, err)
	}
}
