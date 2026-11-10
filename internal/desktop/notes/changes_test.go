package notes

import (
	"math/rand/v2"
	"strings"
	"testing"
)

func mustChanges(t *testing.T, n int, cs ...Change) ChangeSet {
	t.Helper()
	set, err := ChangesOf(n, cs...)
	if err != nil {
		t.Fatal(err)
	}
	return set
}

func mustApply(t *testing.T, cs ChangeSet, s string) string {
	t.Helper()
	out, err := cs.Apply(s)
	if err != nil {
		t.Fatalf("apply %v to %q: %v", cs.Changes(), s, err)
	}
	return out
}

func TestChangesOfAndApply(t *testing.T) {
	doc := "hello world"
	cs := mustChanges(t, len(doc), Change{From: 0, To: 5, Insert: "goodbye"}, Change{From: 11, To: 11, Insert: "!"})
	if got := mustApply(t, cs, doc); got != "goodbye world!" {
		t.Fatalf("got %q", got)
	}
	if cs.Len() != 11 || cs.NewLen() != 14 {
		t.Fatalf("lengths %d %d", cs.Len(), cs.NewLen())
	}
	if _, err := ChangesOf(5, Change{From: 3, To: 4}, Change{From: 2, To: 2}); err == nil {
		t.Fatal("out of order changes made a set")
	}
	if !EmptyChanges(4).Empty() || !(ChangeSet{}).Empty() || cs.Empty() {
		t.Fatal("Empty")
	}
	if _, err := cs.Apply("short"); err == nil {
		t.Fatal("applied to a text of the wrong length")
	}
}

func TestInvert(t *testing.T) {
	doc := "one two three"
	cs := mustChanges(t, len(doc), Change{From: 4, To: 7, Insert: "2"}, Change{From: 13, To: 13, Insert: " four"})
	after := mustApply(t, cs, doc)
	if back := mustApply(t, cs.Invert(doc), after); back != doc {
		t.Fatalf("inverse made %q", back)
	}
}

func TestComposeAndMap(t *testing.T) {
	doc := "abc"
	a := mustChanges(t, 3, Change{From: 1, To: 1, Insert: "X"}) // aXbc
	b := mustChanges(t, 4, Change{From: 4, To: 4, Insert: "Y"}) // aXbcY
	ab, err := a.Compose(b)
	if err != nil {
		t.Fatal(err)
	}
	if got := mustApply(t, ab, doc); got != "aXbcY" {
		t.Fatalf("compose made %q", got)
	}
	// Two writers at once: one inserts at 1, the other deletes "c".
	other := mustChanges(t, 3, Change{From: 2, To: 3})
	mapped, err := a.Map(other, false)
	if err != nil {
		t.Fatal(err)
	}
	if got := mustApply(t, mapped, mustApply(t, other, doc)); got != "aXb" {
		t.Fatalf("map made %q", got)
	}
}

func TestMapPos(t *testing.T) {
	cs := mustChanges(t, 10, Change{From: 2, To: 4, Insert: "xyz"}, Change{From: 6, To: 6, Insert: "!!"})
	cases := []struct{ pos, assoc, want int }{
		{0, -1, 0}, {2, -1, 2}, {2, 1, 2}, {3, -1, 2}, {3, 1, 5}, {5, 1, 6},
		{6, -1, 7}, {6, 1, 9}, {10, 1, 13},
	}
	for _, c := range cases {
		if got := cs.MapPos(c.pos, c.assoc); got != c.want {
			t.Errorf("MapPos(%d, %d) = %d, want %d", c.pos, c.assoc, got, c.want)
		}
	}
}

func TestDiff(t *testing.T) {
	cases := [][2]string{
		{"", ""},
		{"", "new"},
		{"old", ""},
		{"the quick fox", "the quick brown fox"},
		{"a\nb\nc\n", "a\nB\nc\n"},
		{"a\nb\nc\nd\ne\n", "x\na\nc\nd\ny\ne\nz"},
		{"line one\nline two\n", "line one\nline 2\nline three\n"},
		{"héllo wörld", "hello world"},
	}
	for _, c := range cases {
		cs := Diff(c[0], c[1])
		if got := mustApply(t, cs, c[0]); got != c[1] {
			t.Errorf("Diff(%q, %q) applied makes %q", c[0], c[1], got)
		}
	}
	// A word typed in a long note is a change of that word.
	long := strings.Repeat("a line of the note\n", 500)
	edited := long[:4000] + "XYZ " + long[4000:]
	ch := Diff(long, edited).Changes()
	if len(ch) != 1 || ch[0].From != 4000 || ch[0].To != 4000 || ch[0].Insert != "XYZ " {
		t.Fatalf("typing diffs as %+v", ch)
	}
	// Lines moved apart stay separate changes.
	two := Diff("a\nb\nc\nd\ne\nf\n", "A\nb\nc\nd\ne\nF\n").Changes()
	if len(two) != 2 {
		t.Fatalf("two edits diff as %+v", two)
	}
}

// randomText is a few lines of a small alphabet, so random edits collide.
func randomText(r *rand.Rand) string {
	var b strings.Builder
	for range r.IntN(40) {
		switch r.IntN(6) {
		case 0:
			b.WriteByte('\n')
		default:
			b.WriteByte("abcde"[r.IntN(5)])
		}
	}
	return b.String()
}

func randomChanges(r *rand.Rand, n int) ChangeSet {
	var cs []Change
	pos := 0
	for pos < n && r.IntN(3) > 0 {
		from := pos + r.IntN(n-pos+1)
		to := from + r.IntN(n-from+1)
		if r.IntN(2) == 0 {
			to = from
		}
		ins := ""
		if r.IntN(3) > 0 {
			t := randomText(r)
			ins = t[:min(len(t), r.IntN(4))]
		}
		cs = append(cs, Change{From: from, To: to, Insert: ins})
		pos = to + 1
	}
	set, err := ChangesOf(n, cs...)
	if err != nil {
		panic(err)
	}
	return set
}

func TestChangeAlgebraRandom(t *testing.T) {
	r := rand.New(rand.NewPCG(1, 2))
	for i := range 3000 {
		doc := randomText(r)
		a := randomChanges(r, len(doc))
		b := randomChanges(r, len(doc))
		da := mustApply(t, a, doc)
		db := mustApply(t, b, doc)

		if back := mustApply(t, a.Invert(doc), da); back != doc {
			t.Fatalf("#%d invert", i)
		}
		next := randomChanges(r, len(da))
		comp, err := a.Compose(next)
		if err != nil {
			t.Fatalf("#%d compose: %v", i, err)
		}
		if mustApply(t, comp, doc) != mustApply(t, next, da) {
			t.Fatalf("#%d compose differs", i)
		}

		// Concurrent sets converge whichever is applied first.
		aOverB, err := a.Map(b, false)
		if err != nil {
			t.Fatalf("#%d map: %v", i, err)
		}
		bOverA, err := b.Map(a, true)
		if err != nil {
			t.Fatalf("#%d map: %v", i, err)
		}
		if x, y := mustApply(t, aOverB, db), mustApply(t, bOverA, da); x != y {
			t.Fatalf("#%d diverge: %q vs %q (doc %q, a %+v, b %+v)", i, x, y, doc, a.Changes(), b.Changes())
		}

		other := randomText(r)
		if mustApply(t, Diff(doc, other), doc) != other {
			t.Fatalf("#%d diff of %q and %q", i, doc, other)
		}
	}
}
