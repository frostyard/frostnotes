package window

import (
	"testing"

	"github.com/frostyard/frostnotes/api"
)

// A sync.changed that comes while sync.status is asked can be older than
// the answer: sent before a leave, it would bring the hub back, and no
// event follows a leave. The status asked again after it settles it: here
// the vault syncs with no hub.
func TestAnEventWhileTheStatusIsAskedIsSettledByAStatusAskedAfterIt(t *testing.T) {
	f := start(t)
	f.openVault()
	f.a.loadVaultData() // the vault syncs with no hub: the answer holds no status for it
	f.a.event(0, api.SyncChanged{Status: api.SyncStatus{VaultID: f.Vault, URL: "http://hub.test", HubVault: "Shared",
		State: api.SyncStateIdle, Refused: []api.SyncRefusal{}, Skipped: []string{}}})
	f.until("the vault's status with no hub", func() bool { return !f.a.settingsAsked && f.a.syncInfo == nil })
}

// A sync.changed that comes while sync.status is asked can be newer than
// the answer, which would show a state the loop has left: after a join,
// it took the hub's address away until the loop changed state again. The
// event's status stays until a status asked after it comes.
func TestAnEventWhileTheStatusIsAskedOutlivesTheAnswer(t *testing.T) {
	f := start(t)
	f.openVault()
	f.a.loadVaultData()
	event := api.SyncStatus{VaultID: f.Vault, URL: "http://hub.test", HubVault: "Shared",
		State: api.SyncStateIdle, Refused: []api.SyncRefusal{}, Skipped: []string{}}
	f.a.event(0, api.SyncChanged{Status: event})
	f.a.takeSyncStatus(f.Vault, f.a.syncEvents-1, nil) // the answer asked before the event, with no status
	if f.a.syncInfo == nil || f.a.syncInfo.URL != event.URL {
		t.Fatalf("the answer replaced the event's status: %+v", f.a.syncInfo)
	}
	if f.a.syncAsking != f.Vault {
		t.Fatal("the status is not asked again")
	}
	f.until("the loads", func() bool { return !f.a.settingsAsked && f.a.syncAsking == 0 })
}
