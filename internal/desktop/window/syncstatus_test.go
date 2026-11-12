package window

import (
	"testing"

	"github.com/frostyard/frostnotes/api"
)

// hubStatus is a vault's sync status with a hub.
func hubStatus(vault int64, url string) api.SyncStatus {
	return api.SyncStatus{VaultID: vault, URL: url, HubVault: "Shared", State: api.SyncStateIdle,
		Refused: []api.SyncRefusal{}, Skipped: []string{}}
}

// settled waits until no load and no ask of the sync status runs.
func (f *fixture) settled() {
	f.t.Helper()
	f.until("the loads", func() bool { return !f.a.settingsAsked && !f.a.syncWaiting })
}

// A sync.changed that comes while sync.status is asked can be older than
// the answer: sent before a leave, it would bring the hub back, and no
// event follows a leave. The status asked again after it settles it: here
// the vault syncs with no hub.
func TestAnEventWhileTheStatusIsAskedIsSettledByAStatusAskedAfterIt(t *testing.T) {
	f := start(t)
	f.openVault()
	f.a.loadVaultData() // the vault syncs with no hub: the answer holds no status for it
	f.a.event(0, api.SyncChanged{Status: hubStatus(f.Vault, "http://hub.test")})
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
	ask := f.a.syncAsk
	f.a.event(0, api.SyncChanged{Status: hubStatus(f.Vault, "http://hub.test")})
	f.a.takeSyncStatus(f.Vault, ask, nil, nil) // the answer asked before the event, with no status
	if f.a.syncInfo == nil || f.a.syncInfo.URL != "http://hub.test" {
		t.Fatalf("the answer replaced the event's status: %+v", f.a.syncInfo)
	}
	if f.a.syncAsk != ask+1 || !f.a.syncWaiting {
		t.Fatal("the status is not asked again")
	}
	f.settled()
}

// An older ask's answer that comes after a newer one does not show: asked
// before a leave, it would bring back the hub the vault left, and no event
// follows a leave.
func TestAnOlderSyncStatusAnswerDoesNotShow(t *testing.T) {
	f := start(t)
	f.openVault()
	f.a.askSyncStatus(f.Vault)
	before := f.a.syncAsk
	f.a.loadVaultData() // after the leave: the vault syncs with no hub
	f.settled()
	f.a.takeSyncStatus(f.Vault, before, []api.SyncStatus{hubStatus(f.Vault, "http://left.test")}, nil)
	if f.a.syncInfo != nil {
		t.Fatalf("an older answer shows: %+v", f.a.syncInfo)
	}
}

// After a switch to another vault and back, an answer asked before it does
// not show, nor does it keep the last ask from showing.
func TestASyncStatusAnswerFromBeforeAVaultSwitchDoesNotShow(t *testing.T) {
	f := start(t)
	other, err := f.Other.Vault().Add(t.Context(), &api.VaultAddParams{Path: t.TempDir()})
	if err != nil {
		t.Fatal(err)
	}
	f.openVault()
	f.until("the other vault", func() bool { return len(f.a.vaults) == 2 })
	home := *f.a.vault
	f.a.askSyncStatus(home.ID)
	before := f.a.syncAsk
	f.a.showVault(other)
	f.a.showVault(&home)
	f.settled()
	f.a.takeSyncStatus(home.ID, before, []api.SyncStatus{hubStatus(home.ID, "http://left.test")}, nil)
	if f.a.syncInfo != nil {
		t.Fatalf("an answer from before the switch shows: %+v", f.a.syncInfo)
	}
	if f.a.syncWaiting {
		t.Fatal("the last ask still waits")
	}
}
