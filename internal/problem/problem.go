// Package problem names the coded errors that notesd records in the local
// journal, and the small interface through which its packages record them
// (docs/adr/0036-notesd-and-the-app-record-problems-in-the-journal.md).
//
// notesd makes 1 recorder of github.com/frostyard/std/problems in main and
// passes it to each package in its Config as a Recorder. A package never
// imports the journal writer itself, and a nil Recorder records nothing.
// Only the code is the contract: the error's text stays on the host.
package problem

// The codes notesd records. Each is recorded at 1 place.
const (
	// StoreMigrateFailed: the database cannot be opened and migrated: its
	// first transaction or a migration fails, or its schema is not this
	// notesd's (store.Open's *store.MigrateError, recorded by notesd).
	StoreMigrateFailed = "store.migrate_failed"
	// MergeFailed: the merge library panics on a merge of the vault
	// (vault.guardMerge).
	MergeFailed = "merge.failed"
	// VaultWriteFailed: a write to the vault fails for a reason that is not
	// the client's: the blob store, the staged file, its rename, or the
	// index transaction after the rename (vault.Vault.writeFailed).
	VaultWriteFailed = "vault.write_failed"
	// WatchOverflowRescan: the kernel's inotify queue overflowed, so the
	// vault is scanned again (vault.watcher.watchError).
	WatchOverflowRescan = "watch.overflow_rescan"
	// SyncProtocolError: a device's sync session with a hub ended on a
	// protocol fault (syncer.protocolFault).
	SyncProtocolError = "sync.protocol_error"
)

// Recorder records a coded error. *problems.Recorder of
// github.com/frostyard/std implements it.
type Recorder interface {
	Error(code string, err error)
}

// Record records code with err on r. A nil r records nothing.
func Record(r Recorder, code string, err error) {
	if r != nil {
		r.Error(code, err)
	}
}
