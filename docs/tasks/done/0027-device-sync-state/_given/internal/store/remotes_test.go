package store

// CONTRACT TEST for task card T-0027 (docs/tasks). Do not edit.

import (
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/frostyard/frostnotes/api"
)

// deviceTest is a database with one vault synced with a hub.
type deviceTest struct {
	t     *testing.T
	d     *DB
	vault int64
	hub   SyncHub
}

func newDeviceTest(t *testing.T) *deviceTest {
	t.Helper()
	d, _ := openTest(t)
	dt := &deviceTest{t: t, d: d}
	dt.tx(func(tx *Tx) {
		v, err := tx.InsertVault(t.Context(), "Notes", "/v/notes")
		if err != nil {
			t.Fatal(err)
		}
		dt.vault = v.ID
		if dt.hub, err = tx.PutSyncHub(t.Context(), SyncHub{HubID: "h1", URL: "https://hub.example", DeviceID: 4}); err != nil {
			t.Fatal(err)
		}
		if _, err := tx.AddRemote(t.Context(), Remote{VaultID: v.ID, Hub: dt.hub.ID, SyncID: "s1", Name: "Notes", ReplicaID: 9}); err != nil {
			t.Fatal(err)
		}
	})
	return dt
}

func (dt *deviceTest) tx(fn func(tx *Tx)) {
	dt.t.Helper()
	if err := dt.d.Tx(dt.t.Context(), func(tx *Tx) error { fn(tx); return nil }); err != nil {
		dt.t.Fatal(err)
	}
}

func (dt *deviceTest) file(tx *Tx, path, hash string) File {
	dt.t.Helper()
	f, err := tx.PutFile(dt.t.Context(), File{VaultID: dt.vault, Path: path, Size: 1, ModTime: time.Unix(1, 0), Hash: hash})
	if err != nil {
		dt.t.Fatal(err)
	}
	return f
}

func (dt *deviceTest) agree(tx *Tx, a Agreed) {
	dt.t.Helper()
	a.VaultID = dt.vault
	if err := tx.PutAgreed(dt.t.Context(), a); err != nil {
		dt.t.Fatal(err)
	}
}

