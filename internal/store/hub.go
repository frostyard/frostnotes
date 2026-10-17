package store

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"errors"
	"fmt"
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
func (v HubVault) API() api.HubVault {
	return api.HubVault{VaultID: v.VaultID, Sync: v.SyncID, Name: v.Name, Seq: v.Seq, ServedAt: v.ServedAt}
}

// Log converts v to the log domain's form. Task card T-0024.
func (v HubVault) Log() api.LogVault {
	return api.LogVault{Sync: v.SyncID, Name: v.Name, Seq: v.Seq}
}

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
	id, err := readHubIdentity(t.QueryRowContext(ctx, `SELECT hub_id FROM hub WHERE id = 1`))
	if err != nil || id != "" {
		return id, err
	}
	id = newHubID()
	if _, err := t.ExecContext(ctx, `INSERT INTO hub (id, hub_id, created_at) VALUES (1, ?, ?)`, id, FormatTime(t.Now())); err != nil {
		return "", fmt.Errorf("insert hub: %w", err)
	}
	return id, nil
}

// HubIdentity returns the hub ID, or "" when this notesd has never served
// a vault. Task card T-0024.
func (d *DB) HubIdentity(ctx context.Context) (string, error) {
	return readHubIdentity(d.db.QueryRowContext(ctx, `SELECT hub_id FROM hub WHERE id = 1`))
}

// ServeVault makes vault vaultID a hub vault, with a new sync ID (32
// lowercase hex digits from crypto/rand), seq 0 and served_at = t.Now(),
// making the hub ID too if it is missing. A vault that is already served
// is ErrConflict; a vault that does not exist, ErrNotFound. Task card
// T-0024.
func (t *Tx) ServeVault(ctx context.Context, vaultID int64) (HubVault, error) {
	_, err := scanVault(t.QueryRowContext(ctx, `SELECT `+vaultColumns+` FROM vaults WHERE id = ?`, vaultID))
	if err != nil {
		return HubVault{}, hubReadError("serve vault", err)
	}
	if _, err := t.HubVault(ctx, vaultID); err == nil {
		return HubVault{}, fmt.Errorf("serve vault %d: %w", vaultID, ErrConflict)
	} else if !errors.Is(err, ErrNotFound) {
		return HubVault{}, err
	}
	if _, err := t.HubID(ctx); err != nil {
		return HubVault{}, err
	}
	_, err = t.ExecContext(ctx, `INSERT INTO hub_vaults (vault_id, sync_id, seq, served_at) VALUES (?, ?, 0, ?)`,
		vaultID, newHubID(), FormatTime(t.Now()))
	if IsUniqueViolation(err) {
		return HubVault{}, fmt.Errorf("serve vault %d: %w", vaultID, ErrConflict)
	}
	if err != nil {
		return HubVault{}, fmt.Errorf("serve vault %d: %w", vaultID, err)
	}
	return t.HubVault(ctx, vaultID)
}

// HubVaults returns every served vault, by name, then vault ID; a non-nil
// empty slice when none. Task card T-0024.
func (d *DB) HubVaults(ctx context.Context) ([]HubVault, error) {
	rows, err := d.db.QueryContext(ctx, hubVaultSelect+` ORDER BY v.name COLLATE NOCASE, h.vault_id`)
	if err != nil {
		return nil, fmt.Errorf("list hub vaults: %w", err)
	}
	defer rows.Close()
	out := []HubVault{}
	for rows.Next() {
		v, err := scanHubVault(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, v)
	}
	return out, hubReadError("list hub vaults", rows.Err())
}

// HubVault returns served vault vaultID, or ErrNotFound when it is not
// served. Task card T-0024.
func (d *DB) HubVault(ctx context.Context, vaultID int64) (HubVault, error) {
	return scanHubVault(d.db.QueryRowContext(ctx, hubVaultSelect+` WHERE h.vault_id = ?`, vaultID))
}

