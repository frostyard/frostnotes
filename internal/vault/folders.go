package vault

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"maps"
	"os"
	"path"
	"slices"
	"strings"
	"time"

	"github.com/frostyard/frostnotes/api"
	"github.com/frostyard/frostnotes/internal/store"
	"github.com/frostyard/frostnotes/internal/templates"
)

// folderPath checks a folder the note domain names: a vault path outside
// the settings folder, never the root. It returns it without surrounding
// slashes.
func folderPath(p string) (string, error) {
	p = strings.Trim(p, "/")
	return p, checkNotePath(p)
}

// checkDir lstats p's parent folders and p under r, refusing a symlink
// anywhere: p must be a folder. A missing one is an error matching
// fs.ErrNotExist.
func checkDir(r *os.Root, p string) error {
	segs := strings.Split(p, "/")
	for i := range segs {
		prefix := strings.Join(segs[:i+1], "/")
		fi, err := r.Lstat(prefix)
		if err != nil {
			return err
		}
		if fi.Mode()&fs.ModeSymlink != 0 {
			return fmt.Errorf("%w: %q is a symlink", ErrPath, prefix)
		}
		if !fi.IsDir() {
			return fmt.Errorf("%w: %q is not a folder", ErrPath, prefix)
		}
	}
	return nil
}

// checkIgnoredFolders refuses folders the vault's settings ignore. v.mu
// must be held.
func (v *Vault) checkIgnoredFolders(folders ...string) error {
	for _, f := range folders {
		if v.patterns().Match(f, true) {
			return api.InvalidParams("%s is ignored by the vault's settings (%s)", f, SettingsFile)
		}
	}
	return nil
}

// within returns the index's files under folder, by path.
func (v *Vault) within(ctx context.Context, folder string) ([]store.File, error) {
	files, err := v.cfg.DB.Files(ctx, v.id)
	if err != nil {
		return nil, err
	}
	return slices.DeleteFunc(files, func(f store.File) bool { return !strings.HasPrefix(f.Path, folder+"/") }), nil
}

// Mkdir creates folder p and its missing parents; an existing folder is
// not an error. Folders have no events: a client lists note.files again.
func (v *Vault) Mkdir(ctx context.Context, p string) error {
	p, err := folderPath(p)
	if err != nil {
		return err
	}
	r, err := v.open()
	if err != nil {
		return err
	}
	defer func() { _ = r.Close() }()
	if err := v.lock(); err != nil {
		return err
	}
	defer v.mu.Unlock()
	if err := v.checkIgnoredFolders(p); err != nil {
		return err
	}
	return apiError(p, v.settled(mkdirAll(r, p)))
}

