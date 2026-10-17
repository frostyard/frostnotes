package synctest

import (
	"errors"
	"fmt"
	"math/rand/v2"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"testing"

	"github.com/frostyard/frostnotes/api"
)

// Op is what a step does.
type Op int

// The steps a plan is made of (ADR-0020).
const (
	Edit       Op = iota // an editor's write with the current base
	StaleEdit            // an editor's write with an older base: a local merge
	Append               // an agent's append
	Outside              // another program's edit on disk
	Create               // a new note
	Daily                // today's note, a path two devices may create offline
	Delete               // a delete with the current base
	Move                 // a move through notesd
	DiskMove             // a rename on disk
	Attach               // an attachment that is not text, written on disk
	Offline              // the device's link cut
	Online               // the device's link healed
	Crash                // a crash at a point, then a restart
	HubRestart           // the hub stopped and started again
	HubCrash             // the hub crashing after a push's rename
	DropAnswer           // the hub losing a push's answer
	Reset                // sync.reset
	// The ops below plan only in a Profile that weighs them, so the default
	// plans stay what they were.
	Swap          // two notes swap paths through notesd
	CaseRename    // a note's name changes only in case
	BigAttach     // an attachment over 10 MB, written on disk
	RevokeAndPair // the hub revokes the device, which pairs again
	Snapshot      // the device's vault copied, for a later Rejoin
	Rejoin        // the device replaced by a new one joining with its copy
	numOps
)

var opNames = [...]string{"edit", "stale edit", "append", "outside edit", "create", "daily", "delete", "move", "disk move",
	"attach", "offline", "online", "crash", "hub restart", "hub crash", "drop answer", "reset", "swap", "case rename",
	"big attach", "revoke and pair", "snapshot", "rejoin"}

func (o Op) String() string { return opNames[o] }

// weights is how often each op is planned.
var weights = [numOps]int{Edit: 22, StaleEdit: 6, Append: 8, Outside: 8, Create: 10, Daily: 4, Delete: 5, Move: 5,
	DiskMove: 3, Attach: 4, Offline: 6, Online: 8, Crash: 3, HubRestart: 1, HubCrash: 1, DropAnswer: 2, Reset: 1}

// Step is one planned step. A and B are choices, resolved against the
// world as the step runs (the k-th file by path, a line), so a plan with
// steps dropped still runs: what the reducer needs.
type Step struct {
	Op     Op
	Device int
	A, B   uint64
}

func (s Step) String() string {
	return fmt.Sprintf("%s on device %d (%d, %d)", s.Op, s.Device, s.A, s.B)
}

// Plan makes the steps of a seed.
func Plan(seed uint64, devices, steps int) []Step {
	rng := rand.New(rand.NewPCG(seed, 0x5eed))
	total := 0
	for _, w := range weights {
		total += w
	}
	out := make([]Step, steps)
	for i := range out {
		r := rng.IntN(total)
		op := Op(0)
		for ; r >= weights[op]; op++ {
			r -= weights[op]
		}
		out[i] = Step{Op: op, Device: rng.IntN(devices), A: rng.Uint64(), B: rng.Uint64()}
	}
	return out
}

// oracle tracks tokens: every write inserts one, and a writer that removes
// a line it saw, or deletes a file, removes those on purpose.
type oracle struct {
	prefix   string
	next     int
	inserted map[string]bool
	removed  map[string]bool
	origin   map[string]string // where each token was written, for a report
	at       map[string]string // the path each token was written at
	// tried lists, for a report, the writes that took a token's line out
	// and failed.
	tried map[string][]string
	step  int
}

var tokenRE = regexp.MustCompile(`tok[0-9a-f]+-[0-9]+`)

func (o *oracle) token() string {
	o.next++
	return fmt.Sprintf("%s-%d", o.prefix, o.next)
}

func (o *oracle) insert(tok string) { o.inserted[tok] = true }

