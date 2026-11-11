package notes

import (
	"errors"
	"strings"
)

// ChangeSet replaces ranges of a text with other text, as CodeMirror's
// ChangeSet does: an edit, a merge, another writer's change. It is a
// value; its methods return new sets. Offsets count bytes of UTF-8, as
// MyGo's editor buffer does.
//
// sections holds pairs of (len, ins): a span of len bytes of the old text
// that is kept (ins == -1) or replaced by ins bytes (ins >= 0), whose
// text is inserted[i/2] for the pair at i. inserted may be shorter than
// the pairs; a missing entry is "".
type ChangeSet struct {
	sections []int
	inserted []string
}

// Change is one replacement, in the coordinates of the text a set
// applies to: From to To is replaced by Insert.
type Change struct {
	From, To int
	Insert   string
}

// errMismatch means two sets were combined that do not apply to texts of
// the lengths they should.
var errMismatch = errors.New("notes: change sets of mismatched lengths")

// EmptyChanges is the set that keeps a text of n bytes as it is.
func EmptyChanges(n int) ChangeSet {
	if n == 0 {
		return ChangeSet{}
	}
	return ChangeSet{sections: []int{n, -1}}
}

// ChangesOf makes the set of changes, sorted and not overlapping, on a
// text of n bytes. Changes out of order or overlapping are an error.
func ChangesOf(n int, changes ...Change) (ChangeSet, error) {
	var cs ChangeSet
	pos := 0
	for _, c := range changes {
		if c.From < pos || c.To < c.From || c.To > n {
			return ChangeSet{}, errors.New("notes: changes out of order or out of the text")
		}
		addSection(&cs.sections, c.From-pos, -1, false)
		addSection(&cs.sections, c.To-c.From, len(c.Insert), false)
		addInsert(&cs.inserted, cs.sections, c.Insert)
		pos = c.To
	}
	addSection(&cs.sections, n-pos, -1, false)
	return cs, nil
}

// Len is the length of the text the set applies to.
func (cs ChangeSet) Len() int {
	n := 0
	for i := 0; i < len(cs.sections); i += 2 {
		n += cs.sections[i]
	}
	return n
}

// NewLen is the length of the text the set makes.
func (cs ChangeSet) NewLen() int {
	n := 0
	for i := 0; i < len(cs.sections); i += 2 {
		if ins := cs.sections[i+1]; ins < 0 {
			n += cs.sections[i]
		} else {
			n += ins
		}
	}
	return n
}

// Empty reports whether the set changes nothing.
func (cs ChangeSet) Empty() bool {
	return len(cs.sections) == 0 || len(cs.sections) == 2 && cs.sections[1] < 0
}

func (cs ChangeSet) text(i int) string {
	if i < len(cs.inserted) {
		return cs.inserted[i]
	}
	return ""
}

// Changes lists the set's replacements, in the coordinates of the text it
// applies to.
func (cs ChangeSet) Changes() []Change {
	var out []Change
	pos := 0
	for i := 0; i < len(cs.sections); i += 2 {
		l, ins := cs.sections[i], cs.sections[i+1]
		if ins >= 0 {
			out = append(out, Change{From: pos, To: pos + l, Insert: cs.text(i / 2)})
		}
		pos += l
	}
	return out
}

// Apply returns text with the set's changes. text must be Len bytes long.
func (cs ChangeSet) Apply(text string) (string, error) {
	if len(text) != cs.Len() {
		return "", errMismatch
	}
	if cs.Empty() {
		return text, nil
	}
	var b strings.Builder
	b.Grow(cs.NewLen())
	pos := 0
	for i := 0; i < len(cs.sections); i += 2 {
		l, ins := cs.sections[i], cs.sections[i+1]
		if ins < 0 {
			b.WriteString(text[pos : pos+l])
		} else {
			b.WriteString(cs.text(i / 2))
		}
		pos += l
	}
	return b.String(), nil
}

// Compose returns the set that applies cs, then next; next applies to the
// text cs makes.
func (cs ChangeSet) Compose(next ChangeSet) (ChangeSet, error) {
	if cs.NewLen() != next.Len() {
		return ChangeSet{}, errMismatch
	}
	return composeSets(cs, next)
}

// Map returns cs moved over other, so it applies to the text other makes:
// both apply to the same text. Where both insert at one place, before
// puts cs's insertion before other's.
func (cs ChangeSet) Map(other ChangeSet, before bool) (ChangeSet, error) {
	if cs.Len() != other.Len() {
		return ChangeSet{}, errMismatch
	}
	return mapSet(cs, other, before)
}

// MapPos returns where pos, an offset in the text cs applies to, is in the
// text it makes. assoc < 0 keeps a position at an insertion before it,
// assoc > 0 after it.
func (cs ChangeSet) MapPos(pos, assoc int) int {
	posA, posB := 0, 0
	for i := 0; i < len(cs.sections); i += 2 {
		l, ins := cs.sections[i], cs.sections[i+1]
		endA := posA + l
		if ins < 0 {
			if endA > pos {
				return posB + (pos - posA)
			}
			posB += l
		} else {
			if endA > pos || endA == pos && assoc < 0 && l == 0 {
				if pos == posA || assoc < 0 {
					return posB
				}
				return posB + ins
			}
			posB += ins
		}
		posA = endA
	}
	return posB + max(pos-posA, 0)
}

// sectionIter walks a set's sections, splitting them as the other set
// demands.
type sectionIter struct {
	set ChangeSet
	i   int
	len int
	ins int
	off int
}

