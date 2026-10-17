package store

import (
	"context"
	"errors"
	"time"

	"github.com/frostyard/frostnotes/api"
)

// A device's sync state (docs/specs/sync-protocol.md, ADR-0017): the hubs
// it paired with, its synced vaults, each file's agreed state, and the
// pushes it made.

// SyncHub is a sync_hubs row. The device token is in the secret store.
type SyncHub struct {
	ID       int64
	HubID    string
	URL      string
	DeviceID int64 // this device's ID on the hub
	PairedAt time.Time
}

// API converts h to its wire form. Task card T-0027.
func (h SyncHub) API() api.SyncHub { return api.SyncHub{} }

// Remote is a remotes row: a local vault synced with a hub vault.
type Remote struct {
	VaultID   int64
	Hub       int64 // the sync_hubs row
	SyncID    string
	Name      string // the hub vault's name
	ReplicaID int64
	Cursor    int64
	HubSeq    int64
	JoinedAt  time.Time
	SyncedAt  time.Time // zero until the device was first in step
}

// Agreed is a sync_files row: what this device and the hub last agreed a
// file is.
type Agreed struct {
	VaultID    int64
	HubFile    int64
	FileID     int64 // the local files.id
	Seq        int64 // the latest revision of the file applied or answered
	Revision   int64 // the agreed head
	Path       string
	Hash       string
	Conflicted bool
}

// Change is a file with a change to push: the difference between the
// index and the agreed state.
type Change struct {
	FileID  int64   // the local file
	Agreed  *Agreed // nil for a create
	Path    string  // the file's path now; for a delete, the agreed path
	Hash    string  // its hash now; "" for a delete
	Deleted bool
}

// LocalPush is a sync_pushes row: a push this device made.
type LocalPush struct {
	VaultID int64
	PushID  int64
	FileID  int64
	HubFile int64 // 0 for a create
	Base    int64
	Path    string
	Hash    string // "" for a delete
	At      time.Time
	Status  api.PushStatus   // "" until answered
	Reason  api.RefuseReason // "" unless refused
}

// PutSyncHub records a pairing with h.PairedAt = t.Now(): a new row, or,
// for a hub ID the device has paired with before, that row with its URL,
// device ID and paired_at replaced. It returns the row. Task card T-0027.
func (t *Tx) PutSyncHub(ctx context.Context, h SyncHub) (SyncHub, error) {
	return SyncHub{}, errors.New("store.PutSyncHub is not implemented yet (T-0027)")
}

// SyncHubs returns the paired hubs by ID; a non-nil empty slice when none.
// Task card T-0027.
func (d *DB) SyncHubs(ctx context.Context) ([]SyncHub, error) {
	return nil, errors.New("store.SyncHubs is not implemented yet (T-0027)")
}

// SyncHub returns sync_hubs row id, or ErrNotFound. Task card T-0027.
func (d *DB) SyncHub(ctx context.Context, id int64) (SyncHub, error) {
	return SyncHub{}, errors.New("store.SyncHub is not implemented yet (T-0027)")
}

// SyncHubByURL returns the hub paired at exactly url, or ErrNotFound. Task
// card T-0027.
func (d *DB) SyncHubByURL(ctx context.Context, url string) (SyncHub, error) {
	return SyncHub{}, errors.New("store.SyncHubByURL is not implemented yet (T-0027)")
}

// AddRemote records r with joined_at = t.Now(), cursor and hub_seq as
// given, and synced_at null. A vault that is already synced is
// ErrConflict; a vault or hub that does not exist, ErrNotFound. Task card
// T-0027.
func (t *Tx) AddRemote(ctx context.Context, r Remote) (Remote, error) {
	return Remote{}, errors.New("store.AddRemote is not implemented yet (T-0027)")
}

// Remote returns vault vaultID's remote, or ErrNotFound. Task card T-0027.
func (d *DB) Remote(ctx context.Context, vaultID int64) (Remote, error) {
	return Remote{}, errors.New("store.Remote is not implemented yet (T-0027)")
}

// Remotes returns every remote by vault ID; a non-nil empty slice when
// none. Task card T-0027.
func (d *DB) Remotes(ctx context.Context) ([]Remote, error) {
	return nil, errors.New("store.Remotes is not implemented yet (T-0027)")
}