// HubVault returns served vault vaultID as this transaction sees it, or
// ErrNotFound. Task card T-0024.
func (t *Tx) HubVault(ctx context.Context, vaultID int64) (HubVault, error) {
	return scanHubVault(t.QueryRowContext(ctx, hubVaultSelect+` WHERE h.vault_id = ?`, vaultID))
}

// HubVaultBySync returns the served vault with sync ID syncID, or
// ErrNotFound. Task card T-0024.
func (d *DB) HubVaultBySync(ctx context.Context, syncID string) (HubVault, error) {
	return scanHubVault(d.db.QueryRowContext(ctx, hubVaultSelect+` WHERE h.sync_id = ?`, syncID))
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
	v, err := t.HubVault(ctx, r.VaultID)
	if err != nil {
		return Revision{}, err
	}
	if err := t.validateRevision(ctx, r); err != nil {
		return Revision{}, err
	}
	r.Seq = v.Seq + 1
	_, err = t.ExecContext(ctx, `INSERT INTO revisions
		(vault_id, seq, file_id, op, path, hash, size, base, device_id, replica_id, push_id, source, author, at, lost_to, pruned)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, NULLIF(?, 0), NULLIF(?, 0), NULLIF(?, 0), ?, ?, ?, NULLIF(?, 0), ?)`,
		r.VaultID, r.Seq, r.FileID, r.Op, r.Path, r.Hash, r.Size, r.Base, r.DeviceID, r.ReplicaID, r.PushID,
		r.Source, r.Author, FormatTime(t.Now()), r.LostTo, r.Pruned)
	if err != nil {
		return Revision{}, fmt.Errorf("insert revision: %w", err)
	}
	if _, err := t.ExecContext(ctx, `UPDATE hub_vaults SET seq = ? WHERE vault_id = ?`, r.Seq, r.VaultID); err != nil {
		return Revision{}, fmt.Errorf("advance hub vault: %w", err)
	}
	if err := t.advanceHubFile(ctx, r); err != nil {
		return Revision{}, err
	}
	if err := t.Emit(ctx, api.LogAppended{Vault: v.SyncID, Seq: r.Seq}); err != nil {
		return Revision{}, err
	}
	return t.Revision(ctx, r.VaultID, r.Seq)
}

// HubFile returns file fileID's place in vault vaultID's log, or
// ErrNotFound. Task card T-0024.
func (d *DB) HubFile(ctx context.Context, vaultID, fileID int64) (HubFile, error) {
	return scanHubFile(d.db.QueryRowContext(ctx, hubFileSelect, vaultID, fileID))
}

// HubFile returns file fileID's place in the log as this transaction sees
// it, or ErrNotFound. Task card T-0024.
func (t *Tx) HubFile(ctx context.Context, vaultID, fileID int64) (HubFile, error) {
	return scanHubFile(t.QueryRowContext(ctx, hubFileSelect, vaultID, fileID))
}

// Revision returns revision seq of vault vaultID, or ErrNotFound. Task
// card T-0024.
func (d *DB) Revision(ctx context.Context, vaultID, seq int64) (Revision, error) {
	return scanRevision(d.db.QueryRowContext(ctx, revisionSelect, vaultID, seq))
}

// Revision returns revision seq as this transaction sees it, or
// ErrNotFound. Task card T-0024.
func (t *Tx) Revision(ctx context.Context, vaultID, seq int64) (Revision, error) {
	return scanRevision(t.QueryRowContext(ctx, revisionSelect, vaultID, seq))
}

// LogFile returns file fileID as log.pull shows it: its hub_files row with
// its head revision's path, hash, size, author and device name ("hub" for
// the hub's own writers), and Deleted when the head is a delete. ErrNotFound when the log does not have the file. Task card
// T-0024.
func (d *DB) LogFile(ctx context.Context, vaultID, fileID int64) (api.LogFile, error) {
	return scanLogFile(d.db.QueryRowContext(ctx, logFileSelect+` WHERE f.vault_id = ? AND f.file_id = ?`, vaultID, fileID))
}

