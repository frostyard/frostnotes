package merge

import (
	"fmt"
	"math/rand/v2"
	"strings"
	"testing"
)

// mergeCase is one golden merge: three inputs, the merged text, and the
// lines of the merged text where the head won a conflict. These 30 cases
// are M0's spike (plan 0001, Phase 5) and become M2's golden tests.
type mergeCase struct {
	name                       string
	kind                       string // "text", "note" or "create" (base unused)
	base, head, incoming, want string
	conflicts                  []int
}

var cases = []mergeCase{
	{name: "nothing changed", kind: "text",
		base: "a\nb\n", head: "a\nb\n", incoming: "a\nb\n", want: "a\nb\n"},
	{name: "only the head changed", kind: "text",
		base: "a\nb\n", head: "a\nB\n", incoming: "a\nb\n", want: "a\nB\n"},
	{name: "only the incoming side changed", kind: "text",
		base: "a\nb\n", head: "a\nb\n", incoming: "A\nb\n", want: "A\nb\n"},
	{name: "the same change on both sides", kind: "text",
		base: "a\nb\n", head: "a\nX\n", incoming: "a\nX\n", want: "a\nX\n"},
	{name: "different lines", kind: "text",
		base: "a\nb\nc\nd\ne\n", head: "A\nb\nc\nd\ne\n", incoming: "a\nb\nc\nd\nE\n", want: "A\nb\nc\nd\nE\n"},
	{name: "adjacent lines merge by word", kind: "text",
		base: "a\nb\nc\nd\n", head: "a\nB\nc\nd\n", incoming: "a\nb\nC\nd\n", want: "a\nB\nC\nd\n"},
	{name: "different words of one line", kind: "text",
		base: "the quick brown fox\n", head: "the slow brown fox\n", incoming: "the quick brown dog\n",
		want: "the slow brown dog\n"},
	{name: "the same word changed differently", kind: "text",
		base: "meet on monday\n", head: "meet on tuesday\n", incoming: "meet on friday\n",
		want: "meet on tuesday\n", conflicts: []int{1}},
	{name: "both appended lines", kind: "text",
		base: "# log\n", head: "# log\nfrom the laptop\n", incoming: "# log\nfrom the phone\n",
		want: "# log\nfrom the laptop\nfrom the phone\n"},
	{name: "both inserted lines at one place mid-file", kind: "text",
		base: "a\nz\n", head: "a\nh1\nh2\nz\n", incoming: "a\ni1\nz\n", want: "a\nh1\nh2\ni1\nz\n"},
	{name: "a deletion and an edit elsewhere", kind: "text",
		base: "a\nb\nc\nd\ne\n", head: "a\nc\nd\ne\n", incoming: "a\nb\nc\nd\nE\n", want: "a\nc\nd\nE\n"},
	{name: "a deletion against an edit of the same line", kind: "text",
		base: "a\nb\nc\n", head: "a\nc\n", incoming: "a\nB\nc\n", want: "a\nc\n", conflicts: []int{2}},
	{name: "a missing final newline is kept", kind: "text",
		base: "a\nb", head: "top\na\nb", incoming: "a\nB", want: "top\na\nB"},
	{name: "CRLF lines keep their endings", kind: "text",
		base: "a\r\nb\r\nc\r\n", head: "A\r\nb\r\nc\r\n", incoming: "a\r\nb\r\nC\r\n", want: "A\r\nb\r\nC\r\n"},
	{name: "accented and emoji words", kind: "text",
		base: "café 🎉 naïve\n", head: "café 🎊 naïve\n", incoming: "café 🎉 naive\n", want: "café 🎊 naive\n"},
	{name: "both inserted different words at one place", kind: "text",
		base: "the cat\n", head: "the black cat\n", incoming: "the fat cat\n",
		want: "the black cat\n", conflicts: []int{1}},
	{name: "indentation and a word change on one line", kind: "text",
		base: "- item one\n", head: "  - item one\n", incoming: "- item two\n", want: "  - item two\n"},
	{name: "different keys", kind: "note",
		base:     "---\ntitle: Plan\nstatus: draft\n---\nbody\n",
		head:     "---\ntitle: Plan v2\nstatus: draft\n---\nbody\n",
		incoming: "---\ntitle: Plan\nstatus: done\n---\nbody\n",
		want:     "---\ntitle: Plan v2\nstatus: done\n---\nbody\n"},
	{name: "one key changed differently", kind: "note",
		base:     "---\nstatus: draft\n---\nbody\n",
		head:     "---\nstatus: review\n---\nbody\n",
		incoming: "---\nstatus: done\n---\nbody\n",
		want:     "---\nstatus: review\n---\nbody\n", conflicts: []int{2}},
	{name: "each side added a key", kind: "note",
		base:     "---\ntitle: T\n---\n",
		head:     "---\ntitle: T\nowner: me\n---\n",
		incoming: "---\ndue: 2026-10-20\ntitle: T\n---\n",
		want:     "---\ntitle: T\nowner: me\ndue: 2026-10-20\n---\n"},
	{name: "a deleted key against a changed one", kind: "note",
		base:     "---\ntitle: T\nstatus: draft\n---\n",
		head:     "---\ntitle: T\n---\n",
		incoming: "---\ntitle: T\nstatus: done\n---\n",
		want:     "---\ntitle: T\nstatus: done\n---\n", conflicts: []int{3}},
	{name: "both added tags to a list", kind: "note",
		base:     "---\ntags:\n  - work\n---\n",
		head:     "---\ntags:\n  - work\n  - urgent\n---\n",
		incoming: "---\ntags:\n  - work\n  - q4\n---\n",
		want:     "---\ntags:\n  - work\n  - urgent\n  - q4\n---\n"},
	{name: "frontmatter and body on both sides", kind: "note",
		base:     "---\na: 1\nb: 2\n---\none\ntwo\nthree\n",
		head:     "---\na: 10\nb: 2\n---\nONE\ntwo\nthree\n",
		incoming: "---\na: 1\nb: 20\n---\none\ntwo\nTHREE\n",
		want:     "---\na: 10\nb: 20\n---\nONE\ntwo\nTHREE\n"},
	{name: "frontmatter added on one side", kind: "note",
		base:     "one\ntwo\n",
		head:     "one\nTWO\n",
		incoming: "---\naliases: [first]\n---\none\ntwo\n",
		want:     "---\naliases: [first]\n---\none\nTWO\n"},
	{name: "untouched keys keep their bytes", kind: "note",
		base:     "---\n# kept comment\nquote: \"a: b\"   \nlist: [ x,y ]\nn: 1\n---\n",
		head:     "---\n# kept comment\nquote: \"a: b\"   \nlist: [ x,y ]\nn: 2\n---\n",
		incoming: "---\n# kept comment\nquote: \"a: b\"   \nlist: [ x,y ]\nn: 1\nnew: yes\n---\n",
		want:     "---\n# kept comment\nquote: \"a: b\"   \nlist: [ x,y ]\nn: 2\nnew: yes\n---\n"},
	{name: "a rule in the body is not frontmatter", kind: "note",
		base: "intro\n---\nend\n", head: "INTRO\n---\nend\n", incoming: "intro\n---\nEND\n", want: "INTRO\n---\nEND\n"},
	{name: "a repeated key merges as text", kind: "note",
		base: "---\na: 1\na: 2\n---\nx\n", head: "---\na: 1\na: 2\n---\nX\n", incoming: "---\na: 1\na: 3\n---\nx\n",
		want: "---\na: 1\na: 3\n---\nX\n"},
	{name: "two daily notes from one template", kind: "create",
		head:     "# 2026-10-16\n## Tasks\n- [ ] call the bank\n## Notes\n\n",
		incoming: "# 2026-10-16\n## Tasks\n## Notes\nmet Ana at lunch\n\n",
		want:     "# 2026-10-16\n## Tasks\n- [ ] call the bank\n## Notes\nmet Ana at lunch\n\n"},
	{name: "two daily notes with tasks in one section", kind: "create",
		head:     "---\ntags: [daily]\n---\n## Tasks\n- [ ] laptop task\n## Notes\n",
		incoming: "---\ntags: [daily]\n---\n## Tasks\n- [ ] phone task\n## Notes\n",
		want:     "---\ntags: [daily]\n---\n## Tasks\n- [ ] laptop task\n- [ ] phone task\n## Notes\n"},
	{name: "two creates with nothing in common", kind: "create",
		head: "alpha\n", incoming: "beta\n", want: "alpha\n", conflicts: []int{1}},
}

func TestGoldenMerges(t *testing.T) {
	if len(cases) != 30 {
		t.Errorf("%d cases; the spike promised 30", len(cases))
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var r Result
			switch tc.kind {
			case "text":
				r = Text([]byte(tc.base), []byte(tc.head), []byte(tc.incoming))
			case "note":
				r = Note([]byte(tc.base), []byte(tc.head), []byte(tc.incoming))
			case "create":
				r = Create([]byte(tc.head), []byte(tc.incoming))
			default:
				t.Fatalf("unknown kind %q", tc.kind)
			}
			if string(r.Text) != tc.want {
				t.Errorf("merged:\n%q\nwant:\n%q", r.Text, tc.want)
			}
			var lines []int
			for _, c := range r.Conflicts {
				lines = append(lines, c.Line)
			}
			if fmt.Sprint(lines) != fmt.Sprint(tc.conflicts) {
				t.Errorf("conflict lines %v, want %v (%+v)", lines, tc.conflicts, r.Conflicts)
			}
		})
	}
}

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
		other.WriteString(fmt.Sprintf("unrelated %d\n", i))
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
