package packagecheck

import (
	"os"
	"strings"
	"testing"

	"github.com/frostyard/frostnotes/internal/flatpakapp"
)

// notesctl status says why notesd is not running when the last line of
// notesd.log is the launcher's portal failure (internal/flatpakapp,
// docs/specs/ui.md#connecting). The launcher is a shell script that another
// change may rewrite, so its line must stay the one notesctl recognises.
func TestPortalFailureLineMatchesTheApp(t *testing.T) {
	sh, err := os.ReadFile("../../packaging/flatpak/frostnotes.sh")
	if err != nil {
		t.Fatal(err)
	}
	line := `msg="` + flatpakapp.PortalFailure + `; see $state/notesd.log"`
	if !strings.Contains(string(sh), line) {
		t.Errorf("frostnotes.sh no longer holds %s", line)
	}
}
