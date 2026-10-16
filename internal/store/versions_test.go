package store

// CONTRACT TEST for task card T-0018 (docs/tasks). Do not edit.

import (
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/frostyard/frostnotes/api"
)

func (ft *fileTest) version(tx *Tx, f File, hash string, source api.VersionSource) Version {
	ft.t.Helper()
	v, err := tx.AddVersion(ft.t.Context(), Version{ID: 77, VaultID: ft.vault, FileID: f.ID, Path: f.Path, Hash: hash,
		Size: int64(len(hash)), At: time.Unix(1, 0), Source: source, Author: "notesctl test"})
	if err != nil {
		ft.t.Fatal(err)
	}
	return v
}

func versionIDs(vs []Version) []int64 {
	out := []int64{}
	for _, v := range vs {
		out = append(out, v.ID)
	}
	return out
}

func TestAddVersion(t *testing.T) {
	ft := newFileTest(t)
	ctx := t.Context()
	ft.d.Now = func() time.Time { return time.Date(2026, 10, 16, 12, 0, 0, 123_456_789, time.FixedZone("x", 3600)) }
	var a File
	var v1, v2, v3 Version
	ft.tx(func(tx *Tx) {
		a = ft.put(tx, "a.md")
		v1 = ft.version(tx, a, "h1", api.VersionSourceOutside)
		v2 = ft.version(tx, a, "h2", api.VersionSourceAgent)
		v3, _ = tx.AddVersion(ctx, Version{VaultID: ft.vault, FileID: a.ID, Path: "a.md", Hash: "h3", Size: 2,
			Source: api.VersionSourceEditor, Author: "app", Conflict: true})
	})
	at := time.Date(2026, 10, 16, 11, 0, 0, 123_000_000, time.UTC)
	if v1.ID == 0 || v1.ID == 77 || v2.ID <= v1.ID || !v1.At.Equal(at) || v1.At.Location() != time.UTC {
		t.Fatalf("AddVersion = %+v, %+v; want new IDs and At = the clock, in UTC to the millisecond", v1, v2)
	}
	got, err := ft.d.GetVersion(ctx, v2.ID)
	if err != nil || got != v2 {
		t.Fatalf("GetVersion = %+v, %v; want %+v", got, err, v2)
	}
	if _, err := ft.d.GetVersion(ctx, 999); !errors.Is(err, ErrNotFound) {
		t.Fatalf("GetVersion(999) = %v, want ErrNotFound", err)
	}
	all, err := ft.d.Versions(ctx, a.ID, 0)
	if err != nil || fmt.Sprint(versionIDs(all)) != fmt.Sprint([]int64{v3.ID, v2.ID, v1.ID}) || all[0] != v3 {
		t.Fatalf("Versions = %+v, %v; want newest first", all, err)
	}
	if two, _ := ft.d.Versions(ctx, a.ID, 2); fmt.Sprint(versionIDs(two)) != fmt.Sprint([]int64{v3.ID, v2.ID}) {
		t.Fatalf("Versions limit 2 = %v", versionIDs(two))
	}
	if none, err := ft.d.Versions(ctx, 999, 0); err != nil || none == nil || len(none) != 0 {
		t.Fatalf("Versions of an unknown file = %#v, %v; want an empty slice", none, err)
	}

	want := api.Version{ID: v3.ID, Path: "a.md", Hash: "h3", Size: 2, At: at, Source: api.VersionSourceEditor, Author: "app", Conflict: true}
	if got := v3.API(); got != want {
		t.Errorf("API() = %+v, want %+v", got, want)
	}
	deletion := Version{ID: 9, Path: "a.md", Source: api.VersionSourceOutside}
	if got := deletion.API(); !got.Deleted || got.Hash != "" {
		t.Errorf("a deletion's API() = %+v, want Deleted", got)
	}
}

