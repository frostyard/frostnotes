package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"slices"
	"time"

	"github.com/frostyard/frostnotes/api"
)

// Version is a versions row: one version of a file, or its deletion.
type Version struct {
	ID       int64
	VaultID  int64
	FileID   int64  // kept after the file is deleted; file IDs are never reused
	Path     string // the file's path when this version was made
	Hash     string // the content's blob ID; "" records a deletion
	Size     int64
	At       time.Time
	Source   api.VersionSource
	Author   string // "" for an outside edit
	Conflict bool   // a writer's content that lost a conflicting hunk in a merge
}

// API converts v to its wire form. Task card T-0018.
func (v Version) API() api.Version {
	return api.Version{ID: v.ID, Path: v.Path, Hash: v.Hash, Size: v.Size, At: v.At,
		Source: v.Source, Author: v.Author, Deleted: v.Hash == "", Conflict: v.Conflict}
}

const versionColumns = `id, vault_id, file_id, path, hash, size, at, source, author, conflict`

func scanVersion(sc interface{ Scan(...any) error }) (Version, error) {
	var v Version
	var at string
	if err := sc.Scan(&v.ID, &v.VaultID, &v.FileID, &v.Path, &v.Hash, &v.Size, &at, &v.Source, &v.Author, &v.Conflict); err != nil {
		return Version{}, err
	}
	var err error
	v.At, err = ParseTime(at)
	if err != nil {
		return Version{}, err
	}
	return v, nil
}

// AddVersion appends v with At = t.Now(), to the millisecond, and returns
// it with its ID; v.ID and v.At are ignored. Task card T-0018.
func (t *Tx) AddVersion(ctx context.Context, v Version) (Version, error) {
	stored, err := scanVersion(t.QueryRowContext(ctx, `INSERT INTO versions
		(vault_id, file_id, path, hash, size, at, source, author, conflict)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?) RETURNING `+versionColumns,
		v.VaultID, v.FileID, v.Path, v.Hash, v.Size, FormatTime(t.Now()), v.Source, v.Author, v.Conflict))
	if err != nil {
		return Version{}, fmt.Errorf("add version for file %d: %w", v.FileID, err)
	}
	return stored, nil
}

// Versions returns file fileID's versions, newest first, at most limit of
// them (50 when limit is 0 or less). Task card T-0018.
func (d *DB) Versions(ctx context.Context, fileID int64, limit int) ([]Version, error) {
	if limit <= 0 {
		limit = 50
	}
	rows, err := d.db.QueryContext(ctx, `SELECT `+versionColumns+` FROM versions WHERE file_id = ? ORDER BY id DESC LIMIT ?`, fileID, limit)
	if err != nil {
		return nil, fmt.Errorf("list versions: %w", err)
	}
	defer rows.Close()
	versions := []Version{}
	for rows.Next() {
		v, err := scanVersion(rows)
		if err != nil {
			return nil, fmt.Errorf("list versions: %w", err)
		}
		versions = append(versions, v)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("list versions: %w", err)
	}
	return versions, nil
}

// GetVersion returns version id, or ErrNotFound. Task card T-0018.
func (d *DB) GetVersion(ctx context.Context, id int64) (Version, error) {
	v, err := scanVersion(d.db.QueryRowContext(ctx, `SELECT `+versionColumns+` FROM versions WHERE id = ?`, id))
	if errors.Is(err, sql.ErrNoRows) {
		return Version{}, fmt.Errorf("version %d: %w", id, ErrNotFound)
	}
	if err != nil {
		return Version{}, fmt.Errorf("get version %d: %w", id, err)
	}
	return v, nil
}