// DeleteRemote removes vault vaultID's remote with its agreed rows and
// pushes; a vault without one is ErrNotFound. Task card T-0027.
func (t *Tx) DeleteRemote(ctx context.Context, vaultID int64) error {
	return errors.New("store.DeleteRemote is not implemented yet (T-0027)")
}

// SetCursor sets vault vaultID's cursor, and its hub_seq to the larger of
// the stored one and hubSeq; ErrNotFound without a remote. Task card T-0027.
func (t *Tx) SetCursor(ctx context.Context, vaultID, cursor, hubSeq int64) error {
	return errors.New("store.SetCursor is not implemented yet (T-0027)")
}

// MarkSynced sets vault vaultID's synced_at to t.Now(); ErrNotFound
// without a remote. Task card T-0027.
func (t *Tx) MarkSynced(ctx context.Context, vaultID int64) error {
	return errors.New("store.MarkSynced is not implemented yet (T-0027)")
}

// PutAgreed inserts a, or replaces the row with its vault and hub file. A
// local file that is agreed with another hub file is ErrConflict. Task
// card T-0027.
func (t *Tx) PutAgreed(ctx context.Context, a Agreed) error {
	return errors.New("store.PutAgreed is not implemented yet (T-0027)")
}

// DeleteAgreed removes the agreed row of hub file hubFile in vault
// vaultID; a missing row is not an error. Task card T-0027.
func (t *Tx) DeleteAgreed(ctx context.Context, vaultID, hubFile int64) error {
	return errors.New("store.DeleteAgreed is not implemented yet (T-0027)")
}

// AgreedByHubFile returns the agreed row of hub file hubFile in vault
// vaultID, or ErrNotFound. Task card T-0027.
func (d *DB) AgreedByHubFile(ctx context.Context, vaultID, hubFile int64) (Agreed, error) {
	return Agreed{}, errors.New("store.AgreedByHubFile is not implemented yet (T-0027)")
}

// AgreedByFile returns the agreed row of local file fileID, or
// ErrNotFound. Task card T-0027.
func (d *DB) AgreedByFile(ctx context.Context, fileID int64) (Agreed, error) {
	return Agreed{}, errors.New("store.AgreedByFile is not implemented yet (T-0027)")
}

// AgreedRows returns vault vaultID's agreed rows by path; a non-nil empty
// slice when none. Task card T-0027.
func (d *DB) AgreedRows(ctx context.Context, vaultID int64) ([]Agreed, error) {
	return nil, errors.New("store.AgreedRows is not implemented yet (T-0027)")
}

// Pending returns vault vaultID's changes to push, comparing its files
// rows with its agreed rows (docs/specs/sync-protocol.md, Pending changes):
// a file without an agreed row is a create, a file whose path or hash
// differs from its agreed row a put, an agreed row whose file is gone a
// delete. A change whose file, path and hash (or deletion) are those of
// the file's newest push, when that push was refused, is left out. The
// order is deletes, then puts, then creates, each by path. Task card
// T-0027.
func (d *DB) Pending(ctx context.Context, vaultID int64) ([]Change, error) {
	return nil, errors.New("store.Pending is not implemented yet (T-0027)")
}

// AddPush records p with PushID the vault's highest + 1 (from 1), At =
// t.Now() and no answer (p.PushID, p.At, p.Status and p.Reason are
// ignored), and returns it. Task card T-0027.
func (t *Tx) AddPush(ctx context.Context, p LocalPush) (LocalPush, error) {
	return LocalPush{}, errors.New("store.AddPush is not implemented yet (T-0027)")
}

// AnswerPush records the answer to push pushID of vault vaultID; a push
// that does not exist is ErrNotFound. Task card T-0027.
func (t *Tx) AnswerPush(ctx context.Context, vaultID, pushID int64, status api.PushStatus, reason api.RefuseReason) error {
	return errors.New("store.AnswerPush is not implemented yet (T-0027)")
}

// Unanswered returns vault vaultID's pushes without an answer, by push ID:
// a retry sends them again with the same IDs. Task card T-0027.
func (d *DB) Unanswered(ctx context.Context, vaultID int64) ([]LocalPush, error) {
	return nil, errors.New("store.Unanswered is not implemented yet (T-0027)")
}

// Pushes returns vault vaultID's answered pushes, by push ID; a non-nil
// empty slice when none. Task card T-0027.
func (d *DB) Pushes(ctx context.Context, vaultID int64) ([]LocalPush, error) {
	return nil, errors.New("store.Pushes is not implemented yet (T-0027)")
}