func TestVersionsOutliveTheirFile(t *testing.T) {
	ft := newFileTest(t)
	ctx := t.Context()
	var a, b File
	var gone Version
	ft.tx(func(tx *Tx) {
		a = ft.put(tx, "Plan.md")
		ft.version(tx, a, "h1", api.VersionSourceOutside)
		moved, err := tx.MoveFile(ctx, a.ID, "Orchard/Plan.md")
		if err != nil {
			t.Fatal(err)
		}
		ft.version(tx, moved, "h2", api.VersionSourceEditor)
		if err := tx.DeleteFile(ctx, a.ID); err != nil {
			t.Fatal(err)
		}
		gone = ft.version(tx, moved, "", api.VersionSourceEditor)
	})
	if vs, err := ft.d.Versions(ctx, a.ID, 0); err != nil || len(vs) != 3 || vs[0].ID != gone.ID {
		t.Fatalf("a deleted file's versions = %+v, %v", vs, err)
	}
	for path, want := range map[string]int64{"Orchard/Plan.md": a.ID, "Plan.md": a.ID} {
		if id, err := ft.d.LastFileAt(ctx, ft.vault, path); err != nil || id != want {
			t.Errorf("LastFileAt(%s) = %d, %v; want %d", path, id, err, want)
		}
	}
	ft.tx(func(tx *Tx) {
		b = ft.put(tx, "Orchard/Plan.md")
		ft.version(tx, b, "h3", api.VersionSourceEditor)
	})
	if id, err := ft.d.LastFileAt(ctx, ft.vault, "Orchard/Plan.md"); err != nil || id != b.ID {
		t.Errorf("LastFileAt after a new file took the path = %d, %v; want %d", id, err, b.ID)
	}
	if _, err := ft.d.LastFileAt(ctx, ft.vault, "Never.md"); !errors.Is(err, ErrNotFound) {
		t.Errorf("LastFileAt(Never.md) = %v, want ErrNotFound", err)
	}
	if _, err := ft.d.LastFileAt(ctx, ft.vault+1, "Plan.md"); !errors.Is(err, ErrNotFound) {
		t.Errorf("LastFileAt in another vault = %v, want ErrNotFound", err)
	}
}

func TestPruneVersions(t *testing.T) {
	ft := newFileTest(t)
	ctx := t.Context()
	day := func(d int) { ft.d.Now = func() time.Time { return time.Date(2026, 9, d, 0, 0, 0, 0, time.UTC) } }
	var keep, live, gone, other File
	var liveNew, keepOld Version
	day(1)
	ft.tx(func(tx *Tx) {
		keep, live, gone = ft.put(tx, "keep.md"), ft.put(tx, "live.md"), ft.put(tx, "gone.md")
		keepOld = ft.version(tx, keep, "k1", api.VersionSourceOutside) // its newest: kept however old
		ft.version(tx, live, "l1", api.VersionSourceOutside)
		ft.version(tx, live, "shared", api.VersionSourceOutside)
		ft.version(tx, gone, "g1", api.VersionSourceOutside)
		ft.version(tx, gone, "current", api.VersionSourceOutside) // files.hash of live, below
	})
	day(20)
	ft.tx(func(tx *Tx) {
		liveNew = ft.version(tx, live, "l2", api.VersionSourceEditor)
		other = ft.put(tx, "other.md")
		ft.version(tx, other, "shared", api.VersionSourceEditor)
		if _, err := tx.ExecContext(ctx, `UPDATE files SET hash = 'current' WHERE id = ?`, live.ID); err != nil {
			t.Fatal(err)
		}
		if err := tx.DeleteFile(ctx, gone.ID); err != nil {
			t.Fatal(err)
		}
		ft.version(tx, gone, "", api.VersionSourceEditor)
	})
	var freed []string
	ft.tx(func(tx *Tx) {
		var err error
		freed, err = tx.PruneVersions(ctx, time.Date(2026, 9, 10, 0, 0, 0, 0, time.UTC))
		if err != nil {
			t.Fatal(err)
		}
	})
	if fmt.Sprint(freed) != "[g1 l1]" {
		t.Errorf("PruneVersions freed %q, want [g1 l1]: shared is still a version and current a file's hash", freed)
	}
	for f, want := range map[File]int{keep: 1, live: 1, gone: 1, other: 1} {
		if vs, _ := ft.d.Versions(ctx, f.ID, 0); len(vs) != want {
			t.Errorf("%s keeps %d versions, want %d", f.Path, len(vs), want)
		}
	}
	if vs, _ := ft.d.Versions(ctx, keep.ID, 0); len(vs) != 1 || vs[0].ID != keepOld.ID {
		t.Errorf("keep.md lost its old version: %+v", vs)
	}
	if vs, _ := ft.d.Versions(ctx, live.ID, 0); vs[0].ID != liveNew.ID {
		t.Errorf("live.md versions = %+v", vs)
	}
}
