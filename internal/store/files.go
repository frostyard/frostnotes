package store

import (
	"context"
	"errors"
	"time"
)

// FileKind is what a file is to the index.
type FileKind string

// The kinds of file (docs/specs/markdown.md#notes-and-files).
const (
	FileNote       FileKind = "note"       // a name ending in ".md", exactly
	FileAttachment FileKind = "attachment" // any other file
)

// File is a files row: one file of a vault.
type File struct {
	ID      int64
	VaultID int64
	Path    string    // vault-relative and slash-separated, as on disk
	Size    int64     // in bytes
	ModTime time.Time // stored as Unix nanoseconds, so a scan compares it exactly
	Hash    string    // the lowercase hex SHA-256 of the content: its blob ID
}

// Kind is FileNote when f's name ends in ".md", FileAttachment otherwise.
// Task card T-0014.
func (f File) Kind() FileKind { return "" }

// FileName is the key markdown.Resolve's files(name) looks up: the last
// segment of p, lower-cased by strings.ToLower. files.name holds it.
// Task card T-0014.
func FileName(p string) string { return "" }

// PutFile records f in vault f.VaultID: it inserts a row when the vault has
// no file at f.Path, and otherwise updates that row's size, time and hash.
// It returns the row; f.ID is ignored. Inserting a file re-resolves the
// vault's links that could name it. Task card T-0014.
func (t *Tx) PutFile(ctx context.Context, f File) (File, error) {
	return File{}, errors.New("store.PutFile is not implemented yet (T-0014)")
}

// MoveFile gives file id the path to, keeping its ID, its index rows and
// its versions. It returns ErrNotFound when there is no file id and
// ErrConflict when to is taken. The links that could name the file by its
// old or new name, and a moved note's own links, are re-resolved.
// Task card T-0014.
func (t *Tx) MoveFile(ctx context.Context, id int64, to string) (File, error) {
	return File{}, errors.New("store.MoveFile is not implemented yet (T-0014)")
}

// DeleteFile removes file id and its index rows, or returns ErrNotFound.
// The links that resolved to it are re-resolved: another file may now
// match. Its versions stay. Task card T-0014.
func (t *Tx) DeleteFile(ctx context.Context, id int64) error {
	return errors.New("store.DeleteFile is not implemented yet (T-0014)")
}

// FileByPath returns vault's file at path, or ErrNotFound. Task card
// T-0014.
func (d *DB) FileByPath(ctx context.Context, vaultID int64, path string) (File, error) {
	return File{}, errors.New("store.FileByPath is not implemented yet (T-0014)")
}

// FileByPath returns vault's file at path as this transaction sees it, or
// ErrNotFound. Task card T-0014.
func (t *Tx) FileByPath(ctx context.Context, vaultID int64, path string) (File, error) {
	return File{}, errors.New("store.Tx.FileByPath is not implemented yet (T-0014)")
}

// Files returns every file of vault in byte order of path: what a scan
// compares with a walk. Task card T-0014.
func (d *DB) Files(ctx context.Context, vaultID int64) ([]File, error) {
	return nil, errors.New("store.Files is not implemented yet (T-0014)")
}

// FileCounts returns how many notes and attachments vault has. Task card
// T-0014.
func (d *DB) FileCounts(ctx context.Context, vaultID int64) (notes, attachments int, err error) {
	return 0, 0, errors.New("store.FileCounts is not implemented yet (T-0014)")
}

// ResolveLink returns the file that a link from note source to target
// names, by markdown.Resolve over vault's files as this transaction sees
// them, and false when the link is unresolved. Task card T-0014.
func (t *Tx) ResolveLink(ctx context.Context, vaultID int64, source, target string) (File, bool, error) {
	return File{}, false, errors.New("store.ResolveLink is not implemented yet (T-0014)")
}