// Pull returns the files of vault vaultID whose hub_files.seq is after
// after, by that seq, at most limit of them (500 when limit is 0 or less;
// more than 1,000 is 1,000), each as LogFile returns it. Next is the last
// file's seq, or after when there are none; More is whether a file after
// Next remains. Files is non-nil. Task card T-0024.
func (d *DB) Pull(ctx context.Context, vaultID, after int64, limit int) (api.LogPage, error) {
	if limit <= 0 {
		limit = 500
	}
	limit = min(limit, 1000)
	page := api.LogPage{Files: []api.LogFile{}, Next: after}
	rows, err := d.db.QueryContext(ctx, logFileSelect+` WHERE f.vault_id = ? AND f.seq > ? ORDER BY f.seq LIMIT ?`,
		vaultID, after, limit+1)
	if err != nil {
		return page, fmt.Errorf("pull log: %w", err)
	}
	defer rows.Close()
	for rows.Next() {
		if len(page.Files) == limit {
			page.More = true
			break
		}
		f, err := scanLogFile(rows)
		if err != nil {
			return page, err
		}
		page.Files = append(page.Files, f)
		page.Next = f.Seq
	}
	return page, hubReadError("pull log", rows.Err())
}

// HasContent reports whether a create, put, move or conflict revision of a
// served vault that is not pruned names hash: the blobs /sync/blobs hands
// out. A delete or resolve revision holds no content of its own. Task card
// T-0024.
// VaultHasContent reports whether hub vault vaultID's log names content
// hash and keeps it: what a device of that vault may take as present
// without uploading it.
func (d *DB) VaultHasContent(ctx context.Context, vaultID int64, hash string) (bool, error) {
	if hash == "" {
		return false, nil
	}
	var found bool
	err := d.db.QueryRowContext(ctx, `SELECT EXISTS (
		SELECT 1 FROM revisions WHERE vault_id = ? AND hash = ? AND pruned = 0
		AND op IN ('create', 'put', 'move', 'conflict'))`, vaultID, hash).Scan(&found)
	return found, hubReadError("has content", err)
}

func (d *DB) HasContent(ctx context.Context, hash string) (bool, error) {
	if hash == "" {
		return false, nil
	}
	var found bool
	err := d.db.QueryRowContext(ctx, `SELECT EXISTS (
		SELECT 1 FROM revisions WHERE hash = ? AND pruned = 0 AND op IN ('create', 'put', 'move', 'conflict'))`, hash).Scan(&found)
	return found, hubReadError("has content", err)
}

func newHubID() string {
	var b [16]byte
	_, _ = rand.Read(b[:]) // crypto/rand.Read always fills b or terminates the process.
	return hex.EncodeToString(b[:])
}

func readHubIdentity(sc interface{ Scan(...any) error }) (string, error) {
	var id string
	err := sc.Scan(&id)
	if errors.Is(err, sql.ErrNoRows) {
		return "", nil
	}
	return id, hubReadError("read hub identity", err)
}

func hubReadError(operation string, err error) error {
	if err == nil {
		return nil
	}
	if errors.Is(err, sql.ErrNoRows) {
		err = ErrNotFound
	}
	return fmt.Errorf("%s: %w", operation, err)
}

const hubVaultSelect = `SELECT h.vault_id, h.sync_id, v.name, h.seq, h.served_at
	FROM hub_vaults h JOIN vaults v ON v.id = h.vault_id`

func scanHubVault(sc interface{ Scan(...any) error }) (HubVault, error) {
	var v HubVault
	var at string
	if err := sc.Scan(&v.VaultID, &v.SyncID, &v.Name, &v.Seq, &at); err != nil {
		return HubVault{}, hubReadError("read hub vault", err)
	}
	var err error
	v.ServedAt, err = ParseTime(at)
	return v, err
}

