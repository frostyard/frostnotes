package store

import (
	"context"
	"errors"
	"time"

	"github.com/frostyard/frostnotes/api"
)

// The hub's log (docs/specs/sync-protocol.md, ADR-0017): the vaults it
// serves, their revisions, and each file's place in the log.

// HubVault is a hub_vaults row, with its vault's name.
type HubVault struct {
	VaultID  int64
	SyncID   string // 32 lowercase hex digits
	Name     string // the vault's name
	Seq      int64  // the log's latest sequence number; 0 before any revision
	ServedAt time.Time
}

// API converts v to the hub domain's form. Task card T-0024.
func (v HubVault) API() api.HubVault { return api.HubVault{} }

// Log converts v to the log domain's form. Task card T-0024.
func (v HubVault) Log() api.LogVault { return api.LogVault{} }

// Revision is a revisions row (ADR-0017).
type Revision struct {
	VaultID   int64
	Seq       int64
	FileID    int64 // the hub's files.id
	Op        api.RevisionOp
	Path      string // the file's path after this revision
	Hash      string // the content; "" for a delete
	Size      int64
	Base      int64 // the revision it was based on; 0 for a create
	DeviceID  int64 // 0 for the hub's own writers
	ReplicaID int64 // 0 unless a push made it
	PushID    int64 // 0 unless a push made it
	Source    api.VersionSource
	Author    string
	At        time.Time
	LostTo    int64 // a conflict revision's winner; 0 otherwise
	Pruned    bool  // its content is no longer kept (ADR-0019)
}

// HubFile is a hub_files row: a file's place in its vault's log.
type HubFile struct {
	VaultID    int64
	FileID     int64
	Seq        int64 // its latest revision of any kind
	Revision   int64 // its head: the latest create, put, move or delete
	Conflicted bool
}

// HubID returns this notesd's hub ID, making it on first use: 32 lowercase
// hex digits from crypto/rand, stored with created_at = t.Now(). Task card
// T-0024.
func (t *Tx) HubID(ctx context.Context) (string, error) {
	return "", errors.New("store.HubID is not implemented yet (T-0024)")
}

// HubIdentity returns the hub ID, or "" when this notesd has never served
// a vault. Task card T-0024.
func (d *DB) HubIdentity(ctx context.Context) (string, error) {
	return "", errors.New("store.HubIdentity is not implemented yet (T-0024)")
}

// ServeVault makes vault vaultID a hub vault, with a new sync ID (32
// lowercase hex digits from crypto/rand), seq 0 and served_at = t.Now(),
// making the hub ID too if it is missing. A vault that is already served
// is ErrConflict; a vault that does not exist, ErrNotFound. Task card
// T-0024.
func (t *Tx) ServeVault(ctx context.Context, vaultID int64) (HubVault, error) {
	return HubVault{}, errors.New("store.ServeVault is not implemented yet (T-0024)")
}

// HubVaults returns every served vault, by name, then vault ID; a non-nil
// empty slice when none. Task card T-0024.
func (d *DB) HubVaults(ctx context.Context) ([]HubVault, error) {
	return nil, errors.New("store.HubVaults is not implemented yet (T-0024)")
}

// HubVault returns served vault vaultID, or ErrNotFound when it is not
// served. Task card T-0024.
func (d *DB) HubVault(ctx context.Context, vaultID int64) (HubVault, error) {
	return HubVault{}, errors.New("store.HubVault is not implemented yet (T-0024)")
}

// HubVault returns served vault vaultID as this transaction sees it, or
// ErrNotFound. Task card T-0024.
func (t *Tx) HubVault(ctx context.Context, vaultID int64) (HubVault, error) {
	return HubVault{}, errors.New("store.Tx.HubVault is not implemented yet (T-0024)")
}

// HubVaultBySync returns the served vault with sync ID syncID, or
// ErrNotFound. Task card T-0024.
func (d *DB) HubVaultBySync(ctx context.Context, syncID string) (HubVault, error) {
	return HubVault{}, errors.New("store.HubVaultBySync is not implemented yet (T-0024)")
}

// AppendRevision appends r to the log of vault r.VaultID and returns it as
// stored: Seq is the vault's seq + 1 and At is t.Now() to the millisecond
// (r.Seq and r.At are ignored). In the same transaction it sets
// hub_vaults.seq, and the file's hub_files row: seq always; revision for a
// create, put, move or delete; conflicted to 1 for a conflict and to 0 for
// a resolve. It emits api.LogAppended{Vault: the sync ID, Seq}. A vault
// that is not served is ErrNotFound. A create of a file the log already
// has, any other op for a file it does not have, and a delete, conflict or
// resolve whose Hash disagrees with its op (a delete has "", the others
// a hash) are errors, and nothing is appended. Task card T-0024.
func (t *Tx) AppendRevision(ctx context.Context, r Revision) (Revision, error) {
	return Revision{}, errors.New("store.AppendRevision is not implemented yet (T-0024)")
}

// HubFile returns file fileID's place in vault vaultID's log, or
// ErrNotFound. Task card T-0024.
func (d *DB) HubFile(ctx context.Context, vaultID, fileID int64) (HubFile, error) {
	return HubFile{}, errors.New("store.HubFile is not implemented yet (T-0024)")
}

// HubFile returns file fileID's place in the log as this transaction sees
// it, or ErrNotFound. Task card T-0024.
func (t *Tx) HubFile(ctx context.Context, vaultID, fileID int64) (HubFile, error) {
	return HubFile{}, errors.New("store.Tx.HubFile is not implemented yet (T-0024)")
}

// Revision returns revision seq of vault vaultID, or ErrNotFound. Task
// card T-0024.
func (d *DB) Revision(ctx context.Context, vaultID, seq int64) (Revision, error) {
	return Revision{}, errors.New("store.Revision is not implemented yet (T-0024)")
}

// Revision returns revision seq as this transaction sees it, or
// ErrNotFound. Task card T-0024.
func (t *Tx) Revision(ctx context.Context, vaultID, seq int64) (Revision, error) {
	return Revision{}, errors.New("store.Tx.Revision is not implemented yet (T-0024)")
}

// LogFile returns file fileID as log.pull shows it: its hub_files row with
// its head revision's path, hash and size, and Deleted when the head is a
// delete. ErrNotFound when the log does not have the file. Task card
// T-0024.
func (d *DB) LogFile(ctx context.Context, vaultID, fileID int64) (api.LogFile, error) {
	return api.LogFile{}, errors.New("store.LogFile is not implemented yet (T-0024)")
}

// Pull returns the files of vault vaultID whose hub_files.seq is after
// after, by that seq, at most limit of them (500 when limit is 0 or less;
// more than 1,000 is 1,000), each as LogFile returns it. Next is the last
// file's seq, or after when there are none; More is whether a file after
// Next remains. Files is non-nil. Task card T-0024.
func (d *DB) Pull(ctx context.Context, vaultID, after int64, limit int) (api.LogPage, error) {
	return api.LogPage{}, errors.New("store.Pull is not implemented yet (T-0024)")
}

// HasContent reports whether a create, put, move or conflict revision of a
// served vault that is not pruned names hash: the blobs /sync/blobs hands
// out. A delete or resolve revision holds no content of its own. Task card
// T-0024.
func (d *DB) HasContent(ctx context.Context, hash string) (bool, error) {
	return false, errors.New("store.HasContent is not implemented yet (T-0024)")
}
