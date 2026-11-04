package repocheck

import (
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"testing"
	"unicode"

	"go.yaml.in/yaml/v3"
)

// joinBackslashContinuations joins each line that ends with a backslash to
// the next, as the shell does.
func joinBackslashContinuations(text string) string {
	lines := strings.Split(strings.TrimSuffix(text, "\n"), "\n")
	var joined strings.Builder
	for i := 0; i < len(lines); i++ {
		line := strings.TrimRight(lines[i], " \t")
		if strings.HasSuffix(line, "\\") {
			joined.WriteString(strings.TrimSuffix(line, "\\"))
			joined.WriteByte(' ')
			continue
		}
		joined.WriteString(line)
		if i+1 < len(lines) {
			joined.WriteByte('\n')
		}
	}
	return joined.String()
}

// splitCommands splits shell text into commands at newlines, &, | and ;
// outside quotes, after joining backslash continuations. A # that starts a
// word outside quotes starts a comment, to the end of its line. A | inside
// a quoted -run pattern stays in its command.
func splitCommands(text string) []string {
	text = joinBackslashContinuations(text)
	var out []string
	var cur strings.Builder
	var quote rune
	comment := false
	flush := func() {
		if strings.TrimSpace(cur.String()) != "" {
			out = append(out, cur.String())
		}
		cur.Reset()
	}
	for _, r := range text {
		switch {
		case comment:
			if r == '\n' {
				comment = false
				flush()
			}
			continue
		case quote != 0:
			if r == quote {
				quote = 0
			}
		case r == '\'' || r == '"':
			quote = r
		case r == '#' && (cur.Len() == 0 || unicode.IsSpace(rune(cur.String()[cur.Len()-1]))):
			comment = true
			continue
		case r == '\n' || r == '\r' || r == '&' || r == '|' || r == ';':
			flush()
			continue
		}
		cur.WriteRune(r)
	}
	flush()
	return out
}

// words splits one command into words: whitespace outside quotes ends a
// word, and the quotes go.
func words(command string) []string {
	var out []string
	var cur strings.Builder
	var quote rune
	inWord := false
	for _, r := range command {
		switch {
		case quote != 0:
			if r == quote {
				quote = 0
				continue
			}
		case r == '\'' || r == '"':
			quote, inWord = r, true
			continue
		case unicode.IsSpace(r):
			if inWord {
				out = append(out, cur.String())
				cur.Reset()
				inWord = false
			}
			continue
		}
		cur.WriteRune(r)
		inWord = true
	}
	if inWord {
		out = append(out, cur.String())
	}
	return out
}

