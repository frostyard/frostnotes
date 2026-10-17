package merge

import (
	"fmt"
	"math/rand/v2"
	"strings"
	"testing"
)

// Edits to different lines always merge cleanly and keep every edit; the
// same three inputs always give the same bytes.
func TestDisjointEditsMergeCleanly(t *testing.T) {
	for seed := range uint64(300) {
		rng := rand.New(rand.NewPCG(seed, 7))
		n := 5 + rng.IntN(60)
		base := make([]string, n)
		for i := range base {
			base[i] = fmt.Sprintf("line %d of the note\n", i)
		}
		head, incoming := append([]string(nil), base...), append([]string(nil), base...)
		var want []string
		for i := range base {
			switch rng.IntN(6) {
			case 0:
				head[i] = fmt.Sprintf("head edited line %d\n", i)
				want = append(want, head[i])
			case 1:
				incoming[i] = fmt.Sprintf("incoming edited line %d\n", i)
				want = append(want, incoming[i])
			}
		}
		b, h, in := strings.Join(base, ""), strings.Join(head, ""), strings.Join(incoming, "")
		r := Text([]byte(b), []byte(h), []byte(in))
		if len(r.Conflicts) != 0 {
			t.Fatalf("seed %d: conflicts %+v", seed, r.Conflicts)
		}
		for _, w := range want {
			if !strings.Contains(string(r.Text), w) {
				t.Fatalf("seed %d: lost %q in\n%s", seed, w, r.Text)
			}
		}
		if again := Text([]byte(b), []byte(h), []byte(in)); string(again.Text) != string(r.Text) {
			t.Fatalf("seed %d: not deterministic", seed)
		}
	}
}

func TestLCSMatchesIncrease(t *testing.T) {
	a := strings.Split("a b c a b b a", " ")
	b := strings.Split("c b a b a c", " ")
	m := lcs(a, b)
	last, n := -1, 0
	for i, j := range m {
		if j < 0 {
			continue
		}
		if j <= last || a[i] != b[j] {
			t.Fatalf("bad match %v", m)
		}
		last = j
		n++
	}
	if n != 4 {
		t.Fatalf("matched %d, want an LCS of 4 (%v)", n, m)
	}
}

// Merging large notes stays fast and small: linear-space diffs, even when
// the two sides share nothing.
func BenchmarkTenThousandLines(b *testing.B) {
	var base, head, incoming, other strings.Builder
	for i := range 10000 {
		line := fmt.Sprintf("line %d with a few words in it\n", i)
		base.WriteString(line)
		fmt.Fprintf(&other, "unrelated %d\n", i)
		switch {
		case i%97 == 0:
			head.WriteString("head " + line)
			incoming.WriteString(line)
		case i%89 == 0:
			head.WriteString(line)
			incoming.WriteString("incoming " + line)
		default:
			head.WriteString(line)
			incoming.WriteString(line)
		}
	}
	b.Run("scattered edits", func(b *testing.B) {
		for b.Loop() {
			if r := Text([]byte(base.String()), []byte(head.String()), []byte(incoming.String())); len(r.Conflicts) != 0 {
				b.Fatal(r.Conflicts)
			}
		}
	})
	b.Run("nothing shared", func(b *testing.B) {
		for b.Loop() {
			Create([]byte(base.String()), []byte(other.String()))
		}
	})
}