// wrote records where tok was written.
func (o *oracle) wrote(tok, device, p string) {
	o.origin[tok] = fmt.Sprintf("step %d on %s at %s", o.step, device, p)
	o.at[tok], _, _ = strings.Cut(p, " (")
}

func (o *oracle) remove(content string) {
	for _, tok := range tokenRE.FindAllString(content, -1) {
		o.removed[tok] = true
	}
}

// World is a hub and devices syncing one vault.
type World struct {
	t       testing.TB
	o       Options
	Hub     *Node
	Devices []*Node
	oracle  oracle
	// paths are the files' paths the steps made: created, moved to, or
	// seeded. A head at any other path, but for the protocol's "name N"
	// variants, is a conflict copy.
	paths map[string]bool
	trace []string
	// lagging are outside edits a step left for the watcher to index; a
	// quiet point indexes them first, since they are changes to push.
	lagging []lag
	// snapshots are copies of devices' vaults, by index, for Rejoin.
	snapshots map[int]map[string]string
}

type lag struct {
	d *Node
	p string
}

// catchUp indexes the lagging edits of the devices in w.Devices.
func (w *World) catchUp() error {
	keep := w.lagging[:0]
	for _, l := range w.lagging {
		if !slices.Contains(w.Devices, l.d) {
			keep = append(keep, l)
			continue
		}
		if err := w.index(l.d, l.p); err != nil {
			return err
		}
	}
	w.lagging = keep
	return nil
}

// seed files the hub's vault starts with.
var seedFiles = map[string]string{
	"Inbox.md":         "# Inbox\n\n- start\n",
	"Projects/Plan.md": "---\ntags: [work]\nstatus: draft\n---\n# Plan\n\none\ntwo\nthree\n",
	"Journal/day.md":   "# A day\n\nIt rained.\n",
	"attachment.bin":   "\x00\xffseed attachment",
}

