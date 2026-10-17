package store

// CONTRACT TEST for task card T-0025 (docs/tasks). Do not edit.

import (
	"errors"
	"fmt"
	"testing"

	"github.com/frostyard/frostnotes/api"
)

// device inserts a paired device directly (T-0026 adds AddDevice).
func (ht *hubTest) device(name string) int64 {
	ht.t.Helper()
	var id int64
	ht.tx(func(tx *Tx) {
		if err := tx.QueryRowContext(ht.t.Context(), `INSERT INTO hub_devices (name, token_hash, paired_at)
			VALUES (?, ?, '2026-10-16T12:00:00.000Z') RETURNING id`, name, "token-"+name).Scan(&id); err != nil {
			ht.t.Fatal(err)
		}
	})
	return id
}

// pushed appends a revision a device's push made.
func (ht *hubTest) pushed(r Revision) Revision {
	ht.t.Helper()
	r.VaultID = ht.vault.VaultID
	if r.Source == "" {
		r.Source = api.VersionSourceEditor
	}
	if r.Op != api.RevisionOpDelete && r.Hash == "" {
		r.Hash = fakeHash(r.Path)
	}
	var got Revision
	ht.tx(func(tx *Tx) {
		var err error
		if got, err = tx.AppendRevision(ht.t.Context(), r); err != nil {
			ht.t.Fatal(err)
		}
	})
	return got
}

func ptr(n int64) *int64 { return &n }

// sameRevision compares two wire revisions, their optional fields by value.
func sameRevision(a, b api.Revision) bool {
	deref := func(p *int64) int64 {
		if p == nil {
			return -1
		}
		return *p
	}
	if deref(a.Device) != deref(b.Device) || deref(a.LostTo) != deref(b.LostTo) {
		return false
	}
	a.Device, a.LostTo, b.Device, b.LostTo = nil, nil, nil, nil
	return a == b
}

func TestReplicasAndPushAnswers(t *testing.T) {
	ht := newHubTest(t)
	ctx := t.Context()
	laptop := ht.device("laptop")
	var rep Replica
	ht.tx(func(tx *Tx) {
		var err error
		if rep, err = tx.AddReplica(ctx, laptop, ht.vault.VaultID); err != nil {
			t.Fatal(err)
		}
	})
	if rep.ID == 0 || rep.DeviceID != laptop || rep.VaultID != ht.vault.VaultID || !rep.JoinedAt.Equal(testNow) {
		t.Fatalf("AddReplica = %+v", rep)
	}
	if got, err := ht.d.Replica(ctx, rep.ID); err != nil || got != rep {
		t.Fatalf("Replica = %+v, %v; want %+v", got, err, rep)
	}
	if _, err := ht.d.Replica(ctx, rep.ID+1); !errors.Is(err, ErrNotFound) {
		t.Fatalf("Replica(unknown) = %v, want ErrNotFound", err)
	}
	for name, add := range map[string]func(tx *Tx) error{
		"an unknown device": func(tx *Tx) error { _, err := tx.AddReplica(ctx, laptop+99, ht.vault.VaultID); return err },
		"an unserved vault": func(tx *Tx) error { _, err := tx.AddReplica(ctx, laptop, ht.vault.VaultID+99); return err },
	} {
		if err := ht.d.Tx(ctx, add); !errors.Is(err, ErrNotFound) {
			t.Errorf("a replica for %s = %v, want ErrNotFound", name, err)
		}
	}

	refused := PushAnswer{ReplicaID: rep.ID, PushID: 1, Status: api.PushStatusRefused, Reason: api.RefuseReasonCollision}
	merged := PushAnswer{ReplicaID: rep.ID, PushID: 2, FileID: 7, Status: api.PushStatusConflicted, Conflicts: 2, BasePruned: true}
	ht.tx(func(tx *Tx) {
		for _, a := range []PushAnswer{refused, merged} {
			if err := tx.RecordPush(ctx, a); err != nil {
				t.Fatal(err)
			}
		}
		got, err := tx.PushAnswer(ctx, rep.ID, 2)
		merged.At = testNow
		if err != nil || got != merged {
			t.Fatalf("PushAnswer(2) = %+v, %v; want %+v", got, err, merged)
		}
		got, err = tx.PushAnswer(ctx, rep.ID, 1)
		refused.At = testNow
		if err != nil || got != refused {
			t.Fatalf("PushAnswer(1) = %+v, %v; want %+v", got, err, refused)
		}
		if _, err := tx.PushAnswer(ctx, rep.ID, 3); !errors.Is(err, ErrNotFound) {
			t.Fatalf("PushAnswer(3) = %v, want ErrNotFound", err)
		}
	})
	err := ht.d.Tx(ctx, func(tx *Tx) error {
		return tx.RecordPush(ctx, PushAnswer{ReplicaID: rep.ID, PushID: 1, Status: api.PushStatusAccepted})
	})
	if !errors.Is(err, ErrConflict) {
		t.Fatalf("answering push 1 twice = %v, want ErrConflict", err)
	}
}

