package packagecheck

import (
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"
)

const (
	flatpakManifest = "org.frostyard.Frostnotes.json"
	flatpakMetainfo = "org.frostyard.Frostnotes.metainfo.xml"
)

// stampInputs copies the Flatpak's manifest and metainfo into a temporary
// directory, so that stamp-version.sh edits the copies and not the tree.
func stampInputs(t *testing.T) (dir, script string) {
	t.Helper()
	if runtime.GOOS != "linux" {
		t.Skip("the Flatpak builds on Linux")
	}
	src, err := filepath.Abs("../../packaging/flatpak")
	if err != nil {
		t.Fatal(err)
	}
	script = filepath.Join(src, "stamp-version.sh")
	dir = t.TempDir()
	for _, name := range []string{flatpakManifest, flatpakMetainfo} {
		data, err := os.ReadFile(filepath.Join(src, name))
		if err != nil {
			t.Fatal(err)
		}
		write(t, filepath.Join(dir, name), string(data))
	}
	return dir, script
}

// stamp runs stamp-version.sh on dir.
func stamp(t *testing.T, dir, script, version, commit string) (string, error) {
	t.Helper()
	out, err := exec.Command("sh", script, version, commit, dir).CombinedOutput()
	return string(out), err
}

// readString reads path, or fails the test.
func readString(t *testing.T, path string) string {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}

// indent is the leading whitespace of line.
func indent(line string) string {
	return line[:len(line)-len(strings.TrimLeft(line, " \t"))]
}

// A v* tag stamps its version into the manifest and the metainfo, so that
// the Flatpak's notesd reports the release and software centres show it.
func TestStampVersionWritesTheTagIntoTheFlatpak(t *testing.T) {
	dir, script := stampInputs(t)
	out, err := stamp(t, dir, script, "1.2.3", "0123456789ab")
	if err != nil {
		t.Fatalf("stamp 1.2.3: %v\n%s", err, out)
	}

	manifest := readString(t, filepath.Join(dir, flatpakManifest))
	if want := "-X main.version=1.2.3 -X main.commit=0123456789ab"; !strings.Contains(manifest, want) {
		t.Errorf("the manifest does not hold %q:\n%s", want, manifest)
	}
	if strings.Contains(manifest, "-X main.version=flatpak") {
		t.Error("the manifest still names flatpak")
	}

	meta := readString(t, filepath.Join(dir, flatpakMetainfo))
	line := `<release version="1.2.3" date="` + time.Now().UTC().Format("2006-01-02") + `"/>`
	lines := strings.Split(meta, "\n")
	at := -1
	for i, l := range lines {
		if strings.TrimSpace(l) == "<releases>" {
			at = i
			break
		}
	}
	if at < 0 || at+2 >= len(lines) {
		t.Fatalf("no <releases> with a child in the metainfo:\n%s", meta)
	}
	if got := strings.TrimSpace(lines[at+1]); got != line {
		t.Errorf("the first release = %q, want %q", got, line)
	}
	if got, want := indent(lines[at+1]), indent(lines[at+2]); got != want {
		t.Errorf("the new release is indented %q, the one it precedes %q", got, want)
	}
	if !strings.Contains(meta, `<release version="0.0.0"`) {
		t.Error("the metainfo lost its 0.0.0 release")
	}
}

// The script refuses input that no tag produces, and leaves the files as
// they were.
func TestStampVersionRefusesBadArguments(t *testing.T) {
	for _, tc := range []struct{ name, version, commit string }{
		{"two parts", "1.2", "0123456789ab"},
		{"four parts", "1.2.3.4", "0123456789ab"},
		{"a v", "v1.2.3", "0123456789ab"},
		{"uppercase commit", "1.2.3", "0123456789AB"},
		{"short commit", "1.2.3", "012345"},
		{"long commit", "1.2.3", strings.Repeat("a", 41)},
	} {
		t.Run(tc.name, func(t *testing.T) {
			dir, script := stampInputs(t)
			out, err := stamp(t, dir, script, tc.version, tc.commit)
			if err == nil {
				t.Fatalf("stamp %q %q succeeded:\n%s", tc.version, tc.commit, out)
			}
			if !strings.Contains(out, "stamp-version.sh:") {
				t.Errorf("no message for %q %q:\n%s", tc.version, tc.commit, out)
			}
			manifest := readString(t, filepath.Join(dir, flatpakManifest))
			if !strings.Contains(manifest, "-X main.version=flatpak") {
				t.Error("the manifest changed under bad arguments")
			}
		})
	}
}

// A second run with a version the metainfo already lists fails, so that a
// re-run cannot list one release twice.
func TestStampVersionRefusesToStampTwice(t *testing.T) {
	dir, script := stampInputs(t)
	if out, err := stamp(t, dir, script, "1.2.3", "0123456789ab"); err != nil {
		t.Fatalf("first stamp: %v\n%s", err, out)
	}
	out, err := stamp(t, dir, script, "1.2.3", "0123456789ab")
	if err == nil {
		t.Fatalf("a second stamp succeeded:\n%s", out)
	}
	meta := readString(t, filepath.Join(dir, flatpakMetainfo))
	if n := strings.Count(meta, `<release version="1.2.3"`); n != 1 {
		t.Errorf("the metainfo lists 1.2.3 %d times, want once", n)
	}
}

// The metainfo's own record of the version is what stops a second run, even
// when the manifest is fresh again.
func TestStampVersionRefusesAVersionAlreadyListed(t *testing.T) {
	dir, script := stampInputs(t)
	meta := readString(t, filepath.Join(dir, flatpakMetainfo))
	meta = strings.Replace(meta, `    <release version="0.0.0"`,
		"    <release version=\"1.2.3\" date=\"2026-01-01\"/>\n    <release version=\"0.0.0\"", 1)
	write(t, filepath.Join(dir, flatpakMetainfo), meta)
	out, err := stamp(t, dir, script, "1.2.3", "0123456789ab")
	if err == nil {
		t.Fatalf("stamp listed 1.2.3 twice:\n%s", out)
	}
	if !strings.Contains(out, "already lists version 1.2.3") {
		t.Errorf("the message does not name the listed version:\n%s", out)
	}
}
