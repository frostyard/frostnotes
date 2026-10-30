package syncer_test

import (
	"net"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/frostyard/frostnotes/api"
	"github.com/frostyard/frostnotes/internal/rpctest"
)

// crashy is a device whose notesd a test stops at the vault's crash point
// "renamed" (after a change's rename, before its transaction) and starts
// again on the same data, as a crash and a restart would (ADR-0020).
type crashy struct {
	*device
	opts  rpctest.Options
	armed atomic.Bool
	fired chan struct{}
}

func newCrashy(t *testing.T, h *node, name string) *crashy {
	t.Helper()
	d := &crashy{fired: make(chan struct{}, 1)}
	p := &partition{conns: map[net.Conn]bool{}}
	o := deviceOptions(name)
	o.Syncer.Dial = p.dial
	o.DataDir = t.TempDir()
	o.Crash = func(point string) bool {
		if point != "renamed" || !d.armed.CompareAndSwap(true, false) {
			return false
		}
		p.cut() // the device is down: nothing more of it reaches the hub
		d.fired <- struct{}{}
		return true
	}
	d.opts = o
	d.device = &device{node: pairAndJoin(t, h, start(t, o, vaultDir(t, nil)), name), net: p}
	return d
}

// down waits for the armed crash point and stops the device's notesd.
func (d *crashy) down() {
	d.t.Helper()
	select {
	case <-d.fired:
	case <-time.After(settleWait):
		d.t.Fatal("the crash point was not reached")
	}
	d.srv.Stop()
}

// up starts the device's notesd again on its data, and waits for the
// vault's open scan.
func (d *crashy) up() {
	d.t.Helper()
	o := d.opts
	o.Vault.Debounce = 10 * time.Millisecond
	d.srv = rpctest.StartWith(d.t, o)
	d.c = d.srv.Dial(d.t)
	d.waitScanned()
	d.net.heal()
	d.srv.Syncer.Kick(d.vault)
}

// noConflictRevision fails if the hub's log holds a conflict revision.
func noConflictRevision(t *testing.T, h *node) {
	t.Helper()
	hv, err := h.srv.DB.HubVault(t.Context(), h.vault)
	if err != nil {
		t.Fatal(err)
	}
	var ops []string
	conflicts := 0
	for seq := int64(1); seq <= hv.Seq; seq++ {
		r, err := h.srv.DB.Revision(t.Context(), h.vault, seq)
		if err != nil {
			t.Fatal(err)
		}
		ops = append(ops, string(r.Op)+" "+r.Path)
		if r.Op == api.RevisionOpConflict {
			conflicts++
		}
	}
	if conflicts > 0 {
		t.Fatalf("the hub's log holds %d conflict revisions, but no writer lost: %q", conflicts, ops)
	}
}

