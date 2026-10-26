package packagecheck

import (
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/frostyard/lodge/kit/manifest"
)

const (
	appUser = "frostnotes"
)

var binaries = []string{"notesd", "notesctl"}

// stage runs packaging/lodge/stage.sh, as make lodge-package does.
func stage(t *testing.T, args ...string) (string, error) {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("lodge packages for Linux")
	}
	script, err := filepath.Abs("../../packaging/lodge/stage.sh")
	if err != nil {
		t.Fatal(err)
	}
	out, err := exec.Command("sh", append([]string{script}, args...)...).CombinedOutput()
	return string(out), err
}

func write(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

// standIns writes stand-in binaries and a stand-in web build.
func standIns(t *testing.T) string {
	t.Helper()
	build := t.TempDir()
	for _, b := range binaries {
		write(t, filepath.Join(build, b), b)
	}
	write(t, filepath.Join(build, "web/index.html"), "<!doctype html>")
	write(t, filepath.Join(build, "web/icon.svg"), "<svg/>")
	return build
}

// staged stages into an empty root and returns it.
func staged(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	if out, err := stage(t, root, "v1.2.3", standIns(t)); err != nil {
		t.Fatalf("stage.sh: %v\n%s", err, out)
	}
	return root
}

func readLines(t *testing.T, path string) []string {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return strings.Split(string(b), "\n")
}

func TestStagesOnlyUnderUsr(t *testing.T) {
	root := staged(t)
	n := 0
	err := filepath.WalkDir(root, func(p string, _ fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(root, p)
		if err != nil {
			return err
		}
		if rel == "." || rel == "usr" {
			return nil
		}
		n++
		if !strings.HasPrefix(rel, "usr"+string(filepath.Separator)) {
			t.Errorf("%s is staged outside usr/", rel)
		}
		if strings.HasPrefix(rel, "usr/local") {
			t.Errorf("%s is staged under usr/local", rel)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if n == 0 {
		t.Fatal("nothing staged")
	}
}

func TestStagesTheExactFiles(t *testing.T) {
	root := staged(t)
	for _, b := range binaries {
		fi, err := os.Stat(filepath.Join(root, "usr/bin", b))
		if err != nil {
			t.Errorf("usr/bin/%s not staged: %v", b, err)
		} else if fi.Mode().Perm() != 0o755 {
			t.Errorf("usr/bin/%s mode %v, want 0755", b, fi.Mode().Perm())
		}
	}
	for _, p := range []string{
		"usr/lib/systemd/system/frostnotes.service",
		"usr/lib/systemd/system/frostnotes-lodge.socket",
		"usr/lib/sysusers.d/frostnotes.conf",
		"usr/share/lodge/apps/notes/app.json",
		"usr/share/lodge/apps/notes/web/index.html",
		"usr/share/lodge/apps/notes/web/icon.svg",
	} {
		if _, err := os.Stat(filepath.Join(root, p)); err != nil {
			t.Errorf("%s not staged: %v", p, err)
		}
	}
	m, err := manifest.Load(filepath.Join(root, "usr/share/lodge/apps/notes"))
	if err != nil {
		t.Fatalf("staged manifest: %v", err)
	}
	if m.Version != "v1.2.3" || m.ID != "notes" || m.User != appUser {
		t.Errorf("staged manifest %+v, want notes at v1.2.3 for %s", m, appUser)
	}
}

func units(t *testing.T, root string) []string {
	t.Helper()
	paths, err := filepath.Glob(filepath.Join(root, "usr/lib/systemd/system/*"))
	if err != nil || len(paths) == 0 {
		t.Fatalf("no staged units: %v", err)
	}
	return paths
}

func TestUnitRules(t *testing.T) {
	root := staged(t)
	stagedBins := map[string]bool{}
	for _, b := range binaries {
		stagedBins["/usr/bin/"+b] = true
	}
	execs := 0
	for _, u := range units(t, root) {
		name := filepath.Base(u)
		for _, l := range readLines(t, u) {
			l = strings.TrimSpace(l)
			switch {
			case strings.HasPrefix(l, "RequiredBy="):
				t.Errorf("%s has %s (core ADR-0030)", name, l)
			case strings.HasPrefix(l, "Condition"):
				t.Errorf("%s has %s (snosi#1100)", name, l)
			case strings.HasSuffix(name, ".socket") && strings.HasPrefix(l, "DefaultDependencies="):
				t.Errorf("%s has %s; a socket keeps its default dependencies", name, l)
			case strings.HasPrefix(l, "ExecStart="):
				execs++
				fields := strings.Fields(strings.TrimLeft(strings.TrimPrefix(l, "ExecStart="), "@-:+!"))
				if len(fields) == 0 || !stagedBins[fields[0]] {
					t.Errorf("%s: %s does not start a binary staged in /usr/bin", name, l)
				}
			}
		}
	}
	if execs == 0 {
		t.Error("no unit has an ExecStart=")
	}
}

func TestSysusers(t *testing.T) {
	root := staged(t)
	have := map[string]bool{}
	for _, l := range readLines(t, filepath.Join(root, "usr/lib/sysusers.d/frostnotes.conf")) {
		l = strings.TrimSpace(l)
		if l == "" || strings.HasPrefix(l, "#") {
			continue
		}
		f := strings.Fields(l)
		switch {
		case l == "g lodge -":
			have["g"] = true
		case f[0] == "u" && f[1] == appUser:
			have["u"] = true
		case f[0] == "m" && f[1] == appUser && f[2] == "lodge-apps":
			have["m"] = true
		case f[0] == "m" && f[1] == appUser && f[2] == "lodge":
			t.Errorf("%q: the user must not be in group lodge", l)
		}
	}
	for _, k := range []string{"g", "u", "m"} {
		if !have[k] {
			t.Errorf("sysusers.d entry lacks its %q line", k)
		}
	}
}

func TestUnitsVerify(t *testing.T) {
	analyze, err := exec.LookPath("systemd-analyze")
	if err != nil {
		t.Skip("no systemd-analyze")
	}
	root := staged(t)
	dir := t.TempDir()
	var names []string
	for _, u := range units(t, root) {
		lines := readLines(t, u)
		for i, l := range lines {
			// verify wants each ExecStart to exist.
			if strings.HasPrefix(l, "ExecStart=") {
				lines[i] = "ExecStart=/bin/sh -c true"
			}
		}
		name := filepath.Base(u)
		write(t, filepath.Join(dir, name), strings.Join(lines, "\n"))
		names = append(names, "./"+name)
	}
	cmd := exec.Command(analyze, append([]string{"verify"}, names...)...)
	cmd.Dir = dir
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Errorf("systemd-analyze verify: %v\n%s", err, out)
	}
}

// TestStagingKeepsASharedRoot stages into a lodge root that other apps and
// lodge itself use: only notes' files change.
func TestStagingKeepsASharedRoot(t *testing.T) {
	root := t.TempDir()
	others := map[string]string{
		"usr/share/lodge/apps/mail/app.json":      `{"id": "mail"}`,
		"usr/share/lodge/apps/mail/web/a.js":      "mail",
		"var/lib/lodge/state.json":                "lodge's state",
		"var/lib/frostnotes/vault/Plan.md":        "# Plan",
		"run/lodge/apps/mail.sock":                "",
		"usr/bin/lodge":                           "lodge",
		"usr/lib/systemd/system/lodge.service":    "[Unit]",
		"usr/share/lodge/apps/notes/web/old.js":   "a stale bundle file",
		"usr/share/lodge/apps/notes/app.json":     `{"version": "old"}`,
		"usr/share/lodge/apps/notesx/app.json":    "another app",
		"usr/share/lodge/apps/notes.bak/app.json": "a backup",
	}
	for p, c := range others {
		write(t, filepath.Join(root, p), c)
	}
	if out, err := stage(t, root, "v1.2.3-4-gabc", standIns(t)); err != nil {
		t.Fatalf("stage.sh: %v\n%s", err, out)
	}
	for p, c := range others {
		if strings.HasPrefix(p, "usr/share/lodge/apps/notes/") {
			continue
		}
		if got, err := os.ReadFile(filepath.Join(root, p)); err != nil || string(got) != c {
			t.Errorf("%s = %q, %v after staging; want %q kept", p, got, err, c)
		}
	}
	if _, err := os.Stat(filepath.Join(root, "usr/share/lodge/apps/notes/web/old.js")); !os.IsNotExist(err) {
		t.Errorf("a stale file of notes' web build survived: %v", err)
	}
	m, err := manifest.Load(filepath.Join(root, "usr/share/lodge/apps/notes"))
	if err != nil {
		t.Fatalf("staged manifest: %v", err)
	}
	if m.Version != "v1.2.3-4-gabc" {
		t.Errorf("staged manifest version %q, want v1.2.3-4-gabc", m.Version)
	}
}

func TestStagingRefusesAnEmptyRootOrSlash(t *testing.T) {
	build := standIns(t)
	for _, dest := range []string{"", "/", "//", "/.", "/usr/.."} {
		if out, err := stage(t, dest, "v1", build); err == nil {
			t.Errorf("stage.sh %q succeeded:\n%s", dest, out)
		}
	}
	if out, err := stage(t, t.TempDir(), `v1"; rm -rf /tmp/x; "`, build); err == nil {
		t.Errorf("stage.sh with a version a manifest cannot carry succeeded:\n%s", out)
	}
}

// TestStagingFailsWithoutItsInputsAndChangesNothing: a build lacking a
// binary or the web build fails before it touches the destination, as lodge
// disables an app without web/index.html.
func TestStagingFailsWithoutItsInputsAndChangesNothing(t *testing.T) {
	for _, missing := range []string{"notesd", "notesctl", "web/index.html", "web/icon.svg", "web"} {
		root, build := t.TempDir(), standIns(t)
		if err := os.RemoveAll(filepath.Join(build, missing)); err != nil {
			t.Fatal(err)
		}
		kept := filepath.Join(root, "usr/share/lodge/apps/notes/web/index.html")
		write(t, kept, "old")
		write(t, filepath.Join(root, "usr/share/lodge/apps/notes/app.json"), `{"version": "old"}`)
		out, err := stage(t, root, "v1", build)
		if err == nil {
			t.Errorf("stage.sh without %s succeeded:\n%s", missing, out)
			continue
		}
		if !strings.Contains(out, "run make") {
			t.Errorf("without %s the message %q does not say what to run", missing, out)
		}
		if got, err := os.ReadFile(kept); err != nil || string(got) != "old" {
			t.Errorf("without %s: %s = %q, %v after the failed run; want kept", missing, kept, got, err)
		}
		if _, err := os.Stat(filepath.Join(root, "usr/bin")); !os.IsNotExist(err) {
			t.Errorf("without %s: the failed run staged usr/bin: %v", missing, err)
		}
	}
}
