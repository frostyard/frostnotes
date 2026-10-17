package store

import (
	"context"
	"database/sql"
	"fmt"
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
func (h SyncHub) API() api.SyncHub {
	return api.SyncHub{ID: h.ID, Hub: h.HubID, URL: h.URL, Device: h.DeviceID, PairedAt: h.PairedAt}
}

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
	VaultID  int64
	PushID   int64
	FileID   int64
	HubFile  int64 // 0 for a create
	Base     int64
	Path     string
	Hash     string // "" for a delete
	At       time.Time
	Status   api.PushStatus   // "" until answered
	Reason   api.RefuseReason // "" unless refused
	Conflict bool             // a local conflict version, for the hub's log
}

// PutSyncHub records a pairing with h.PairedAt = t.Now(): a new row, or,
// for a hub ID the device has paired with before, that row with its URL,
// device ID and paired_at replaced. It returns the row. Task card T-0027.
func (t *Tx) PutSyncHub(ctx context.Context, h SyncHub) (SyncHub, error) {
	return scanSyncHub(t.QueryRowContext(ctx, `INSERT INTO sync_hubs (hub_id, url, device_id, paired_at)
		VALUES (?, ?, ?, ?) ON CONFLICT (hub_id) DO UPDATE SET
		url = excluded.url, device_id = excluded.device_id, paired_at = excluded.paired_at
		RETURNING `+syncHubColumns, h.HubID, h.URL, h.DeviceID, FormatTime(t.Now())))
}

