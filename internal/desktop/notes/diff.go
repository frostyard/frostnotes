package notes

import (
	"slices"
	"strings"
)

// diffBudget bounds the edit distance, in lines, that Diff searches for a
// minimal script; past it the differing middle becomes one change. A
// coarser change is still correct: it only maps another set's positions
// inside it less finely.
const diffBudget = 1000

// Diff returns the changes that make b from a: whole lines first (Myers'
// algorithm), then each changed run of lines trimmed to the bytes that
// differ, so a typed word is a change of a word.
func Diff(a, b string) ChangeSet {
	if a == b {
		return EmptyChanges(len(a))
	}
	p := commonPrefix(a, b)
	s := commonSuffix(a[p:], b[p:])
	ma, mb := a[p:len(a)-s], b[p:len(b)-s]
	var changes []Change
	if ma == "" || mb == "" || !strings.Contains(ma, "\n") && !strings.Contains(mb, "\n") {
		changes = []Change{{From: p, To: p + len(ma), Insert: mb}}
	} else {
		// Diff whole lines of the region the prefix and suffix leave,
		// widened to the lines they cut.
		ls := strings.LastIndexByte(a[:p], '\n') + 1
		le := len(a) - s
		if i := strings.IndexByte(a[le:], '\n'); i >= 0 {
			le += i + 1
		} else {
			le = len(a)
		}
		lsB, leB := ls, le+(len(b)-len(a))
		changes = diffLines(a[ls:le], b[lsB:leB], ls)
	}
	cs, err := ChangesOf(len(a), changes...)
	if err != nil {
		cs, _ = ChangesOf(len(a), Change{From: p, To: len(a) - s, Insert: mb})
	}
	return cs
}

func commonPrefix(a, b string) int {
	n := min(len(a), len(b))
	i := 0
	for i < n && a[i] == b[i] {
		i++
	}
	return i
}

func commonSuffix(a, b string) int {
	n := min(len(a), len(b))
	i := 0
	for i < n && a[len(a)-1-i] == b[len(b)-1-i] {
		i++
	}
	return i
}

// splitLines splits s into lines that keep their "\n", with each line's
// offset in s.
func splitLines(s string) (lines []string, starts []int) {
	for pos := 0; pos < len(s); {
		i := strings.IndexByte(s[pos:], '\n')
		end := len(s)
		if i >= 0 {
			end = pos + i + 1
		}
		lines = append(lines, s[pos:end])
		starts = append(starts, pos)
		pos = end
	}
	return lines, starts
}

// diffLines diffs a and b by lines; off is a's offset in the whole text.
func diffLines(a, b string, off int) []Change {
	la, sa := splitLines(a)
	lb, sb := splitLines(b)
	ids := map[string]int{}
	id := func(lines []string) []int {
		out := make([]int, len(lines))
		for i, l := range lines {
			v, ok := ids[l]
			if !ok {
				v = len(ids)
				ids[l] = v
			}
			out[i] = v
		}
		return out
	}
	hunks, ok := myers(id(la), id(lb), diffBudget)
	if !ok {
		hunks = []hunk{{0, len(la), 0, len(lb)}}
	}
	at := func(starts []int, s string, i int) int {
		if i < len(starts) {
			return starts[i]
		}
		return len(s)
	}
	var out []Change
	for _, h := range hunks {
		fa, ta := at(sa, a, h.a0), at(sa, a, h.a1)
		fb, tb := at(sb, b, h.b0), at(sb, b, h.b1)
		x, y := a[fa:ta], b[fb:tb]
		p := commonPrefix(x, y)
		s := commonSuffix(x[p:], y[p:])
		out = append(out, Change{From: off + fa + p, To: off + ta - s, Insert: y[p : len(y)-s]})
	}
	return out
}

// hunk is a run of lines a0 to a1 of a replaced by b0 to b1 of b.
type hunk struct{ a0, a1, b0, b1 int }

// myers finds the shortest edit script from a to b by Myers' O(ND)
// algorithm and returns its runs of change; ok is false past maxD edits.
func myers(a, b []int, maxD int) (hunks []hunk, ok bool) {
	n, m := len(a), len(b)
	maxD = min(maxD, n+m)
	off := maxD + 1
	v := make([]int, 2*maxD+3)
	var trace [][]int
	for d := 0; d <= maxD; d++ {
		snapshot := make([]int, 2*d+1)
		for k := -d; k <= d; k += 2 {
			var x int
			if k == -d || k != d && v[off+k-1] < v[off+k+1] {
				x = v[off+k+1]
			} else {
				x = v[off+k-1] + 1
			}
			y := x - k
			for x < n && y < m && a[x] == b[y] {
				x++
				y++
			}
			v[off+k] = x
			if x >= n && y >= m {
				return backtrack(trace, n, m, d), true
			}
		}
		copy(snapshot, v[off-d:off+d+1])
		trace = append(trace, snapshot)
	}
	return nil, false
}

// backtrack walks the trace from (n, m) back to (0, 0) and collects the
// runs of non-matching lines, in order.
func backtrack(trace [][]int, n, m, d int) []hunk {
	type step struct{ x, y, px, py int }
	var steps []step // each d's move, from the last to the first
	x, y := n, m
	for ; d > 0; d-- {
		prev := trace[d-1] // indexed by k + (d-1)
		k := x - y
		get := func(k int) int { return prev[k+d-1] }
		var pk int
		if k == -d || k != d && get(k-1) < get(k+1) {
			pk = k + 1
		} else {
			pk = k - 1
		}
		px := get(pk)
		py := px - pk
		// The snake from the move's end back to (x, y) matches; the move
		// itself is (px, py) to (mx, my).
		mx, my := px, py
		if pk == k+1 {
			my++ // an insertion of b[py]
		} else {
			mx++ // a deletion of a[px]
		}
		steps = append(steps, step{mx, my, px, py})
		x, y = px, py
	}
	var hunks []hunk
	for _, s := range slices.Backward(steps) {
		if len(hunks) > 0 {
			h := &hunks[len(hunks)-1]
			if h.a1 == s.px && h.b1 == s.py {
				h.a1, h.b1 = s.x, s.y
				continue
			}
		}
		hunks = append(hunks, hunk{s.px, s.x, s.py, s.y})
	}
	return hunks
}
