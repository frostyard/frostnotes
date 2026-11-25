//go:build linux && integration

package main

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// TestNoLiteralLooks holds rule 2 of the kit's standard for an app
// (frostui's docs/specs/workspaceui.md) for this client: its code takes a
// kit part, an NSGo control or a token, and writes no look as a number. It
// reads the package's own files, not the app.
//
// A color, a corner and a padding are not written at all: a padding takes
// the kit's spacing. 1 text size is left, the size of a Markdown heading in
// live preview, which the kit has no scale for. A file's count there may
// only go down, and the rule is whole when the list is empty.
func TestNoLiteralLooks(t *testing.T) {
	never := []struct {
		what string
		re   *regexp.Regexp
	}{
		{"a color written as a number", regexp.MustCompile(`core\.RGBA?\(|\b0x[0-9a-fA-F]{6,8}\b`)},
		{"a corner written as a number", regexp.MustCompile(`CornerRadius:\s*[0-9]`)},
		// A number anywhere in what a padding is built from.
		{"a padding written as a number", regexp.MustCompile(`Padding:\s*core\.(All|Symmetric|Edges)[({][^)}]*\b[0-9]`)},
	}
	counted := []struct {
		what string
		re   *regexp.Regexp
		left map[string]int
	}{
		{"text sizes", regexp.MustCompile(`FontSize\s*[:=]|ui\.Text\(.*,\s*[0-9]+,\s*ui\.`), nil},
	}
	dir := filepath.Join(repoRoot(t), "cmd", "frostnotes-nsgo")
	files, err := filepath.Glob(filepath.Join(dir, "*.go"))
	if err != nil || len(files) == 0 {
		t.Fatalf("no source files in %s: %v", dir, err)
	}
	seen := make([]map[string]int, len(counted))
	for i := range seen {
		seen[i] = map[string]int{}
	}
	for _, path := range files {
		name := filepath.Base(path)
		if strings.HasSuffix(name, "_test.go") {
			continue
		}
		data, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		for n, line := range strings.Split(string(data), "\n") {
			if strings.HasPrefix(strings.TrimSpace(line), "//") {
				continue
			}
			for _, rule := range never {
				if rule.re.MatchString(line) {
					t.Errorf("%s:%d: %s: take a token of the kit or of NSGo", name, n+1, rule.what)
				}
			}
			for i, rule := range counted {
				seen[i][name] += len(rule.re.FindAllString(line, -1))
			}
		}
	}
	for _, bad := range []string{"Padding: core.All(16)", "Padding: core.Edges{Left: core.Points(depth * 12)}"} {
		if !never[2].re.MatchString(bad) {
			t.Errorf("the padding rule misses %q", bad)
		}
	}
	if good := "Padding: core.All(workspaceui.SpaceL)"; never[2].re.MatchString(good) {
		t.Errorf("the padding rule refuses %q", good)
	}
	for i, rule := range counted {
		for name, got := range seen[i] {
			if want := rule.left[name]; got > want {
				t.Errorf("%s writes %d %s as numbers, and %d are allowed: take a kit part or token", name, got, rule.what, want)
			} else if got < want {
				t.Errorf("%s writes %d %s as numbers, fewer than the %d listed: lower its count", name, got, rule.what, want)
			}
		}
		for name := range rule.left {
			if _, ok := seen[i][name]; !ok {
				t.Errorf("the list of %s names %s, which is not a file of the client", rule.what, name)
			}
		}
	}
}
