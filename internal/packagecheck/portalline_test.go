package packagecheck

import (
	"os"
	"regexp"
	"strings"
	"testing"
)

// portalFailure is the Rust constant that holds the start of the launcher's
// line for a notesd the Flatpak portal did not start.
var portalFailure = regexp.MustCompile(`(?m)^const PORTAL_FAILURE: &str = "([^"\\]+)";$`)

// The window says why notesd is not running when the last line of
// notesd.log is the launcher's portal failure (app/src-tauri/src/notesd_log.rs,
// docs/specs/ui.md#connecting). The launcher is a shell script that another
// change may rewrite, so its line must stay the one the app recognises. This
// check lives here and not in the Rust crate: the Flatpak builds and tests
// the crate from app/ alone, with no packaging/ tree to read.
func TestPortalFailureLineMatchesTheApp(t *testing.T) {
	rs, err := os.ReadFile("../../app/src-tauri/src/notesd_log.rs")
	if err != nil {
		t.Fatal(err)
	}
	m := portalFailure.FindSubmatch(rs)
	if m == nil {
		t.Fatal("notesd_log.rs holds no PORTAL_FAILURE constant in the expected form")
	}
	sh, err := os.ReadFile("../../packaging/flatpak/frostnotes.sh")
	if err != nil {
		t.Fatal(err)
	}
	line := `msg="` + string(m[1]) + `; see $state/notesd.log"`
	if !strings.Contains(string(sh), line) {
		t.Errorf("frostnotes.sh no longer holds %s", line)
	}
}
