package syncer_test

import (
	"context"
	"errors"
	"net"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/frostyard/frostnotes/api"
	"github.com/frostyard/frostnotes/internal/rpctest"
	"github.com/frostyard/frostnotes/internal/store"
)

// partition cuts a device off from its hub and heals it again: every
// connection the device opens goes through it.
type partition struct {
	mu      sync.Mutex
	offline bool
	conns   map[net.Conn]bool
}

func (p *partition) dial(ctx context.Context, addr string) (net.Conn, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.offline {
		return nil, errors.New("partitioned")
	}
	c, err := (&net.Dialer{}).DialContext(ctx, "tcp", addr)
	if err == nil {
		p.conns[c] = true
	}
	return c, err
}

func (p *partition) cut() {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.offline = true
	for c := range p.conns {
		_ = c.Close()
	}
	p.conns = map[net.Conn]bool{}
}

func (p *partition) heal() {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.offline = false
}

// device is a node whose link to the hub a test controls.
type device struct {
	*node
	net *partition
}

func newPartitioned(t *testing.T, h *node, name string, files map[string]string) *device {
	t.Helper()
	p := &partition{conns: map[net.Conn]bool{}}
	o := deviceOptions(name)
	o.Syncer.Dial = p.dial
	d := pairAndJoin(t, h, start(t, o, vaultDir(t, files)), name)
	return &device{node: d, net: p}
}

// inOrder runs fn with the devices cut off, then heals them one at a time
// in the order given, each syncing before the next, so the hub sees their
// changes in that order.
func inOrder(t *testing.T, h *node, devices []*device, fn func()) {
	t.Helper()
	for _, d := range devices {
		d.net.cut()
	}
	fn()
	var done []*node
	for _, d := range devices {
		d.net.heal()
		d.srv.Syncer.Kick(d.vault)
		done = append(done, d.node)
		settle(t, h, done...)
	}
}

// twoDevices starts a hub with files and two devices that have synced
// them.
func twoDevices(t *testing.T, files map[string]string) (*node, *device, *device) {
	t.Helper()
	h := newHub(t, files)
	a := newPartitioned(t, h, "a", nil)
	b := newPartitioned(t, h, "b", nil)
	settle(t, h, a.node, b.node)
	same(t, h, a.node, b.node)
	return h, a, b
}

// outside edits a file on disk, as another program would.
func (n *node) outside(p, content string) {
	n.t.Helper()
	full := filepath.Join(n.dir, filepath.FromSlash(p))
	if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
		n.t.Fatal(err)
	}
	if err := os.WriteFile(full, []byte(content), 0o644); err != nil {
		n.t.Fatal(err)
	}
}

func (n *node) remove(p string) {
	n.t.Helper()
	note, err := n.c.Note().Read(n.t.Context(), &api.NoteReadParams{VaultID: n.vault, Path: p})
	if err != nil {
		n.t.Fatal(err)
	}
	if err := n.c.Note().Delete(n.t.Context(), &api.NoteDeleteParams{VaultID: n.vault, Path: p, Base: note.Hash}); err != nil {
		n.t.Fatal(err)
	}
}

func (n *node) move(from, to string) {
	n.t.Helper()
	if _, err := n.c.Note().Move(n.t.Context(), &api.NoteMoveParams{VaultID: n.vault, Path: from, To: to}); err != nil {
		n.t.Fatal(err)
	}
}

// rescan makes the device's index catch up with its disk now.
func (n *node) rescan() {
	n.t.Helper()
	if _, err := n.c.Vault().Reindex(n.t.Context(), &api.VaultReindexParams{ID: n.vault}); err != nil {
		n.t.Fatal(err)
	}
	n.waitScanned()
}

const fiveLines = "one\ntwo\nthree\nfour\nfive\n"