func newSectionIter(set ChangeSet) *sectionIter {
	it := &sectionIter{set: set}
	it.next()
	return it
}

func (it *sectionIter) next() {
	if it.i < len(it.set.sections) {
		it.len = it.set.sections[it.i]
		it.ins = it.set.sections[it.i+1]
		it.i += 2
	} else {
		it.len, it.ins = 0, -2
	}
	it.off = 0
}

func (it *sectionIter) done() bool { return it.ins == -2 }

func (it *sectionIter) len2() int {
	if it.ins < 0 {
		return it.len
	}
	return it.ins
}

func (it *sectionIter) text() string { return it.set.text((it.i - 2) / 2) }

func (it *sectionIter) textBit(n int) string {
	t := it.text()
	end := min(it.off+n, len(t))
	return t[min(it.off, len(t)):end]
}

func (it *sectionIter) forward(n int) {
	if n == it.len {
		it.next()
	} else {
		it.len -= n
		it.off += n
	}
}

func (it *sectionIter) forward2(n int) {
	switch {
	case it.ins == -1:
		it.forward(n)
	case n == it.ins:
		it.next()
	default:
		it.ins -= n
		it.off += n
	}
}

func addSection(sections *[]int, l, ins int, forceJoin bool) {
	if l == 0 && ins <= 0 {
		return
	}
	s := *sections
	last := len(s) - 2
	switch {
	case last >= 0 && ins <= 0 && ins == s[last+1]:
		s[last] += l
	case last >= 0 && l == 0 && s[last] == 0:
		s[last+1] += ins
	case forceJoin && last >= 0:
		s[last] += l
		s[last+1] += ins
	default:
		s = append(s, l, ins)
	}
	*sections = s
}

func addInsert(values *[]string, sections []int, value string) {
	if value == "" {
		return
	}
	index := (len(sections) - 2) / 2
	v := *values
	if index < len(v) {
		v[len(v)-1] += value
	} else {
		for len(v) < index {
			v = append(v, "")
		}
		v = append(v, value)
	}
	*values = v
}

func composeSets(setA, setB ChangeSet) (ChangeSet, error) {
	var out ChangeSet
	a, b := newSectionIter(setA), newSectionIter(setB)
	open := false
	for {
		switch {
		case a.done() && b.done():
			return out, nil
		case a.ins == 0: // a deletion in A
			addSection(&out.sections, a.len, 0, open)
			a.next()
		case b.len == 0 && !b.done(): // an insertion in B
			addSection(&out.sections, 0, b.ins, open)
			addInsert(&out.inserted, out.sections, b.text())
			b.next()
		case a.done() || b.done():
			return ChangeSet{}, errMismatch
		default:
			n := min(a.len2(), b.len)
			sectionLen := len(out.sections)
			switch {
			case a.ins == -1:
				insB := b.ins
				if b.ins != -1 && b.off != 0 {
					insB = 0
				}
				addSection(&out.sections, n, insB, open)
				if insB > 0 {
					addInsert(&out.inserted, out.sections, b.text())
				}
			case b.ins == -1:
				l := a.len
				if a.off != 0 {
					l = 0
				}
				addSection(&out.sections, l, n, open)
				addInsert(&out.inserted, out.sections, a.textBit(n))
			default:
				l, ins := a.len, b.ins
				if a.off != 0 {
					l = 0
				}
				if b.off != 0 {
					ins = 0
				}
				addSection(&out.sections, l, ins, open)
				if b.off == 0 {
					addInsert(&out.inserted, out.sections, b.text())
				}
			}
			open = (a.ins > n || b.ins >= 0 && b.len > n) && (open || len(out.sections) > sectionLen)
			a.forward2(n)
			b.forward(n)
		}
	}
}

func mapSet(setA, setB ChangeSet, before bool) (ChangeSet, error) {
	var out ChangeSet
	a, b := newSectionIter(setA), newSectionIter(setB)
	inserted := -1
	for {
		switch {
		case a.done() && b.len != 0 || b.done() && a.len != 0:
			return ChangeSet{}, errMismatch
		case a.ins == -1 && b.ins == -1:
			n := min(a.len, b.len)
			addSection(&out.sections, n, -1, false)
			a.forward(n)
			b.forward(n)
		case b.ins >= 0 && (a.ins < 0 || inserted == a.i || a.off == 0 && (b.len < a.len || b.len == a.len && !before)):
			// A change in B comes before the next change in A: skip it,
			// with what of A it covers.
			n := b.len
			addSection(&out.sections, b.ins, -1, false)
			for n > 0 {
				piece := min(a.len, n)
				if a.ins >= 0 && inserted < a.i && a.len <= piece {
					addSection(&out.sections, 0, a.ins, false)
					addInsert(&out.inserted, out.sections, a.text())
					inserted = a.i
				}
				a.forward(piece)
				n -= piece
			}
			b.next()
		case a.ins >= 0:
			// The part of a change in A up to the next change in B that
			// is not a deletion.
			n, left := 0, a.len
			for left > 0 {
				if b.ins == -1 {
					piece := min(left, b.len)
					n += piece
					left -= piece
					b.forward(piece)
				} else if b.ins == 0 && b.len < left {
					left -= b.len
					b.next()
				} else {
					break
				}
			}
			ins := 0
			if inserted < a.i {
				ins = a.ins
			}
			addSection(&out.sections, n, ins, false)
			if inserted < a.i {
				addInsert(&out.inserted, out.sections, a.text())
			}
			inserted = a.i
			a.forward(a.len - left)
		case a.done() && b.done():
			return out, nil
		default:
			return ChangeSet{}, errMismatch
		}
	}
}
