package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
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
func (r Revision) API(deviceName string) api.Revision {
	out := api.Revision{
		Seq: r.Seq, File: r.FileID, Op: r.Op, Path: r.Path, Hash: r.Hash,
		Size: r.Size, Base: r.Base, DeviceName: deviceName, Source: r.Source,
		Author: r.Author, At: r.At, Pruned: r.Pruned,
	}
	if r.DeviceID != 0 {
		out.Device = &r.DeviceID
	}
	if r.LostTo != 0 {
		out.LostTo = &r.LostTo
	}
	return out
}

// AddReplica records a new replica of vault vaultID for device deviceID,
// with joined_at = t.Now(). ErrNotFound when the vault is not served or the
// device does not exist. Task card T-0025.
func (t *Tx) AddReplica(ctx context.Context, deviceID, vaultID int64) (Replica, error) {
	if _, err := t.HubVault(ctx, vaultID); err != nil {
		return Replica{}, err
	}
	var id int64
	if err := t.QueryRowContext(ctx, `SELECT id FROM hub_devices WHERE id = ?`, deviceID).Scan(&id); err != nil {
		return Replica{}, hubReadError("add replica device", err)
	}
	return scanReplica(t.QueryRowContext(ctx, `INSERT INTO hub_replicas (device_id, vault_id, joined_at)
		VALUES (?, ?, ?) RETURNING id, device_id, vault_id, joined_at`, deviceID, vaultID, FormatTime(t.Now())))
}

// Replica returns replica id, or ErrNotFound. Task card T-0025.
func (d *DB) Replica(ctx context.Context, id int64) (Replica, error) {
	return scanReplica(d.db.QueryRowContext(ctx,
		`SELECT id, device_id, vault_id, joined_at FROM hub_replicas WHERE id = ?`, id))
}

// RecordPush stores a's answer with at = t.Now() (a.At is ignored). A
// push ID the replica has an answer for is ErrConflict. Task card T-0025.
func (t *Tx) RecordPush(ctx context.Context, a PushAnswer) error {
	_, err := t.ExecContext(ctx, `INSERT INTO hub_pushes
		(replica_id, push_id, file_id, status, reason, conflicts, base_pruned, at)
		VALUES (?, ?, NULLIF(?, 0), ?, ?, ?, ?, ?)`,
		a.ReplicaID, a.PushID, a.FileID, a.Status, a.Reason, a.Conflicts, a.BasePruned, FormatTime(t.Now()))
	if IsUniqueViolation(err) {
		return fmt.Errorf("record push %d of replica %d: %w", a.PushID, a.ReplicaID, ErrConflict)
	}
	if err != nil {
		return fmt.Errorf("record push: %w", err)
	}
	return nil
}

// PushAnswer returns the answer to push pushID of replica replicaID as this
// transaction sees it, or ErrNotFound. Task card T-0025.
func (t *Tx) PushAnswer(ctx context.Context, replicaID, pushID int64) (PushAnswer, error) {
	var a PushAnswer
	var at string
	err := t.QueryRowContext(ctx, `SELECT replica_id, push_id, COALESCE(file_id, 0),
		status, reason, conflicts, base_pruned, at FROM hub_pushes WHERE replica_id = ? AND push_id = ?`,
		replicaID, pushID).Scan(&a.ReplicaID, &a.PushID, &a.FileID, &a.Status, &a.Reason, &a.Conflicts, &a.BasePruned, &at)
	if err != nil {
		return PushAnswer{}, hubReadError("read push answer", err)
	}
	a.At, err = ParseTime(at)
	return a, err
}

// PushRecords returns, for each push ID in pushes, in that order, the
// revisions of vault vaultID that replica replicaID's push made, by seq,
// with their device's name; Revisions is a non-nil empty slice when there
// are none. Task card T-0025.
func (d *DB) PushRecords(ctx context.Context, vaultID, replicaID int64, pushes []int64) ([]api.PushRecord, error) {
	out := make([]api.PushRecord, 0, len(pushes))
	for _, push := range pushes {
		revisions, err := d.namedRevisions(ctx, namedRevisionSelect+`
			WHERE r.vault_id = ? AND r.replica_id = ? AND r.push_id = ? ORDER BY r.seq`, vaultID, replicaID, push)
		if err != nil {
			return nil, err
		}
		out = append(out, api.PushRecord{Push: push, Revisions: revisions})
	}
	return out, nil
}

