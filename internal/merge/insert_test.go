package merge

import (
	"fmt"
	"math/rand/v2"
	"slices"
	"strings"
	"testing"
)

// A line the incoming side inserts is in the result of every merge that
// reports no conflict, whatever the head did: the property sync's token
// oracle relies on (ADR-0020), over seeded small cases.
func TestAnInsertSurvivesEveryCleanMerge(t *testing.T) {
	rng := rand.New(rand.NewPCG(1, 2))
	fails := 0
	for n := 0; n < 50000 && fails < 5; n++ {
		var base []string
		for i := range rng.IntN(6) {
			base = append(base, fmt.Sprintf("l%d\n", i))
		}
		// The head inserts, deletes and changes lines.
		head := slices.Clone(base)
		for range rng.IntN(4) {
			switch rng.IntN(3) {
			case 0:
				head = slices.Insert(head, rng.IntN(len(head)+1), fmt.Sprintf("h%d\n", rng.IntN(100)))
			case 1:
				if len(head) > 0 {
					i := rng.IntN(len(head))
					head = slices.Delete(head, i, i+1)
				}
			case 2:
				if len(head) > 0 {
					head[rng.IntN(len(head))] = fmt.Sprintf("x%d\n", rng.IntN(100))
				}
			}
		}
		inc := slices.Clone(base)
		inc = slices.Insert(inc, rng.IntN(len(inc)+1), "TOK\n")
		b, h, i := strings.Join(base, ""), strings.Join(head, ""), strings.Join(inc, "")
		for name, f := range map[string]func(a, b, c []byte) Result{"Text": Text, "Note": Note} {
			r := f([]byte(b), []byte(h), []byte(i))
			if len(r.Conflicts) == 0 && !strings.Contains(string(r.Text), "TOK\n") {
				fails++
				t.Errorf("%s lost the insert:\nbase %q\nhead %q\ninc  %q\n= %q", name, b, h, i, r.Text)
			}
		}
	}
}
