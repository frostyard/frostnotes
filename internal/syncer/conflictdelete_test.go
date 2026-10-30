package syncer_test

import (
	"net"
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
	if deletes != 1 {
		t.Fatalf("the hub's log deletes f.md %d times, want once: %q", deletes, ops)
	}
	// The device deleted nothing itself, so it pushed no delete.
	noDeletePushed(t, a)
	if got := files(t, a.dir); len(got) != 1 || got["g.md"] == "" {
		t.Fatalf("a's vault = %q, want only g.md", got)
	}
	if rows, err := a.srv.DB.AgreedRows(ctx, a.vault); err != nil || len(rows) != 1 || rows[0].Path != "g.md" {
		t.Fatalf("a's agreed rows = %+v, %v; want only g.md's", rows, err)
	}
}

// noDeletePushed fails if device d ever pushed a delete.
func noDeletePushed(t *testing.T, d *node) {
	t.Helper()
	ps, err := d.srv.DB.Pushes(t.Context(), d.vault)
	if err != nil {
		t.Fatal(err)
	}
	un, err := d.srv.DB.Unanswered(t.Context(), d.vault)
	if err != nil {
		t.Fatal(err)
	}
	for _, p := range append(ps, un...) {
		if p.Hash == "" {
			t.Fatalf("the device pushed a delete of %s (push %d), but deleted nothing itself", p.Path, p.PushID)
		}
	}
}

// A conflict version a device carries past a pulled delete must not make
// it push a delete of its own: another device may bring the hub's file
// back meanwhile, here b's rename of it with its content unchanged, which
// the hub applies as a restore of the same file (an edit beats a delete),
// and that restored file must stay.
func TestAConflictVersionCarriedPastADeleteLeavesARestoredFile(t *testing.T) {
	ctx := t.Context()
	h := newHub(t, map[string]string{"f.md": "one\ntwo\nthree\n"})
	var armed atomic.Int32 // the "uploaded" points still to stop at
	at := make(chan int)   // the point the device's push stopped at
	resume := make(chan struct{})
	o := deviceOptions("a")
	o.Crash = func(point string) bool {
		if point != "uploaded" {
			return false
		}
		n := armed.Load()
		if n == 0 || !armed.CompareAndSwap(n, n-1) {
			return false
		}
		at <- int(n)
		<-resume
		return false
	}
	p := &partition{conns: map[net.Conn]bool{}}
	o.Syncer.Dial = p.dial
	a := &device{node: pairAndJoin(t, h, start(t, o, vaultDir(t, nil)), "a"), net: p}
	b := newPartitioned(t, h, "b", nil)
	settle(t, h, a.node, b.node)
	first, err := a.c.Note().Read(ctx, &api.NoteReadParams{VaultID: a.vault, Path: "f.md"})
	if err != nil {
		t.Fatal(err)
	}
	a.write("f.md", "one\ntwo, edited\nthree\n")
	settle(t, h, a.node, b.node)
	// b renames f.md while cut off; the rename waits there.
	b.net.cut()
	b.move("f.md", "g.md")

	armed.Store(2)
	a.write("other.md", "another file\n")
	// a's push has looked for conflict versions: the hub deletes f.md, and
	// a stale write on a conflicts in every hunk, so only a conflict
	// version holds it. The pull that follows applies the delete.
	if n := <-at; n != 2 {
		t.Fatalf("stopped at point %d, want 2", n)
	}
	cur, err := h.c.Note().Read(ctx, &api.NoteReadParams{VaultID: h.vault, Path: "f.md"})
	if err != nil {
		t.Fatal(err)
	}
	if err := h.c.Note().Delete(ctx, &api.NoteDeleteParams{VaultID: h.vault, Path: "f.md", Base: cur.Hash}); err != nil {
		t.Fatal(err)
	}
	res, err := a.c.Note().Write(ctx, &api.NoteWriteParams{VaultID: a.vault, Path: "f.md", Base: first.Hash,
		Content: "one\ntwo, written stale\nthree\n"})
	if err != nil || res.Conflicts == 0 {
		t.Fatalf("the stale write = %+v, %v; want it to conflict", res, err)
	}
	resume <- struct{}{}
	// a's next push, with the conflict version: before it goes, b comes
	// back and pushes its rename, which restores the hub's file at g.md.
	if n := <-at; n != 1 {
		t.Fatalf("stopped at point %d, want 1", n)
	}
	if got := files(t, a.dir); got["f.md"] != "" {
		t.Fatalf("a still holds f.md: %q", got)
	}
	b.net.heal()
	b.srv.Syncer.Kick(b.vault)
	settle(t, h, b.node)
	if got := files(t, h.dir); got["g.md"] != "one\ntwo, edited\nthree\n" {
		t.Fatalf("the hub after b's rename = %q, want g.md restored", got)
	}
	resume <- struct{}{}
	settle(t, h, a.node, b.node)

	got := same(t, h, a.node, b.node)
	if got["g.md"] != "one\ntwo, edited\nthree\n" || got["f.md"] != "" {
		t.Fatalf("the vaults = %q, want g.md, which b's rename restored", got)
	}
	noDeletePushed(t, a.node)
	hv, err := h.srv.DB.HubVault(ctx, h.vault)
	if err != nil {
		t.Fatal(err)
	}
	for seq := int64(1); seq <= hv.Seq; seq++ {
		r, err := h.srv.DB.Revision(ctx, h.vault, seq)
		if err != nil {
			t.Fatal(err)
		}
		if r.Op != api.RevisionOpConflict {
			continue
		}
		if data, err := h.srv.Blobs.Get(r.Hash); err == nil && strings.Contains(string(data), "written stale") {
			return
		}
	}
	t.Fatal("no conflict revision on the hub holds the stale write")
}