// MoveFolder renames or moves folder from, with everything in it, to to,
// which must not exist, creating missing folders: one rename, after which
// each file the index has under it keeps its ID and versions, as Move's
// does, and with updateLinks the links Move would rewrite for each are
// rewritten.
func (v *Vault) MoveFolder(ctx context.Context, from, to string, updateLinks bool, who Author) (*api.FolderMoveResult, error) {
	from, err := folderPath(from)
	if err != nil {
		return nil, err
	}
	if to, err = folderPath(to); err != nil {
		return nil, err
	}
	switch {
	case from == to:
		return nil, api.InvalidParams("%s is already at %s", from, to)
	case strings.HasPrefix(to, from+"/"):
		return nil, api.InvalidParams("%s cannot move into itself, to %s", from, to)
	}
	r, err := v.open()
	if err != nil {
		return nil, err
	}
	defer func() { _ = r.Close() }()
	if err := v.lock(); err != nil {
		return nil, err
	}
	defer v.mu.Unlock()
	if err := v.checkIgnoredFolders(from, to); err != nil {
		return nil, err
	}
	if err := checkDir(r, from); err != nil {
		return nil, apiError(from, err)
	}
	if _, err := r.Lstat(to); err == nil {
		return nil, api.Conflict("%s exists", to)
	} else if !errors.Is(err, fs.ErrNotExist) {
		return nil, apiError(to, err)
	}
	// Rows under to are files gone from the disk: let them go first.
	stale, err := v.within(ctx, to)
	if err != nil {
		return nil, err
	}
	for _, f := range stale {
		if _, _, err := v.refresh(ctx, r, f.Path); err != nil && !errors.Is(err, fs.ErrNotExist) {
			return nil, err
		}
	}
	moved, err := v.within(ctx, from)
	if err != nil {
		return nil, err
	}
	moves := make(map[string]string, len(moved))
	for _, f := range moved {
		moves[f.Path] = to + strings.TrimPrefix(f.Path, from)
	}
	var plans []rewritePlan
	if updateLinks {
		if plans, err = v.planRewrites(ctx, r, moved, moves); err != nil {
			return nil, err
		}
	}
	if err := mkdirs(r, to); err != nil {
		return nil, apiError(to, err)
	}
	if err := v.settled(rename(r, v.root, from, to)); errors.Is(err, fs.ErrExist) {
		return nil, api.Conflict("%s exists", to)
	} else if err != nil {
		return nil, err
	}
	if v.crashed("renamed") {
		return nil, ErrCrashed
	}
	// The folder has moved: record its files, and rewrite their links,
	// whatever the client does now.
	ctx = context.WithoutCancel(ctx)
	res := &api.FolderMoveResult{Path: to, Moved: make([]string, 0, len(moved))}
	err = v.cfg.DB.Tx(ctx, func(tx *store.Tx) error {
		for _, row := range moved {
			f, err := tx.MoveFile(ctx, row.ID, moves[row.Path])
			if err != nil {
				return err
			}
			if err := v.logRevision(ctx, tx, api.RevisionOpMove, f.ID, f.Path, f.Hash, f.Size, who); err != nil {
				return err
			}
			if err := tx.Emit(ctx, api.NoteMoved{VaultID: v.id, From: row.Path, To: f.Path}); err != nil {
				return err
			}
			res.Moved = append(res.Moved, f.Path)
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	res.Rewritten = v.rewrite(ctx, r, plans, moves, who)
	return res, nil
}

// DeleteFolder deletes the files of folder that files lists, each as
// Delete does with its base, then every folder left empty, the folder
// itself included. A file changed since its base, or not listed, is kept:
// an edit beats a delete (ADR-0005).
func (v *Vault) DeleteFolder(ctx context.Context, folder string, files []api.FileBase, who Author) (*api.FolderDeleteResult, error) {
	folder, err := folderPath(folder)
	if err != nil {
		return nil, err
	}
	bases := make(map[string]string, len(files))
	for _, f := range files {
		if !strings.HasPrefix(f.Path, folder+"/") {
			return nil, api.InvalidParams("%s is not in %s", f.Path, folder)
		}
		if err := checkNotePath(f.Path); err != nil {
			return nil, err
		}
		bases[f.Path] = f.Base
	}
	r, err := v.open()
	if err != nil {
		return nil, err
	}
	defer func() { _ = r.Close() }()
	if err := v.lock(); err != nil {
		return nil, err
	}
	defer v.mu.Unlock()
	if err := v.checkIgnoredFolders(folder); err != nil {
		return nil, err
	}
	if err := checkDir(r, folder); err != nil {
		return nil, apiError(folder, err)
	}
	rows, err := v.within(ctx, folder)
	if err != nil {
		return nil, err
	}
	paths := slices.Collect(maps.Keys(bases))
	for _, f := range rows {
		paths = append(paths, f.Path)
	}
	slices.Sort(paths)
	paths = slices.Compact(paths)
	// Each delete is final: once one has run, the rest run whatever the
	// client does.
	ctx = context.WithoutCancel(ctx)
	res := &api.FolderDeleteResult{Deleted: []string{}, Kept: []string{}}
	for _, p := range paths {
		base, listed := bases[p]
		if !listed {
			res.Kept = append(res.Kept, p)
			continue
		}
		err := v.delete(ctx, r, p, base, who)
		switch {
		case err == nil:
			res.Deleted = append(res.Deleted, p)
		case errors.Is(err, api.ErrConflict):
			res.Kept = append(res.Kept, p)
		case errors.Is(err, api.ErrNotFound), errors.Is(err, api.ErrInvalidParams):
			// Gone already, or no vault file (a symlink), which no listing
			// gives.
		default:
			return nil, err
		}
	}
	removeEmptyFolders(r, folder)
	_, err = r.Lstat(folder)
	res.Removed = errors.Is(err, fs.ErrNotExist)
	return res, nil
}

// removeEmptyFolders removes the folders under dir that hold nothing,
// deepest first, then dir if it is empty; hidden folders, and folders
// holding anything, stay.
func removeEmptyFolders(r *os.Root, dir string) {
	f, err := r.Open(dir)
	if err != nil {
		return
	}
	entries, err := f.ReadDir(-1)
	_ = f.Close()
	if err != nil {
		return
	}
	for _, e := range entries {
		if e.IsDir() && !strings.HasPrefix(e.Name(), ".") {
			removeEmptyFolders(r, path.Join(dir, e.Name()))
		}
	}
	if !strings.HasPrefix(path.Base(dir), ".") && r.Remove(dir) == nil {
		_ = syncDir(r, path.Dir(dir))
	}
}

// Template returns the template note at p with its variables expanded
// (docs/design/vault.md#daily-notes): {{title}} is title, and {{date}} and
// {{time}} give at, in the template settings' formats.
func (v *Vault) Template(ctx context.Context, p, title string, at time.Time) (string, error) {
	if kindOf(p) != store.FileNote {
		return "", api.InvalidParams("%s is not a note: a template is a note", p)
	}
	settings, err := LoadSettings(v.root)
	if err != nil {
		return "", err
	}
	n, err := v.Read(ctx, p)
	if err != nil {
		return "", err
	}
	return string(templates.Expand([]byte(n.Content), templates.Values{Title: title, Now: at,
		DateFormat: settings.Templates.DateFormat, TimeFormat: settings.Templates.TimeFormat})), nil
}
