package store

// CONTRACT TEST for task card T-0024 (docs/tasks). Do not edit.

import (
	"errors"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/frostyard/frostnotes/api"
)

var hex32 = regexp.MustCompile(`^[0-9a-f]{32}$`)

// hubTest is a database with one served vault, for the hub's log. Later
// cards' tests use it too.
type hubTest struct {
	t      *testing.T
	d      *DB
	events *[]api.EventEnvelope
	vault  HubVault
}

func newHubTest(t *testing.T) *hubTest {
	t.Helper()
	d, events := openTest(t)
	ht := &hubTest{t: t, d: d, events: events}
	ht.tx(func(tx *Tx) {
		v, err := tx.InsertVault(t.Context(), "Notes", "/v/notes")
		if err != nil {
			t.Fatal(err)
		}
		if ht.vault, err = tx.ServeVault(t.Context(), v.ID); err != nil {
			t.Fatal(err)
		}
	})
	*events = nil
	return ht
}

func (ht *hubTest) tx(fn func(tx *Tx)) {
	ht.t.Helper()
	if err := ht.d.Tx(ht.t.Context(), func(tx *Tx) error { fn(tx); return nil }); err != nil {
		ht.t.Fatal(err)
	}
}

// fakeHash is a fake content hash made from s.
func fakeHash(s string) string { return strings.Repeat(s, 64)[:64] }

// add appends a revision of file in the test vault and returns it.
func (ht *hubTest) add(file int64, op api.RevisionOp, path, content string) Revision {
	ht.t.Helper()
	r := Revision{VaultID: ht.vault.VaultID, FileID: file, Op: op, Path: path, Size: int64(len(content)),
		Source: api.VersionSourceEditor, Author: "notesctl"}
	if op != api.RevisionOpDelete {
		r.Hash = fakeHash(content)
	}
	var got Revision
	ht.tx(func(tx *Tx) {
		var err error
		if got, err = tx.AppendRevision(ht.t.Context(), r); err != nil {
			ht.t.Fatalf("AppendRevision(%+v): %v", r, err)
		}
	})
	return got
}

