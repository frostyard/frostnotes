// Package synctest is sync's seeded simulation (ADR-0020,
// docs/design/sync.md#testing): a hub and devices, each a whole notesd
// from rpctest over its own data directory and vault, syncing over
// loopback through links the simulation cuts and heals, while a seeded
// plan of steps edits, creates, moves, deletes, crashes and restarts. At
// each quiet point it checks that every vault equals the hub's heads, that
// no inserted token was lost, that no conflict copy exists, that the log
// holds its invariants, and that every device agrees with the hub.
package synctest

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/frostyard/frostnotes/api"
	"github.com/frostyard/frostnotes/internal/rpctest"
	"github.com/frostyard/frostnotes/internal/syncer"
)

// Options configure a run.
type Options struct {
	Seed    uint64
	Devices int // [3]
	Steps   int
	// Concurrent lets steps overlap sync; stepwise (the default) lets the
	// world go quiet after every step, so a seed replays exactly.
	Concurrent bool
	// CheckEvery is how many steps pass between full checks [25 stepwise,
	// 100 concurrent]; a run always checks at its end.
	CheckEvery int
	Log        *slog.Logger // [discard]
	// Watch, when set, logs each node's copy of the file at that path
	// after every step: a debugging aid.
	Watch string
}

func (o Options) withDefaults() Options {
	if o.Devices == 0 {
		o.Devices = 3
	}
	if o.CheckEvery == 0 {
		o.CheckEvery = 25
		if o.Concurrent {
			o.CheckEvery = 100
		}
	}
	if o.Log == nil {
		o.Log = slog.New(slog.DiscardHandler)
	}
	return o
}

// link is a device's connection to the hub: cut, every connection closes
// and new ones fail; healed, they work again.
type link struct {
	mu      sync.Mutex
	offline bool
	conns   map[net.Conn]bool
}

func (l *link) dial(ctx context.Context, addr string) (net.Conn, error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.offline {
		return nil, errors.New("synctest: the link is cut")
	}
	c, err := (&net.Dialer{}).DialContext(ctx, "tcp", addr)
	if err == nil {
		l.conns[c] = true
	}
	return c, err
}

func (l *link) cut() {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.offline = true
	for c := range l.conns {
		_ = c.Close()
	}
	l.conns = map[net.Conn]bool{}
}

func (l *link) heal() {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.offline = false
}

func (l *link) isOffline() bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.offline
}

// crasher arms one crash point of a notesd at a time.
type crasher struct {
	mu    sync.Mutex
	point string
	fired chan struct{}
	then  func() // run as it fires, before the notesd goes on
}

func (c *crasher) arm(point string) <-chan struct{} { return c.armThen(point, nil) }

// armThen arms point, and runs then as it fires.
func (c *crasher) armThen(point string, then func()) <-chan struct{} {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.point, c.fired, c.then = point, make(chan struct{}), then
	return c.fired
}

func (c *crasher) disarm() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.point = ""
}

func (c *crasher) hook(point string) bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.point == "" || c.point != point {
		return false
	}
	c.point = ""
	if c.then != nil {
		c.then()
	}
	close(c.fired)
	return true
}

// Node is one notesd of the world, with one vault.
type Node struct {
	Name  string
	Dir   string // the vault's folder
	data  string
	opts  rpctest.Options
	Srv   *rpctest.Server
	C     *api.Client
	Vault int64
	link  *link // a device's; nil for the hub
	crash *crasher
}

// start runs the node's notesd on its data, registering its vault on the
// first start.
func (n *Node) start(t testing.TB) {
	t.Helper()
	n.Srv = rpctest.StartWith(t, n.opts)
	n.C = n.Srv.Dial(t)
	vs, err := n.C.Vault().List(t.Context(), &api.VaultListParams{})
	if err != nil {
		t.Fatal(err)
	}
	for _, v := range vs {
		if v.Path == n.Dir {
			n.Vault = v.ID
			return
		}
	}
	v, err := n.C.Vault().Add(t.Context(), &api.VaultAddParams{Path: n.Dir})
	if err != nil {
		t.Fatal(err)
	}
	n.Vault = v.ID
}

