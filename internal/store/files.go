package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"path"
	"strings"
	"time"

	"github.com/frostyard/frostnotes/internal/markdown"
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
func (f File) Kind() FileKind {
	if strings.HasSuffix(path.Base(f.Path), ".md") {
		return FileNote
	}
	return FileAttachment
}

// FileName is the key markdown.Resolve's files(name) looks up: the last
// segment of p, lower-cased by strings.ToLower. files.name holds it.
// Task card T-0014.
func FileName(p string) string { return strings.ToLower(path.Base(p)) }

// PutFile records f in vault f.VaultID: it inserts a row when the vault has
// no file at f.Path, and otherwise updates that row's size, time and hash.
// It returns the row; f.ID is ignored. Inserting a file re-resolves the
// vault's links that could name it. Task card T-0014.
func (t *Tx) PutFile(ctx context.Context, f File) (File, error) {
	row := t.QueryRowContext(ctx, `UPDATE files SET size = ?, mtime = ?, hash = ?
		WHERE vault_id = ? AND path = ? RETURNING `+fileColumns,
		f.Size, f.ModTime.UnixNano(), f.Hash, f.VaultID, f.Path)
	stored, err := scanFile(row)
	if err == nil {
		return stored, nil
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return File{}, fmt.Errorf("update file %s: %w", f.Path, err)
	}
	row = t.QueryRowContext(ctx, `INSERT INTO files (vault_id, path, name, kind, size, mtime, hash)
		VALUES (?, ?, ?, ?, ?, ?, ?) RETURNING `+fileColumns,
		f.VaultID, f.Path, FileName(f.Path), string(f.Kind()), f.Size, f.ModTime.UnixNano(), f.Hash)
	stored, err = scanFile(row)
	if err != nil {
		return File{}, fmt.Errorf("insert file %s: %w", f.Path, err)
	}
	if err := t.resolveFileLinks(ctx, stored.VaultID, stored.Path, stored.Path, 0); err != nil {
		return File{}, err
	}
	return stored, nil
}

// RestoreFile records f, which DeleteFile removed, again with its ID, so
// its versions name it once more: note.revert bringing a deleted file back.
// It returns ErrConflict when vault f.VaultID has a file at f.Path or a
// file with f.ID. Like PutFile's insert, it re-resolves the vault's links
// that could name it.
func (t *Tx) RestoreFile(ctx context.Context, f File) (File, error) {
	row := t.QueryRowContext(ctx, `INSERT INTO files (id, vault_id, path, name, kind, size, mtime, hash)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?) RETURNING `+fileColumns,
		f.ID, f.VaultID, f.Path, FileName(f.Path), string(f.Kind()), f.Size, f.ModTime.UnixNano(), f.Hash)
	stored, err := scanFile(row)
	if IsUniqueViolation(err) {
		return File{}, fmt.Errorf("restore file %d at %s: %w", f.ID, f.Path, ErrConflict)
	}
	if err != nil {
		return File{}, fmt.Errorf("restore file %d at %s: %w", f.ID, f.Path, err)
	}
	if err := t.resolveFileLinks(ctx, stored.VaultID, stored.Path, stored.Path, 0); err != nil {
		return File{}, err
	}
	return stored, nil
}

// MoveFile gives file id the path to, keeping its ID, its index rows and
// its versions. It returns ErrNotFound when there is no file id and
// ErrConflict when to is taken. The links that could name the file by its
// old or new name, and a moved note's own links, are re-resolved, and a
// renamed note's title follows its name.
// Task card T-0014.
func (t *Tx) MoveFile(ctx context.Context, id int64, to string) (File, error) {
	f, err := scanFile(t.QueryRowContext(ctx, `SELECT `+fileColumns+` FROM files WHERE id = ?`, id))
	if errors.Is(err, sql.ErrNoRows) {
		return File{}, fmt.Errorf("file %d: %w", id, ErrNotFound)
	}
	if err != nil {
		return File{}, fmt.Errorf("get file %d: %w", id, err)
	}
	old := f
	f.Path = to
	_, err = t.ExecContext(ctx, `UPDATE files SET path = ?, name = ?, kind = ? WHERE id = ?`,
		to, FileName(to), string(f.Kind()), id)
	if IsUniqueViolation(err) {
		return File{}, fmt.Errorf("move file %d to %s: %w", id, to, ErrConflict)
	}
	if err != nil {
		return File{}, fmt.Errorf("move file %d: %w", id, err)
	}
	if old.Kind() == FileNote && f.Kind() == FileAttachment {
		if _, err := t.ExecContext(ctx, `DELETE FROM notes WHERE file_id = ?`, id); err != nil {
			return File{}, fmt.Errorf("delete note index %d: %w", id, err)
		}
	}
	// A note's title is its file name, as IndexNote sets it.
	if title := strings.TrimSuffix(path.Base(to), ".md"); old.Kind() == FileNote && f.Kind() == FileNote &&
		title != strings.TrimSuffix(path.Base(old.Path), ".md") {
		if _, err := t.ExecContext(ctx, `UPDATE notes SET title = ? WHERE file_id = ?`, title, id); err != nil {
			return File{}, fmt.Errorf("retitle note %d: %w", id, err)
		}
		if _, err := t.ExecContext(ctx, `UPDATE notes_fts SET title = ? WHERE rowid = ?`, title, id); err != nil {
			return File{}, fmt.Errorf("retitle note %d: %w", id, err)
		}
	}
	if err := t.resolveFileLinks(ctx, f.VaultID, old.Path, to, id); err != nil {
		return File{}, err
	}
	return f, nil
}