func TestServeVault(t *testing.T) {
	d, events := openTest(t)
	ctx := t.Context()
	if id, err := d.HubIdentity(ctx); err != nil || id != "" {
		t.Fatalf("HubIdentity before serving = %q, %v; want empty", id, err)
	}
	var work, notes Vault
	var served HubVault
	err := d.Tx(ctx, func(tx *Tx) error {
		var err error
		if work, err = tx.InsertVault(ctx, "work", "/v/work"); err != nil {
			return err
		}
		if notes, err = tx.InsertVault(ctx, "Notes", "/v/notes"); err != nil {
			return err
		}
		served, err = tx.ServeVault(ctx, work.ID)
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	if served.VaultID != work.ID || served.Name != "work" || served.Seq != 0 || !hex32.MatchString(served.SyncID) ||
		!served.ServedAt.Equal(testNow) {
		t.Fatalf("ServeVault = %+v", served)
	}
	hubID, err := d.HubIdentity(ctx)
	if err != nil || !hex32.MatchString(hubID) {
		t.Fatalf("HubIdentity after serving = %q, %v; want 32 hex digits", hubID, err)
	}
	var again string
	if err := d.Tx(ctx, func(tx *Tx) error { var err error; again, err = tx.HubID(ctx); return err }); err != nil || again != hubID {
		t.Fatalf("HubID = %q, %v; want the same ID, %q", again, err, hubID)
	}
	err = d.Tx(ctx, func(tx *Tx) error { _, err := tx.ServeVault(ctx, work.ID); return err })
	if !errors.Is(err, ErrConflict) {
		t.Fatalf("serving twice = %v, want ErrConflict", err)
	}
	err = d.Tx(ctx, func(tx *Tx) error { _, err := tx.ServeVault(ctx, 999); return err })
	if !errors.Is(err, ErrNotFound) {
		t.Fatalf("serving vault 999 = %v, want ErrNotFound", err)
	}
	var second HubVault
	if err := d.Tx(ctx, func(tx *Tx) error { var err error; second, err = tx.ServeVault(ctx, notes.ID); return err }); err != nil {
		t.Fatal(err)
	}
	if second.SyncID == served.SyncID {
		t.Fatal("two vaults got one sync ID")
	}
	all, err := d.HubVaults(ctx)
	if err != nil || len(all) != 2 || all[0] != second || all[1] != served {
		t.Fatalf("HubVaults = %+v, %v; want Notes, then work", all, err)
	}
	if got, err := d.HubVault(ctx, work.ID); err != nil || got != served {
		t.Fatalf("HubVault = %+v, %v", got, err)
	}
	if got, err := d.HubVaultBySync(ctx, served.SyncID); err != nil || got != served {
		t.Fatalf("HubVaultBySync = %+v, %v", got, err)
	}
	if _, err := d.HubVaultBySync(ctx, strings.Repeat("0", 32)); !errors.Is(err, ErrNotFound) {
		t.Fatalf("HubVaultBySync(unknown) = %v, want ErrNotFound", err)
	}
	var unserved Vault
	err = d.Tx(ctx, func(tx *Tx) error {
		var err error
		if unserved, err = tx.InsertVault(ctx, "solo", "/v/solo"); err != nil {
			return err
		}
		_, err = tx.HubVault(ctx, unserved.ID)
		return err
	})
	if !errors.Is(err, ErrNotFound) {
		t.Fatalf("Tx.HubVault of an unserved vault = %v, want ErrNotFound", err)
	}
	if _, err := d.HubVault(ctx, work.ID+999); !errors.Is(err, ErrNotFound) {
		t.Fatalf("HubVault(unknown) = %v, want ErrNotFound", err)
	}
	if got := served.API(); got != (api.HubVault{VaultID: work.ID, Sync: served.SyncID, Name: "work", Seq: 0, ServedAt: testNow}) {
		t.Fatalf("API() = %+v", got)
	}
	if got := served.Log(); got != (api.LogVault{Sync: served.SyncID, Name: "work", Seq: 0}) {
		t.Fatalf("Log() = %+v", got)
	}
	for _, ev := range *events {
		if ev.Event == "log.appended" {
			t.Fatalf("serving an empty vault appended to its log: %+v", ev)
		}
	}
}

func TestAppendRevision(t *testing.T) {
	ht := newHubTest(t)
	ctx := t.Context()
	ht.d.Now = func() time.Time { return time.Date(2026, 10, 16, 12, 0, 0, 987_654_321, time.FixedZone("x", 7200)) }
	at := time.Date(2026, 10, 16, 10, 0, 0, 987_000_000, time.UTC)

	c := ht.add(7, api.RevisionOpCreate, "a.md", "one")
	want := Revision{VaultID: ht.vault.VaultID, Seq: 1, FileID: 7, Op: api.RevisionOpCreate, Path: "a.md", Hash: fakeHash("one"),
		Size: 3, Source: api.VersionSourceEditor, Author: "notesctl", At: at}
	if c != want {
		t.Fatalf("create = %+v\nwant %+v", c, want)
	}
	if f, err := ht.d.HubFile(ctx, ht.vault.VaultID, 7); err != nil || f != (HubFile{VaultID: ht.vault.VaultID, FileID: 7, Seq: 1, Revision: 1}) {
		t.Fatalf("HubFile after the create = %+v, %v", f, err)
	}

	var p Revision
	ht.tx(func(tx *Tx) {
		var err error
		p, err = tx.AppendRevision(ctx, Revision{VaultID: ht.vault.VaultID, Seq: 99, FileID: 7, Op: api.RevisionOpPut, Path: "a.md",
			Hash: fakeHash("two"), Size: 3, Base: 1, DeviceID: 0, Source: api.VersionSourceAgent, Author: "claude-code", At: time.Unix(5, 0)})
		if err != nil {
			t.Fatal(err)
		}
		// The transaction sees what it appended.
		if f, err := tx.HubFile(ctx, ht.vault.VaultID, 7); err != nil || f.Seq != 2 || f.Revision != 2 {
			t.Fatalf("Tx.HubFile = %+v, %v", f, err)
		}
		if r, err := tx.Revision(ctx, ht.vault.VaultID, 2); err != nil || r != p {
			t.Fatalf("Tx.Revision = %+v, %v", r, err)
		}
		if v, err := tx.HubVault(ctx, ht.vault.VaultID); err != nil || v.Seq != 2 {
			t.Fatalf("Tx.HubVault = %+v, %v; want seq 2", v, err)
		}
	})
	if p.Seq != 2 || !p.At.Equal(at) || p.Base != 1 || p.Author != "claude-code" {
		t.Fatalf("put = %+v; want seq 2, the clock's time and the fields given", p)
	}
	ht.add(7, api.RevisionOpConflict, "a.md", "lost")
	if f, _ := ht.d.HubFile(ctx, ht.vault.VaultID, 7); f.Seq != 3 || f.Revision != 2 || !f.Conflicted {
		t.Fatalf("HubFile after a conflict = %+v; want seq 3, head 2, flagged", f)
	}
	ht.add(7, api.RevisionOpMove, "b/a.md", "two")
	ht.add(7, api.RevisionOpResolve, "b/a.md", "two")
	if f, _ := ht.d.HubFile(ctx, ht.vault.VaultID, 7); f.Seq != 5 || f.Revision != 4 || f.Conflicted {
		t.Fatalf("HubFile after a move and a resolve = %+v; want seq 5, head 4, not flagged", f)
	}
	ht.add(7, api.RevisionOpDelete, "b/a.md", "")
	if f, _ := ht.d.HubFile(ctx, ht.vault.VaultID, 7); f.Seq != 6 || f.Revision != 6 {
		t.Fatalf("HubFile after a delete = %+v", f)
	}
	if r, err := ht.d.Revision(ctx, ht.vault.VaultID, 3); err != nil || r.Op != api.RevisionOpConflict || r.Hash != fakeHash("lost") {
		t.Fatalf("Revision(3) = %+v, %v", r, err)
	}
	if _, err := ht.d.Revision(ctx, ht.vault.VaultID, 7); !errors.Is(err, ErrNotFound) {
		t.Fatalf("Revision(7) = %v, want ErrNotFound", err)
	}
	if _, err := ht.d.HubFile(ctx, ht.vault.VaultID, 8); !errors.Is(err, ErrNotFound) {
		t.Fatalf("HubFile(8) = %v, want ErrNotFound", err)
	}
	if v, _ := ht.d.HubVault(ctx, ht.vault.VaultID); v.Seq != 6 {
		t.Fatalf("vault seq = %d, want 6", v.Seq)
	}
	var seqs []int64
	for _, ev := range *ht.events {
		if ev.Event != "log.appended" {
			continue
		}
		e, err := api.DecodeEvent(ev.Event, ev.Data)
		if err != nil {
			t.Fatal(err)
		}
		a := e.(api.LogAppended)
		if a.Vault != ht.vault.SyncID {
			t.Fatalf("log.appended names vault %q, want the sync ID %q", a.Vault, ht.vault.SyncID)
		}
		seqs = append(seqs, a.Seq)
	}
	if len(seqs) != 6 || seqs[0] != 1 || seqs[5] != 6 {
		t.Fatalf("log.appended seqs = %v, want 1 to 6", seqs)
	}
}

func TestAppendRevisionRefusesWhatTheLogCannotHold(t *testing.T) {
	ht := newHubTest(t)
	ctx := t.Context()
	ht.add(1, api.RevisionOpCreate, "a.md", "one")
	v := ht.vault.VaultID
	bad := map[string]Revision{
		"a create of a known file":   {VaultID: v, FileID: 1, Op: api.RevisionOpCreate, Path: "a.md", Hash: fakeHash("x")},
		"a put of an unknown file":   {VaultID: v, FileID: 2, Op: api.RevisionOpPut, Path: "b.md", Hash: fakeHash("x")},
		"a delete with a hash":       {VaultID: v, FileID: 1, Op: api.RevisionOpDelete, Path: "a.md", Hash: fakeHash("x")},
		"a put without a hash":       {VaultID: v, FileID: 1, Op: api.RevisionOpPut, Path: "a.md"},
		"a conflict without a hash":  {VaultID: v, FileID: 1, Op: api.RevisionOpConflict, Path: "a.md"},
		"a create without a hash":    {VaultID: v, FileID: 3, Op: api.RevisionOpCreate, Path: "c.md"},
		"a resolve without a hash":   {VaultID: v, FileID: 1, Op: api.RevisionOpResolve, Path: "a.md"},
		"an unknown op":              {VaultID: v, FileID: 1, Op: "rename", Path: "a.md", Hash: fakeHash("x")},
		"an empty path":              {VaultID: v, FileID: 1, Op: api.RevisionOpPut, Hash: fakeHash("x")},
		"a revision of no file at 0": {VaultID: v, FileID: 0, Op: api.RevisionOpCreate, Path: "z.md", Hash: fakeHash("x")},
	}
	for name, r := range bad {
		err := ht.d.Tx(ctx, func(tx *Tx) error { _, err := tx.AppendRevision(ctx, r); return err })
		if err == nil {
			t.Errorf("%s: appended", name)
		}
	}
	err := ht.d.Tx(ctx, func(tx *Tx) error {
		_, err := tx.AppendRevision(ctx, Revision{VaultID: 999, FileID: 1, Op: api.RevisionOpCreate, Path: "a.md", Hash: fakeHash("x")})
		return err
	})
	if !errors.Is(err, ErrNotFound) {
		t.Errorf("a revision of an unserved vault = %v, want ErrNotFound", err)
	}
	if hv, _ := ht.d.HubVault(ctx, v); hv.Seq != 1 {
		t.Fatalf("seq = %d after refused revisions, want 1", hv.Seq)
	}
}

func TestPull(t *testing.T) {
	ht := newHubTest(t)
	ctx := t.Context()
	v := ht.vault.VaultID
	ht.add(1, api.RevisionOpCreate, "a.md", "a1")      // seq 1
	ht.add(2, api.RevisionOpCreate, "b.md", "b1")      // seq 2
	ht.add(3, api.RevisionOpCreate, "c.png", "c1")     // seq 3
	ht.add(1, api.RevisionOpPut, "a.md", "a2")         // seq 4
	ht.add(2, api.RevisionOpDelete, "b.md", "")        // seq 5
	ht.add(3, api.RevisionOpConflict, "c.png", "lost") // seq 6

	page, err := ht.d.Pull(ctx, v, 0, 0)
	if err != nil {
		t.Fatal(err)
	}
	want := []api.LogFile{
		{File: 1, Seq: 4, Revision: 4, Path: "a.md", Hash: fakeHash("a2"), Size: 2, Author: "notesctl", DeviceName: "hub"},
		{File: 2, Seq: 5, Revision: 5, Path: "b.md", Deleted: true, Author: "notesctl", DeviceName: "hub"},
		{File: 3, Seq: 6, Revision: 3, Path: "c.png", Hash: fakeHash("c1"), Size: 2, Conflicted: true, Author: "notesctl", DeviceName: "hub"},
	}
	if len(page.Files) != 3 || page.Next != 6 || page.More {
		t.Fatalf("Pull(0) = %+v; want three files, next 6, no more", page)
	}
	for i := range want {
		if page.Files[i] != want[i] {
			t.Errorf("file %d = %+v\nwant %+v", i, page.Files[i], want[i])
		}
	}
	first, err := ht.d.Pull(ctx, v, 0, 2)
	if err != nil || len(first.Files) != 2 || first.Next != 5 || !first.More {
		t.Fatalf("Pull(0, 2) = %+v, %v; want two files, next 5, more", first, err)
	}
	rest, err := ht.d.Pull(ctx, v, first.Next, 2)
	if err != nil || len(rest.Files) != 1 || rest.Files[0].File != 3 || rest.Next != 6 || rest.More {
		t.Fatalf("Pull(5, 2) = %+v, %v", rest, err)
	}
	empty, err := ht.d.Pull(ctx, v, 6, 0)
	if err != nil || empty.Files == nil || len(empty.Files) != 0 || empty.Next != 6 || empty.More {
		t.Fatalf("Pull(6) = %+v, %v; want an empty page that stays at 6", empty, err)
	}
	if f, err := ht.d.LogFile(ctx, v, 2); err != nil || f != want[1] {
		t.Fatalf("LogFile(2) = %+v, %v", f, err)
	}
	if _, err := ht.d.LogFile(ctx, v, 9); !errors.Is(err, ErrNotFound) {
		t.Fatalf("LogFile(9) = %v, want ErrNotFound", err)
	}
}

func TestPullCapsItsPages(t *testing.T) {
	ht := newHubTest(t)
	ht.tx(func(tx *Tx) {
		for i := range int64(1005) {
			r := Revision{VaultID: ht.vault.VaultID, FileID: i + 1, Op: api.RevisionOpCreate, Path: "n.md", Hash: fakeHash("x"), Size: 1,
				Source: api.VersionSourceOutside}
			if _, err := tx.AppendRevision(t.Context(), r); err != nil {
				t.Fatal(err)
			}
		}
	})
	for limit, want := range map[int]int{0: 500, -1: 500, 2000: 1000, 7: 7} {
		page, err := ht.d.Pull(t.Context(), ht.vault.VaultID, 0, limit)
		if err != nil || len(page.Files) != want || !page.More {
			t.Errorf("Pull with limit %d gave %d files, %v; want %d and more", limit, len(page.Files), err, want)
		}
	}
}

func TestHasContent(t *testing.T) {
	ht := newHubTest(t)
	ctx := t.Context()
	ht.add(1, api.RevisionOpCreate, "a.md", "kept")
	ht.add(2, api.RevisionOpCreate, "b.md", "gone")
	if _, err := ht.d.db.ExecContext(ctx, `UPDATE revisions SET pruned = 1 WHERE hash = ?`, fakeHash("gone")); err != nil {
		t.Fatal(err)
	}
	ht.add(3, api.RevisionOpCreate, "c.md", "c1")
	ht.add(3, api.RevisionOpPut, "c.md", "c2")
	ht.add(3, api.RevisionOpResolve, "c.md", "c1") // a resolve names a hash, but holds no content of its own
	if _, err := ht.d.db.ExecContext(ctx, `UPDATE revisions SET pruned = 1 WHERE hash = ? AND op = 'create'`, fakeHash("c1")); err != nil {
		t.Fatal(err)
	}
	for content, want := range map[string]bool{"kept": true, "gone": false, "never": false, "c1": false, "c2": true} {
		if got, err := ht.d.HasContent(ctx, fakeHash(content)); err != nil || got != want {
			t.Errorf("HasContent(%s) = %v, %v; want %v", content, got, err, want)
		}
	}
	if got, _ := ht.d.HasContent(ctx, ""); got {
		t.Error("HasContent(\"\") = true; a delete names no content")
	}
}