// NewWorld starts a hub serving the seed files and o.Devices devices that
// have joined it.
func NewWorld(t testing.TB, o Options) *World {
	t.Helper()
	o = o.withDefaults()
	w := &World{t: t, o: o, oracle: oracle{prefix: fmt.Sprintf("tok%x", o.Seed), inserted: map[string]bool{},
		removed: map[string]bool{}, origin: map[string]string{}, at: map[string]string{}, tried: map[string][]string{}}, paths: map[string]bool{},
		snapshots: map[int]map[string]string{}}
	w.Hub = newNode(t, "hub", o, true, freeAddr(t))
	for p, c := range seedFiles {
		full := filepath.Join(w.Hub.Dir, filepath.FromSlash(p))
		if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(full, []byte(c), 0o644); err != nil {
			t.Fatal(err)
		}
		w.paths[p] = true
	}
	w.Hub.start(t)
	w.Hub.probe(t)
	for p := range seedFiles {
		// A read brings the index up to date with the file.
		if _, err := w.Hub.C.Note().Read(t.Context(), &api.NoteReadParams{VaultID: w.Hub.Vault, Path: p}); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := w.Hub.C.Hub().Serve(t.Context(), &api.HubServeParams{VaultID: w.Hub.Vault}); err != nil {
		t.Fatal(err)
	}
	for i := range o.Devices {
		d := newNode(t, fmt.Sprintf("d%d", i), o, false, "")
		d.start(t)
		d.probe(t)
		if err := w.pair(d); err != nil {
			t.Fatal(err)
		}
		w.Devices = append(w.Devices, d)
	}
	if err := w.settle(); err != nil {
		t.Fatal(err)
	}
	return w
}

// Close stops every notesd of the world.
func (w *World) Close() {
	for _, n := range append([]*Node{w.Hub}, w.Devices...) {
		n.stop()
	}
}

// Run makes the world, runs plan's steps in it and returns the first
// violated check, or nil.
func Run(t testing.TB, o Options, plan []Step) (err error) {
	t.Helper()
	o = o.withDefaults()
	w := NewWorld(t, o)
	defer w.Close()
	defer func() {
		if err != nil {
			err = fmt.Errorf("%w\nthe last steps:\n  %s", err, strings.Join(w.trace[max(0, len(w.trace)-15):], "\n  "))
		}
	}()
	for i, s := range plan {
		w.trace = append(w.trace, fmt.Sprintf("%d: %s", i, s))
		w.oracle.step = i
		if err := w.step(i, s); err != nil {
			return fmt.Errorf("step %d (%s): %w", i, s, err)
		}
		if !o.Concurrent {
			if err := w.settleAfter(s); err != nil {
				return fmt.Errorf("step %d (%s): %w", i, s, err)
			}
		}
		if o.Watch != "" {
			w.watch(t, i, s)
		}
		if (i+1)%o.CheckEvery == 0 {
			if err := w.Check(); err != nil {
				return fmt.Errorf("after step %d (%s): %w", i, s, err)
			}
		}
	}
	if err := w.Check(); err != nil {
		return err
	}
	if err := w.CheckIndexes(); err != nil {
		return err
	}
	seq, _ := w.hubSeq()
	t.Logf("seed %d (%s): %d steps; %d tokens written, %d removed on purpose; the hub's log at %d, %d files",
		o.Seed, map[bool]string{false: "stepwise", true: "concurrent"}[o.Concurrent], len(plan), len(w.oracle.inserted),
		len(w.oracle.removed), seq, len(files(w.Hub)))
	return nil
}

// settleAfter lets the world go quiet after a stepwise step, but leaves
// cut links cut: the devices behind them sync when they come back.
func (w *World) settleAfter(Step) error {
	var online []*Node
	for _, d := range w.Devices {
		if !d.link.isOffline() {
			online = append(online, d)
		}
	}
	if len(online) == len(w.Devices) {
		return w.settle()
	}
	keep := w.Devices
	w.Devices = online
	defer func() { w.Devices = keep }()
	if len(online) == 0 {
		return nil
	}
	return w.settleOnline()
}

// settleOnline is settle for the devices whose links are up, healing none.
func (w *World) settleOnline() error {
	if err := w.catchUp(); err != nil {
		return err
	}
	for _, d := range w.Devices {
		d.Srv.Syncer.Kick(d.Vault)
	}
	seq, err := w.hubSeq()
	if err != nil {
		return err
	}
	for range 200 {
		quiet := true
		for _, d := range w.Devices {
			if err := d.Srv.Syncer.WaitIdle(w.t.Context(), d.Vault); err != nil {
				return err
			}
		}
		after, err := w.hubSeq()
		if err != nil {
			return err
		}
		for _, d := range w.Devices {
			st, err := d.C.Sync().Status(w.t.Context(), &api.SyncStatusParams{VaultID: &d.Vault})
			if err != nil {
				return err
			}
			if st[0].Pending != 0 || st[0].Cursor < after {
				quiet = false
			}
		}
		if quiet && after == seq {
			return nil
		}
		seq = after
	}
	return errors.New("the online devices did not go quiet")
}

func (w *World) watch(t testing.TB, i int, s Step) {
	line := fmt.Sprintf("after %d (%s):", i, s)
	for _, n := range append([]*Node{w.Hub}, w.Devices...) {
		if strings.HasPrefix(w.o.Watch, "tok") {
			// A token: every file holding it.
			var at []string
			for p, c := range files(n) {
				if strings.Contains(c, w.o.Watch) {
					at = append(at, p)
				}
			}
			line += fmt.Sprintf("\n    %s: %v", n.Name, at)
			continue
		}
		c, ok := files(n)[w.o.Watch]
		if !ok {
			c = "(none)"
		}
		line += fmt.Sprintf("\n    %s: %q", n.Name, c)
	}
	t.Log(line)
}
