package vault

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path"
	"strings"

	"github.com/frostyard/frostnotes/api"
	"github.com/frostyard/frostnotes/internal/markdown"
	"github.com/frostyard/frostnotes/internal/store"
)

// ErrChanged means a file is not in the state a change expected: it
// changed since, on disk or in the index. Sync pushes the local change
// first and applies the hub's answer (ADR-0017).
var ErrChanged = errors.New("vault: the file is not as expected")

// Record runs in the transaction that records a change, after the change's
// own rows, with the file as it now is (a deleted file's last row): sync
// sets the file's agreed state there, so the two cannot disagree after a
// crash.
type Record func(ctx context.Context, tx *store.Tx, f store.File) error

// Expect is the local state a pulled head may replace: the file at Path
// whose content hashes to Hash, or, when Hash is "", no file at Path.
type Expect struct {
	Path string
	Hash string
}

// Head is the state Apply makes: the blob Hash at Path, or no file when
// Deleted.
type Head struct {
	Path    string
	Hash    string
	Deleted bool
}

// Apply makes the vault hold head where it holds exp, and nothing
// otherwise: a device applying what it pulled, which compares instead of
// merging (ADR-0017, docs/specs/sync-protocol.md). The file is compared
// with exp once more just before each rename; when it differs, nothing is
// written and Apply returns ErrChanged. Content streams from the blob
// store at any size. Every path passes the path guard, so a hub cannot
// write outside the vault or into its settings. rec, when not nil, runs in
// the transaction of the last change, or in one of its own when nothing
// needs to change. A folder the change leaves empty is removed.
func (v *Vault) Apply(ctx context.Context, exp Expect, head Head, who Author, rec Record) (store.File, error) {
	for _, p := range []string{exp.Path, head.Path} {
		if err := checkNotePath(p); err != nil {
			return store.File{}, err
		}
	}
	r, err := v.open()
	if err != nil {
		return store.File{}, err
	}
	defer func() { _ = r.Close() }()
	if err := v.lock(); err != nil {
		return store.File{}, err
	}
	defer v.mu.Unlock()
	if err := v.checkIgnored(exp.Path, head.Path); err != nil {
		return store.File{}, err
	}
	switch {
	case head.Deleted && exp.Hash == "":
		if _, _, err := v.expectAt(ctx, r, exp.Path, ""); err != nil {
			return store.File{}, err
		}
		return store.File{}, v.recordOnly(ctx, store.File{}, rec)
	case head.Deleted:
		return v.removeFile(ctx, r, exp.Path, exp.Hash, who, rec)
	case exp.Hash == "":
		return v.putBlob(ctx, r, head.Path, "", head.Hash, 0, who, rec)
	}
	p := exp.Path
	if head.Path != exp.Path {
		var moveRec Record
		if head.Hash == exp.Hash {
			moveRec = rec
		}
		f, err := v.relocate(ctx, r, exp.Path, exp.Hash, head.Path, who, moveRec)
		if err != nil || head.Hash == exp.Hash {
			return f, err
		}
		p = head.Path
	}
	return v.putBlob(ctx, r, p, exp.Hash, head.Hash, 0, who, rec)
}

// recordOnly runs rec in a transaction of its own, for a change that
// needed no change to the vault.
func (v *Vault) recordOnly(ctx context.Context, f store.File, rec Record) error {
	if rec == nil {
		return nil
	}
	return v.cfg.DB.Tx(context.WithoutCancel(ctx), func(tx *store.Tx) error { return rec(ctx, tx, f) })
}

// expectAt brings the index up to date with p on disk and checks that p
// holds content hashed want, or, when want is "", that nothing is at p
// (no file, folder or symlink, and no file spelled otherwise on a file
// system that ignores case). v.mu must be held.
func (v *Vault) expectAt(ctx context.Context, r *os.Root, p, want string) (store.File, content, error) {
	row, cur, err := v.refresh(ctx, r, p)
	switch {
	case want == "" && errors.Is(err, fs.ErrNotExist):
		if _, err := r.Lstat(p); err == nil {
			return store.File{}, content{}, fmt.Errorf("%s: %w", p, ErrChanged)
		}
		return store.File{}, content{}, nil
	case errors.Is(err, fs.ErrNotExist):
		return store.File{}, content{}, fmt.Errorf("%s is gone: %w", p, ErrChanged)
	case err != nil:
		return store.File{}, content{}, apiError(p, err)
	case want == "" || row.Hash != want:
		return store.File{}, content{}, fmt.Errorf("%s holds %.12s: %w", p, row.Hash, ErrChanged)
	}
	return row, cur, nil
}