// A device that crashes after it changed its disk for a head, and before
// the transaction that agrees the head, must not push that change back
// as its own after the restart: the device only applied the hub's head,
// and the hub may have gone on meanwhile. Each case's first step makes a
// head that device a pulls (or an answer that a applies) and crashes
// applying; its second step changes the hub while a is down. After a
// restarts, every vault must hold what the hub had before a came back.
func TestACrashWhileApplyingAHeadPushesNothingBack(t *testing.T) {
	const keep = "keep.md" // so a's vault is never empty: an empty vault fails its scan
	five := "1\n2\n3\n4\n5\n"
	cases := []struct {
		name      string
		files     map[string]string
		applying  func(t *testing.T, h *node, a *crashy, b *device)
		meanwhile func(t *testing.T, h *node, b *device)
		onDisk    func(a *crashy) // a's disk while a is down
		want      map[string]string
	}{{
		// The hub deletes f.md while b, cut off, renames it; a pulls the
		// delete. b's rename then brings f.md back on the hub, with its
		// content unchanged (rule 5): a's stale delete, from f.md's first
		// revision, would delete it again (rule 4).
		name:  "a pulled delete",
		files: map[string]string{"f.md": "one\n", keep: "k\n"},
		applying: func(t *testing.T, h *node, a *crashy, b *device) {
			b.net.cut()
			b.move("f.md", "g.md")
			h.remove("f.md")
		},
		meanwhile: func(t *testing.T, h *node, b *device) {
			b.net.heal()
			b.srv.Syncer.Kick(b.vault)
			settle(t, h, b.node)
		},
		want: map[string]string{"g.md": "one\n", keep: "k\n"},
	}, {
		// b moves f.md twice; a's stale move, from f.md's first revision,
		// would move it back (the latest move wins).
		name:  "a pulled move",
		files: map[string]string{"f.md": "one\n", keep: "k\n"},
		applying: func(t *testing.T, h *node, a *crashy, b *device) {
			b.move("f.md", "g.md")
		},
		meanwhile: func(t *testing.T, h *node, b *device) {
			settle(t, h, b.node)
			b.move("g.md", "h.md")
			settle(t, h, b.node)
		},
		want: map[string]string{"h.md": "one\n", keep: "k\n"},
	}, {
		// b moves and edits f.md in one push; a crashes after the move's
		// rename, before the edit's. b moves the file again: a's stale
		// move would move it back.
		name:  "a pulled move and edit",
		files: map[string]string{"f.md": "one\n", keep: "k\n"},
		applying: func(t *testing.T, h *node, a *crashy, b *device) {
			b.net.cut()
			b.move("f.md", "g.md")
			b.write("g.md", "one, edited\n")
			b.net.heal()
			b.srv.Syncer.Kick(b.vault)
			settle(t, h, b.node)
		},
		meanwhile: func(t *testing.T, h *node, b *device) {
			b.move("g.md", "h.md")
			settle(t, h, b.node)
		},
		want: map[string]string{"h.md": "one, edited\n", keep: "k\n"},
	}, {
		// b edits f.md, then deletes it; a's stale put would bring it back
		// (a put on a deleted file).
		name:  "a pulled put",
		files: map[string]string{"f.md": "one\n", keep: "k\n"},
		applying: func(t *testing.T, h *node, a *crashy, b *device) {
			b.write("f.md", "one, edited\n")
		},
		meanwhile: func(t *testing.T, h *node, b *device) {
			settle(t, h, b.node)
			b.remove("f.md")
			settle(t, h, b.node)
		},
		want: map[string]string{keep: "k\n"},
	}, {
		// b creates n.md, then deletes it; a's stale create would make it
		// again as a new file.
		name:  "a pulled create",
		files: map[string]string{keep: "k\n"},
		applying: func(t *testing.T, h *node, a *crashy, b *device) {
			b.write("n.md", "new\n")
		},
		meanwhile: func(t *testing.T, h *node, b *device) {
			settle(t, h, b.node)
			b.remove("n.md")
			settle(t, h, b.node)
		},
		want: map[string]string{keep: "k\n"},
	}, {
		// a's edit merges on the hub with b's; a crashes applying the
		// merge, and b edits the merged line again. a's stale push of the
		// merge, from f.md's first revision, would conflict with b's edit.
		name:  "an answer",
		files: map[string]string{"f.md": five, keep: "k\n"},
		applying: func(t *testing.T, h *node, a *crashy, b *device) {
			b.write("f.md", "1b\n2\n3\n4\n5\n")
			settle(t, h, b.node)
			a.write("f.md", "1\n2\n3\n4\n5a\n")
		},
		meanwhile: func(t *testing.T, h *node, b *device) {
			settle(t, h, b.node)
			if got := b.read("f.md"); got != "1b\n2\n3\n4\n5a\n" {
				t.Fatalf("b's f.md after the merge = %q", got)
			}
			b.write("f.md", "1b\n2\n3\n4\n5b\n")
			settle(t, h, b.node)
		},
		want: map[string]string{"f.md": "1b\n2\n3\n4\n5b\n", keep: "k\n"},
	}, {
		// Another program edits the file a pulled while a is down: the
		// head is not what a holds after the restart, so a pushes its
		// edit from the agreed revision, and the hub merges it.
		name:  "a pulled put edited after the crash",
		files: map[string]string{"f.md": five, keep: "k\n"},
		applying: func(t *testing.T, h *node, a *crashy, b *device) {
			b.write("f.md", "1b\n2\n3\n4\n5\n")
		},
		meanwhile: func(t *testing.T, h *node, b *device) { settle(t, h, b.node) },
		onDisk: func(a *crashy) {
			if got := a.read("f.md"); got != "1b\n2\n3\n4\n5\n" {
				a.t.Fatalf("a's f.md after the crash = %q, want b's edit", got)
			}
			a.outside("f.md", "1b\n2\n3\n4\n5c\n")
		},
		want: map[string]string{"f.md": "1b\n2\n3\n4\n5c\n", keep: "k\n"},
	}}
	for _, tc := range cases {
		t.Run(strings.ReplaceAll(tc.name, " ", "_"), func(t *testing.T) {
			h := newHub(t, tc.files)
			a := newCrashy(t, h, "a")
			b := newPartitioned(t, h, "b", nil)
			settle(t, h, a.node, b.node)
			// a is cut off while the head is made, so the crash point is
			// armed before a pulls it (and after a's own write, which has
			// a rename of its own).
			a.net.cut()
			tc.applying(t, h, a, b)
			a.armed.Store(true)
			a.net.heal()
			a.srv.Syncer.Kick(a.vault)
			a.down()
			tc.meanwhile(t, h, b)
			if tc.onDisk != nil {
				tc.onDisk(a)
			} else if got := files(t, h.dir); !equalFiles(got, tc.want) {
				t.Fatalf("the hub while a is down = %q, want %q", got, tc.want)
			}
			a.up()
			settle(t, h, a.node, b.node)
			got := same(t, h, a.node, b.node)
			if !equalFiles(got, tc.want) {
				t.Fatalf("the vaults after a's restart = %q, want %q", got, tc.want)
			}
			noDeletePushed(t, a.node)
			noConflictRevision(t, h)
			if rows, err := a.srv.DB.Applying(t.Context(), a.vault); err != nil || len(rows) != 0 {
				t.Fatalf("a's heads being applied = %+v, %v; want none", rows, err)
			}
		})
	}
}

func equalFiles(a, b map[string]string) bool {
	if len(a) != len(b) {
		return false
	}
	for p, c := range a {
		if b[p] != c {
			return false
		}
	}
	return true
}