func (t *Tx) validateRevision(ctx context.Context, r Revision) error {
	if !r.Op.Valid() || r.FileID <= 0 || r.Path == "" {
		return errors.New("append revision: invalid op, file id or path")
	}
	if (r.Op == api.RevisionOpDelete) != (r.Hash == "") {
		return errors.New("append revision: hash disagrees with op")
	}
	_, err := t.HubFile(ctx, r.VaultID, r.FileID)
	if err != nil && !errors.Is(err, ErrNotFound) {
		return err
	}
	if r.Op == api.RevisionOpCreate {
		if err == nil {
			return fmt.Errorf("create hub file %d: %w", r.FileID, ErrConflict)
		}
		return nil
	}
	return err
}

func (t *Tx) advanceHubFile(ctx context.Context, r Revision) error {
	_, err := t.ExecContext(ctx, `INSERT INTO hub_files (vault_id, file_id, seq, revision, conflicted)
		VALUES (?, ?, ?, ?, 0)
		ON CONFLICT (vault_id, file_id) DO UPDATE SET
		seq = excluded.seq,
		revision = CASE WHEN ? IN ('conflict', 'resolve') THEN hub_files.revision ELSE excluded.revision END,
		conflicted = CASE ? WHEN 'conflict' THEN 1 WHEN 'resolve' THEN 0 ELSE hub_files.conflicted END`,
		r.VaultID, r.FileID, r.Seq, r.Seq, r.Op, r.Op)
	if err != nil {
		return fmt.Errorf("advance hub file: %w", err)
	}
	return nil
}

const hubFileSelect = `SELECT vault_id, file_id, seq, revision, conflicted
	FROM hub_files WHERE vault_id = ? AND file_id = ?`

func scanHubFile(sc interface{ Scan(...any) error }) (HubFile, error) {
	var f HubFile
	err := sc.Scan(&f.VaultID, &f.FileID, &f.Seq, &f.Revision, &f.Conflicted)
	return f, hubReadError("read hub file", err)
}

const revisionSelect = `SELECT vault_id, seq, file_id, op, path, hash, size, base,
	COALESCE(device_id, 0), COALESCE(replica_id, 0), COALESCE(push_id, 0), source, author, at, COALESCE(lost_to, 0), pruned
	FROM revisions WHERE vault_id = ? AND seq = ?`

func scanRevision(sc interface{ Scan(...any) error }) (Revision, error) {
	var r Revision
	var at string
	err := sc.Scan(&r.VaultID, &r.Seq, &r.FileID, &r.Op, &r.Path, &r.Hash, &r.Size, &r.Base,
		&r.DeviceID, &r.ReplicaID, &r.PushID, &r.Source, &r.Author, &at, &r.LostTo, &r.Pruned)
	if err != nil {
		return Revision{}, hubReadError("read revision", err)
	}
	r.At, err = ParseTime(at)
	return r, err
}

const logFileSelect = `SELECT f.file_id, f.seq, f.revision, r.path, r.hash, r.size, r.op = 'delete', f.conflicted,
	r.author, COALESCE(d.name, 'hub')
	FROM hub_files f JOIN revisions r ON r.vault_id = f.vault_id AND r.seq = f.revision
	LEFT JOIN hub_devices d ON d.id = r.device_id`

func scanLogFile(sc interface{ Scan(...any) error }) (api.LogFile, error) {
	var f api.LogFile
	err := sc.Scan(&f.File, &f.Seq, &f.Revision, &f.Path, &f.Hash, &f.Size, &f.Deleted, &f.Conflicted, &f.Author, &f.DeviceName)
	if err != nil {
		return api.LogFile{}, hubReadError("read log file", err)
	}
	if f.Deleted {
		f.Hash, f.Size = "", 0
	}
	return f, nil
}