// Match answers log.match for vault vaultID, in the order of qs: for each
// query, File is the file last at q.Path (the file of the newest revision
// whose path is q.Path, other than a conflict or resolve), and Revision is
// that file's newest create, put or move with hash q.Hash. Either is nil
// when there is none. Task card T-0025.
func (d *DB) Match(ctx context.Context, vaultID int64, qs []api.MatchQuery) ([]api.LogMatch, error) {
	out := make([]api.LogMatch, 0, len(qs))
	for _, q := range qs {
		m := api.LogMatch{Path: q.Path, Hash: q.Hash}
		err := d.db.QueryRowContext(ctx, `SELECT f.file_id,
			(SELECT MAX(r.seq) FROM revisions r WHERE r.vault_id = f.vault_id AND r.file_id = f.file_id
				AND r.op IN ('create', 'put', 'move') AND r.hash = ?)
			FROM revisions f WHERE f.vault_id = ? AND f.path = ? AND f.op NOT IN ('conflict', 'resolve')
			ORDER BY f.seq DESC LIMIT 1`, q.Hash, vaultID, q.Path).Scan(&m.File, &m.Revision)
		if err != nil && !errors.Is(err, sql.ErrNoRows) {
			return nil, hubReadError("match log", err)
		}
		out = append(out, m)
	}
	return out, nil
}

// History returns file fileID's revisions in vault vaultID, newest first,
// only those before seq before when before > 0, at most limit (50 when
// limit is 0 or less; more than 200 is 200), with their device's name, or
// "hub" when DeviceID is 0. A file the log does not have is ErrNotFound.
// Task card T-0025.
func (d *DB) History(ctx context.Context, vaultID, fileID, before int64, limit int) ([]api.Revision, error) {
	if _, err := d.HubFile(ctx, vaultID, fileID); err != nil {
		return nil, err
	}
	if limit <= 0 {
		limit = 50
	}
	limit = min(limit, 200)
	return d.namedRevisions(ctx, namedRevisionSelect+`
		WHERE r.vault_id = ? AND r.file_id = ? AND (? <= 0 OR r.seq < ?) ORDER BY r.seq DESC LIMIT ?`,
		vaultID, fileID, before, before, limit)
}

func scanReplica(sc interface{ Scan(...any) error }) (Replica, error) {
	var r Replica
	var at string
	if err := sc.Scan(&r.ID, &r.DeviceID, &r.VaultID, &at); err != nil {
		return Replica{}, hubReadError("read replica", err)
	}
	var err error
	r.JoinedAt, err = ParseTime(at)
	return r, err
}

// The revision columns have the order scanRevision expects, followed by the name.
const namedRevisionSelect = `SELECT r.vault_id, r.seq, r.file_id, r.op, r.path, r.hash, r.size, r.base,
	COALESCE(r.device_id, 0), COALESCE(r.replica_id, 0), COALESCE(r.push_id, 0),
	r.source, r.author, r.at, COALESCE(r.lost_to, 0), r.pruned, COALESCE(d.name, 'hub')
	FROM revisions r LEFT JOIN hub_devices d ON d.id = r.device_id`

type namedRevisionScanner struct {
	rows *sql.Rows
	name string
}

// Scan reads the revision's fields and its joined device name together.
func (s *namedRevisionScanner) Scan(dest ...any) error {
	return s.rows.Scan(append(dest, &s.name)...)
}

func (d *DB) namedRevisions(ctx context.Context, query string, args ...any) ([]api.Revision, error) {
	rows, err := d.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("read named revisions: %w", err)
	}
	defer rows.Close()
	out := []api.Revision{}
	s := namedRevisionScanner{rows: rows}
	for rows.Next() {
		r, err := scanRevision(&s)
		if err != nil {
			return nil, err
		}
		out = append(out, r.API(s.name))
	}
	return out, hubReadError("read named revisions", rows.Err())
}
