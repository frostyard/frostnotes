package workflowcheck

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
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
	manifestGo       = regexp.MustCompile(`/go/go([0-9.]+)\.linux-amd64\.tar\.gz",\s*"sha256": "([0-9a-f]+)"`)
)

// goPinProblems reports where mise.lock, the nsl machine's script and the
// Flatpak's manifest disagree with the Go that go.mod names, its toolchain
// line if it has one.
func goPinProblems(goMod, miseLock, provision, manifest string) []string {
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
	if found := manifestGo.FindStringSubmatch(manifest); found == nil {
		problems = append(problems, "the manifest pins no Go tarball")
	} else {
		if found[1] != want {
			problems = append(problems, fmt.Sprintf("the manifest has Go %s, go.mod %s", found[1], want))
		}
		if lock != "" && found[2] != lock {
			problems = append(problems, fmt.Sprintf("the manifest's checksum %s is not mise.lock's %s", found[2], lock))
		}
	}
	if !strings.Contains(manifest, `"GOTOOLCHAIN": "local"`) {
		problems = append(problems, "the manifest lets the build fetch another Go")
	}
	return problems
}

// TestGoPinsAgree keeps a Go bump whole: go.mod, mise.lock, the nsl machine's
// script and the Flatpak's manifest name one release, so a security release
// cannot reach some of the builds and not others.
func TestGoPinsAgree(t *testing.T) {
	read := func(parts ...string) string {
		data, err := os.ReadFile(filepath.Join(append([]string{"..", ".."}, parts...)...))
		if err != nil {
			t.Fatal(err)
		}
		return string(data)
	}
	manifest := read("packaging", "flatpak", "org.frostyard.Frostnotes.json")
	for _, p := range goPinProblems(read("go.mod"), read("mise.lock"), read("dev", "nsl", "provision.sh"), manifest) {
		t.Error(p)
	}
}

func TestGoPinProblems(t *testing.T) {
	const (
		mod    = "module m\n\ngo 1.2.3\n"
		lock   = "[[tools.go]]\nversion = \"1.2.3\"\n\n[tools.go.\"platforms.linux-x64\"]\nchecksum = \"sha256:abc\"\nurl = \"https://dl.google.com/go/go1.2.3.linux-amd64.tar.gz\"\n"
		script = "GO_VERSION=1.2.3\nGO_SHA256=abc\n"
		pinned = `"url": "https://dl.google.com/go/go1.2.3.linux-amd64.tar.gz",
          "sha256": "abc", "GOTOOLCHAIN": "local"`
	)
	cases := []struct {
		name, mod, lock, script, manifest string
		problems                          bool
	}{
		{"agreeing", mod, lock, script, pinned, false},
		{"toolchain line wins", "go 1.2.0\ntoolchain go1.2.3\n", lock, script, pinned, false},
		{"old toolchain line", "go 1.2.3\ntoolchain go1.2.2\n", lock, script, pinned, true},
		{"old lock", "go 1.2.4\n", lock, "GO_VERSION=1.2.4\nGO_SHA256=abc\n", strings.ReplaceAll(pinned, "1.2.3", "1.2.4"), true},
		{"old script", mod, lock, "GO_VERSION=1.2.2\nGO_SHA256=abc\n", pinned, true},
		{"old checksum", mod, lock, "GO_VERSION=1.2.3\nGO_SHA256=def\n", pinned, true},
		{"no go line", "module m\n", lock, script, pinned, true},
		{"old manifest", mod, lock, script, strings.ReplaceAll(pinned, "1.2.3", "1.2.2"), true},
		{"manifest's checksum", mod, lock, script, strings.ReplaceAll(pinned, "abc", "def"), true},
		{"no tarball", mod, lock, script, `"GOTOOLCHAIN": "local"`, true},
		{"fetches another Go", mod, lock, script, strings.ReplaceAll(pinned, `"local"`, `"auto"`), true},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := goPinProblems(c.mod, c.lock, c.script, c.manifest)
			if c.problems != (len(got) > 0) {
				t.Fatalf("problems %v, want any: %v", got, c.problems)
			}
		})
	}
}
