package store

import (
	"context"
	"errors"
	"time"

	"github.com/frostyard/frostnotes/api"
)

// Replicas, pushes and the queries devices make of the hub's log
// (docs/specs/sync-protocol.md).

// Replica is a hub_replicas row: one device's join of one vault.
type Replica struct {
	ID       int64
	DeviceID int64
	VaultID  int64
	JoinedAt time.Time
}

// PushAnswer is a hub_pushes row: what the hub answered one push.
type PushAnswer struct {
	ReplicaID  int64
	PushID     int64
	FileID     int64 // 0 for a refused create
	Status     api.PushStatus
	Reason     api.RefuseReason // "" unless refused
	Conflicts  int64
	BasePruned bool
	At         time.Time
}

// API converts r to its wire form, with deviceName its device's name ("hub"
// for the hub's own writers). Task card T-0025.
func (r Revision) API(deviceName string) api.Revision { return api.Revision{} }

// AddReplica records a new replica of vault vaultID for device deviceID,
// with joined_at = t.Now(). ErrNotFound when the vault is not served or the
// device does not exist. Task card T-0025.
func (t *Tx) AddReplica(ctx context.Context, deviceID, vaultID int64) (Replica, error) {
	return Replica{}, errors.New("store.AddReplica is not implemented yet (T-0025)")
}

// Replica returns replica id, or ErrNotFound. Task card T-0025.
func (d *DB) Replica(ctx context.Context, id int64) (Replica, error) {
	return Replica{}, errors.New("store.Replica is not implemented yet (T-0025)")
}

// RecordPush stores a's answer with at = t.Now() (a.At is ignored). A
// push ID the replica has an answer for is ErrConflict. Task card T-0025.
func (t *Tx) RecordPush(ctx context.Context, a PushAnswer) error {
	return errors.New("store.RecordPush is not implemented yet (T-0025)")
}

// PushAnswer returns the answer to push pushID of replica replicaID as this
// transaction sees it, or ErrNotFound. Task card T-0025.
func (t *Tx) PushAnswer(ctx context.Context, replicaID, pushID int64) (PushAnswer, error) {
	return PushAnswer{}, errors.New("store.Tx.PushAnswer is not implemented yet (T-0025)")
}

// PushRecords returns, for each push ID in pushes, in that order, the
// revisions of vault vaultID that replica replicaID's push made, by seq,
// with their device's name; Revisions is a non-nil empty slice when there
// are none. Task card T-0025.
func (d *DB) PushRecords(ctx context.Context, vaultID, replicaID int64, pushes []int64) ([]api.PushRecord, error) {
	return nil, errors.New("store.PushRecords is not implemented yet (T-0025)")
}

// Match answers log.match for vault vaultID, in the order of qs: for each
// query, File is the file last at q.Path (the file of the newest revision
// whose path is q.Path, other than a conflict or resolve), and Revision is
// that file's newest create, put or move with hash q.Hash. Either is nil
// when there is none. Task card T-0025.
func (d *DB) Match(ctx context.Context, vaultID int64, qs []api.MatchQuery) ([]api.LogMatch, error) {
	return nil, errors.New("store.Match is not implemented yet (T-0025)")
}

// History returns file fileID's revisions in vault vaultID, newest first,
// only those before seq before when before > 0, at most limit (50 when
// limit is 0 or less; more than 200 is 200), with their device's name, or
// "hub" when DeviceID is 0. A file the log does not have is ErrNotFound.
// Task card T-0025.
func (d *DB) History(ctx context.Context, vaultID, fileID, before int64, limit int) ([]api.Revision, error) {
	return nil, errors.New("store.History is not implemented yet (T-0025)")
}
