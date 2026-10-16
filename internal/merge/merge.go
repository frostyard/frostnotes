// Package merge merges three versions of a text file (ADR-0005,
// docs/design/sync.md#merge): the base both sides started from, the head
// (the hub's current version, or the version on disk for a local merge) and
// the incoming version. The head wins whatever still conflicts, and every
// conflict is reported so the caller can keep the incoming version as a
// conflict revision. Merges are deterministic and byte-exact: no input is
// normalized, and text that only one side touched keeps its bytes.
//
// This is the M0 shape (plan 0001, Phase 5); M2's specs/merge.md is the
// contract.
package merge

import (
	"strings"
	"unicode"
)

// Result is a merge's output.
type Result struct {
	// Text is the merged file.
	Text []byte
	// Conflicts are the hunks where both sides changed the same text
	// differently; Text holds the head's side of each.
	Conflicts []Conflict
}

// Conflict is one hunk the head won.
type Conflict struct {
	// Line is the 1-based line of Text where the head's side starts.
	Line int
	// Base, Head and Incoming are the hunk's three versions.
	Base, Head, Incoming string
}

// Text merges plain text by line. A hunk both sides changed differently
// merges again by word; where both sides only inserted lines at the same
// place, both insertions are kept, the head's first.
func Text(base, head, incoming []byte) Result {
	var m merger
	m.text(string(base), string(head), string(incoming))
	return m.result()
}

type merger struct {
	out       strings.Builder
	conflicts []Conflict
}

func (m *merger) result() Result {
	return Result{Text: []byte(m.out.String()), Conflicts: m.conflicts}
}

// line is the 1-based line the next output byte lands on.
func (m *merger) line() int { return strings.Count(m.out.String(), "\n") + 1 }

func (m *merger) text(base, head, incoming string) {
	for _, c := range diff3(lines(base), lines(head), lines(incoming)) {
		switch {
		case c.stable, equal(c.head, c.incoming), equal(c.incoming, c.base):
			m.write(c.head)
		case equal(c.head, c.base):
			m.write(c.incoming)
		case len(c.base) == 0:
			m.write(c.head)
			m.write(c.incoming)
		default:
			m.words(strings.Join(c.base, ""), strings.Join(c.head, ""), strings.Join(c.incoming, ""))
		}
	}
}

// words merges one conflicting line hunk by word. Insertions at the same
// place conflict here: interleaving words reads worse than losing them to
// a conflict revision.
func (m *merger) words(base, head, incoming string) {
	start := m.line()
	conflict := false
	for _, c := range diff3(words(base), words(head), words(incoming)) {
		switch {
		case c.stable, equal(c.head, c.incoming), equal(c.incoming, c.base):
			m.write(c.head)
		case equal(c.head, c.base):
			m.write(c.incoming)
		default:
			m.write(c.head)
			conflict = true
		}
	}
	if conflict {
		m.conflicts = append(m.conflicts, Conflict{Line: start, Base: base, Head: head, Incoming: incoming})
	}
}

func (m *merger) write(parts []string) {
	for _, p := range parts {
		m.out.WriteString(p)
	}
}

func equal(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

// lines splits s after each '\n'; the last line may lack one.
func lines(s string) []string {
	var out []string
	for s != "" {
		i := strings.IndexByte(s, '\n') + 1
		if i == 0 {
			i = len(s)
		}
		out = append(out, s[:i])
		s = s[i:]
	}
	return out
}

// words splits s into alternating runs of space and non-space, so joining
// them restores s exactly.
func words(s string) []string {
	var out []string
	start, space := 0, false
	for i, r := range s {
		sp := unicode.IsSpace(r)
		if i > start && sp != space {
			out = append(out, s[start:i])
			start = i
		}
		if i == start {
			space = sp
		}
	}
	if start < len(s) {
		out = append(out, s[start:])
	}
	return out
}