// probe checks that the node's watcher sees its vault: a note written on
// disk must be indexed as an outside edit, and its removal too. Without a
// watcher (inotify instances run out, as when many notesd run at once)
// edits wait for a rescan, and the run would not test what it means to.
func (n *Node) probe(t testing.TB) {
	t.Helper()
	ctx, cancel := context.WithTimeout(t.Context(), 20*time.Second)
	defer cancel()
	c, _, err := api.Dial(ctx, n.Srv.Socket, "synctest probe")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = c.Close() }()
	if _, err := c.Events().Subscribe(ctx, &api.EventsSubscribeParams{}); err != nil {
		t.Fatal(err)
	}
	const p = "synctest-probe.md"
	full := filepath.Join(n.Dir, p)
	wait := func(what string, match func(api.Event) bool) {
		for {
			select {
			case ev := <-c.Notifications():
				if e, err := api.DecodeEvent(ev.Event, ev.Data); err == nil && match(e) {
					return
				}
			case <-ctx.Done():
				t.Fatalf("%s's watcher did not see %s: is fs.inotify.max_user_instances used up?", n.Name, what)
			}
		}
	}
	if err := os.WriteFile(full, []byte("probe\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	wait("a new note", func(e api.Event) bool {
		ch, ok := e.(api.NoteChanged)
		return ok && ch.VaultID == n.Vault && ch.Path == p
	})
	if err := os.Remove(full); err != nil {
		t.Fatal(err)
	}
	wait("a removed note", func(e api.Event) bool {
		rm, ok := e.(api.NoteRemoved)
		return ok && rm.VaultID == n.Vault && rm.Path == p
	})
}

// stop ends the node's notesd now, as a crash between transactions would.
func (n *Node) stop() {
	_ = n.C.Close()
	n.Srv.Stop()
}