// DeleteFile removes file id and its index rows, or returns ErrNotFound.
// The links that resolved to it are re-resolved: another file may now
// match. Its versions stay. Task card T-0014.
func (t *Tx) DeleteFile(ctx context.Context, id int64) error {
	f, err := scanFile(t.QueryRowContext(ctx, `DELETE FROM files WHERE id = ? RETURNING `+fileColumns, id))
	if errors.Is(err, sql.ErrNoRows) {
		return fmt.Errorf("file %d: %w", id, ErrNotFound)
	}
	if err != nil {
		return fmt.Errorf("delete file %d: %w", id, err)
	}
	return t.resolveFileLinks(ctx, f.VaultID, f.Path, f.Path, 0)
}

// FileByID returns file id, or ErrNotFound.
func (d *DB) FileByID(ctx context.Context, id int64) (File, error) {
	f, err := scanFile(d.db.QueryRowContext(ctx, `SELECT `+fileColumns+` FROM files WHERE id = ?`, id))
	if errors.Is(err, sql.ErrNoRows) {
		return File{}, fmt.Errorf("file %d: %w", id, ErrNotFound)
	}
	if err != nil {
		return File{}, fmt.Errorf("get file %d: %w", id, err)
	}
	return f, nil
}

// FileByPath returns vault's file at path, or ErrNotFound. Task card
// T-0014.
func (d *DB) FileByPath(ctx context.Context, vaultID int64, path string) (File, error) {
	return fileByPath(ctx, d.db, vaultID, path)
}

// FileByPath returns vault's file at path as this transaction sees it, or
// ErrNotFound. Task card T-0014.
func (t *Tx) FileByPath(ctx context.Context, vaultID int64, path string) (File, error) {
	return fileByPath(ctx, t, vaultID, path)
}

// Files returns every file of vault in byte order of path: what a scan
// compares with a walk. Task card T-0014.
func (d *DB) Files(ctx context.Context, vaultID int64) ([]File, error) {
	rows, err := d.db.QueryContext(ctx, `SELECT `+fileColumns+` FROM files WHERE vault_id = ? ORDER BY path`, vaultID)
	if err != nil {
		return nil, fmt.Errorf("list files: %w", err)
	}
	defer rows.Close()
	files := []File{}
	for rows.Next() {
		f, err := scanFile(rows)
		if err != nil {
			return nil, fmt.Errorf("list files: %w", err)
		}
		files = append(files, f)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("list files: %w", err)
	}
	return files, nil
}

// FilesNamed returns the paths of vault's files whose FileName is name:
// markdown.Resolve's files(name), outside a transaction.
func (d *DB) FilesNamed(ctx context.Context, vaultID int64, name string) ([]string, error) {
	return filePathsByName(ctx, d.db, vaultID, name)
}

// FileCounts returns how many notes and attachments vault has. Task card
// T-0014.
func (d *DB) FileCounts(ctx context.Context, vaultID int64) (notes, attachments int, err error) {
	err = d.db.QueryRowContext(ctx, `SELECT count(CASE WHEN kind = 'note' THEN 1 END),
		count(CASE WHEN kind = 'attachment' THEN 1 END) FROM files WHERE vault_id = ?`, vaultID).
		Scan(&notes, &attachments)
	if err != nil {
		return 0, 0, fmt.Errorf("count files: %w", err)
	}
	return notes, attachments, nil
}