var (
	assignment = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]*=\S*$`)
	makeVar    = regexp.MustCompile(`^\$[({][A-Za-z_][A-Za-z0-9_]*[)}]$`)
)

// isGo reports whether word names the go command.
func isGo(word string) bool {
	return word == "go" || word == "$(GO)" || word == "${GO}" || word == "$GO"
}

// goTests returns the arguments after "test" of every go test run in shell
// text. A run counts only where a command starts: after its VAR=value
// words and wrapper variables such as $(XVFB), so "echo go test" is no
// run. A word that holds a space, such as the script of sh -lc or of
// $(IN_NSL), is searched as shell text again.
func goTests(text string) [][]string {
	var out [][]string
	for _, command := range splitCommands(text) {
		ws := words(command)
		for _, w := range ws {
			if strings.ContainsFunc(w, unicode.IsSpace) {
				out = append(out, goTests(w)...)
			}
		}
		i := 0
		for i < len(ws) && (assignment.MatchString(ws[i]) || (makeVar.MatchString(ws[i]) && !isGo(ws[i]))) {
			i++
		}
		if i+1 < len(ws) && isGo(ws[i]) && ws[i+1] == "test" {
			out = append(out, ws[i+2:])
		}
	}
	return out
}

// integrationRun reports whether the last -tags flag of args names the
// integration tag; its value lists tags split by commas or spaces.
func integrationRun(args []string) bool {
	var tags []string
	for i := 0; i < len(args); i++ {
		name, value, hasValue := strings.Cut(strings.TrimLeft(args[i], "-"), "=")
		if !strings.HasPrefix(args[i], "-") || name != "tags" {
			continue
		}
		if !hasValue && i+1 < len(args) {
			i++
			value = args[i]
		}
		tags = strings.FieldsFunc(value, func(r rune) bool { return r == ',' || unicode.IsSpace(r) })
	}
	return slices.Contains(tags, "integration")
}

// selectsNames reports whether args select tests by name.
func selectsNames(args []string) bool {
	for _, a := range args {
		if !strings.HasPrefix(a, "-") {
			continue
		}
		name, _, _ := strings.Cut(strings.TrimLeft(a, "-"), "=")
		if name == "run" || name == "skip" {
			return true
		}
	}
	return false
}

// filtersTestNames reports whether text holds a go test run without the
// integration tag that selects tests by name (-run or -skip). Such a run
// skips fuzz seeds and examples as well as tests, so a unit run must not
// have one (frostyard/core ADR-0038, docs/adr/0035).
func filtersTestNames(text string) bool {
	for _, args := range goTests(text) {
		if !integrationRun(args) && selectsNames(args) {
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
		{name: "multiline continuation", cmd: "go test \\\n  -run '^Test[^I]' ./...", want: true},
		{name: "tagged then untagged", cmd: "go test -tags=integration ./... && go test -run '^Test' ./...", want: true},
		{name: "go variable", cmd: "$(GO) test -run X ./...", want: true},
		{name: "leading quote", cmd: "\"go test -run X ./...\"", want: true},
		{name: "last tag wins", cmd: "go test -tags integration -tags foo -run X ./...", want: true},
		{name: "no filter", cmd: "go test ./...", want: false},
		{name: "race no filter", cmd: "go test -race -timeout 25m ./...", want: false},
		{name: "integration tags", cmd: "go test -tags integration -run '^TestIKillNotesd$$' -count=1 -v ./internal/vault", want: false},
		{name: "integration tags quotes", cmd: "go test -tags integration -count=1 -v -run \"^TestI\" ./internal/appevidence", want: false},
		{name: "integration comma tags", cmd: "go test -tags=foo,integration -run X ./...", want: false},
		{name: "integration quoted space tags", cmd: "go test -tags 'foo integration' -run X ./...", want: false},
		{name: "comment", cmd: "# go test -run X", want: false},
		{name: "pipe inside a quoted pattern", cmd: "go test -run 'A|B' -tags integration ./...", want: false},
		{name: "skip after a quoted pipe", cmd: "go test -run 'A|B' ./...", want: true},
		{name: "no go test", cmd: "make test", want: false},
		{name: "inline comment names the tag", cmd: "go test -run X ./... # -tags integration", want: true},
		{name: "echo is no run", cmd: "echo go test -run X ./...", want: false},
		{name: "quoted equals tags with a space", cmd: "go test -tags=\"foo integration\" -run X ./...", want: false},
		{name: "wrapper script", cmd: "$(IN_NSL) 'FOO=1 $(XVFB) go test -run X ./...'", want: true},
		{name: "wrapper script, integration", cmd: "$(IN_NSL) 'FOO=$$PWD/x $(XVFB) go test -tags integration -count=1 -run \"^TestI\" ./internal/appevidence'", want: false},
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

type workflowRun struct {
	line  int
	value string
}

func workflowRunValues(data []byte) ([]workflowRun, error) {
	var document yaml.Node
	if err := yaml.Unmarshal(data, &document); err != nil {
		return nil, err
	}

	var runs []workflowRun
	var walk func(*yaml.Node)
	walk = func(node *yaml.Node) {
		if node == nil {
			return
		}
		if node.Kind == yaml.MappingNode {
			for i := 0; i+1 < len(node.Content); i += 2 {
				key := node.Content[i]
				value := node.Content[i+1]
				if key.Value == "run" {
					runs = append(runs, workflowRun{line: key.Line, value: value.Value})
				}
				walk(value)
			}
			return
		}
		for _, child := range node.Content {
			walk(child)
		}
	}
	walk(&document)
	return runs, nil
}

func TestWorkflowRunValues(t *testing.T) {
	text := []byte("steps:\n  - run: |\n      go test \\\n        -run '^Test[^I]' ./...\n")
	runs, err := workflowRunValues(text)
	if err != nil {
		t.Fatal(err)
	}
	if len(runs) != 1 {
		t.Fatalf("workflow run values = %d, want 1", len(runs))
	}
	if runs[0].line != 2 {
		t.Errorf("run line = %d, want 2", runs[0].line)
	}
	if !filtersTestNames(joinBackslashContinuations(runs[0].value)) {
		t.Errorf("workflow run value was not detected: %q", runs[0].value)
	}
}

// countGoTestCommands counts the go test runs in shell text.
func countGoTestCommands(text string) int {
	return len(goTests(text))
}

// Every go test line of the Makefile and the workflows that is not an
// integration run runs every test, fuzz seed and example.
func TestUnitRunsFilterNoTestNames(t *testing.T) {
	root := filepath.Join("..", "..")
	goTestCommands := 0

	makefile := filepath.Join(root, "Makefile")
	makeData, err := os.ReadFile(makefile)
	if err != nil {
		t.Fatalf("Makefile not found: %v", err)
	}
	for _, line := range joinMakefileLines(makeData) {
		text := strings.TrimSpace(line.text)
		if text == "" || strings.HasPrefix(text, "#") {
			continue
		}
		goTestCommands += countGoTestCommands(line.text)
		for _, command := range splitCommands(line.text) {
			if filtersTestNames(command) {
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
		runs, err := workflowRunValues(data)
		if err != nil {
			t.Fatalf("%s: parse workflow: %v", path, err)
		}
		rel, err := filepath.Rel(root, path)
		if err != nil {
			t.Fatal(err)
		}
		for _, run := range runs {
			value := joinBackslashContinuations(run.value)
			goTestCommands += countGoTestCommands(value)
			for _, command := range splitCommands(value) {
				if filtersTestNames(command) {
					t.Errorf("%s:%d: a unit run filters test names; every hermetic test must run (frostyard/core ADR-0038)", rel, run.line)
				}
			}
		}
	}

	if goTestCommands == 0 {
		t.Fatal("zero go test commands found")
	}
}
