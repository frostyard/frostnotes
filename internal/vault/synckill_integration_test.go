//go:build integration

package vault

import (
	"context"
	"errors"
	"fmt"
	"math/rand/v2"
	"path"
	"strings"
	"testing"

	"github.com/frostyard/frostnotes/api"
	"github.com/frostyard/frostnotes/internal/store"
)

// killed lists the notes a kill child may delete and move: those it made.
func (tv *testVault) killed(t *testing.T) ([]api.NoteEntry, []api.NoteEntry) {
	t.Helper()
	notes, err := tv.db.ListNotes(context.Background(), tv.v.id, store.NoteFilter{})
	if err != nil {
		t.Fatal(err)
	}
	var created []api.NoteEntry
	for _, n := range notes {
		if strings.HasPrefix(n.Path, "Kill/") {
			created = append(created, n)
		}
	}
	return notes, created
}

func (tv *testVault) put(t *testing.T, content string) string {
	t.Helper()
	h, err := tv.blobs.Put(context.Background(), strings.NewReader(content))
	if err != nil {
		t.Fatal(err)
	}
	return h
}

// movedPath is where a kill child moves a note it made, and back.
func movedPath(p string) string {
	if strings.HasPrefix(p, "Kill/moved/") {
		return "Kill/new-" + strings.TrimPrefix(path.Base(p), "moved-")
	}
	return "Kill/moved/moved-" + strings.TrimPrefix(path.Base(p), "new-")
}

// hubChild is a served vault applying a device's pushes until it is
// killed: creates, edits, moves and deletes, each with the head as its base.
func hubChild(t *testing.T, tv *testVault, seed int) {
	ctx := context.Background()
	tv.scan()
	if _, err := tv.db.HubVault(ctx, tv.v.id); errors.Is(err, store.ErrNotFound) {
		if _, err := tv.v.Serve(ctx); err != nil {
			t.Fatal(err)
		}
	}
	var device, replica int64
	if err := tv.db.Tx(ctx, func(tx *store.Tx) error {
		d, err := tx.AddDevice(ctx, fmt.Sprint("device ", seed), fmt.Sprint("token ", seed))
		if err != nil {
			return err
		}
		r, err := tx.AddReplica(ctx, d.ID, tv.v.id)
		device, replica = d.ID, r.ID
		return err
	}); err != nil {
		t.Fatal(err)
	}
	fmt.Println("ready")
	rng := rand.New(rand.NewPCG(uint64(seed), 13))
	for i := 0; ; i++ {
		notes, created := tv.killed(t)
		id := fmt.Sprintf("%d-%d", seed, i)
		var c PushChange
		head := func(n api.NoteEntry) api.LogFile {
			row := tv.row(n.Path)
			f, err := tv.db.LogFile(ctx, tv.v.id, row.ID)
			if err != nil {
				t.Fatal(err)
			}
			return f
		}
		switch op := rng.IntN(10); {
		case op < 2 || len(created) == 0:
			c = PushChange{Path: "Kill/new-" + id + ".md", Hash: tv.put(t, "created "+id)}
		case op == 2:
			f := head(created[rng.IntN(len(created))])
			c = PushChange{FileID: f.File, Base: f.Revision, BaseHash: f.Hash, Path: f.Path, Deleted: true}
		case op == 3:
			f := head(created[rng.IntN(len(created))])
			c = PushChange{FileID: f.File, Base: f.Revision, BaseHash: f.Hash, Path: movedPath(f.Path), Hash: f.Hash}
		default:
			n := notes[rng.IntN(len(notes))]
			cur, err := tv.v.Read(ctx, n.Path)
			if err != nil {
				t.Fatal(err)
			}
			f := head(n)
			c = PushChange{FileID: f.File, Base: f.Revision, BaseHash: f.Hash, Path: f.Path,
				Hash: tv.put(t, cur.Content+editLine(i))}
		}
		who := Author{Source: api.VersionSourceEditor, Name: "device",
			Push: &Push{DeviceID: device, ReplicaID: replica, PushID: int64(i + 1), Base: c.Base}}
		out, err := tv.v.Push(ctx, c, who)
		if err != nil {
			t.Fatalf("push %d: %v", i, err)
		}
		if out.Status == api.PushStatusRefused {
			t.Fatalf("push %d refused: %s", i, out.Reason)
		}
	}
}

