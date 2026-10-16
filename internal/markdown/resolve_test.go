package markdown

// CONTRACT TEST for task card T-0004 (docs/tasks). Do not edit.

import (
	"bufio"
	"os"
	"path"
	"slices"
	"strings"
	"testing"
)

// specFixtures reads the fixture vault and cases from the spec's "Link
// resolution" section, so the spec and this test cannot drift apart.
func specFixtures(t *testing.T) (vault []string, cases [][3]string) {
	t.Helper()
	f, err := os.Open("../../docs/specs/markdown.md")
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	section, inVault := "", false
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		line := sc.Text()
		switch {
		case strings.HasPrefix(line, "### "):
			section = line
		case section == "### Fixture vault" && line == "```text":
			inVault = true
		case inVault && line == "```":
			inVault = false
		case inVault:
			vault = append(vault, line)
		case section == "### Fixture cases" && strings.HasPrefix(line, "| `"):
			cells := strings.Split(line, "|")
			var c [3]string
			for i := range c {
				s := strings.Trim(strings.TrimSpace(cells[i+1]), "`")
				if s == "—" || s == "(empty)" {
					s = ""
				}
				c[i] = s
			}
			cases = append(cases, c)
		}
	}
	if err := sc.Err(); err != nil {
		t.Fatal(err)
	}
	if len(vault) < 20 || len(cases) < 30 {
		t.Fatalf("read %d vault files and %d cases from the spec; the sections moved or changed shape", len(vault), len(cases))
	}
	return vault, cases
}

// lookup is files(name) over a list of vault paths, as the index answers it.
func lookup(vault []string) func(string) []string {
	return func(name string) []string {
		var out []string
		for _, p := range vault {
			if strings.ToLower(path.Base(p)) == name {
				out = append(out, p)
			}
		}
		return out
	}
}

func TestResolveSpecFixtures(t *testing.T) {
	vault, cases := specFixtures(t)
	for _, c := range cases {
		source, target, want := c[0], c[1], c[2]
		if got := Resolve(source, target, lookup(vault)); got != want {
			t.Errorf("Resolve(%q, %q) = %q, want %q", source, target, got, want)
		}
	}
}

// The order the lookup returns files in must not matter.
func TestResolveIgnoresLookupOrder(t *testing.T) {
	vault, cases := specFixtures(t)
	reversed := slices.Clone(vault)
	slices.Reverse(reversed)
	for _, c := range cases {
		if got := Resolve(c[0], c[1], lookup(reversed)); got != c[2] {
			t.Errorf("with the lookup reversed, Resolve(%q, %q) = %q, want %q", c[0], c[1], got, c[2])
		}
	}
}

func TestResolveAsksForLowercaseNames(t *testing.T) {
	var asked []string
	files := func(name string) []string {
		asked = append(asked, name)
		return nil
	}
	if got := Resolve("A/Note.md", "Projects/Big PLAN", files); got != "" {
		t.Fatalf("Resolve = %q, want unresolved", got)
	}
	for _, name := range asked {
		if name != "big plan.md" && name != "big plan" {
			t.Fatalf("files asked for %q; want only the last segment, lower-cased (%q)", name, asked)
		}
	}
	if !slices.Contains(asked, "big plan.md") {
		t.Fatalf("files was never asked for %q (asked %q)", "big plan.md", asked)
	}
}

func TestResolveEmptyTargetNeedsNoLookup(t *testing.T) {
	files := func(string) []string {
		t.Fatal("files called for an empty target")
		return nil
	}
	if got := Resolve("A/Note.md", "", files); got != "A/Note.md" {
		t.Fatalf("Resolve = %q, want the source", got)
	}
}

// Resolve returns paths exactly as the lookup spells them.
func TestResolveKeepsTheFilesSpelling(t *testing.T) {
	vault := []string{"Ünïcode/Ünïcode Plan.md"}
	if got := Resolve("x.md", "ünïcode/ünïcode plan", lookup(vault)); got != vault[0] {
		t.Fatalf("Resolve = %q, want %q", got, vault[0])
	}
}