// putBlob makes p hold the blob hash, where p holds content hashed want,
// or nothing when want is "" (then a new file, or, with id, the deleted
// file id coming back). It stages the blob beside p, compares p with want
// once more and renames, then indexes the file and records its version,
// its revision on a hub, and rec, in one transaction. v.mu must be held.
func (v *Vault) putBlob(ctx context.Context, r *os.Root, p, want, hash string, id int64, who Author, rec Record) (store.File, error) {
	row, cur, err := v.expectAt(ctx, r, p, want)
	if err != nil {
		return store.File{}, err
	}
	if want != "" && hash == want {
		return row, v.recordOnly(ctx, row, rec)
	}
	src, err := v.cfg.Blobs.Open(hash)
	if err != nil {
		return store.File{}, fmt.Errorf("content %s of %s: %w", hash, p, err)
	}
	defer func() { _ = src.Close() }()
	if want == "" {
		if err := mkdirs(r, p); err != nil {
			return store.File{}, apiError(p, err)
		}
	}
	st, err := stageFrom(r, v.root, p, src, cur.info)
	if err != nil {
		return store.File{}, err
	}
	if v.staged != nil {
		v.staged(p)
	}
	if want != "" {
		if now, err := read(r, p); err != nil || now.hash != want || !now.sameStat(cur.info) {
			st.discard()
			return store.File{}, fmt.Errorf("%s changed while it was written: %w", p, ErrChanged)
		}
	}
	if err := v.settled(st.commit(want == "")); errors.Is(err, fs.ErrExist) {
		return store.File{}, fmt.Errorf("%s appeared while it was written: %w", p, ErrChanged)
	} else if err != nil {
		return store.File{}, err
	}
	// The file holds the new content: record it whatever the caller does.
	ctx = context.WithoutCancel(ctx)
	var data []byte
	if kindOf(p) == store.FileNote && st.info.Size() <= bigFile {
		if data, err = v.cfg.Blobs.Get(hash); err != nil {
			return store.File{}, err
		}
	}
	var f store.File
	err = v.cfg.DB.Tx(ctx, func(tx *store.Tx) error {
		f = store.File{ID: id, VaultID: v.id, Path: p, Size: st.info.Size(), ModTime: st.info.ModTime(), Hash: hash}
		var err error
		if id != 0 {
			f, err = tx.RestoreFile(ctx, f)
		} else {
			f, err = tx.PutFile(ctx, f)
		}
		if err != nil {
			return err
		}
		if err := v.indexWrite(ctx, tx, f, data, who); err != nil {
			return err
		}
		if rec != nil {
			return rec(ctx, tx, f)
		}
		return nil
	})
	return f, err
}

// relocate moves the file at from, hashed want, to to, which must be free,
// keeping its ID and versions, and records the move, its revision on a hub
// and rec in one transaction. Links to the file are not rewritten: on a
// device the change came from elsewhere, where they were. v.mu must be
// held.
func (v *Vault) relocate(ctx context.Context, r *os.Root, from, want, to string, who Author, rec Record) (store.File, error) {
	row, _, err := v.expectAt(ctx, r, from, want)
	if err != nil {
		return store.File{}, err
	}
	if _, _, err := v.expectAt(ctx, r, to, ""); err != nil {
		return store.File{}, err
	}
	if err := mkdirs(r, to); err != nil {
		return store.File{}, apiError(to, err)
	}
	if err := v.settled(rename(r, v.root, from, to)); errors.Is(err, fs.ErrExist) {
		return store.File{}, fmt.Errorf("%s appeared while %s moved there: %w", to, from, ErrChanged)
	} else if err != nil {
		return store.File{}, err
	}
	ctx = context.WithoutCancel(ctx)
	var doc markdown.Doc
	var data []byte
	becameNote := kindOf(to) == store.FileNote && row.Kind() != store.FileNote
	if becameNote {
		if c, err := read(r, to); err == nil && c.data != nil {
			data, doc = c.data, markdown.Parse(c.data)
		}
	}
	var f store.File
	err = v.cfg.DB.Tx(ctx, func(tx *store.Tx) error {
		var err error
		if f, err = tx.MoveFile(ctx, row.ID, to); err != nil {
			return err
		}
		if becameNote {
			if err := tx.IndexNote(ctx, f, data, doc); err != nil {
				return err
			}
		}
		if err := v.logRevision(ctx, tx, api.RevisionOpMove, f.ID, f.Path, f.Hash, f.Size, who); err != nil {
			return err
		}
		if err := tx.Emit(ctx, api.NoteMoved{VaultID: v.id, From: from, To: to}); err != nil {
			return err
		}
		if rec != nil {
			return rec(ctx, tx, f)
		}
		return nil
	})
	if err == nil {
		removeEmptyDirs(r, path.Dir(from))
	}
	return f, err
}

// removeFile deletes the file at p, hashed want, compared once more just
// before, and records its deletion version, its revision on a hub and rec
// in one transaction. v.mu must be held.
func (v *Vault) removeFile(ctx context.Context, r *os.Root, p, want string, who Author, rec Record) (store.File, error) {
	row, _, err := v.expectAt(ctx, r, p, want)
	if err != nil {
		return store.File{}, err
	}
	if now, err := read(r, p); err != nil || now.hash != want {
		return store.File{}, fmt.Errorf("%s changed while it was deleted: %w", p, ErrChanged)
	}
	if err := v.settled(remove(r, p)); err != nil {
		return store.File{}, err
	}
	ctx = context.WithoutCancel(ctx)
	err = v.cfg.DB.Tx(ctx, func(tx *store.Tx) error {
		if err := v.deleteRow(ctx, tx, row, who); err != nil {
			return err
		}
		if rec != nil {
			return rec(ctx, tx, row)
		}
		return nil
	})
	if err == nil {
		removeEmptyDirs(r, path.Dir(p))
	}
	return row, err
}

// removeEmptyDirs removes dir and each parent while it is empty, up to
// the vault's root; removing a folder that is not empty fails, which ends
// it. Hidden folders, the settings folder among them, are left alone.
func removeEmptyDirs(r *os.Root, dir string) {
	for dir != "." && dir != "/" && dir != "" {
		if strings.HasPrefix(path.Base(dir), ".") || r.Remove(dir) != nil {
			return
		}
		dir = path.Dir(dir)
	}
}
