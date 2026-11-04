package repocheck

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// filtersTestNames reports whether cmd is a go test run without the
// integration tag that selects tests by name (-run or -skip). Such a run
// skips fuzz seeds and examples as well as tests, so a unit run must not
// have one (frostyard/core ADR-0038, docs/adr/0035).
func filtersTestNames(cmd string) bool {
	fields := strings.Fields(cmd)

	hasGoTest := false
	integrationTag := false
	for i, token := range fields {
		if token == "go" && i+1 < len(fields) && fields[i+1] == "test" {
			hasGoTest = true
		}

		if token == "-tags" && i+1 < len(fields) {
			value := strings.Trim(fields[i+1], "'\"")
			if value == "integration" {
				integrationTag = true
			}
		}
		if strings.HasPrefix(token, "-tags=") {
			value := strings.Trim(strings.TrimPrefix(token, "-tags="), "'\"")
			for _, part := range strings.Split(value, ",") {
				if part == "integration" {
					integrationTag = true
				}
			}
		}
	}
	if !hasGoTest || integrationTag {
		return false
	}

	for _, token := range fields {
		if token == "-run" || token == "-skip" || token == "--run" || token == "--skip" {
			return true
		}
		if strings.HasPrefix(token, "-run=") || strings.HasPrefix(token, "-skip=") ||
			strings.HasPrefix(token, "--run=") || strings.HasPrefix(token, "--skip=") {
			return true
		}
	}
	return false
}

func TestFiltersTestNames(t *testing.T) {
	tests := []struct {
		name string
		cmd  string
		want bool
	}{
		{name: "old make filter", cmd: "go test -run '^Test[^I]' ./...", want: true},
		{name: "old race filter", cmd: "go test -race -timeout 25m -run '^Test[^I]' ./...", want: true},
		{name: "skip", cmd: "go test -skip Integration ./...", want: true},
		{name: "run equals", cmd: "go test -run=^Test ./...", want: true},
		{name: "double dash run", cmd: "go test --run '^Test' ./...", want: true},
		{name: "no filter", cmd: "go test ./...", want: false},
		{name: "race no filter", cmd: "go test -race -timeout 25m ./...", want: false},
		{name: "integration tags", cmd: "go test -tags integration -run '^TestIKillNotesd$$' -count=1 -v ./internal/vault", want: false},
		{name: "integration tags quotes", cmd: "go test -tags integration -count=1 -v -run \"^TestI\" ./internal/appevidence", want: false},
		{name: "no go test", cmd: "make test", want: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := filtersTestNames(tt.cmd); got != tt.want {
				t.Errorf("filtersTestNames(%q) = %v, want %v", tt.cmd, got, tt.want)
			}
		})
	}
}

type numberedLine struct {
	number int
	text   string
}

// joinMakefileLines joins each line that ends with a backslash to the next,
// and keeps the number of the first line.
func joinMakefileLines(data []byte) []numberedLine {
	raw := strings.Split(strings.TrimSuffix(string(data), "\n"), "\n")
	lines := make([]numberedLine, 0, len(raw))
	for i := 0; i < len(raw); {
		start := i
		text := strings.TrimRight(raw[i], " \t")
		for strings.HasSuffix(text, "\\") {
			text = strings.TrimSuffix(text, "\\")
			i++
			if i >= len(raw) {
				break
			}
			text += " " + strings.TrimSpace(raw[i])
			text = strings.TrimRight(text, " \t")
		}
		lines = append(lines, numberedLine{number: start + 1, text: text})
		i++
	}
	return lines
}

// Every go test line of the Makefile and the workflows that is not an
// integration run runs every test, fuzz seed and example.
func TestUnitRunsFilterNoTestNames(t *testing.T) {
	root := filepath.Join("..", "..")
	goTestLines := 0

	makefile := filepath.Join(root, "Makefile")
	makeData, err := os.ReadFile(makefile)
	if err != nil {
		t.Fatalf("Makefile not found: %v", err)
	}
	for _, line := range joinMakefileLines(makeData) {
		if strings.Contains(line.text, "go test") {
			goTestLines++
			if filtersTestNames(line.text) {
				t.Errorf("Makefile:%d: a unit run filters test names; every hermetic test must run (frostyard/core ADR-0038)", line.number)
			}
		}
	}

	workflowFiles := make([]string, 0)
	for _, pattern := range []string{
		filepath.Join(root, ".github", "workflows", "*.yml"),
		filepath.Join(root, ".github", "workflows", "*.yaml"),
	} {
		matches, err := filepath.Glob(pattern)
		if err != nil {
			t.Fatal(err)
		}
		workflowFiles = append(workflowFiles, matches...)
	}
	if len(workflowFiles) == 0 {
		t.Fatal("no workflow file found")
	}

	for _, path := range workflowFiles {
		data, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		rel, err := filepath.Rel(root, path)
		if err != nil {
			t.Fatal(err)
		}
		for i, line := range strings.Split(string(data), "\n") {
			if strings.Contains(line, "go test") {
				goTestLines++
				if filtersTestNames(line) {
					t.Errorf("%s:%d: a unit run filters test names; every hermetic test must run (frostyard/core ADR-0038)", rel, i+1)
				}
			}
		}
	}

	if goTestLines == 0 {
		t.Fatal("zero go test lines found")
	}
}
