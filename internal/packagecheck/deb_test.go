package packagecheck

import (
	"os"
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