func TestPushRecords(t *testing.T) {
	ht := newHubTest(t)
	ctx := t.Context()
	laptop := ht.device("laptop")
	var rep Replica
	ht.tx(func(tx *Tx) { rep, _ = tx.AddReplica(ctx, laptop, ht.vault.VaultID) })
	by := func(r Revision, push int64) Revision {
		r.DeviceID, r.ReplicaID, r.PushID, r.Source, r.Author = laptop, rep.ID, push, api.VersionSourceEditor, "app"
		return r
	}
	c := ht.pushed(by(Revision{FileID: 1, Op: api.RevisionOpCreate, Path: "a.md"}, 1))
	ht.add(1, api.RevisionOpPut, "a.md", "hub edit") // the hub's own writer
	p := ht.pushed(by(Revision{FileID: 1, Op: api.RevisionOpPut, Path: "a.md", Hash: fakeHash("merged"), Base: 1}, 2))
	k := ht.pushed(by(Revision{FileID: 1, Op: api.RevisionOpConflict, Path: "a.md", Hash: fakeHash("lost"), Base: 1, LostTo: p.Seq}, 2))

	recs, err := ht.d.PushRecords(ctx, ht.vault.VaultID, rep.ID, []int64{2, 99, 1})
	if err != nil {
		t.Fatal(err)
	}
	if len(recs) != 3 || recs[0].Push != 2 || recs[1].Push != 99 || recs[2].Push != 1 {
		t.Fatalf("PushRecords = %+v; want pushes 2, 99, 1 in that order", recs)
	}
	if recs[1].Revisions == nil || len(recs[1].Revisions) != 0 {
		t.Fatalf("push 99's revisions = %#v, want a non-nil empty slice", recs[1].Revisions)
	}
	if len(recs[0].Revisions) != 2 || !sameRevision(recs[0].Revisions[0], p.API("laptop")) ||
		!sameRevision(recs[0].Revisions[1], k.API("laptop")) {
		t.Fatalf("push 2's revisions = %+v; want the put, then the conflict", recs[0].Revisions)
	}
	if len(recs[2].Revisions) != 1 || recs[2].Revisions[0].Seq != c.Seq {
		t.Fatalf("push 1's revisions = %+v", recs[2].Revisions)
	}
	want := api.Revision{Seq: k.Seq, File: 1, Op: api.RevisionOpConflict, Path: "a.md", Hash: fakeHash("lost"), Size: 0, Base: 1,
		Device: ptr(laptop), DeviceName: "laptop", Source: api.VersionSourceEditor, Author: "app", At: testNow, LostTo: ptr(p.Seq)}
	if got := k.API("laptop"); !sameRevision(got, want) {
		t.Fatalf("API = %+v\nwant %+v", got, want)
	}
	if got := c.API("laptop"); got.LostTo != nil {
		t.Fatalf("a create's API has LostTo %v; want none", *got.LostTo)
	}
	hub, err := ht.d.Revision(ctx, ht.vault.VaultID, 2)
	if err != nil {
		t.Fatal(err)
	}
	if got := hub.API("hub"); got.Device != nil || got.DeviceName != "hub" {
		t.Fatalf("the hub's own revision's API = %+v; want no device", got)
	}
}

