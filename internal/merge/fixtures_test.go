package merge

import (
	"bufio"
	"encoding/json/v2"
	"fmt"
	"os"
	"regexp"
	"strings"
	"testing"
)

// fixture is one "### Mnn" fixture of docs/specs/merge.md: three inputs,
// the merged bytes, and the lines where the head won a conflict.
type fixture struct {
	Name      string `json:"-"`
	Kind      string `json:"kind"` // text, note or create (no base)
	Base      string `json:"base"`
	Head      string `json:"head"`
	Incoming  string `json:"incoming"`
	Want      string `json:"want"`
	Conflicts []int  `json:"conflicts"`
}

var fixtureHeading = regexp.MustCompile(`^### (M(\d\d) — .*)$`)

// loadFixtures reads the spec's fixtures, in order, checking that they are
// numbered from M01 without gaps.
func loadFixtures(t *testing.T) []fixture {
	t.Helper()
	f, err := os.Open("../../docs/specs/merge.md")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = f.Close() }()
	var out []fixture
	var cur *fixture
	var block strings.Builder
	inJSON := false
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		line := sc.Text()
		switch {
		case inJSON && line == "```":
			inJSON = false
			if err := json.Unmarshal([]byte(block.String()), cur, json.RejectUnknownMembers(true)); err != nil {
				t.Fatalf("%s: %v", cur.Name, err)
			}
			out = append(out, *cur)
			cur = nil
		case inJSON:
			block.WriteString(line + "\n")
		case cur != nil && line == "```json":
			inJSON = true
			block.Reset()
		default:
			if m := fixtureHeading.FindStringSubmatch(line); m != nil {
				if want := fmt.Sprintf("%02d", len(out)+1); m[2] != want {
					t.Fatalf("fixture %q: want M%s next", m[1], want)
				}
				cur = &fixture{Name: m[1]}
			}
		}
	}
	if err := sc.Err(); err != nil {
		t.Fatal(err)
	}
	return out
}

func TestSpecFixtures(t *testing.T) {
	fixtures := loadFixtures(t)
	if len(fixtures) < 44 {
		t.Fatalf("%d fixtures; the spec has at least 44", len(fixtures))
	}
	for _, fx := range fixtures {
		t.Run(fx.Name, func(t *testing.T) {
			var r Result
			switch fx.Kind {
			case "text":
				r = Text([]byte(fx.Base), []byte(fx.Head), []byte(fx.Incoming))
			case "note":
				r = Note([]byte(fx.Base), []byte(fx.Head), []byte(fx.Incoming))
			case "create":
				if fx.Base != "" {
					t.Fatal("a create fixture has no base")
				}
				r = Create([]byte(fx.Head), []byte(fx.Incoming))
			default:
				t.Fatalf("unknown kind %q", fx.Kind)
			}
			if fx.Want == "" && fx.Head != "" && fx.Incoming != "" {
				t.Fatal("a fixture with two non-empty sides wants an empty file")
			}
			if string(r.Text) != fx.Want {
				t.Errorf("merged:\n%q\nwant:\n%q", r.Text, fx.Want)
			}
			var lines []int
			for _, c := range r.Conflicts {
				lines = append(lines, c.Line)
			}
			if fmt.Sprint(lines) != fmt.Sprint(fx.Conflicts) {
				t.Errorf("conflict lines %v, want %v (%+v)", lines, fx.Conflicts, r.Conflicts)
			}
			if again := mergeKind(fx); string(again.Text) != string(r.Text) {
				t.Error("not deterministic")
			}
		})
	}
}

func mergeKind(fx fixture) Result {
	switch fx.Kind {
	case "note":
		return Note([]byte(fx.Base), []byte(fx.Head), []byte(fx.Incoming))
	case "create":
		return Create([]byte(fx.Head), []byte(fx.Incoming))
	}
	return Text([]byte(fx.Base), []byte(fx.Head), []byte(fx.Incoming))
}
