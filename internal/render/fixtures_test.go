package render

import (
	"bufio"
	"os"
	"path"
	"regexp"
	"slices"
	"strings"
	"testing"

	"github.com/frostyard/frostnotes/internal/markdown"
)

// renderFixture is one "### Rnn" fixture of docs/specs/render.md: a note
// rendered as Notes/Today.md and the HTML that HTML returns for it.
type renderFixture struct {
	Name, Tag string
	Note      string
	Want      string
}

var (
	fixtureHeading = regexp.MustCompile(`^### (R\d\d — .*) \[(\w+)\]$`)
	vaultRow       = regexp.MustCompile("^\\| `([^`]+)` \\| (?:note|`([0-9a-f]{64})`) \\|$")
	vaultNote      = regexp.MustCompile("^#### `([^`]+)`$")
)

// fixtureVault holds the spec's fixture vault: every path, the hashes of
// files that are not notes, and the notes' contents.
type fixtureVault struct {
	paths  []string
	hashes map[string]string
	notes  map[string]string
}

func (v fixtureVault) options() Options {
	return Options{
		Path: "Notes/Today.md",
		Resolve: func(from, target string) string {
			return markdown.Resolve(from, target, func(name string) []string {
				var out []string
				for _, p := range v.paths {
					if strings.ToLower(path.Base(p)) == name {
						out = append(out, p)
					}
				}
				return out
			})
		},
		File: func(p string) (string, bool) { h, ok := v.hashes[p]; return h, ok },
		Note: func(p string) ([]byte, bool) { n, ok := v.notes[p]; return []byte(n), ok },
	}
}

// readFence returns the lines of the fenced block that starts at the next
// line opening a fence, without its fences.
func readFence(t *testing.T, sc *bufio.Scanner) string {
	t.Helper()
	var fence string
	for sc.Scan() {
		if l := sc.Text(); strings.HasPrefix(l, "```") {
			fence = strings.TrimRight(l, "abcdefghijklmnopqrstuvwxyz")
			break
		}
	}
	var b strings.Builder
	for sc.Scan() {
		if sc.Text() == fence {
			return b.String()
		}
		b.WriteString(sc.Text() + "\n")
	}
	t.Fatal("docs/specs/render.md: a fenced block does not close")
	return ""
}

// readRenderSpec reads the fixture vault and the fixtures.
func readRenderSpec(t *testing.T) (fixtureVault, []renderFixture) {
	t.Helper()
	f, err := os.Open("../../docs/specs/render.md")
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	v := fixtureVault{hashes: map[string]string{}, notes: map[string]string{}}
	var fixtures []renderFixture
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		line := sc.Text()
		if m := vaultRow.FindStringSubmatch(line); m != nil {
			v.paths = append(v.paths, m[1])
			if m[2] != "" {
				v.hashes[m[1]] = m[2]
			}
		} else if m := vaultNote.FindStringSubmatch(line); m != nil {
			v.notes[m[1]] = readFence(t, sc)
		} else if m := fixtureHeading.FindStringSubmatch(line); m != nil {
			fx := renderFixture{Name: m[1], Tag: m[2]}
			fx.Note = readFence(t, sc)
			fx.Want = readFence(t, sc)
			fixtures = append(fixtures, fx)
		}
	}
	if err := sc.Err(); err != nil {
		t.Fatal(err)
	}
	if len(v.paths) == 0 || len(fixtures) == 0 {
		t.Fatal("docs/specs/render.md: no fixture vault or no fixtures found")
	}
	return v, fixtures
}

// runFixtures checks every fixture whose tag is one of tags.
func runFixtures(t *testing.T, tags ...string) {
	v, fixtures := readRenderSpec(t)
	ran := 0
	for _, fx := range fixtures {
		if !slices.Contains(tags, fx.Tag) {
			continue
		}
		ran++
		t.Run(fx.Name, func(t *testing.T) {
			if got := HTML([]byte(fx.Note), v.options()); got != fx.Want {
				t.Errorf("HTML(%q)\n got: %q\nwant: %q", fx.Note, got, fx.Want)
			}
		})
	}
	if ran == 0 {
		t.Fatalf("no fixture is tagged %q", tags)
	}
}

// TestBaseFixtures checks the fixtures render.go alone draws.
func TestBaseFixtures(t *testing.T) { runFixtures(t, "base") }

func TestSanitizeEscapesUntilPhase4(t *testing.T) {
	if got := Sanitize(`<script>x</script>`); got != "&lt;script&gt;x&lt;/script&gt;" {
		t.Fatalf("Sanitize = %q", got)
	}
}