func TestTwoEditorsOnOneBaseMerge(t *testing.T) {
	h, a, b := twoDevices(t, map[string]string{"n.md": fiveLines})
	inOrder(t, h, []*device{a, b}, func() {
		a.write("n.md", "ONE\ntwo\nthree\nfour\nfive\n")
		b.write("n.md", "one\ntwo\nthree\nfour\nFIVE\n")
	})
	got := same(t, h, a.node, b.node)
	if got["n.md"] != "ONE\ntwo\nthree\nfour\nFIVE\n" {
		t.Fatalf("merged = %q, want both edits", got["n.md"])
	}
}

func TestAnOutsideEditAndAPullMerge(t *testing.T) {
	h, a, b := twoDevices(t, map[string]string{"n.md": fiveLines})
	inOrder(t, h, []*device{a, b}, func() {
		a.write("n.md", "one\ntwo\nTHREE\nfour\nfive\n")
		b.outside("n.md", "one\ntwo\nthree\nfour\nfive\nsix, from vim\n")
		b.rescan()
	})
	if got := same(t, h, a.node, b.node)["n.md"]; got != "one\ntwo\nTHREE\nfour\nfive\nsix, from vim\n" {
		t.Fatalf("merged = %q", got)
	}
}

func TestAnAgentAppendAndAnEditMerge(t *testing.T) {
	h, a, b := twoDevices(t, map[string]string{"daily.md": "# today\n\n- first\n"})
	ctx := t.Context()
	inOrder(t, h, []*device{a, b}, func() {
		a.write("daily.md", "# Today\n\n- first\n")
		note, err := b.c.Note().Read(ctx, &api.NoteReadParams{VaultID: b.vault, Path: "daily.md"})
		if err != nil {
			t.Fatal(err)
		}
		agent := api.VersionSourceAgent
		who := "claude-code"
		if _, err := b.c.Note().Write(ctx, &api.NoteWriteParams{VaultID: b.vault, Path: "daily.md", Base: note.Hash,
			Content: note.Content + "- from the agent\n", Source: &agent, Author: &who}); err != nil {
			t.Fatal(err)
		}
	})
	if got := same(t, h, a.node, b.node)["daily.md"]; got != "# Today\n\n- first\n- from the agent\n" {
		t.Fatalf("merged = %q", got)
	}
	vs, err := a.c.Note().Versions(ctx, &api.NoteVersionsParams{VaultID: a.vault, Path: "daily.md"})
	if err != nil {
		t.Fatal(err)
	}
	if vs[0].Source != api.VersionSourceSync || !strings.Contains(vs[0].Author, "@") {
		t.Fatalf("a's newest version = %+v; want source sync and the head's author@device", vs[0])
	}
}

func TestAnEditBeatsADelete(t *testing.T) {
	for _, deleteFirst := range []bool{true, false} {
		t.Run(map[bool]string{true: "delete first", false: "edit first"}[deleteFirst], func(t *testing.T) {
			h, a, b := twoDevices(t, map[string]string{"n.md": fiveLines})
			order := []*device{a, b}
			if !deleteFirst {
				order = []*device{b, a}
			}
			inOrder(t, h, order, func() {
				a.remove("n.md")
				b.write("n.md", fiveLines+"six\n")
			})
			if got := same(t, h, a.node, b.node)["n.md"]; got != fiveLines+"six\n" {
				t.Fatalf("after a delete and an edit: %q, want the edit", got)
			}
		})
	}
}

func TestAMoveAndAnEditBothApply(t *testing.T) {
	for _, moveFirst := range []bool{true, false} {
		t.Run(map[bool]string{true: "move first", false: "edit first"}[moveFirst], func(t *testing.T) {
			h, a, b := twoDevices(t, map[string]string{"n.md": fiveLines})
			order := []*device{a, b}
			if !moveFirst {
				order = []*device{b, a}
			}
			inOrder(t, h, order, func() {
				a.move("n.md", "archive/n.md")
				b.write("n.md", fiveLines+"six\n")
			})
			got := same(t, h, a.node, b.node)
			if _, ok := got["n.md"]; ok || got["archive/n.md"] != fiveLines+"six\n" {
				t.Fatalf("after a move and an edit: %q", got)
			}
		})
	}
}

