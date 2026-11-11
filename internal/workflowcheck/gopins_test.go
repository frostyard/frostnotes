package workflowcheck

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"testing"
)

var (
	goModToolchain   = regexp.MustCompile(`(?m)^toolchain go(\S+)$`)
	goModGo          = regexp.MustCompile(`(?m)^go (\S+)$`)
	miseGoVersion    = regexp.MustCompile(`\[\[tools\.go\]\]\nversion = "([^"]+)"`)
	miseGoLinuxSum   = regexp.MustCompile(`\[tools\.go\."platforms\.linux-x64"\]\nchecksum = "sha256:([0-9a-f]+)"`)
	miseGoURLVersion = regexp.MustCompile(`/go/go([0-9.]+)\.(?:linux|darwin|windows)-`)
	provisionVersion = regexp.MustCompile(`(?m)^GO_VERSION=(\S+)$`)
	provisionSum     = regexp.MustCompile(`(?m)^GO_SHA256=(\S+)$`)
)

// goPinProblems reports where mise.lock and the nsl machine's script disagree
// with the Go that go.mod names, its toolchain line if it has one.
func goPinProblems(goMod, miseLock, provision string) []string {
	m := goModToolchain.FindStringSubmatch(goMod)
	if m == nil {
		m = goModGo.FindStringSubmatch(goMod)
	}
	if m == nil {
		return []string{"go.mod names no Go"}
	}
	want := m[1]
	var problems []string
	first := func(name string, re *regexp.Regexp, text string) string {
		found := re.FindStringSubmatch(text)
		if found == nil {
			problems = append(problems, name+" not found")
			return ""
		}
		return found[1]
	}
	if got := first("mise.lock's Go version", miseGoVersion, miseLock); got != "" && got != want {
		problems = append(problems, fmt.Sprintf("mise.lock has Go %s, go.mod %s", got, want))
	}
	for _, u := range miseGoURLVersion.FindAllStringSubmatch(miseLock, -1) {
		if u[1] != want {
			problems = append(problems, fmt.Sprintf("mise.lock downloads Go %s, go.mod has %s", u[1], want))
		}
	}
	if got := first("provision.sh's GO_VERSION", provisionVersion, provision); got != "" && got != want {
		problems = append(problems, fmt.Sprintf("provision.sh has Go %s, go.mod %s", got, want))
	}
	lock := first("mise.lock's linux-x64 checksum", miseGoLinuxSum, miseLock)
	script := first("provision.sh's GO_SHA256", provisionSum, provision)
	if lock != "" && script != "" && lock != script {
		problems = append(problems, fmt.Sprintf("provision.sh's checksum %s is not mise.lock's %s", script, lock))
	}
	return problems
}

// TestGoPinsAgree keeps a Go bump whole: go.mod, mise.lock and the nsl
// machine's script name one release, so a security release cannot reach some
// of the builds and not others.
func TestGoPinsAgree(t *testing.T) {
	read := func(parts ...string) string {
		data, err := os.ReadFile(filepath.Join(append([]string{"..", ".."}, parts...)...))
		if err != nil {
			t.Fatal(err)
		}
		return string(data)
	}
	for _, p := range goPinProblems(read("go.mod"), read("mise.lock"), read("dev", "nsl", "provision.sh")) {
		t.Error(p)
	}
}

func TestGoPinProblems(t *testing.T) {
	const (
		mod    = "module m\n\ngo 1.2.3\n"
		lock   = "[[tools.go]]\nversion = \"1.2.3\"\n\n[tools.go.\"platforms.linux-x64\"]\nchecksum = \"sha256:abc\"\nurl = \"https://dl.google.com/go/go1.2.3.linux-amd64.tar.gz\"\n"
		script = "GO_VERSION=1.2.3\nGO_SHA256=abc\n"
	)
	cases := []struct {
		name                string
		mod, lock, script   string
		wantProblemsAtLeast int
	}{
		{"agreeing", mod, lock, script, 0},
		{"toolchain line wins", "go 1.2.0\ntoolchain go1.2.3\n", lock, script, 0},
		{"old toolchain line", "go 1.2.3\ntoolchain go1.2.2\n", lock, script, 1},
		{"old lock", "go 1.2.4\n", lock, "GO_VERSION=1.2.4\nGO_SHA256=abc\n", 1},
		{"old script", mod, lock, "GO_VERSION=1.2.2\nGO_SHA256=abc\n", 1},
		{"old checksum", mod, lock, "GO_VERSION=1.2.3\nGO_SHA256=def\n", 1},
		{"no go line", "module m\n", lock, script, 1},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := goPinProblems(c.mod, c.lock, c.script)
			if c.wantProblemsAtLeast == 0 && len(got) != 0 {
				t.Fatalf("problems: %v", got)
			}
			if len(got) < c.wantProblemsAtLeast {
				t.Fatalf("got %v, want at least %d", got, c.wantProblemsAtLeast)
			}
		})
	}
}
