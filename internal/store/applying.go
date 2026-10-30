package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"github.com/frostyard/frostnotes/api"
)

// Applying is a sync_applying row: a head the device is applying to its
// vault (ADR-0034). The device records it before vault.Apply changes the
// disk, and the transaction that agrees the head deletes it, so a row
// that remains is a head whose change may be on disk without its agreed
// row: a crash came between the two.
type Applying struct {
	VaultID int64
	HubFile int64
	FileID  int64  // the local file the head replaces; 0 for a new file
	From    string // the path the head replaces the file at
	// FromHash is the content the head replaces at From; "" for a new
	// file.
	FromHash string
	// BaseSeq is the agreed row's seq when the head was recorded, 0
	// without a row; PutApplying sets it.
	BaseSeq  int64
	Seq      int64
	Revision int64
	Path     string
	Hash     string // "" for a delete
	// At is where the change puts the file: Path, but for the answer to
	// a create the hub merged, whose file moved meanwhile, where it is
	// now (its move is pushed next).
	At         string
	Deleted    bool
	Conflicted bool
	// PushID is the push whose answer the head is, with that answer's
	// Status and Reason; 0 for a pulled head.
	PushID int64
	Status api.PushStatus
	Reason api.RefuseReason
}

// Agreed is the agreed row the head makes for local file fileID.
func (a Applying) Agreed(fileID int64) Agreed {
	return Agreed{VaultID: a.VaultID, HubFile: a.HubFile, FileID: fileID, Seq: a.Seq, Revision: a.Revision,
		Path: a.Path, Hash: a.Hash, Conflicted: a.Conflicted}
}

// PutApplying records a, replacing the row of its vault and hub file, with
// BaseSeq the seq of the file's agreed row as this transaction sees it (0
// without one). It returns the row. A vault without a remote is
// ErrNotFound.
func (t *Tx) PutApplying(ctx context.Context, a Applying) (Applying, error) {
	a.BaseSeq = 0
	if prev, err := t.AgreedByHubFileTx(ctx, a.VaultID, a.HubFile); err == nil {
		a.BaseSeq = prev.Seq
	} else if !errors.Is(err, ErrNotFound) {
		return Applying{}, err
	}
	_, err := t.ExecContext(ctx, `INSERT INTO sync_applying (`+applyingColumns+`)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?) ON CONFLICT (vault_id, hub_file) DO UPDATE SET
		file_id = excluded.file_id, from_path = excluded.from_path, from_hash = excluded.from_hash, base_seq = excluded.base_seq,
		seq = excluded.seq, revision = excluded.revision, path = excluded.path, hash = excluded.hash,
		at_path = excluded.at_path, deleted = excluded.deleted, conflicted = excluded.conflicted, push_id = excluded.push_id,
		status = excluded.status, reason = excluded.reason`,
		a.VaultID, a.HubFile, a.FileID, a.From, a.FromHash, a.BaseSeq, a.Seq, a.Revision, a.Path, a.Hash, a.At, a.Deleted,
		a.Conflicted, a.PushID, a.Status, a.Reason)
	if IsForeignKeyViolation(err) {
		return Applying{}, fmt.Errorf("put applying: %w", ErrNotFound)
	}
	if err != nil {
		return Applying{}, fmt.Errorf("put applying: %w", err)
	}
	return a, nil
}

// DeleteApplying removes the row of hub file hubFile in vault vaultID; a
// missing row is not an error.
func (t *Tx) DeleteApplying(ctx context.Context, vaultID, hubFile int64) error {
	_, err := t.ExecContext(ctx, `DELETE FROM sync_applying WHERE vault_id = ? AND hub_file = ?`, vaultID, hubFile)
	return hubReadError("delete applying", err)
}

// Applying returns vault vaultID's sync_applying rows by hub file; a
// non-nil empty slice when none.
func (d *DB) Applying(ctx context.Context, vaultID int64) ([]Applying, error) {
	rows, err := d.db.QueryContext(ctx, `SELECT `+applyingColumns+` FROM sync_applying WHERE vault_id = ?
		ORDER BY hub_file`, vaultID)
	if err != nil {
		return nil, fmt.Errorf("list sync_applying: %w", err)
	}
	defer rows.Close()
	out := []Applying{}
	for rows.Next() {
		var a Applying
		if err := rows.Scan(&a.VaultID, &a.HubFile, &a.FileID, &a.From, &a.FromHash, &a.BaseSeq, &a.Seq, &a.Revision, &a.Path,
			&a.Hash, &a.At, &a.Deleted, &a.Conflicted, &a.PushID, &a.Status, &a.Reason); err != nil {
			return nil, fmt.Errorf("read sync_applying: %w", err)
		}
		out = append(out, a)
	}
	return out, hubReadError("list sync_applying", rows.Err())
}

// FileExists reports whether local file id has an index row, as this
// transaction sees it.
func (t *Tx) FileExists(ctx context.Context, id int64) (bool, error) {
	var one int
	err := t.QueryRowContext(ctx, `SELECT 1 FROM files WHERE id = ?`, id).Scan(&one)
	if errors.Is(err, sql.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("file %d: %w", id, err)
	}
	return true, nil
}

const applyingColumns = `vault_id, hub_file, file_id, from_path, from_hash, base_seq, seq, revision, path, hash, at_path,
	deleted, conflicted, push_id, status, reason`