func TestSyncHubs(t *testing.T) {
	d, _ := openTest(t)
	ctx := t.Context()
	var first, again, other SyncHub
	err := d.Tx(ctx, func(tx *Tx) error {
		var err error
		if first, err = tx.PutSyncHub(ctx, SyncHub{ID: 50, HubID: "h1", URL: "https://hub.example", DeviceID: 4, PairedAt: time.Unix(1, 0)}); err != nil {
			return err
		}
		if other, err = tx.PutSyncHub(ctx, SyncHub{HubID: "h2", URL: "https://other.example", DeviceID: 1}); err != nil {
			return err
		}
		again, err = tx.PutSyncHub(ctx, SyncHub{HubID: "h1", URL: "https://hub.example:8443", DeviceID: 5})
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	if first.ID == 0 || first.ID == 50 || !first.PairedAt.Equal(testNow) || first.DeviceID != 4 {
		t.Fatalf("PutSyncHub = %+v", first)
	}
	if again.ID != first.ID || again.URL != "https://hub.example:8443" || again.DeviceID != 5 {
		t.Fatalf("pairing again = %+v; want hub row %d with the new URL and device", again, first.ID)
	}
	all, err := d.SyncHubs(ctx)
	if err != nil || len(all) != 2 || all[0] != again || all[1] != other {
		t.Fatalf("SyncHubs = %+v, %v", all, err)
	}
	if got, err := d.SyncHub(ctx, other.ID); err != nil || got != other {
		t.Fatalf("SyncHub = %+v, %v", got, err)
	}
	if got, err := d.SyncHubByURL(ctx, "https://hub.example:8443"); err != nil || got != again {
		t.Fatalf("SyncHubByURL = %+v, %v", got, err)
	}
	for name, err := range map[string]error{
		"SyncHubByURL(old URL)": func() error { _, err := d.SyncHubByURL(ctx, "https://hub.example"); return err }(),
		"SyncHub(999)":          func() error { _, err := d.SyncHub(ctx, 999); return err }(),
	} {
		if !errors.Is(err, ErrNotFound) {
			t.Errorf("%s = %v, want ErrNotFound", name, err)
		}
	}
	if got := other.API(); got != (api.SyncHub{ID: other.ID, Hub: "h2", URL: "https://other.example", Device: 1, PairedAt: testNow}) {
		t.Fatalf("API = %+v", got)
	}
	if none, err := func() ([]SyncHub, error) { d2, _ := openTest(t); return d2.SyncHubs(ctx) }(); err != nil || none == nil || len(none) != 0 {
		t.Fatalf("SyncHubs with none = %#v, %v", none, err)
	}
}

func TestRemotes(t *testing.T) {
	dt := newDeviceTest(t)
	ctx := t.Context()
	r, err := dt.d.Remote(ctx, dt.vault)
	if err != nil {
		t.Fatal(err)
	}
	want := Remote{VaultID: dt.vault, Hub: dt.hub.ID, SyncID: "s1", Name: "Notes", ReplicaID: 9, JoinedAt: testNow}
	if r != want {
		t.Fatalf("Remote = %+v\nwant %+v", r, want)
	}
	add := func(r Remote) error {
		return dt.d.Tx(ctx, func(tx *Tx) error { _, err := tx.AddRemote(ctx, r); return err })
	}
	if err := add(Remote{VaultID: dt.vault, Hub: dt.hub.ID, SyncID: "s2", Name: "x", ReplicaID: 1}); !errors.Is(err, ErrConflict) {
		t.Fatalf("a second remote for the vault = %v, want ErrConflict", err)
	}
	for name, r := range map[string]Remote{
		"an unknown vault": {VaultID: 999, Hub: dt.hub.ID, SyncID: "s", Name: "x", ReplicaID: 1},
		"an unknown hub":   {VaultID: dt.vault + 1, Hub: 999, SyncID: "s", Name: "x", ReplicaID: 1},
	} {
		if err := add(r); !errors.Is(err, ErrNotFound) {
			t.Errorf("a remote for %s = %v, want ErrNotFound", name, err)
		}
	}
	dt.tx(func(tx *Tx) {
		if err := tx.SetCursor(ctx, dt.vault, 12, 20); err != nil {
			t.Fatal(err)
		}
		if err := tx.SetCursor(ctx, dt.vault, 15, 18); err != nil { // an older hub seq does not win
			t.Fatal(err)
		}
		if err := tx.MarkSynced(ctx, dt.vault); err != nil {
			t.Fatal(err)
		}
	})
	r, _ = dt.d.Remote(ctx, dt.vault)
	if r.Cursor != 15 || r.HubSeq != 20 || !r.SyncedAt.Equal(testNow) {
		t.Fatalf("after SetCursor and MarkSynced: %+v; want cursor 15, hub seq 20, synced now", r)
	}
	err = dt.d.Tx(ctx, func(tx *Tx) error { return tx.SetCursor(ctx, 999, 1, 1) })
	if !errors.Is(err, ErrNotFound) {
		t.Fatalf("SetCursor without a remote = %v, want ErrNotFound", err)
	}
	if all, err := dt.d.Remotes(ctx); err != nil || len(all) != 1 || all[0] != r {
		t.Fatalf("Remotes = %+v, %v", all, err)
	}
	dt.tx(func(tx *Tx) {
		dt.agree(tx, Agreed{HubFile: 1, FileID: 1, Seq: 1, Revision: 1, Path: "a.md", Hash: "h"})
		if _, err := tx.AddPush(ctx, LocalPush{VaultID: dt.vault, FileID: 1, Path: "a.md", Hash: "h"}); err != nil {
			t.Fatal(err)
		}
		if err := tx.DeleteRemote(ctx, dt.vault); err != nil {
			t.Fatal(err)
		}
	})
	if _, err := dt.d.Remote(ctx, dt.vault); !errors.Is(err, ErrNotFound) {
		t.Fatalf("Remote after DeleteRemote = %v, want ErrNotFound", err)
	}
	if rows, _ := dt.d.AgreedRows(ctx, dt.vault); len(rows) != 0 {
		t.Fatalf("agreed rows outlived their remote: %+v", rows)
	}
	if pushes, _ := dt.d.Unanswered(ctx, dt.vault); len(pushes) != 0 {
		t.Fatalf("pushes outlived their remote: %+v", pushes)
	}
	if err := dt.d.Tx(ctx, func(tx *Tx) error { return tx.DeleteRemote(ctx, dt.vault) }); !errors.Is(err, ErrNotFound) {
		t.Fatalf("deleting a missing remote = %v, want ErrNotFound", err)
	}
}

func TestAgreed(t *testing.T) {
	dt := newDeviceTest(t)
	ctx := t.Context()
	a := Agreed{VaultID: dt.vault, HubFile: 7, FileID: 3, Seq: 5, Revision: 4, Path: "a.md", Hash: "h1", Conflicted: true}
	dt.tx(func(tx *Tx) {
		dt.agree(tx, a)
		dt.agree(tx, Agreed{HubFile: 8, FileID: 2, Seq: 1, Revision: 1, Path: "0.md", Hash: "h0"})
	})
	if got, err := dt.d.AgreedByHubFile(ctx, dt.vault, 7); err != nil || got != a {
		t.Fatalf("AgreedByHubFile = %+v, %v; want %+v", got, err, a)
	}
	if got, err := dt.d.AgreedByFile(ctx, 3); err != nil || got != a {
		t.Fatalf("AgreedByFile = %+v, %v", got, err)
	}
	a.Seq, a.Revision, a.Path, a.Hash, a.Conflicted = 9, 9, "b.md", "h2", false
	dt.tx(func(tx *Tx) { dt.agree(tx, a) })
	if got, _ := dt.d.AgreedByHubFile(ctx, dt.vault, 7); got != a {
		t.Fatalf("after replacing: %+v, want %+v", got, a)
	}
	err := dt.d.Tx(ctx, func(tx *Tx) error {
		return tx.PutAgreed(ctx, Agreed{VaultID: dt.vault, HubFile: 9, FileID: 3, Seq: 1, Revision: 1, Path: "c.md", Hash: "h"})
	})
	if !errors.Is(err, ErrConflict) {
		t.Fatalf("agreeing one local file with two hub files = %v, want ErrConflict", err)
	}
	rows, err := dt.d.AgreedRows(ctx, dt.vault)
	if err != nil || len(rows) != 2 || rows[0].Path != "0.md" || rows[1] != a {
		t.Fatalf("AgreedRows = %+v, %v; want by path", rows, err)
	}
	dt.tx(func(tx *Tx) {
		for range 2 { // a missing row is not an error
			if err := tx.DeleteAgreed(ctx, dt.vault, 7); err != nil {
				t.Fatal(err)
			}
		}
	})
	for name, err := range map[string]error{
		"AgreedByHubFile": func() error { _, err := dt.d.AgreedByHubFile(ctx, dt.vault, 7); return err }(),
		"AgreedByFile":    func() error { _, err := dt.d.AgreedByFile(ctx, 3); return err }(),
	} {
		if !errors.Is(err, ErrNotFound) {
			t.Errorf("%s after DeleteAgreed = %v, want ErrNotFound", name, err)
		}
	}
}

func TestPending(t *testing.T) {
	dt := newDeviceTest(t)
	ctx := t.Context()
	if p, err := dt.d.Pending(ctx, dt.vault); err != nil || p == nil || len(p) != 0 {
		t.Fatalf("Pending with nothing = %#v, %v; want a non-nil empty slice", p, err)
	}
	var same, edited, moved, created, refused, retried File
	dt.tx(func(tx *Tx) {
		same = dt.file(tx, "same.md", "h-same")
		edited = dt.file(tx, "edited.md", "h-new")
		moved = dt.file(tx, "z/moved.md", "h-moved")
		created = dt.file(tx, "created.md", "h-created")
		refused = dt.file(tx, "Refused.md", "h-refused")
		retried = dt.file(tx, "retried.md", "h-retried-2")
		dt.agree(tx, Agreed{HubFile: 1, FileID: same.ID, Seq: 1, Revision: 1, Path: "same.md", Hash: "h-same"})
		dt.agree(tx, Agreed{HubFile: 2, FileID: edited.ID, Seq: 2, Revision: 2, Path: "edited.md", Hash: "h-old"})
		dt.agree(tx, Agreed{HubFile: 3, FileID: moved.ID, Seq: 3, Revision: 3, Path: "moved.md", Hash: "h-moved"})
		dt.agree(tx, Agreed{HubFile: 4, FileID: 999, Seq: 4, Revision: 4, Path: "gone.md", Hash: "h-gone"})
		// Refused once in this state: not sent again until the file changes.
		p, err := tx.AddPush(ctx, LocalPush{VaultID: dt.vault, FileID: refused.ID, Path: "Refused.md", Hash: "h-refused"})
		if err != nil {
			t.Fatal(err)
		}
		if err := tx.AnswerPush(ctx, dt.vault, p.PushID, api.PushStatusRefused, api.RefuseReasonCollision); err != nil {
			t.Fatal(err)
		}
		// Refused in an earlier state: the file changed since, so it goes again.
		p, _ = tx.AddPush(ctx, LocalPush{VaultID: dt.vault, FileID: retried.ID, Path: "retried.md", Hash: "h-retried-1"})
		_ = tx.AnswerPush(ctx, dt.vault, p.PushID, api.PushStatusRefused, api.RefuseReasonMissing)
	})
	got, err := dt.d.Pending(ctx, dt.vault)
	if err != nil {
		t.Fatal(err)
	}
	type row struct {
		file       int64
		agreed     int64
		path, hash string
		deleted    bool
	}
	var rows []row
	for _, c := range got {
		r := row{file: c.FileID, path: c.Path, hash: c.Hash, deleted: c.Deleted}
		if c.Agreed != nil {
			r.agreed = c.Agreed.HubFile
		}
		rows = append(rows, r)
	}
	want := []row{
		{file: 999, agreed: 4, path: "gone.md", deleted: true},
		{file: edited.ID, agreed: 2, path: "edited.md", hash: "h-new"},
		{file: moved.ID, agreed: 3, path: "z/moved.md", hash: "h-moved"},
		{file: created.ID, path: "created.md", hash: "h-created"},
		{file: retried.ID, path: "retried.md", hash: "h-retried-2"},
	}
	if fmt.Sprint(rows) != fmt.Sprint(want) {
		t.Fatalf("Pending =\n%+v\nwant deletes, then puts, then creates, each by path:\n%+v", rows, want)
	}
	if got[1].Agreed.Hash != "h-old" || got[0].Agreed.Path != "gone.md" {
		t.Fatalf("a change's Agreed is not its agreed row: %+v, %+v", got[1].Agreed, got[0].Agreed)
	}
}

func TestLocalPushes(t *testing.T) {
	dt := newDeviceTest(t)
	ctx := t.Context()
	var p1, p2, p3 LocalPush
	dt.tx(func(tx *Tx) {
		var err error
		if p1, err = tx.AddPush(ctx, LocalPush{PushID: 40, VaultID: dt.vault, FileID: 1, Path: "a.md", Hash: "h1",
			Status: api.PushStatusAccepted, At: time.Unix(1, 0)}); err != nil {
			t.Fatal(err)
		}
		p2, _ = tx.AddPush(ctx, LocalPush{VaultID: dt.vault, FileID: 2, HubFile: 5, Base: 3, Path: "b.md"})
		p3, _ = tx.AddPush(ctx, LocalPush{VaultID: dt.vault, FileID: 3, Path: "c.md", Hash: "h3"})
		if err := tx.AnswerPush(ctx, dt.vault, p2.PushID, api.PushStatusMerged, ""); err != nil {
			t.Fatal(err)
		}
	})
	if p1.PushID != 1 || p2.PushID != 2 || p3.PushID != 3 || !p1.At.Equal(testNow) || p1.Status != "" {
		t.Fatalf("AddPush = %+v, %+v, %+v; want IDs 1, 2, 3, now, and no answer", p1, p2, p3)
	}
	un, err := dt.d.Unanswered(ctx, dt.vault)
	if err != nil || len(un) != 2 || un[0] != p1 || un[1] != p3 {
		t.Fatalf("Unanswered = %+v, %v; want pushes 1 and 3", un, err)
	}
	done, err := dt.d.Pushes(ctx, dt.vault)
	p2.Status = api.PushStatusMerged
	if err != nil || len(done) != 1 || done[0] != p2 {
		t.Fatalf("Pushes = %+v, %v; want push 2, merged", done, err)
	}
	err = dt.d.Tx(ctx, func(tx *Tx) error { return tx.AnswerPush(ctx, dt.vault, 99, api.PushStatusAccepted, "") })
	if !errors.Is(err, ErrNotFound) {
		t.Fatalf("answering push 99 = %v, want ErrNotFound", err)
	}
}
