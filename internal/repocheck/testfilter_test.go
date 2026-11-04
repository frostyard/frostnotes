package repocheck

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"unicode"

	"go.yaml.in/yaml/v3"
)

func normalizeToken(token string) string {
	return strings.Trim(strings.TrimSpace(token), "'\"")
}

func splitTagValue(value string) []string {
	value = strings.Trim(value, "'\"")
	return strings.FieldsFunc(value, func(r rune) bool {
		return r == ',' || unicode.IsSpace(r)
	})
}

func tagValueAt(fields []string, start int) (string, int) {
	if start >= len(fields) {
		return "", start
	}

	raw := fields[start]
	if len(raw) >= 1 && (raw[0] == '\'' || raw[0] == '"') {
		quote := raw[0]
		if len(raw) > 1 && raw[len(raw)-1] == byte(quote) {
			return strings.Trim(raw, string(quote)), start
		}

		value := strings.TrimPrefix(raw, string(quote))
		for i := start + 1; i < len(fields); i++ {
			part := fields[i]
			if strings.HasSuffix(part, string(quote)) {
				value += " " + strings.TrimSuffix(part, string(quote))
				return value, i
			}
			value += " " + part
		}
	}
	return strings.Trim(raw, "'\""), start
}

func integrationTagSet(fields []string) bool {
	var tags []string
	for i := 0; i < len(fields); i++ {
		token := normalizeToken(fields[i])
		if token == "-tags" || token == "--tags" {
			value, end := tagValueAt(fields, i+1)
			tags = splitTagValue(value)
			i = end
			continue
		}
		for _, prefix := range []string{"-tags=", "--tags="} {
			if strings.HasPrefix(token, prefix) {
				value := strings.TrimPrefix(token, prefix)
				tags = splitTagValue(value)
				break
			}
		}
	}
	for _, tag := range tags {
		if tag == "integration" {
			return true
		}
	}
	return false
}

func joinBackslashContinuations(text string) string {
	lines := strings.Split(strings.TrimSuffix(text, "\n"), "\n")
	var joined strings.Builder
	for i := 0; i < len(lines); i++ {
		line := strings.TrimRight(lines[i], " \t")
		if strings.HasSuffix(line, "\\") {
			line = strings.TrimSuffix(line, "\\")
			joined.WriteString(line)
			if i+1 < len(lines) {
				joined.WriteByte(' ')
			}
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
// outside quotes, after joining backslash continuations. A | inside a
// quoted -run pattern stays in its command.
func splitCommands(text string) []string {
	text = joinBackslashContinuations(text)
	var out []string
	var cur strings.Builder
	var quote rune
	for _, r := range text {
		switch {
		case quote != 0:
			if r == quote {
				quote = 0
			}
		case r == '\'' || r == '"':
			quote = r
		case r == '\n' || r == '\r' || r == '&' || r == '|' || r == ';':
			if cur.Len() > 0 {
				out = append(out, cur.String())
				cur.Reset()
			}
			continue
		}
		cur.WriteRune(r)
	}
	if cur.Len() > 0 {
		out = append(out, cur.String())
	}
	return out
}

func isGoTestCommand(fields []string) bool {
	for i := 0; i+1 < len(fields); i++ {
		first := normalizeToken(fields[i])
		second := normalizeToken(fields[i+1])
		if second != "test" {
			continue
		}
		if first == "go" || first == "$(GO)" || first == "${GO}" || first == "$GO" {
			return true
		}
	}
	return false
}

func hasNameFlag(fields []string) bool {
	for _, token := range fields {
		token = normalizeToken(token)
		if token == "-run" || token == "-skip" || token == "--run" || token == "--skip" {
			return true
		}
		for _, prefix := range []string{"-run=", "-skip=", "--run=", "--skip="} {
			if strings.HasPrefix(token, prefix) {
				return true
			}
		}
	}
	return false
}

func filtersTestNames(cmd string) bool {
	for _, command := range splitCommands(cmd) {
		command = strings.TrimSpace(command)
		if command == "" || strings.HasPrefix(command, "#") {
			continue
		}
		fields := strings.Fields(command)
		if isGoTestCommand(fields) && !integrationTagSet(fields) && hasNameFlag(fields) {
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

func countGoTestCommands(text string) int {
	count := 0
	for _, command := range splitCommands(text) {
		command = strings.TrimSpace(command)
		if command == "" || strings.HasPrefix(command, "#") {
			continue
		}
		if isGoTestCommand(strings.Fields(command)) {
			count++
		}
	}
	return count
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