// ResolveLink returns the file that a link from note source to target
// names, by markdown.Resolve over vault's files as this transaction sees
// them, and false when the link is unresolved. Task card T-0014.
func (t *Tx) ResolveLink(ctx context.Context, vaultID int64, source, target string) (File, bool, error) {
	var lookupErr error
	resolved := markdown.Resolve(source, target, func(name string) []string {
		if lookupErr != nil {
			return nil
		}
		var paths []string
		paths, lookupErr = t.filePathsByName(ctx, vaultID, name)
		return paths
	})
	if lookupErr != nil {
		return File{}, false, fmt.Errorf("resolve link %s: %w", target, lookupErr)
	}
	if resolved == "" {
		return File{}, false, nil
	}
	f, err := t.FileByPath(ctx, vaultID, resolved)
	if errors.Is(err, ErrNotFound) {
		return File{}, false, nil
	}
	if err != nil {
		return File{}, false, fmt.Errorf("resolve link %s: %w", target, err)
	}
	return f, true, nil
}

const fileColumns = `id, vault_id, path, size, mtime, hash`

func scanFile(sc interface{ Scan(...any) error }) (File, error) {
	var f File
	var mtime int64
	if err := sc.Scan(&f.ID, &f.VaultID, &f.Path, &f.Size, &mtime, &f.Hash); err != nil {
		return File{}, err
	}
	f.ModTime = time.Unix(0, mtime).UTC()
	return f, nil
}

func fileByPath(ctx context.Context, q interface {
	QueryRowContext(context.Context, string, ...any) *sql.Row
}, vaultID int64, path string) (File, error) {
	f, err := scanFile(q.QueryRowContext(ctx, `SELECT `+fileColumns+` FROM files WHERE vault_id = ? AND path = ?`, vaultID, path))
	if errors.Is(err, sql.ErrNoRows) {
		return File{}, fmt.Errorf("file %s: %w", path, ErrNotFound)
	}
	if err != nil {
		return File{}, fmt.Errorf("get file %s: %w", path, err)
	}
	return f, nil
}

func (t *Tx) filePathsByName(ctx context.Context, vaultID int64, name string) ([]string, error) {
	return filePathsByName(ctx, t, vaultID, name)
}

func filePathsByName(ctx context.Context, q interface {
	QueryContext(context.Context, string, ...any) (*sql.Rows, error)
}, vaultID int64, name string) ([]string, error) {
	rows, err := q.QueryContext(ctx, `SELECT path FROM files WHERE vault_id = ? AND name = ? ORDER BY path`, vaultID, name)
	if err != nil {
		return nil, fmt.Errorf("find files named %s: %w", name, err)
	}
	defer rows.Close()
	var paths []string
	for rows.Next() {
		var p string
		if err := rows.Scan(&p); err != nil {
			return nil, fmt.Errorf("find files named %s: %w", name, err)
		}
		paths = append(paths, p)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("find files named %s: %w", name, err)
	}
	return paths, nil
}

type fileLink struct {
	fileID int64
	ord    int
	source string
	target string
}

// fileLinks collects affected links and closes the query before any resolution
// or updates run. movedID includes a moved file's own relative links.
func (t *Tx) fileLinks(ctx context.Context, vaultID int64, oldPath, newPath string, movedID int64) ([]fileLink, error) {
	rows, err := t.QueryContext(ctx, `SELECT l.file_id, l.ord, f.path, l.target
		FROM links l JOIN files f ON f.id = l.file_id
		WHERE f.vault_id = ? AND (l.name IN (?, ?) OR l.file_id = ?)`,
		vaultID, strings.TrimSuffix(FileName(oldPath), ".md"), strings.TrimSuffix(FileName(newPath), ".md"), movedID)
	if err != nil {
		return nil, fmt.Errorf("list affected links: %w", err)
	}
	defer rows.Close()
	var links []fileLink
	for rows.Next() {
		var link fileLink
		if err := rows.Scan(&link.fileID, &link.ord, &link.source, &link.target); err != nil {
			return nil, fmt.Errorf("list affected links: %w", err)
		}
		links = append(links, link)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("list affected links: %w", err)
	}
	return links, nil
}

func (t *Tx) resolveFileLinks(ctx context.Context, vaultID int64, oldPath, newPath string, movedID int64) error {
	links, err := t.fileLinks(ctx, vaultID, oldPath, newPath, movedID)
	if err != nil {
		return err
	}
	for _, link := range links {
		f, ok, err := t.ResolveLink(ctx, vaultID, link.source, link.target)
		if err != nil {
			return err
		}
		dest := sql.NullInt64{Int64: f.ID, Valid: ok}
		if _, err := t.ExecContext(ctx, `UPDATE links SET dest_id = ? WHERE file_id = ? AND ord = ?`,
			dest, link.fileID, link.ord); err != nil {
			return fmt.Errorf("resolve link %d of file %d: %w", link.ord, link.fileID, err)
		}
	}
	return nil
}
