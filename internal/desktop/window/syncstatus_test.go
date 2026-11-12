package window

import (
	"testing"

	"github.com/frostyard/frostnotes/api"
)

// A sync.changed that comes while sync.status is asked is newer than the
// answer, which must not show the state the loop has left. After a join,
// the answer could take the hub's address away from the status bar.
func TestASyncEventWhileTheStatusIsAskedOutlivesTheAnswer(t *testing.T) {
	f := start(t)
	f.openVault()
	f.a.loadVaultData() // the vault syncs with no hub: the answer holds no status for it
	f.a.event(0, api.SyncChanged{Status: api.SyncStatus{VaultID: f.Vault, URL: "http://hub.test", HubVault: "Shared",
		State: api.SyncStateIdle, Refused: []api.SyncRefusal{}, Skipped: []string{}}})
	f.until("the answer", func() bool { return !f.a.settingsAsked })
	if f.a.syncInfo == nil || f.a.syncInfo.URL != "http://hub.test" {
		t.Fatalf("the answer replaced the event's status: %+v", f.a.syncInfo)
	}
}