// HasVersion reports whether file fileID has a version with content hash,
// a conflict version included, a deletion not: whether hash may be the
// base of a write.
func (d *DB) HasVersion(ctx context.Context, fileID int64, hash string) (bool, error) {
	var ok bool
	err := d.db.QueryRowContext(ctx, `SELECT EXISTS (SELECT 1 FROM versions WHERE file_id = ? AND hash = ? AND hash != '')`,
		fileID, hash).Scan(&ok)
	if err != nil {
		return false, fmt.Errorf("find version %s of file %d: %w", hash, fileID, err)
	}
	return ok, nil
}

// UsedHashes returns every hash a version, a file or a hub revision whose
// content is kept names, in every vault: the blobs that must stay.
func (d *DB) UsedHashes(ctx context.Context) (map[string]bool, error) {
	rows, err := d.db.QueryContext(ctx, `SELECT hash FROM versions WHERE hash != '' UNION SELECT hash FROM files
		UNION SELECT hash FROM revisions WHERE hash != '' AND pruned = 0`)
	if err != nil {
		return nil, fmt.Errorf("list used hashes: %w", err)
	}
	defer rows.Close()
	used := map[string]bool{}
	for rows.Next() {
		var hash string
		if err := rows.Scan(&hash); err != nil {
			return nil, fmt.Errorf("list used hashes: %w", err)
		}
		used[hash] = true
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("list used hashes: %w", err)
	}
	return used, nil
}

// LastFileAt returns the ID of the file whose newest version was recorded
// at path in vault, the way to a deleted file's versions, or ErrNotFound.
// Task card T-0018.
func (d *DB) LastFileAt(ctx context.Context, vaultID int64, path string) (int64, error) {
	var id int64
	err := d.db.QueryRowContext(ctx, `SELECT file_id FROM versions
		WHERE vault_id = ? AND path = ? ORDER BY id DESC LIMIT 1`, vaultID, path).Scan(&id)
	if errors.Is(err, sql.ErrNoRows) {
		return 0, fmt.Errorf("last file at %s: %w", path, ErrNotFound)
	}
	if err != nil {
		return 0, fmt.Errorf("last file at %s: %w", path, err)
	}
	return id, nil
}

// PruneVersions deletes the versions made before before, except the newest
// version of each file that still exists and the versions holding a
// file's current content (a conflict version can be newer), and returns,
// sorted, the hashes that no version, no file and no hub revision whose
// content is kept names any more: the blobs that may go. Task card T-0018.
func (t *Tx) PruneVersions(ctx context.Context, before time.Time) ([]string, error) {
	rows, err := t.QueryContext(ctx, `DELETE FROM versions WHERE at < ? AND id NOT IN (
		SELECT MAX(v.id) FROM versions v JOIN files f ON f.id = v.file_id GROUP BY v.file_id
	) AND NOT EXISTS (SELECT 1 FROM files f WHERE f.id = versions.file_id AND f.hash = versions.hash)
	RETURNING hash`, FormatTime(before))
	if err != nil {
		return nil, fmt.Errorf("prune versions: %w", err)
	}
	defer rows.Close()
	hashes := map[string]struct{}{}
	for rows.Next() {
		var hash string
		if err := rows.Scan(&hash); err != nil {
			return nil, fmt.Errorf("prune versions: %w", err)
		}
		if hash != "" {
			hashes[hash] = struct{}{}
		}
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("prune versions: %w", err)
	}
	if err := rows.Close(); err != nil {
		return nil, fmt.Errorf("prune versions: %w", err)
	}
	freed := []string{}
	for hash := range hashes {
		var used bool
		if err := t.QueryRowContext(ctx, `SELECT EXISTS (SELECT 1 FROM versions WHERE hash = ?)
			OR EXISTS (SELECT 1 FROM files WHERE hash = ?)
			OR EXISTS (SELECT 1 FROM revisions WHERE hash = ? AND pruned = 0)`, hash, hash, hash).Scan(&used); err != nil {
			return nil, fmt.Errorf("check pruned hash %s: %w", hash, err)
		}
		if !used {
			freed = append(freed, hash)
		}
	}
	slices.Sort(freed)
	return freed, nil
}