func TestTwoDailyNotesFromOneTemplateMerge(t *testing.T) {
	h, a, b := twoDevices(t, nil)
	inOrder(t, h, []*device{a, b}, func() {
		a.write("2026-10-17.md", "# 2026-10-17\n## Tasks\n- [ ] laptop task\n## Notes\n")
		b.write("2026-10-17.md", "# 2026-10-17\n## Tasks\n## Notes\nmet Ana\n")
	})
	got := same(t, h, a.node, b.node)
	if got["2026-10-17.md"] != "# 2026-10-17\n## Tasks\n- [ ] laptop task\n## Notes\nmet Ana\n" {
		t.Fatalf("merged daily note = %q", got["2026-10-17.md"])
	}
	if len(got) != 1 {
		t.Fatalf("the vaults hold %d files, want one: no conflict copies", len(got))
	}
}

func TestAMoveOntoATakenPathGetsANumber(t *testing.T) {
	h, a, b := twoDevices(t, map[string]string{"a.md": "from a\n", "b.md": "from b\n"})
	// Two devices move different files onto one free path at once.
	inOrder(t, h, []*device{a, b}, func() {
		a.move("a.md", "c.md")
		b.move("b.md", "c.md")
	})
	got := same(t, h, a.node, b.node)
	if len(got) != 2 || got["c.md"] != "from a\n" || got["c 1.md"] != "from b\n" {
		t.Fatalf("two moves onto one path = %q, want a's at c.md and b's at c 1.md", got)
	}
}

func TestACaseOnlyCollisionIsRefused(t *testing.T) {
	h, _, b := twoDevices(t, map[string]string{"plan.md": "the hub's\n"})
	b.write("Plan.md", "b's own\n")
	b.srv.Syncer.Kick(b.vault)
	settle(t, h, b.node)
	st, err := b.c.Sync().Status(t.Context(), &api.SyncStatusParams{VaultID: &b.vault})
	if err != nil {
		t.Fatal(err)
	}
	if len(st[0].Refused) != 1 || st[0].Refused[0].Path != "Plan.md" || st[0].Refused[0].Reason != api.RefuseReasonCollision {
		t.Fatalf("status = %+v; want Plan.md refused as a collision", st[0])
	}
	if _, ok := files(t, h.dir)["Plan.md"]; ok {
		t.Fatal("the hub took a path differing only by case")
	}
}

func TestRevocationStopsSync(t *testing.T) {
	h, a, b := twoDevices(t, map[string]string{"n.md": fiveLines})
	ctx := t.Context()
	ds, err := h.c.Hub().Devices(ctx, &api.HubDevicesParams{})
	if err != nil {
		t.Fatal(err)
	}
	var bID int64
	for _, d := range ds {
		if d.Name == "b" {
			bID = d.ID
		}
	}
	if err := h.c.Hub().Revoke(ctx, &api.HubRevokeParams{Device: bID}); err != nil {
		t.Fatal(err)
	}
	b.write("n.md", "after revocation\n")
	waitState(t, b.node, api.SyncStateRevoked)
	a.srv.Syncer.Kick(a.vault)
	settle(t, h, a.node)
	if a.read("n.md") != fiveLines || b.read("n.md") != "after revocation\n" {
		t.Fatal("a revoked device's edit reached the hub, or the device lost its own")
	}
}