func newNode(t testing.TB, name string, o Options, hub bool, hubAddr string) *Node {
	t.Helper()
	root := t.TempDir()
	n := &Node{Name: name, Dir: filepath.Join(root, "vault"), data: filepath.Join(root, "data"), crash: &crasher{}}
	for _, d := range []string{n.Dir, n.data} {
		if err := os.MkdirAll(d, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	n.opts = rpctest.Options{Log: o.Log.With("node", name), DataDir: n.data, Crash: n.crash.hook}
	n.opts.Vault.Debounce = 5 * time.Millisecond
	if hub {
		n.opts.HubListen, n.opts.HubAddr = true, hubAddr
	} else {
		n.link = &link{conns: map[net.Conn]bool{}}
		n.opts.Syncer = syncer.Config{Device: name, Dial: n.link.dial, Quiet: time.Millisecond, MaxQuiet: 5 * time.Millisecond,
			Idle: 500 * time.Millisecond, Backoff: 5 * time.Millisecond, MaxBackoff: 20 * time.Millisecond}
	}
	return n
}

// freeAddr picks a loopback address the hub keeps across restarts.
func freeAddr(t testing.TB) string {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	addr := ln.Addr().String()
	_ = ln.Close()
	return addr
}

// pair pairs device d with the hub and joins the hub's vault.
func (w *World) pair(d *Node) error {
	ctx := w.t.Context()
	name := d.Name
	p, err := d.C.Sync().Pair(ctx, &api.SyncPairParams{URL: w.Hub.Srv.HubURL, Name: &name})
	if err != nil {
		return fmt.Errorf("pair %s: %w", d.Name, err)
	}
	if _, err := w.Hub.C.Hub().Approve(ctx, &api.HubApproveParams{Code: p.Code}); err != nil {
		return err
	}
	if r, err := d.C.Sync().Paired(ctx, &api.SyncPairedParams{Pairing: p.Pairing}); err != nil || r.State != api.PairingStateApproved {
		return fmt.Errorf("paired %s: %+v, %v", d.Name, r, err)
	}
	vs, err := w.Hub.C.Hub().Vaults(ctx, &api.HubVaultsParams{})
	if err != nil {
		return err
	}
	_, err = d.C.Sync().Join(ctx, &api.SyncJoinParams{VaultID: d.Vault, URL: w.Hub.Srv.HubURL, Name: vs[0].Name})
	return err
}

// hubSeq is the hub vault's latest sequence number.
func (w *World) hubSeq() (int64, error) {
	vs, err := w.Hub.C.Hub().Vaults(w.t.Context(), &api.HubVaultsParams{})
	if err != nil {
		return 0, err
	}
	if len(vs) != 1 {
		return 0, fmt.Errorf("the hub serves %d vaults", len(vs))
	}
	return vs[0].Seq, nil
}

// settle heals every link and waits until every device has nothing to
// push and has pulled all the hub's log holds: a quiet point.
func (w *World) settle() error {
	if err := w.catchUp(); err != nil {
		return err
	}
	for _, d := range w.Devices {
		d.link.heal()
		d.Srv.Syncer.Kick(d.Vault)
	}
	ctx, cancel := context.WithTimeout(w.t.Context(), 30*time.Second)
	defer cancel()
	for range 500 {
		seq, err := w.hubSeq()
		if err != nil {
			return err
		}
		quiet := true
		for _, d := range w.Devices {
			if err := d.Srv.Syncer.WaitIdle(ctx, d.Vault); err != nil {
				return fmt.Errorf("%s did not go idle: %w%s", d.Name, err, w.dump(d))
			}
			st, err := d.C.Sync().Status(ctx, &api.SyncStatusParams{VaultID: &d.Vault})
			if err != nil {
				return err
			}
			switch st[0].State {
			case api.SyncStateRevoked, api.SyncStateReset:
				return fmt.Errorf("%s's sync is %s: %v", d.Name, st[0].State, st[0].Error)
			}
			if st[0].Pending != 0 || st[0].Cursor < seq {
				quiet = false
			}
		}
		if after, err := w.hubSeq(); err == nil && quiet && after == seq {
			return nil
		}
	}
	return errors.New("the world did not go quiet")
}

// dump lists device d's index and agreed rows, and its status, for a
// failed check.
func (w *World) dump(d *Node) string {
	ctx := context.WithoutCancel(w.t.Context())
	out := ""
	if fs, err := d.Srv.DB.Files(ctx, d.Vault); err == nil {
		for _, f := range fs {
			out += fmt.Sprintf("\n  file %d %s %.8s", f.ID, f.Path, f.Hash)
		}
	}
	if rows, err := d.Srv.DB.AgreedRows(ctx, d.Vault); err == nil {
		for _, a := range rows {
			out += fmt.Sprintf("\n  agreed hub %d = file %d %s %.8s rev %d seq %d", a.HubFile, a.FileID, a.Path, a.Hash, a.Revision, a.Seq)
		}
	}
	if ps, err := d.Srv.DB.Pushes(ctx, d.Vault); err == nil {
		for _, p := range ps[max(0, len(ps)-12):] {
			out += fmt.Sprintf("\n  push %d file %d hub %d base %d %s %.8s conflict %v: %s %s", p.PushID, p.FileID, p.HubFile,
				p.Base, p.Path, p.Hash, p.Conflict, p.Status, p.Reason)
		}
	}
	if _, revs, err := w.hubLog(ctx); err == nil {
		for _, r := range revs[max(0, len(revs)-25):] {
			out += fmt.Sprintf("\n  hub rev %d file %d %s %s %.8s base %d device %d push %d", r.Seq, r.FileID, r.Op, r.Path, r.Hash,
				r.Base, r.DeviceID, r.PushID)
		}
	}
	if st, err := d.C.Sync().Status(ctx, &api.SyncStatusParams{VaultID: &d.Vault}); err == nil {
		out += fmt.Sprintf("\n  status: %+v", st[0])
	}
	return out
}
