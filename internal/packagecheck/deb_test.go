package packagecheck

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// The .deb's nFPM config names the package, takes its version from
// DEB_VERSION and packs only the staged /usr tree (core ADR-0011, ADR-0058).
func TestNfpmConfig(t *testing.T) {
	b, err := os.ReadFile("../../packaging/nfpm.yaml")
	if err != nil {
		t.Fatal(err)
	}
	cfg := string(b)
	for _, want := range []string{
		"name: frostyard-notesd\n",
		"version: ${DEB_VERSION}\n",
		"arch: amd64\n",
		"src: ./build/deb/root/usr\n",
		"dst: /usr\n",
	} {
		if !strings.Contains(cfg, want) {
			t.Errorf("packaging/nfpm.yaml lacks %q", want)
		}
	}
	for _, bad := range []string{"scripts:", "/etc", "/var", "/opt"} {
		if strings.Contains(cfg, bad) {
			t.Errorf("packaging/nfpm.yaml names %q; a sysext payload is /usr only", bad)
		}
	}
}

// deb-check.sh refuses a .deb that is missing and a version argument that
// is absent; the full check runs on a real package in make deb and CI.
func TestDebCheckRefusesMissingInput(t *testing.T) {
	script, err := filepath.Abs("../../packaging/deb-check.sh")
	if err != nil {
		t.Fatal(err)
	}
	if out, err := exec.Command("sh", script).CombinedOutput(); err == nil {
		t.Errorf("deb-check.sh without arguments passed:\n%s", out)
	}
	missing := filepath.Join(t.TempDir(), "x.deb")
	out, err := exec.Command("sh", script, missing, "frostyard-notesd", "1.2.3").CombinedOutput()
	if err == nil || !strings.Contains(string(out), "is missing") {
		t.Errorf("deb-check.sh on a missing file: err=%v\n%s", err, out)
	}
}
