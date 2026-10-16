package repocheck

import (
	"bufio"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

var testIFunc = regexp.MustCompile(`^func (TestI\w*)\(`)

// make test runs -run '^Test[^I]' (frostyard/core ADR-0022): TestI is
// reserved for integration tests, so a unit test named TestIndex or
// TestInit would be skipped silently. A TestI function may only live in a
// file built with the integration tag.
func TestNoUnitTestStartsWithI(t *testing.T) {
	root := filepath.Join("..", "..")
	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			switch d.Name() {
			case ".git", "node_modules", "build", "tasks":
				return filepath.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(path, "_test.go") {
			return nil
		}
		f, err := os.Open(path)
		if err != nil {
			return err
		}
		defer f.Close()
		integration := false
		sc := bufio.NewScanner(f)
		for sc.Scan() {
			line := sc.Text()
			if strings.HasPrefix(line, "//go:build") && strings.Contains(line, "integration") {
				integration = true
			}
			if m := testIFunc.FindStringSubmatch(line); m != nil && !integration {
				t.Errorf("%s: %s starts with TestI, which make test skips; rename it (TestReindex…, TestOpenIndex…) or tag the file integration", path, m[1])
			}
		}
		return sc.Err()
	})
	if err != nil {
		t.Fatal(err)
	}
}