// applyChild is a device applying pulled heads until it is killed, its
// agreed state set in each apply's transaction.
func applyChild(t *testing.T, tv *testVault, seed int) {
	ctx := context.Background()
	tv.scan()
	if _, err := tv.db.Remote(ctx, tv.v.id); errors.Is(err, store.ErrNotFound) {
		if err := tv.db.Tx(ctx, func(tx *store.Tx) error {
			h, err := tx.PutSyncHub(ctx, store.SyncHub{HubID: "hub", URL: "https://hub.example", DeviceID: 1})
			if err != nil {
				return err
			}
			_, err = tx.AddRemote(ctx, store.Remote{VaultID: tv.v.id, Hub: h.ID, SyncID: "sync", Name: "v", ReplicaID: 1})
			return err
		}); err != nil {
			t.Fatal(err)
		}
	}
	fmt.Println("ready")
	rng := rand.New(rand.NewPCG(uint64(seed), 17))
	who := Author{Source: api.VersionSourceSync, Name: "app@hub"}
	for i := 0; ; i++ {
		notes, created := tv.killed(t)
		id := fmt.Sprintf("%d-%d", seed, i)
		agree := func(seq int64) Record {
			return func(ctx context.Context, tx *store.Tx, f store.File) error {
				return tx.PutAgreed(ctx, store.Agreed{VaultID: tv.v.id, HubFile: f.ID, FileID: f.ID, Seq: seq, Revision: seq,
					Path: f.Path, Hash: f.Hash})
			}
		}
		var err error
		switch op := rng.IntN(10); {
		case op < 2 || len(created) == 0:
			p := "Kill/new-" + id + ".md"
			_, err = tv.v.Apply(ctx, Expect{Path: p}, Head{Path: p, Hash: tv.put(t, "created "+id)}, who, agree(int64(i)))
		case op == 2:
			n := created[rng.IntN(len(created))]
			row := tv.row(n.Path)
			_, err = tv.v.Apply(ctx, Expect{Path: n.Path, Hash: n.Hash}, Head{Path: n.Path, Deleted: true}, who,
				func(ctx context.Context, tx *store.Tx, _ store.File) error {
					return tx.DeleteAgreed(ctx, tv.v.id, row.ID)
				})
		case op == 3:
			n := created[rng.IntN(len(created))]
			_, err = tv.v.Apply(ctx, Expect{Path: n.Path, Hash: n.Hash}, Head{Path: movedPath(n.Path), Hash: n.Hash}, who, agree(int64(i)))
		default:
			n := notes[rng.IntN(len(notes))]
			cur, rerr := tv.v.Read(ctx, n.Path)
			if rerr != nil {
				t.Fatal(rerr)
			}
			_, err = tv.v.Apply(ctx, Expect{Path: n.Path, Hash: cur.Hash}, Head{Path: n.Path,
				Hash: tv.put(t, cur.Content+editLine(i))}, who, agree(int64(i)))
		}
		if err != nil {
			t.Fatalf("apply %d: %v", i, err)
		}
	}
}

// checkLogAfterKill restarts the hub's vault and checks that its log
// agrees with its files: each live file's head names its path and content,
// each head that is not a delete is a live file, and the sequence numbers
// have no gaps (ADR-0017).
func checkLogAfterKill(t *testing.T, data, dir string, run int) {
	t.Helper()
	ctx := t.Context()
	tv := openTestVault(t, dir, data, func(c *Config) { c.Batch = killBatch })
	tv.scan()
	defer func() { _ = tv.db.Close() }()
	files, err := tv.db.Files(ctx, tv.v.id)
	if err != nil {
		t.Fatal(err)
	}
	for _, f := range files {
		head, err := tv.db.LogFile(ctx, tv.v.id, f.ID)
		if err != nil || head.Deleted || head.Path != f.Path || head.Hash != f.Hash {
			t.Fatalf("run %d: %s's head in the log is %+v (%v), not the file", run, f.Path, head, err)
		}
	}
	hv, err := tv.db.HubVault(ctx, tv.v.id)
	if err != nil {
		t.Fatal(err)
	}
	page, err := tv.db.Pull(ctx, tv.v.id, 0, 1000)
	if err != nil {
		t.Fatal(err)
	}
	live := 0
	for _, f := range page.Files {
		if !f.Deleted {
			live++
		}
	}
	if live != len(files) {
		t.Fatalf("run %d: the log has %d live files, the vault %d", run, live, len(files))
	}
	for seq := int64(1); seq <= hv.Seq; seq++ {
		r, err := tv.db.Revision(ctx, tv.v.id, seq)
		if err != nil {
			t.Fatalf("run %d: revision %d of %d is missing: %v", run, seq, hv.Seq, err)
		}
		if r.Base >= r.Seq {
			t.Fatalf("run %d: revision %d is based on %d", run, r.Seq, r.Base)
		}
	}
}

// checkAgreedAfterKill restarts the device's vault and checks that every
// agreed state names a version its file had: a crash may leave a file
// newer than its agreed state (a change to push), never an agreed state
// the file never held.
func checkAgreedAfterKill(t *testing.T, data, dir string, run int) {
	t.Helper()
	ctx := t.Context()
	tv := openTestVault(t, dir, data, func(c *Config) { c.Batch = killBatch })
	tv.scan()
	defer func() { _ = tv.db.Close() }()
	rows, err := tv.db.AgreedRows(ctx, tv.v.id)
	if err != nil {
		t.Fatal(err)
	}
	for _, a := range rows {
		ok, err := tv.db.HasVersion(ctx, a.FileID, a.Hash)
		if err != nil || !ok {
			t.Fatalf("run %d: %s is agreed at %.12s, which the file never held", run, a.Path, a.Hash)
		}
		if f, err := tv.db.FileByID(ctx, a.FileID); err == nil && f.Path == a.Path && f.Hash != a.Hash {
			if have, _ := tv.blobs.Has(f.Hash); !have {
				t.Fatalf("run %d: %s holds content with no blob", run, f.Path)
			}
		}
	}
}
