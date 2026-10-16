package merge

// lcs matches a against b along a common subsequence: m[i] is the index in
// b that a[i] is matched to, or -1. Matches increase strictly.
func lcs(a, b []string) []int {
	m := make([]int, len(a))
	for i := range m {
		m[i] = -1
	}
	match(a, b, 0, 0, m)
	return m
}

// match records in m the matches between a and b, which start at offsets
// ao and bo of the full sequences. It trims the common prefix and suffix,
// then splits the rest at the middle snake of Myers' linear-space
// algorithm and recurses, so memory stays linear in the input.
func match(a, b []string, ao, bo int, m []int) {
	p := 0
	for p < len(a) && p < len(b) && a[p] == b[p] {
		m[ao+p] = bo + p
		p++
	}
	a, b, ao, bo = a[p:], b[p:], ao+p, bo+p
	s := 0
	for s < len(a) && s < len(b) && a[len(a)-1-s] == b[len(b)-1-s] {
		m[ao+len(a)-1-s] = bo + len(b) - 1 - s
		s++
	}
	a, b = a[:len(a)-s], b[:len(b)-s]
	if len(a) == 0 || len(b) == 0 {
		return
	}
	if x, y, ok := bisect(a, b); ok {
		match(a[:x], b[:y], ao, bo, m)
		match(a[x:], b[y:], ao+x, bo+y, m)
	}
}

// bisect finds where a forward and a backward shortest edit path meet
// (Myers 1986, section 4b; the formulation of diff-match-patch's
// diff_bisect). It reports false when a and b share nothing.
func bisect(a, b []string) (x, y int, ok bool) {
	n, m := len(a), len(b)
	maxD := (n + m + 1) / 2
	off, size := maxD, 2*maxD+2
	vf, vb := make([]int, size), make([]int, size)
	for i := range vf {
		vf[i], vb[i] = -1, -1
	}
	vf[off+1], vb[off+1] = 0, 0
	delta := n - m
	front := delta%2 != 0
	k1start, k1end, k2start, k2end := 0, 0, 0, 0
	for d := 0; d < maxD; d++ {
		for k1 := -d + k1start; k1 <= d-k1end; k1 += 2 {
			i := off + k1
			var x1 int
			if k1 == -d || (k1 != d && vf[i-1] < vf[i+1]) {
				x1 = vf[i+1]
			} else {
				x1 = vf[i-1] + 1
			}
			y1 := x1 - k1
			for x1 < n && y1 < m && a[x1] == b[y1] {
				x1++
				y1++
			}
			vf[i] = x1
			switch {
			case x1 > n:
				k1end += 2
			case y1 > m:
				k1start += 2
			case front:
				if j := off + delta - k1; j >= 0 && j < size && vb[j] != -1 && x1 >= n-vb[j] {
					return x1, y1, true
				}
			}
		}
		for k2 := -d + k2start; k2 <= d-k2end; k2 += 2 {
			i := off + k2
			var x2 int
			if k2 == -d || (k2 != d && vb[i-1] < vb[i+1]) {
				x2 = vb[i+1]
			} else {
				x2 = vb[i-1] + 1
			}
			y2 := x2 - k2
			for x2 < n && y2 < m && a[n-x2-1] == b[m-y2-1] {
				x2++
				y2++
			}
			vb[i] = x2
			switch {
			case x2 > n:
				k2end += 2
			case y2 > m:
				k2start += 2
			case !front:
				if j := off + delta - k2; j >= 0 && j < size && vf[j] != -1 {
					x1 := vf[j]
					if y1 := off + x1 - j; x1 >= n-x2 {
						return x1, y1, true
					}
				}
			}
		}
	}
	return 0, 0, false
}

// chunk is a region of a three-way comparison: stable where base, head and
// incoming agree, else the three differing slices.
type chunk struct {
	stable               bool
	base, head, incoming []string
}

// diff3 splits three sequences into stable and unstable chunks, anchored on
// the elements of base that both head and incoming kept.
func diff3(base, head, incoming []string) []chunk {
	mh, mi := lcs(base, head), lcs(base, incoming)
	var out []chunk
	i, j, k := 0, 0, 0
	for {
		s := i
		for i < len(base) && mh[i] == j && mi[i] == k {
			i++
			j++
			k++
		}
		if i > s {
			out = append(out, chunk{stable: true, base: base[s:i], head: base[s:i], incoming: base[s:i]})
		}
		a := i
		for a < len(base) && (mh[a] < 0 || mi[a] < 0) {
			a++
		}
		jh, ki := len(head), len(incoming)
		if a < len(base) {
			jh, ki = mh[a], mi[a]
		}
		if a == i && jh == j && ki == k {
			if a == len(base) {
				return out
			}
			continue
		}
		out = append(out, chunk{base: base[i:a], head: head[j:jh], incoming: incoming[k:ki]})
		i, j, k = a, jh, ki
	}
}
