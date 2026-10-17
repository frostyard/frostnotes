package synctest

import (
	"fmt"
	"maps"
	"math/rand/v2"
	"path"
	"slices"
	"strings"
	"unicode"

	"github.com/frostyard/frostnotes/api"
)

// Profile shapes a seeded run for a scenario (plan 0004, Phase 6): which
// ops it plans and how often, and checks of its own at each quiet point.
type Profile struct {
	// Weights are how often each op is planned, against the others; an op
	// without a weight is never planned. Nil plans the default weights.
	Weights map[Op]int
	// Check runs at each quiet point, after the world's own checks.
	Check func(w *World) error
}

// PlanWith makes the steps of a seed with weights, as Plan does with the
// default ones.
func PlanWith(seed uint64, devices, steps int, weights map[Op]int) []Step {
	if weights == nil {
		return Plan(seed, devices, steps)
	}
	ops := slices.Sorted(maps.Keys(weights))
	total := 0
	for _, op := range ops {
		total += weights[op]
	}
	rng := rand.New(rand.NewPCG(seed, 0x5eed))
	out := make([]Step, steps)
	for i := range out {
		r := rng.IntN(total)
		op := ops[0]
		for _, o := range ops {
			if r < weights[o] {
				op = o
				break
			}
			r -= weights[o]
		}
		out[i] = Step{Op: op, Device: rng.IntN(devices), A: rng.Uint64(), B: rng.Uint64()}
	}
	return out
}

// bigSize is a big attachment's size: over 10 MB, where the hub stops
// merging and the pushed content wins.
const bigSize = 10<<20 + 4096

// swap swaps two notes' paths on device d through notesd, by way of a
// third path, as a user renaming a pair does.
func (w *World) swap(d *Node, i int, s Step) error {
	p, q := pick(d, s.A, true), pick(d, s.B, true)
	if p == "" || p == q {
		return nil
	}
	tmp := path.Join(path.Dir(p), fmt.Sprintf("swap-%d.md", i))
	links := false
	for _, mv := range [][2]string{{p, tmp}, {q, p}, {tmp, q}} {
		if _, err := d.C.Note().Move(w.t.Context(), &api.NoteMoveParams{VaultID: d.Vault, Path: mv[0], To: mv[1],
			UpdateLinks: &links}); err != nil {
			return ignoreBenign(err)
		}
		w.paths[mv[1]] = true
	}
	return nil
}

// caseRename moves a note on device d to its own path with its name's
// first letter in the other case.
func (w *World) caseRename(d *Node, s Step) error {
	p := pick(d, s.A, true)
	if p == "" {
		return nil
	}
	base := path.Base(p)
	r := []rune(base)
	switch {
	case unicode.IsUpper(r[0]):
		r[0] = unicode.ToLower(r[0])
	case unicode.IsLower(r[0]):
		r[0] = unicode.ToUpper(r[0])
	default:
		return nil
	}
	to := strings.TrimSuffix(p, base) + string(r)
	links := true
	_, err := d.C.Note().Move(w.t.Context(), &api.NoteMoveParams{VaultID: d.Vault, Path: p, To: to, UpdateLinks: &links})
	if err == nil {
		w.paths[to] = true
	}
	return ignoreBenign(err)
}

// bigAttach writes one of two attachments over 10 MB on device d's disk.
func (w *World) bigAttach(d *Node, s Step) error {
	p := fmt.Sprintf("files/big%d.bin", s.A%2)
	if w.o.Concurrent {
		p = fmt.Sprintf("files/big-%s.bin", w.oracle.token())
	}
	tok := w.oracle.token()
	rng := rand.New(rand.NewPCG(s.B, 4))
	junk := make([]byte, bigSize)
	for i := range junk {
		junk[i] = byte(rng.Uint32())
	}
	old, had := files(d)[p]
	if err := writeFile(d, p, "\x00\xff"+tok+"\xff"+string(junk), false); err != nil {
		return err
	}
	w.oracle.insert(tok)
	w.oracle.wrote(tok, d.Name, p+" (big)")
	if had {
		w.oracle.remove(old)
	}
	w.paths[p] = true
	return w.index(d, p)
}

// revokeAndPair revokes device d on the hub, then pairs it again: what it
// wrote meanwhile syncs once it has.
func (w *World) revokeAndPair(d *Node) error {
	if d.link.isOffline() {
		return nil
	}
	if err := w.Revoke(d); err != nil {
		return err
	}
	return w.PairAgain(d)
}

// rejoin lets the world go quiet, then replaces device i with a new
// device that joins with the copy of its vault the last Snapshot step
// took, or with its vault as it is: a machine restored from a backup.
func (w *World) rejoin(i int) error {
	if err := w.settle(); err != nil {
		return err
	}
	old := w.Devices[i]
	vault, ok := w.snapshots[i]
	if !ok {
		vault = files(old)
	}
	w.RemoveDevice(old)
	d, err := w.AddDevice(old.Name+"'", vault)
	if err != nil {
		return err
	}
	// Back in its place, so later steps name it as they named the old one.
	w.Devices = slices.Insert(w.Devices[:len(w.Devices)-1], i, d)
	delete(w.snapshots, i)
	return nil
}