// waitState waits for a device's sync.changed event with state.
func waitState(t *testing.T, n *node, state api.SyncState) {
	t.Helper()
	ctx, cancel := context.WithTimeout(t.Context(), 20*time.Second)
	defer cancel()
	c, _, err := api.Dial(ctx, n.srv.Socket, "watch")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = c.Close() }()
	if _, err := c.Events().Subscribe(ctx, &api.EventsSubscribeParams{}); err != nil {
		t.Fatal(err)
	}
	if st, _ := n.c.Sync().Status(ctx, &api.SyncStatusParams{VaultID: &n.vault}); len(st) == 1 && st[0].State == state {
		return
	}
	for {
		select {
		case ev := <-c.Notifications():
			if e, err := api.DecodeEvent(ev.Event, ev.Data); err == nil {
				if ch, ok := e.(api.SyncChanged); ok && ch.Status.State == state {
					return
				}
			}
		case <-ctx.Done():
			t.Fatalf("the device's sync did not become %s", state)
		}
	}
}

func TestADeviceSkipsAPathOutsideItsVault(t *testing.T) {
	h, a, _ := twoDevices(t, map[string]string{"n.md": fiveLines})
	ctx := t.Context()
	// A hub gone wrong: a revision naming a path outside the vault.
	hash, err := h.srv.Blobs.Put(ctx, strings.NewReader("evil\n"))
	if err != nil {
		t.Fatal(err)
	}
	hv, err := h.srv.DB.HubVaults(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if err := h.srv.DB.Tx(ctx, func(tx *store.Tx) error {
		_, err := tx.AppendRevision(ctx, store.Revision{VaultID: hv[0].VaultID, FileID: 999, Op: api.RevisionOpCreate,
			Path: "../escaped.md", Hash: hash, Size: 5, Source: api.VersionSourceEditor})
		return err
	}); err != nil {
		t.Fatal(err)
	}
	a.srv.Syncer.Kick(a.vault)
	settle(t, h, a.node)
	if _, err := os.Stat(filepath.Join(filepath.Dir(a.dir), "escaped.md")); err == nil {
		t.Fatal("a pulled path escaped the vault")
	}
	st, _ := a.c.Sync().Status(ctx, &api.SyncStatusParams{VaultID: &a.vault})
	if len(st[0].Skipped) != 1 || st[0].Skipped[0] != "../escaped.md" {
		t.Fatalf("skipped = %v", st[0].Skipped)
	}
}

func TestVerifyAndHistory(t *testing.T) {
	h, a, b := twoDevices(t, map[string]string{"n.md": fiveLines})
	ctx := t.Context()
	a.write("n.md", fiveLines+"six\n")
	a.srv.Syncer.Kick(a.vault)
	settle(t, h, a.node, b.node)
	for _, d := range []*device{a, b} {
		rep, err := d.c.Sync().Verify(ctx, &api.SyncVerifyParams{VaultID: d.vault})
		if err != nil || !rep.Ok || rep.Files != 1 {
			t.Fatalf("verify = %+v, %v", rep, err)
		}
	}
	revs, err := b.c.Sync().History(ctx, &api.SyncHistoryParams{VaultID: b.vault, Path: "n.md"})
	if err != nil || len(revs) != 2 || revs[0].DeviceName != "a" || revs[1].Op != api.RevisionOpCreate {
		t.Fatalf("history = %+v, %v", revs, err)
	}
	// A file that differs is reported.
	b.outside("n.md", "changed while b is offline\n")
	b.net.cut()
	b.rescan()
	b.net.heal()
	rep, err := b.c.Sync().Verify(ctx, &api.SyncVerifyParams{VaultID: b.vault})
	if err != nil || rep.Ok || len(rep.Differing) != 1 {
		t.Fatalf("verify of a differing vault = %+v, %v", rep, err)
	}
}

// lossyHub starts a hub that, once armed, loses its answer to a push and
// cuts the pushing device off, as a dropped connection would.
func lossyHub(t *testing.T, files map[string]string) (*node, func(*device) <-chan struct{}) {
	t.Helper()
	var mu sync.Mutex
	var d *device
	var fired chan struct{}
	h := start(t, rpctest.Options{HubListen: true, Crash: func(point string) bool {
		mu.Lock()
		defer mu.Unlock()
		if point != "pushed" || fired == nil {
			return false
		}
		d.net.cut()
		close(fired)
		fired = nil
		return true
	}}, vaultDir(t, files))
	if _, err := h.c.Hub().Serve(t.Context(), &api.HubServeParams{VaultID: h.vault}); err != nil {
		t.Fatal(err)
	}
	arm := func(of *device) <-chan struct{} {
		mu.Lock()
		defer mu.Unlock()
		d, fired = of, make(chan struct{})
		return fired
	}
	return h, arm
}

// lose has device d push its changes to a hub armed by lossyHub, and
// returns once the hub has lost the answer.
func lose(t *testing.T, d *device, fired <-chan struct{}) {
	t.Helper()
	d.net.heal()
	d.srv.Syncer.Kick(d.vault)
	select {
	case <-fired:
	case <-time.After(30 * time.Second):
		t.Fatal("the device never pushed")
	}
}

func TestALostAnswerAndAMoveMakeOneFile(t *testing.T) {
	// The hub loses its answer to a's create; a moves the file before it
	// hears again. The retry is answered first, and the move follows it:
	// one file on the hub, not a second create at the new path.
	h, arm := lossyHub(t, nil)
	a := newPartitioned(t, h, "a", nil)
	settle(t, h, a.node)
	a.net.cut()
	a.write("new.md", "a new note\n")
	lose(t, a, arm(a))
	a.move("new.md", "moved.md")
	a.net.heal()
	a.srv.Syncer.Kick(a.vault)
	settle(t, h, a.node)
	got := same(t, h, a.node)
	if len(got) != 1 || got["moved.md"] != "a new note\n" {
		t.Fatalf("a lost answer and a move = %q, want one file at moved.md", got)
	}
}

func TestAMergedCreateThatMovedMakesOneFile(t *testing.T) {
	// The hub merges a's create into its own note at the path, and loses
	// the answer; a moves the note before it hears again. The merge
	// applies where the note now is, and the move follows.
	h, arm := lossyHub(t, nil)
	a := newPartitioned(t, h, "a", nil)
	settle(t, h, a.node)
	a.net.cut()
	h.write("2026-10-17.md", "# 2026-10-17\n## Tasks\n- [ ] hub task\n## Notes\n")
	a.write("2026-10-17.md", "# 2026-10-17\n## Tasks\n## Notes\nmet Ana\n")
	lose(t, a, arm(a))
	a.move("2026-10-17.md", "days/2026-10-17.md")
	a.net.heal()
	a.srv.Syncer.Kick(a.vault)
	settle(t, h, a.node)
	got := same(t, h, a.node)
	if len(got) != 1 || got["days/2026-10-17.md"] != "# 2026-10-17\n## Tasks\n- [ ] hub task\n## Notes\nmet Ana\n" {
		t.Fatalf("a merged create that moved = %q, want one merged note at days/2026-10-17.md", got)
	}
}

func TestANewFileWaitsForTheOneLeavingItsPath(t *testing.T) {
	// While a is away, the hub moves plan.md to old.md, makes a new
	// plan.md, then edits old.md: the new file comes first in a's pull,
	// while a's copy of the old one still holds the path. It waits for
	// the old one's move, rather than being skipped.
	h, a, _ := twoDevices(t, map[string]string{"plan.md": "the old plan\n"})
	a.net.cut()
	h.move("plan.md", "old.md")
	h.write("plan.md", "a new plan\n")
	h.write("old.md", "the old plan\nkept\n")
	a.net.heal()
	a.srv.Syncer.Kick(a.vault)
	settle(t, h, a.node)
	got := same(t, h, a.node)
	if len(got) != 2 || got["plan.md"] != "a new plan\n" || got["old.md"] != "the old plan\nkept\n" {
		t.Fatalf("a's vault = %q, want the new plan.md and the edited old.md", got)
	}
}
