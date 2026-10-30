package syncer_test

import (
	"strings"
	"sync/atomic"
	"testing"

	"github.com/frostyard/frostnotes/api"
)

// A writer's content that lost a merge on a device reaches the hub as a
// conflict revision even when the hub deletes the file first, and the
// device pulls that delete before it has pushed the conflict version: the
// version is made after a push looked for one and before the pull that
// follows it applies the delete (seen in TestScenarioContentionProfile's
// concurrent runs, where the token of a stale edit was lost).
func TestAConflictVersionReachesTheHubWhenAPulledDeleteComesFirst(t *testing.T) {
	ctx := t.Context()
	h := newHub(t, map[string]string{"f.md": "one\ntwo\nthree\n"})
	var (
		armed atomic.Bool
		a     *node
		stale string // f.md's first hash: the stale write's base
		err   error
		fired = make(chan error, 1)
	)
	o := deviceOptions("a")
	// The device's push has looked for conflict versions and uploaded what
	// it sends: the hub deletes f.md, and a writer with a stale base then
	// edits it here, every hunk conflicting, so the file stays as it was
	// and only a conflict version holds the edit.
	o.Crash = func(point string) bool {
		if point != "uploaded" || !armed.CompareAndSwap(true, false) {
			return false
		}
		fired <- func() error {
			cur, err := h.c.Note().Read(ctx, &api.NoteReadParams{VaultID: h.vault, Path: "f.md"})
			if err != nil {
				return err
			}
			if err := h.c.Note().Delete(ctx, &api.NoteDeleteParams{VaultID: h.vault, Path: "f.md", Base: cur.Hash}); err != nil {
				return err
			}
			res, err := a.c.Note().Write(ctx, &api.NoteWriteParams{VaultID: a.vault, Path: "f.md", Base: stale,
				Content: "one\ntwo, written stale\nthree\n"})
			if err == nil && res.Conflicts == 0 {
				t.Errorf("the stale write = %+v; want it to conflict", res)
			}
			return err
		}()
		return false
	}
	a = pairAndJoin(t, h, start(t, o, vaultDir(t, nil)), "a")
	settle(t, h, a)
	first, err := a.c.Note().Read(ctx, &api.NoteReadParams{VaultID: a.vault, Path: "f.md"})
	if err != nil {
		t.Fatal(err)
	}
	stale = first.Hash
	a.write("f.md", "one\ntwo, edited\nthree\n")
	settle(t, h, a)
	// A change of another file makes the push that the hook runs in.
	armed.Store(true)
	a.write("g.md", "another file\n")
	if err := <-fired; err != nil {
		t.Fatal(err)
	}
	settle(t, h, a)

	hv, err := h.srv.DB.HubVault(ctx, h.vault)
	if err != nil {
		t.Fatal(err)
	}
	var ops []string
	kept, deletes := false, 0
	for seq := int64(1); seq <= hv.Seq; seq++ {
		r, err := h.srv.DB.Revision(ctx, h.vault, seq)
		if err != nil {
			t.Fatal(err)
		}
		ops = append(ops, string(r.Op)+" "+r.Path)
		switch r.Op {
		case api.RevisionOpDelete:
			deletes++
		case api.RevisionOpConflict:
			if data, err := h.srv.Blobs.Get(r.Hash); err == nil && strings.Contains(string(data), "written stale") {
				kept = true
			}
		}
	}
	if !kept {
		t.Fatalf("no conflict revision on the hub holds the stale write; the hub's log: %q", ops)
	}
	// The delete the device pushes back, to drop its agreed row, appends
	// nothing: the file is deleted once.
	if deletes != 1 {
		t.Fatalf("the hub's log deletes f.md %d times, want once: %q", deletes, ops)
	}
	if got := files(t, a.dir); len(got) != 1 || got["g.md"] == "" {
		t.Fatalf("a's vault = %q, want only g.md", got)
	}
	if rows, err := a.srv.DB.AgreedRows(ctx, a.vault); err != nil || len(rows) != 1 || rows[0].Path != "g.md" {
		t.Fatalf("a's agreed rows = %+v, %v; want only g.md's", rows, err)
	}
}