func TestMatch(t *testing.T) {
	ht := newHubTest(t)
	ctx := t.Context()
	ht.add(1, api.RevisionOpCreate, "a.md", "h1")   // 1
	ht.add(1, api.RevisionOpPut, "a.md", "h2")      // 2
	ht.add(1, api.RevisionOpMove, "b.md", "h2")     // 3
	ht.add(2, api.RevisionOpCreate, "a.md", "h3")   // 4: a new file at a.md
	ht.add(3, api.RevisionOpCreate, "d.md", "h4")   // 5
	ht.add(3, api.RevisionOpDelete, "d.md", "")     // 6
	ht.add(2, api.RevisionOpConflict, "e.md", "h5") // 7: a conflict names no path a file is at
	ht.add(4, api.RevisionOpCreate, "c.md", "h6")   // 8
	ht.add(4, api.RevisionOpPut, "c.md", "h7")      // 9
	ht.add(4, api.RevisionOpPut, "c.md", "h6")      // 10: back to h6
	ht.add(5, api.RevisionOpCreate, "f.md", "h8")   // 11
	ht.add(5, api.RevisionOpMove, "g.md", "h8")     // 12
	ht.add(5, api.RevisionOpPut, "g.md", "h9")      // 13
	ht.add(5, api.RevisionOpMove, "f.md", "h9")     // 14: back at f.md
	ht.add(5, api.RevisionOpMove, "h.md", "h9")     // 15
	ht.add(6, api.RevisionOpCreate, "k.md", "h10")  // 16
	ht.add(6, api.RevisionOpMove, "m.md", "h10")    // 17
	ht.add(6, api.RevisionOpPut, "m.md", "h11")     // 18: a push into the moved file

	qs := []api.MatchQuery{
		{Path: "a.md", Hash: fakeHash("h1")}, // the file last at a.md is file 2, which never had h1
		{Path: "a.md", Hash: fakeHash("h3")},
		{Path: "b.md", Hash: fakeHash("h2")}, // the move carries h2
		{Path: "b.md", Hash: fakeHash("h1")}, // file 1 had h1 at another path
		{Path: "d.md", Hash: fakeHash("h4")}, // a deleted file still matches
		{Path: "e.md", Hash: fakeHash("h5")},
		{Path: "c.md", Hash: fakeHash("h6")}, // the newest revision with the hash
		{Path: "nowhere.md", Hash: fakeHash("h1")},
		// h9 is newest at h.md, but a copy at f.md is the one before the
		// move away from it: the pull brings it the move.
		{Path: "f.md", Hash: fakeHash("h9")},
		{Path: "f.md", Hash: fakeHash("h8")}, // the create, not the move away
		// The head's content at the path the file left: a put from the
		// create, whose answer moves it.
		{Path: "k.md", Hash: fakeHash("h11")},
	}
	got, err := ht.d.Match(ctx, ht.vault.VaultID, qs)
	if err != nil {
		t.Fatal(err)
	}
	want := []api.LogMatch{
		{File: ptr(2)},
		{File: ptr(2), Revision: ptr(4)},
		{File: ptr(1), Revision: ptr(3)},
		{File: ptr(1), Revision: ptr(1)},
		{File: ptr(3), Revision: ptr(5)},
		{},
		{File: ptr(4), Revision: ptr(10)},
		{},
		{File: ptr(5), Revision: ptr(14)},
		{File: ptr(5), Revision: ptr(11)},
		{File: ptr(6), Revision: ptr(16)},
	}
	// The matched revisions' contents: the query's hash, but for the copy
	// with a later revision's content at an earlier path.
	wantHash := []string{"", fakeHash("h3"), fakeHash("h2"), fakeHash("h1"), fakeHash("h4"), "", fakeHash("h6"), "",
		fakeHash("h9"), fakeHash("h8"), fakeHash("h10")}
	if len(got) != len(want) {
		t.Fatalf("Match gave %d answers for %d queries", len(got), len(qs))
	}
	show := func(p *int64) string {
		if p == nil {
			return "none"
		}
		return fmt.Sprint(*p)
	}
	for i := range want {
		if want[i].Revision != nil && (got[i].RevisionHash == nil || *got[i].RevisionHash != wantHash[i]) {
			t.Errorf("Match(%s, %s) gives revision hash %v; want %s", qs[i].Path, qs[i].Hash[:2], got[i].RevisionHash, wantHash[i][:2])
		}
		if got[i].Path != qs[i].Path || got[i].Hash != qs[i].Hash ||
			show(got[i].File) != show(want[i].File) || show(got[i].Revision) != show(want[i].Revision) {
			t.Errorf("Match(%s, %s) = file %s, revision %s; want %s, %s", qs[i].Path, qs[i].Hash[:2],
				show(got[i].File), show(got[i].Revision), show(want[i].File), show(want[i].Revision))
		}
	}
}

func TestHistory(t *testing.T) {
	ht := newHubTest(t)
	ctx := t.Context()
	laptop := ht.device("laptop")
	ht.add(1, api.RevisionOpCreate, "a.md", "v0")
	for i := range 250 {
		r := Revision{FileID: 1, Op: api.RevisionOpPut, Path: "a.md", Hash: fakeHash(fmt.Sprintf("v%d", i+1))}
		if i%2 == 1 {
			r.DeviceID = laptop
		}
		ht.pushed(r)
	}
	ht.add(2, api.RevisionOpCreate, "b.md", "other")
	all, err := ht.d.History(ctx, ht.vault.VaultID, 1, 0, 0)
	if err != nil || len(all) != 50 || all[0].Seq != 251 || all[49].Seq != 202 {
		t.Fatalf("History(0, 0) = %d revisions from %d, %v; want the newest 50, from 251", len(all), all[0].Seq, err)
	}
	if all[0].DeviceName != "laptop" || all[1].DeviceName != "hub" || all[1].Device != nil {
		t.Fatalf("device names = %q, %q; want laptop, then hub", all[0].DeviceName, all[1].DeviceName)
	}
	older, err := ht.d.History(ctx, ht.vault.VaultID, 1, 4, 10)
	if err != nil || len(older) != 3 || older[0].Seq != 3 || older[2].Seq != 1 || older[2].Op != api.RevisionOpCreate {
		t.Fatalf("History(before 4) = %+v, %v; want 3, 2, 1", older, err)
	}
	if capped, _ := ht.d.History(ctx, ht.vault.VaultID, 1, 0, 1000); len(capped) != 200 {
		t.Fatalf("History with limit 1000 gave %d, want 200", len(capped))
	}
	if _, err := ht.d.History(ctx, ht.vault.VaultID, 9, 0, 0); !errors.Is(err, ErrNotFound) {
		t.Fatalf("History(unknown file) = %v, want ErrNotFound", err)
	}
}
