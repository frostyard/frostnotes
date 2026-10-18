package store

import (
	"errors"
	"fmt"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/frostyard/frostnotes/api"
	"github.com/frostyard/frostnotes/internal/markdown"
)

// The store methods plan 0003's Phase 4 added for the engine.

func TestListNotesRootFolderIsTheWholeVault(t *testing.T) {
	ft := newFileTest(t)
	queryVault(t, ft, map[string]*markdown.Doc{"Inbox.md": {}, "Orchard/Plan.md": {}}, "Inbox.md", "Orchard/Plan.md")
	for _, folder := range []string{"/", "//", ""} {
		got, err := ft.d.ListNotes(t.Context(), ft.vault, NoteFilter{Folder: folder})
		if err != nil || fmt.Sprint(entryPaths(got)) != "[Inbox.md Orchard/Plan.md]" {
			t.Errorf("ListNotes(folder %q) = %v, %v; want every note", folder, entryPaths(got), err)
		}
	}
}

func TestListNotesCarriesAliases(t *testing.T) {
	ft := newFileTest(t)
	queryVault(t, ft, map[string]*markdown.Doc{
		"Plan.md":  {Aliases: []string{"Roadmap", "The plan"}},
		"Notes.md": {},
	}, "Notes.md", "Plan.md")
	got, err := ft.d.ListNotes(t.Context(), ft.vault, NoteFilter{})
	if err != nil {
		t.Fatal(err)
	}
	aliases := map[string][]string{}
	for _, e := range got {
		aliases[e.Path] = e.Aliases
	}
	if fmt.Sprint(aliases) != "map[Notes.md:[] Plan.md:[Roadmap The plan]]" || aliases["Notes.md"] == nil {
		t.Errorf("ListNotes aliases = %v; want Plan's two, Notes' none (not nil)", aliases)
	}
}

func TestRestoreFileKeepsItsID(t *testing.T) {
	ft := newFileTest(t)
	ctx := t.Context()
	var plan, linker File
	ft.tx(func(tx *Tx) {
		plan = ft.put(tx, "Plan.md")
		linker = ft.put(tx, "A.md")
		ft.link(tx, linker, 0, "Plan", "plan")
		if err := tx.DeleteFile(ctx, plan.ID); err != nil {
			t.Fatal(err)
		}
	})
	if ft.dest(linker, 0) != "" {
		t.Fatal("the link still resolves after the delete")
	}
	ft.tx(func(tx *Tx) {
		got, err := tx.RestoreFile(ctx, plan)
		if err != nil || got.ID != plan.ID || got.Path != "Plan.md" {
			t.Fatalf("RestoreFile = %+v, %v", got, err)
		}
		if _, err := tx.RestoreFile(ctx, File{ID: plan.ID, VaultID: ft.vault, Path: "Other.md", ModTime: time.Unix(1, 0)}); !errors.Is(err, ErrConflict) {
			t.Errorf("restoring an ID in use = %v, want ErrConflict", err)
		}
	})
	if ft.dest(linker, 0) != "Plan.md" {
		t.Error("a restored file did not resolve the links waiting for it")
	}
	if f, err := ft.d.FileByID(ctx, plan.ID); err != nil || f.Path != "Plan.md" {
		t.Errorf("FileByID = %+v, %v", f, err)
	}
	if _, err := ft.d.FileByID(ctx, 999); !errors.Is(err, ErrNotFound) {
		t.Errorf("FileByID(999) = %v, want ErrNotFound", err)
	}
	if paths, err := ft.d.FilesNamed(ctx, ft.vault, "plan.md"); err != nil || fmt.Sprint(paths) != "[Plan.md]" {
		t.Errorf("FilesNamed = %v, %v", paths, err)
	}
}

func TestHasVersionAndUsedHashes(t *testing.T) {
	ft := newFileTest(t)
	ctx := t.Context()
	var f File
	ft.tx(func(tx *Tx) {
		f = ft.put(tx, "Plan.md")
		for _, v := range []Version{
			{VaultID: ft.vault, FileID: f.ID, Path: f.Path, Hash: "old", Source: api.VersionSourceOutside},
			{VaultID: ft.vault, FileID: f.ID, Path: f.Path, Hash: "lost", Source: api.VersionSourceAgent, Conflict: true},
			{VaultID: ft.vault, FileID: 99, Path: "Gone.md", Source: api.VersionSourceEditor},
		} {
			if _, err := tx.AddVersion(ctx, v); err != nil {
				t.Fatal(err)
			}
		}
	})
	for hash, want := range map[string]bool{"old": true, "lost": true, "h": false, "": false} {
		if ok, err := ft.d.HasVersion(ctx, f.ID, hash); err != nil || ok != want {
			t.Errorf("HasVersion(%q) = %v, %v; want %v", hash, ok, err, want)
		}
	}
	used, err := ft.d.UsedHashes(ctx)
	if err != nil {
		t.Fatal(err)
	}
	var got []string
	for h := range used {
		got = append(got, h)
	}
	slices.Sort(got)
	if fmt.Sprint(got) != "[h lost old]" {
		t.Errorf("UsedHashes = %v, want the file's and the versions' hashes, no deletion's", got)
	}
}