// SyncHubs returns the paired hubs by ID; a non-nil empty slice when none.
// Task card T-0027.
func (d *DB) SyncHubs(ctx context.Context) ([]SyncHub, error) {
	rows, err := d.db.QueryContext(ctx, `SELECT `+syncHubColumns+` FROM sync_hubs ORDER BY id`)
	if err != nil {
		return nil, fmt.Errorf("list sync_hubs: %w", err)
	}
	defer rows.Close()
	out := []SyncHub{}
	for rows.Next() {
		r, err := scanSyncHub(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	return out, hubReadError("list sync_hubs", rows.Err())
}

// SyncHub returns sync_hubs row id, or ErrNotFound. Task card T-0027.
func (d *DB) SyncHub(ctx context.Context, id int64) (SyncHub, error) {
	return scanSyncHub(d.db.QueryRowContext(ctx, `SELECT `+syncHubColumns+` FROM sync_hubs WHERE id = ?`, id))
}

// SyncHubByURL returns the hub paired at exactly url, or ErrNotFound. Task
// card T-0027.
func (d *DB) SyncHubByURL(ctx context.Context, url string) (SyncHub, error) {
	return scanSyncHub(d.db.QueryRowContext(ctx, `SELECT `+syncHubColumns+` FROM sync_hubs WHERE url = ?`, url))
}

// AddRemote records r with joined_at = t.Now(), cursor and hub_seq as
// given, and synced_at null. A vault that is already synced is
// ErrConflict; a vault or hub that does not exist, ErrNotFound. Task card
// T-0027.
func (t *Tx) AddRemote(ctx context.Context, r Remote) (Remote, error) {
	stored, err := scanRemote(t.QueryRowContext(ctx, `INSERT INTO remotes
		(vault_id, hub, sync_id, name, replica_id, cursor, hub_seq, joined_at, synced_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, NULL) RETURNING `+remoteColumns,
		r.VaultID, r.Hub, r.SyncID, r.Name, r.ReplicaID, r.Cursor, r.HubSeq, FormatTime(t.Now())))
	if IsUniqueViolation(err) {
		return Remote{}, fmt.Errorf("add remote: %w", ErrConflict)
	}
	if IsForeignKeyViolation(err) {
		return Remote{}, fmt.Errorf("add remote: %w", ErrNotFound)
	}
	return stored, err
}

// Remote returns vault vaultID's remote, or ErrNotFound. Task card T-0027.
func (d *DB) Remote(ctx context.Context, vaultID int64) (Remote, error) {
	return scanRemote(d.db.QueryRowContext(ctx, `SELECT `+remoteColumns+` FROM remotes WHERE vault_id = ?`, vaultID))
}

// Remotes returns every remote by vault ID; a non-nil empty slice when
// none. Task card T-0027.
func (d *DB) Remotes(ctx context.Context) ([]Remote, error) {
	rows, err := d.db.QueryContext(ctx, `SELECT `+remoteColumns+` FROM remotes ORDER BY vault_id`)
	if err != nil {
		return nil, fmt.Errorf("list remotes: %w", err)
	}
	defer rows.Close()
	out := []Remote{}
	for rows.Next() {
		r, err := scanRemote(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	return out, hubReadError("list remotes", rows.Err())
}

// DeleteRemote removes vault vaultID's remote with its agreed rows and
// pushes; a vault without one is ErrNotFound. Task card T-0027.
func (t *Tx) DeleteRemote(ctx context.Context, vaultID int64) error {
	var id int64
	err := t.QueryRowContext(ctx, `DELETE FROM remotes WHERE vault_id = ? RETURNING vault_id`, vaultID).Scan(&id)
	return hubReadError("delete remote", err)
}

// SetReplica records replicaID as vault vaultID's replica on its hub: a
// device that paired again joins anew, keeping its agreed state and
// cursor. ErrNotFound without a remote.
func (t *Tx) SetReplica(ctx context.Context, vaultID, replicaID int64) error {
	var id int64
	err := t.QueryRowContext(ctx, `UPDATE remotes SET replica_id = ? WHERE vault_id = ? RETURNING vault_id`,
		replicaID, vaultID).Scan(&id)
	return hubReadError("set replica", err)
}

// SetCursor sets vault vaultID's cursor, and its hub_seq to the larger of
// the stored one and hubSeq; ErrNotFound without a remote. Task card T-0027.
func (t *Tx) SetCursor(ctx context.Context, vaultID, cursor, hubSeq int64) error {
	var id int64
	err := t.QueryRowContext(ctx, `UPDATE remotes SET cursor = ?, hub_seq = MAX(hub_seq, ?)
		WHERE vault_id = ? RETURNING vault_id`, cursor, hubSeq, vaultID).Scan(&id)
	return hubReadError("set cursor", err)
}

// MarkSynced sets vault vaultID's synced_at to t.Now(); ErrNotFound
// without a remote. Task card T-0027.
func (t *Tx) MarkSynced(ctx context.Context, vaultID int64) error {
	var id int64
	err := t.QueryRowContext(ctx, `UPDATE remotes SET synced_at = ? WHERE vault_id = ? RETURNING vault_id`,
		FormatTime(t.Now()), vaultID).Scan(&id)
	return hubReadError("mark synced", err)
}

// PutAgreed inserts a, or replaces the row with its vault and hub file. A
// local file that is agreed with another hub file is ErrConflict. Task
// card T-0027.
func (t *Tx) PutAgreed(ctx context.Context, a Agreed) error {
	_, err := t.ExecContext(ctx, `INSERT INTO sync_files (`+agreedColumns+`)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?) ON CONFLICT (vault_id, hub_file) DO UPDATE SET
		file_id = excluded.file_id, seq = excluded.seq, revision = excluded.revision,
		path = excluded.path, hash = excluded.hash, conflicted = excluded.conflicted`,
		a.VaultID, a.HubFile, a.FileID, a.Seq, a.Revision, a.Path, a.Hash, a.Conflicted)
	if IsUniqueViolation(err) {
		return fmt.Errorf("put agreed: hub file %d at %s as local file %d: %w", a.HubFile, a.Path, a.FileID, ErrConflict)
	}
	if IsForeignKeyViolation(err) {
		return fmt.Errorf("put agreed: %w", ErrNotFound)
	}
	return hubReadError("put agreed", err)
}

// DeleteAgreed removes the agreed row of hub file hubFile in vault
// vaultID; a missing row is not an error. Task card T-0027.
func (t *Tx) DeleteAgreed(ctx context.Context, vaultID, hubFile int64) error {
	_, err := t.ExecContext(ctx, `DELETE FROM sync_files WHERE vault_id = ? AND hub_file = ?`, vaultID, hubFile)
	return hubReadError("delete agreed", err)
}

// AgreedByHubFile returns the agreed row of hub file hubFile in vault
// vaultID, or ErrNotFound. Task card T-0027.
func (d *DB) AgreedByHubFile(ctx context.Context, vaultID, hubFile int64) (Agreed, error) {
	return scanAgreed(d.db.QueryRowContext(ctx, `SELECT `+agreedColumns+`
		FROM sync_files WHERE vault_id = ? AND hub_file = ?`, vaultID, hubFile))
}

// AgreedByFile returns the agreed row of local file fileID, or
// ErrNotFound. Task card T-0027.
func (d *DB) AgreedByFile(ctx context.Context, fileID int64) (Agreed, error) {
	return scanAgreed(d.db.QueryRowContext(ctx, `SELECT `+agreedColumns+` FROM sync_files WHERE file_id = ?`, fileID))
}

// AgreedRows returns vault vaultID's agreed rows by path; a non-nil empty
// slice when none. Task card T-0027.
func (d *DB) AgreedRows(ctx context.Context, vaultID int64) ([]Agreed, error) {
	rows, err := d.db.QueryContext(ctx, `SELECT `+agreedColumns+` FROM sync_files WHERE vault_id = ? ORDER BY path`, vaultID)
	if err != nil {
		return nil, fmt.Errorf("list sync_files: %w", err)
	}
	defer rows.Close()
	out := []Agreed{}
	for rows.Next() {
		r, err := scanAgreed(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	return out, hubReadError("list sync_files", rows.Err())
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
	// One statement gives all three tables a consistent read snapshot.
	rows, err := d.db.QueryContext(ctx, `WITH changes AS (
		SELECT f.id AS file_id, f.path, f.hash, CASE WHEN a.file_id IS NULL THEN 2 ELSE 1 END AS kind
		FROM files f LEFT JOIN sync_files a ON a.file_id = f.id
		WHERE f.vault_id = ? AND (a.file_id IS NULL OR f.path <> a.path OR f.hash <> a.hash)
		UNION ALL
		SELECT a.file_id, a.path, '', 0 FROM sync_files a
		WHERE a.vault_id = ? AND NOT EXISTS (SELECT 1 FROM files f WHERE f.id = a.file_id)
	)
	SELECT c.file_id, c.path, c.hash, c.kind,
		COALESCE(a.vault_id, 0), COALESCE(a.hub_file, 0), COALESCE(a.file_id, 0),
		COALESCE(a.seq, 0), COALESCE(a.revision, 0), COALESCE(a.path, ''),
		COALESCE(a.hash, ''), COALESCE(a.conflicted, 0)
	FROM changes c LEFT JOIN sync_files a ON a.file_id = c.file_id
	WHERE NOT EXISTS (
		SELECT 1 FROM sync_pushes p WHERE p.vault_id = ? AND p.file_id = c.file_id
		AND p.push_id = (SELECT MAX(latest.push_id) FROM sync_pushes latest
			WHERE latest.vault_id = p.vault_id AND latest.file_id = p.file_id)
		AND p.status = 'refused' AND p.reason <> 'missing' AND p.path = c.path AND p.hash = c.hash
	) ORDER BY c.kind, c.path`, vaultID, vaultID, vaultID)
	if err != nil {
		return nil, fmt.Errorf("list pending: %w", err)
	}
	defer rows.Close()
	out := []Change{}
	for rows.Next() {
		var c Change
		var a Agreed
		var kind int
		if err := rows.Scan(&c.FileID, &c.Path, &c.Hash, &kind,
			&a.VaultID, &a.HubFile, &a.FileID, &a.Seq, &a.Revision, &a.Path, &a.Hash, &a.Conflicted); err != nil {
			return nil, fmt.Errorf("read pending: %w", err)
		}
		c.Deleted = kind == 0
		if kind != 2 {
			c.Agreed = &a
		}
		out = append(out, c)
	}
	return out, hubReadError("list pending", rows.Err())
}

// AddPush records p with PushID the vault's highest + 1 (from 1), At =
// t.Now() and no answer (p.PushID, p.At, p.Status and p.Reason are
// ignored), and returns it. Task card T-0027.
func (t *Tx) AddPush(ctx context.Context, p LocalPush) (LocalPush, error) {
	stored, err := scanLocalPush(t.QueryRowContext(ctx, `INSERT INTO sync_pushes
		(vault_id, push_id, file_id, hub_file, base, path, hash, at, status, reason)
		SELECT ?, COALESCE(MAX(push_id), 0) + 1, ?, NULLIF(?, 0), ?, ?, ?, ?, NULL, ''
		FROM sync_pushes WHERE vault_id = ? RETURNING `+localPushColumns,
		p.VaultID, p.FileID, p.HubFile, p.Base, p.Path, p.Hash, FormatTime(t.Now()), p.VaultID))
	if err == nil && p.Conflict {
		if _, err := t.ExecContext(ctx, `UPDATE sync_pushes SET conflict = 1 WHERE vault_id = ? AND push_id = ?`,
			stored.VaultID, stored.PushID); err != nil {
			return LocalPush{}, err
		}
		stored.Conflict = true
	}
	if IsForeignKeyViolation(err) {
		return LocalPush{}, fmt.Errorf("add push: %w", ErrNotFound)
	}
	return stored, err
}

// AnswerPush records the answer to push pushID of vault vaultID; a push
// that does not exist is ErrNotFound. Task card T-0027.
func (t *Tx) AnswerPush(ctx context.Context, vaultID, pushID int64, status api.PushStatus, reason api.RefuseReason) error {
	var id int64
	err := t.QueryRowContext(ctx, `UPDATE sync_pushes SET status = ?, reason = ?
		WHERE vault_id = ? AND push_id = ? RETURNING push_id`, status, reason, vaultID, pushID).Scan(&id)
	return hubReadError("answer push", err)
}

// Unanswered returns vault vaultID's pushes without an answer, by push ID:
// a retry sends them again with the same IDs. Task card T-0027.
func (d *DB) Unanswered(ctx context.Context, vaultID int64) ([]LocalPush, error) {
	return d.localPushes(ctx, vaultID, false)
}

// Pushes returns vault vaultID's answered pushes, by push ID; a non-nil
// empty slice when none. Task card T-0027.
func (d *DB) Pushes(ctx context.Context, vaultID int64) ([]LocalPush, error) {
	return d.localPushes(ctx, vaultID, true)
}

const syncHubColumns = `id, hub_id, url, device_id, paired_at`
const remoteColumns = `vault_id, hub, sync_id, name, replica_id, cursor, hub_seq, joined_at, synced_at`
const agreedColumns = `vault_id, hub_file, file_id, seq, revision, path, hash, conflicted`
const localPushColumns = `vault_id, push_id, file_id, COALESCE(hub_file, 0), base, path, hash, at, COALESCE(status, ''), reason, conflict`

func scanSyncHub(sc interface{ Scan(...any) error }) (SyncHub, error) {
	var h SyncHub
	var at string
	if err := sc.Scan(&h.ID, &h.HubID, &h.URL, &h.DeviceID, &at); err != nil {
		return SyncHub{}, hubReadError("read sync hub", err)
	}
	var err error
	h.PairedAt, err = ParseTime(at)
	return h, hubReadError("read sync hub", err)
}

func scanRemote(sc interface{ Scan(...any) error }) (Remote, error) {
	var r Remote
	var joined string
	var synced sql.NullString
	if err := sc.Scan(&r.VaultID, &r.Hub, &r.SyncID, &r.Name, &r.ReplicaID,
		&r.Cursor, &r.HubSeq, &joined, &synced); err != nil {
		return Remote{}, hubReadError("read remote", err)
	}
	var err error
	r.JoinedAt, err = ParseTime(joined)
	if err != nil {
		return Remote{}, fmt.Errorf("read remote: %w", err)
	}
	if synced.Valid {
		r.SyncedAt, err = ParseTime(synced.String)
	}
	return r, hubReadError("read remote", err)
}

func scanAgreed(sc interface{ Scan(...any) error }) (Agreed, error) {
	var a Agreed
	err := sc.Scan(&a.VaultID, &a.HubFile, &a.FileID, &a.Seq, &a.Revision, &a.Path, &a.Hash, &a.Conflicted)
	return a, hubReadError("read agreed", err)
}

func scanLocalPush(sc interface{ Scan(...any) error }) (LocalPush, error) {
	var p LocalPush
	var at string
	if err := sc.Scan(&p.VaultID, &p.PushID, &p.FileID, &p.HubFile, &p.Base,
		&p.Path, &p.Hash, &at, &p.Status, &p.Reason, &p.Conflict); err != nil {
		return LocalPush{}, hubReadError("read local push", err)
	}
	var err error
	p.At, err = ParseTime(at)
	return p, hubReadError("read local push", err)
}

func (d *DB) localPushes(ctx context.Context, vaultID int64, answered bool) ([]LocalPush, error) {
	rows, err := d.db.QueryContext(ctx, `SELECT `+localPushColumns+`
		FROM sync_pushes WHERE vault_id = ? AND (status IS NOT NULL) = ? ORDER BY push_id`, vaultID, answered)
	if err != nil {
		return nil, fmt.Errorf("list local pushes: %w", err)
	}
	defer rows.Close()
	out := []LocalPush{}
	for rows.Next() {
		p, err := scanLocalPush(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, p)
	}
	return out, hubReadError("list local pushes", rows.Err())
}

// LostLocally is a conflict version of a synced file that no push has
// carried to the hub yet: content a writer with a stale base lost in a
// merge on this device.
type LostLocally struct {
	FileID int64
	Agreed Agreed
	Path   string
	Hash   string
}

// PendingConflicts returns vault vaultID's local conflict versions of
// files it agrees with the hub on, that no push has carried, by version.
// The device pushes each as a conflict item, so the hub's log keeps it
// and flags the file (docs/specs/sync-protocol.md, Pending changes).
func (d *DB) PendingConflicts(ctx context.Context, vaultID int64) ([]LostLocally, error) {
	rows, err := d.db.QueryContext(ctx, `SELECT v.file_id, v.path, v.hash, `+agreedColumnsOf("a")+`
		FROM versions v JOIN sync_files a ON a.file_id = v.file_id AND a.vault_id = ?
		WHERE v.vault_id = ? AND v.conflict = 1 AND v.hash != ''
		AND NOT EXISTS (SELECT 1 FROM sync_pushes p WHERE p.vault_id = ? AND p.file_id = v.file_id AND p.hash = v.hash)
		ORDER BY v.id`, vaultID, vaultID, vaultID)
	if err != nil {
		return nil, fmt.Errorf("list pending conflicts: %w", err)
	}
	defer rows.Close()
	out := []LostLocally{}
	for rows.Next() {
		var l LostLocally
		a := &l.Agreed
		if err := rows.Scan(&l.FileID, &l.Path, &l.Hash, &a.VaultID, &a.HubFile, &a.FileID, &a.Seq, &a.Revision,
			&a.Path, &a.Hash, &a.Conflicted); err != nil {
			return nil, fmt.Errorf("read pending conflict: %w", err)
		}
		out = append(out, l)
	}
	return out, hubReadError("list pending conflicts", rows.Err())
}

func agreedColumnsOf(alias string) string {
	return alias + ".vault_id, " + alias + ".hub_file, " + alias + ".file_id, " + alias + ".seq, " + alias + ".revision, " +
		alias + ".path, " + alias + ".hash, " + alias + ".conflicted"
}

// AgreedByFileTx is AgreedByFile as this transaction sees it.
func (t *Tx) AgreedByFileTx(ctx context.Context, fileID int64) (Agreed, error) {
	return scanAgreed(t.QueryRowContext(ctx, `SELECT `+agreedColumns+` FROM sync_files WHERE file_id = ?`, fileID))
}

// AgreedByHubFileTx is AgreedByHubFile as this transaction sees it.
func (t *Tx) AgreedByHubFileTx(ctx context.Context, vaultID, hubFile int64) (Agreed, error) {
	return scanAgreed(t.QueryRowContext(ctx, `SELECT `+agreedColumns+` FROM sync_files WHERE vault_id = ? AND hub_file = ?`,
		vaultID, hubFile))
}
