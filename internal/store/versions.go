package store

import (
	"context"
	"errors"
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
func (v Version) API() api.Version { return api.Version{} }

// AddVersion appends v with At = t.Now(), to the millisecond, and returns
// it with its ID; v.ID and v.At are ignored. Task card T-0018.
func (t *Tx) AddVersion(ctx context.Context, v Version) (Version, error) {
	return Version{}, errors.New("store.AddVersion is not implemented yet (T-0018)")
}

// Versions returns file fileID's versions, newest first, at most limit of
// them (50 when limit is 0 or less). Task card T-0018.
func (d *DB) Versions(ctx context.Context, fileID int64, limit int) ([]Version, error) {
	return nil, errors.New("store.Versions is not implemented yet (T-0018)")
}

// GetVersion returns version id, or ErrNotFound. Task card T-0018.
func (d *DB) GetVersion(ctx context.Context, id int64) (Version, error) {
	return Version{}, errors.New("store.GetVersion is not implemented yet (T-0018)")
}

// LastFileAt returns the ID of the file whose newest version was recorded
// at path in vault, the way to a deleted file's versions, or ErrNotFound.
// Task card T-0018.
func (d *DB) LastFileAt(ctx context.Context, vaultID int64, path string) (int64, error) {
	return 0, errors.New("store.LastFileAt is not implemented yet (T-0018)")
}

// PruneVersions deletes the versions made before before, except the newest
// version of each file that still exists, and returns, sorted, the hashes
// that no version and no file names any more: the blobs that may go. Task
// card T-0018.
func (t *Tx) PruneVersions(ctx context.Context, before time.Time) ([]string, error) {
	return nil, errors.New("store.PruneVersions is not implemented yet (T-0018)")
}