func TestSnapshotOfAnIndexIgnoresIDs(t *testing.T) {
	snapshot := func(order []string) []string {
		ft := newFileTest(t)
		ft.tx(func(tx *Tx) {
			if _, err := tx.PutFile(t.Context(), File{VaultID: ft.vault, Path: "unrelated.png", ModTime: time.Unix(1, 0), Hash: "u"}); err != nil {
				t.Fatal(err)
			}
		})
		ft.tx(func(tx *Tx) {
			if err := tx.DeleteFile(t.Context(), 1); err != nil {
				t.Fatal(err)
			}
		})
		docs := map[string]*markdown.Doc{
			"A.md": {Links: []markdown.Link{{Kind: markdown.LinkWiki, Target: "B", Start: 0, End: 5, Line: 1}},
				Tags: []markdown.Tag{{Name: "x", Line: 1}}, Headings: []markdown.Heading{{Level: 1, Text: "A", Line: 1}}},
			"B.md": {Tasks: []markdown.Task{{Status: " ", Text: "do", Line: 2}}},
		}
		queryVault(t, ft, docs, order...)
		rows, err := ft.d.IndexSnapshot(t.Context(), ft.vault)
		if err != nil {
			t.Fatal(err)
		}
		return rows
	}
	a, b := snapshot([]string{"A.md", "B.md"}), snapshot([]string{"B.md", "A.md"})
	strip := func(rows []string) []string {
		// queryVault gives each file a size, time and hash by its position.
		var out []string
		for _, r := range rows {
			if !strings.HasPrefix(r, "file ") {
				out = append(out, r)
			}
		}
		return out
	}
	if !slices.Equal(strip(a), strip(b)) {
		t.Fatalf("snapshots of one index built in two orders differ:\n%s\n---\n%s", strings.Join(a, "\n"), strings.Join(b, "\n"))
	}
	if !slices.ContainsFunc(a, func(r string) bool {
		return strings.HasPrefix(r, "link A.md 0 wiki 'B'") && strings.HasSuffix(r, "-> 'B.md'")
	}) {
		t.Errorf("no resolved link row in\n%s", strings.Join(a, "\n"))
	}
}

// A conflict version can be a file's newest while an older version holds
// its content; pruning keeps that one, which a write's base must name.
func TestPruneKeepsTheCurrentContent(t *testing.T) {
	ft := newFileTest(t)
	ctx := t.Context()
	var f File
	ft.tx(func(tx *Tx) {
		f = ft.put(tx, "Plan.md") // hash "h"
		for _, v := range []Version{
			{VaultID: ft.vault, FileID: f.ID, Path: f.Path, Hash: "older", Source: api.VersionSourceOutside},
			{VaultID: ft.vault, FileID: f.ID, Path: f.Path, Hash: "h", Source: api.VersionSourceEditor},
			{VaultID: ft.vault, FileID: f.ID, Path: f.Path, Hash: "lost", Source: api.VersionSourceAgent, Conflict: true},
		} {
			if _, err := tx.AddVersion(ctx, v); err != nil {
				t.Fatal(err)
			}
		}
	})
	var freed []string
	ft.tx(func(tx *Tx) {
		var err error
		if freed, err = tx.PruneVersions(ctx, testNow.Add(time.Hour)); err != nil {
			t.Fatal(err)
		}
	})
	vs, err := ft.d.Versions(ctx, f.ID, 10)
	if err != nil {
		t.Fatal(err)
	}
	var kept []string
	for _, v := range vs {
		kept = append(kept, v.Hash)
	}
	if fmt.Sprint(kept) != "[lost h]" || fmt.Sprint(freed) != "[older]" {
		t.Errorf("after the prune: kept %v, freed %v; want the newest and the current content kept", kept, freed)
	}
}
