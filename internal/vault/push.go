package vault

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"path"
	"regexp"
	"strings"
	"unicode/utf8"

	"github.com/frostyard/frostnotes/api"
	"github.com/frostyard/frostnotes/internal/blobs"
	"github.com/frostyard/frostnotes/internal/merge"
	"github.com/frostyard/frostnotes/internal/store"
)

// PushChange is one device's change to a file of a hub vault: the file's
// whole new state and the revision it was based on (log.push's PushItem).
type PushChange struct {
	FileID   int64 // the hub's file ID; 0 for a create
	Base     int64 // the agreed revision; 0 for a create
	BaseHash string
	Path     string
	Hash     string // "" when Deleted
	Deleted  bool
	// Conflict: Hash lost conflicting hunks in a merge on the device; the
	// hub keeps it as a conflict revision of the file.
	Conflict bool
}

// PushOutcome is what the hub did with a PushChange.
type PushOutcome struct {
	Status     api.PushStatus
	Reason     api.RefuseReason // "" unless refused
	FileID     int64            // 0 for a refused create
	Conflicts  int64
	BasePruned bool
}

func refuse(reason api.RefuseReason) PushOutcome {
	return PushOutcome{Status: api.PushStatusRefused, Reason: reason}
}

var hashRE = regexp.MustCompile(`^[0-9a-f]{64}$`)

// Push applies a device's change to this hub vault, as
// docs/specs/sync-protocol.md ("A push, on the hub") says: it checks the
// change, then creates, merges, moves, deletes or refuses, through the
// vault's write path, so every change appends its revision in the same
// transaction as its version; who carries the push's device, replica and
// push ID. The repeat of a push ID is the caller's to answer. A refusal is
// an outcome, not an error; errors are failures of the hub itself.
func (v *Vault) Push(ctx context.Context, c PushChange, who Author) (PushOutcome, error) {
	if checkNotePath(c.Path) != nil || (c.Deleted && (c.Hash != "" || c.FileID == 0)) ||
		(!c.Deleted && !hashRE.MatchString(c.Hash)) {
		return refuse(api.RefuseReasonInvalid), nil
	}
	r, err := v.open()
	if err != nil {
		return PushOutcome{}, err
	}
	defer func() { _ = r.Close() }()
	if err := v.lock(); err != nil {
		return PushOutcome{}, err
	}
	defer v.mu.Unlock()
	if v.ignored(c.Path) {
		return refuse(api.RefuseReasonIgnored), nil
	}
	if !c.Deleted {
		if ok, err := v.cfg.Blobs.Has(c.Hash); err != nil {
			return PushOutcome{}, err
		} else if !ok {
			return refuse(api.RefuseReasonMissing), nil
		}
	}
	if c.FileID == 0 {
		if c.Conflict {
			return refuse(api.RefuseReasonInvalid), nil
		}
		return v.pushCreate(ctx, r, c, who)
	}
	if _, err := v.cfg.DB.HubFile(ctx, v.id, c.FileID); errors.Is(err, store.ErrNotFound) {
		return refuse(api.RefuseReasonUnknown), nil
	} else if err != nil {
		return PushOutcome{}, err
	}
	base, err := v.cfg.DB.Revision(ctx, v.id, c.Base)
	if errors.Is(err, store.ErrNotFound) || (err == nil && (base.FileID != c.FileID || base.Hash != c.BaseHash ||
		base.Op == api.RevisionOpDelete || base.Op == api.RevisionOpConflict || base.Op == api.RevisionOpResolve)) {
		return refuse(api.RefuseReasonBase), nil
	} else if err != nil {
		return PushOutcome{}, err
	}
	// An edit on the hub's disk that no scan has seen yet becomes the head
	// first, so the push merges with it.
	if row, err := v.cfg.DB.FileByID(ctx, c.FileID); err == nil {
		if _, _, err := v.refresh(ctx, r, row.Path); err != nil && !isGone(err) {
			return PushOutcome{}, apiError(row.Path, err)
		}
	} else if !errors.Is(err, store.ErrNotFound) {
		return PushOutcome{}, err
	}
	row, err := v.cfg.DB.FileByID(ctx, c.FileID)
	if c.Conflict {
		return v.pushConflict(ctx, c, row, err, who)
	}
	switch {
	case errors.Is(err, store.ErrNotFound) && c.Deleted:
		return PushOutcome{Status: api.PushStatusAccepted, FileID: c.FileID}, nil
	case errors.Is(err, store.ErrNotFound):
		return v.pushRestore(ctx, r, c, who)
	case err != nil:
		return PushOutcome{}, err
	case c.Deleted && row.Hash != c.BaseHash:
		// The answer carries the head, which the device restores.
		out := refuse(api.RefuseReasonEdited)
		out.FileID = c.FileID
		return out, nil
	case c.Deleted:
		if _, err := v.removeFile(ctx, r, row.Path, row.Hash, who, nil); err != nil {
			return PushOutcome{}, err
		}
		return PushOutcome{Status: api.PushStatusAccepted, FileID: c.FileID}, nil
	}
	return v.pushPut(ctx, r, c, base, row, who)
}

// pushConflict keeps a device's local conflict version as a conflict
// revision of the file, live or deleted, which flags it. v.mu must be held.
func (v *Vault) pushConflict(ctx context.Context, c PushChange, row store.File, rowErr error, who Author) (PushOutcome, error) {
	if rowErr != nil && !errors.Is(rowErr, store.ErrNotFound) {
		return PushOutcome{}, rowErr
	}
	if rowErr != nil {
		f, err := v.cfg.DB.LogFile(ctx, v.id, c.FileID)
		if err != nil {
			return PushOutcome{}, err
		}
		row = store.File{ID: c.FileID, VaultID: v.id, Path: f.Path}
	}
	size, err := v.cfg.Blobs.Size(c.Hash)
	if err != nil {
		return PushOutcome{}, err
	}
	if err := v.flagConflict(ctx, row, c.Hash, size, who); err != nil {
		return PushOutcome{}, err
	}
	return PushOutcome{Status: api.PushStatusAccepted, FileID: c.FileID}, nil
}

// pushCreate applies a create: a new file at a free path, or a merge as
// two creates into the file at that exact path. v.mu must be held.
func (v *Vault) pushCreate(ctx context.Context, r *os.Root, c PushChange, who Author) (PushOutcome, error) {
	taken, err := v.livePath(ctx, c.Path)
	if err != nil {
		return PushOutcome{}, err
	}
	if taken == "" {
		f, err := v.putBlob(ctx, r, c.Path, "", c.Hash, 0, who, nil)
		if err != nil {
			return PushOutcome{}, err
		}
		return PushOutcome{Status: api.PushStatusAccepted, FileID: f.ID}, nil
	}
	if taken != c.Path {
		// Only the case differs: a file of its own, which a file system
		// that ignores case could not hold beside the other, at the first
		// free name.
		p, err := v.freePath(ctx, r, c.Path, 0)
		if err != nil {
			return PushOutcome{}, err
		}
		f, err := v.putBlob(ctx, r, p, "", c.Hash, 0, who, nil)
		if err != nil {
			return PushOutcome{}, err
		}
		return PushOutcome{Status: api.PushStatusAccepted, FileID: f.ID}, nil
	}
	row, cur, err := v.refresh(ctx, r, c.Path)
	if err != nil {
		return PushOutcome{}, apiError(c.Path, err)
	}
	if row.Hash == c.Hash {
		return PushOutcome{Status: api.PushStatusAccepted, FileID: row.ID}, nil
	}
	return v.mergeInto(ctx, r, row, cur.data, nil, c.Hash, who)
}

// pushRestore brings a deleted file back with the pushed content: an edit
// beats a delete. v.mu must be held.
func (v *Vault) pushRestore(ctx context.Context, r *os.Root, c PushChange, who Author) (PushOutcome, error) {
	p, err := v.freePath(ctx, r, c.Path, 0)
	if err != nil {
		return PushOutcome{}, err
	}
	if _, err := v.putBlob(ctx, r, p, "", c.Hash, c.FileID, who, nil); err != nil {
		return PushOutcome{}, err
	}
	return PushOutcome{Status: api.PushStatusAccepted, FileID: c.FileID}, nil
}

// pushPut applies a put to a live file: its path, then its content.
// v.mu must be held.
func (v *Vault) pushPut(ctx context.Context, r *os.Root, c PushChange, base store.Revision, row store.File,
	who Author) (PushOutcome, error) {
	out := PushOutcome{Status: api.PushStatusAccepted, FileID: row.ID}
	if c.Path != base.Path && c.Path != row.Path {
		to, err := v.freePath(ctx, r, c.Path, row.ID)
		if err != nil {
			return PushOutcome{}, err
		}
		if to != row.Path {
			if row, err = v.relocate(ctx, r, row.Path, row.Hash, to, who, nil); err != nil {
				return PushOutcome{}, err
			}
		}
	}
	switch {
	case c.Hash == c.BaseHash || c.Hash == row.Hash:
		return out, nil
	case row.Hash == c.BaseHash:
		if _, err := v.putBlob(ctx, r, row.Path, row.Hash, c.Hash, 0, who, nil); err != nil {
			return PushOutcome{}, err
		}
		return out, nil
	}
	_, cur, err := v.refresh(ctx, r, row.Path)
	if err != nil {
		return PushOutcome{}, apiError(row.Path, err)
	}
	baseData, err := v.cfg.Blobs.Get(c.BaseHash)
	if err != nil {
		baseData = nil // pruned (ADR-0019): the two merge as creates
	}
	return v.mergeInto(ctx, r, row, cur.data, baseData, c.Hash, who)
}

// mergeInto merges the pushed blob hash into file row, whose content is
// head (nil when too big to hold), three ways with base, or as two creates
// when base is nil. A file that is not text, or is over 10 MB on either
// side, does not merge: the pushed content wins, and what it replaced is
// kept as a conflict revision, flagging the file. v.mu must be held.
func (v *Vault) mergeInto(ctx context.Context, r *os.Root, row store.File, head, base []byte, hash string,
	who Author) (PushOutcome, error) {
	out := PushOutcome{Status: api.PushStatusMerged, FileID: row.ID, BasePruned: base == nil}
	var incoming []byte
	mergeable := head != nil && row.Size <= bigFile
	if mergeable {
		size, err := v.cfg.Blobs.Size(hash)
		if err != nil {
			return PushOutcome{}, err
		}
		if mergeable = size <= bigFile; mergeable {
			if incoming, err = v.cfg.Blobs.Get(hash); err != nil {
				return PushOutcome{}, err
			}
			mergeable = kindOf(row.Path) == store.FileNote ||
				(utf8.Valid(head) && utf8.Valid(incoming) && (base == nil || utf8.Valid(base)))
		}
	}
	if !mergeable {
		if _, err := v.putBlob(ctx, r, row.Path, row.Hash, hash, 0, who, nil); err != nil {
			return PushOutcome{}, err
		}
		if err := v.flagConflict(ctx, row, row.Hash, row.Size, who); err != nil {
			return PushOutcome{}, err
		}
		out.Status, out.Conflicts, out.BasePruned = api.PushStatusConflicted, 1, false
		return out, nil
	}
	if base != nil {
		res, err := v.write(ctx, r, row.Path, blobs.ID(base), incoming, false, who)
		if err != nil {
			return PushOutcome{}, err
		}
		out.Conflicts = res.Conflicts
		if res.Conflicts > 0 {
			out.Status = api.PushStatusConflicted
		}
		return out, nil
	}
	res := merge.Create(head, incoming)
	if len(res.Conflicts) > 0 {
		if err := v.keepConflict(ctx, row, incoming, who); err != nil {
			return PushOutcome{}, err
		}
		out.Status, out.Conflicts = api.PushStatusConflicted, int64(len(res.Conflicts))
	}
	if !bytes.Equal(res.Text, head) {
		if _, err := v.write(ctx, r, row.Path, row.Hash, res.Text, false, who); err != nil {
			return PushOutcome{}, err
		}
	}
	return out, nil
}

// livePath returns the path of the live file whose path is p ignoring
// case, or "" when there is none.
func (v *Vault) livePath(ctx context.Context, p string) (string, error) {
	paths, err := v.cfg.DB.FilesNamed(ctx, v.id, store.FileName(p))
	if err != nil {
		return "", err
	}
	for _, q := range paths {
		if q == p {
			return q, nil
		}
	}
	for _, q := range paths {
		if strings.EqualFold(q, p) {
			return q, nil
		}
	}
	return "", nil
}

// freePath returns p when no other live file has it (ignoring case) and
// nothing is there on disk, and otherwise the first of "name 1.ext",
// "name 2.ext", … that is free. self is a file that may keep its own path.
func (v *Vault) freePath(ctx context.Context, r *os.Root, p string, self int64) (string, error) {
	dir, base := path.Split(p)
	ext := path.Ext(base)
	stem := strings.TrimSuffix(base, ext)
	for n := 0; n < 10000; n++ {
		q := p
		if n > 0 {
			q = fmt.Sprintf("%s%s %d%s", dir, stem, n, ext)
		}
		taken, err := v.livePath(ctx, q)
		if err != nil {
			return "", err
		}
		if taken != "" {
			if row, err := v.cfg.DB.FileByPath(ctx, v.id, taken); err == nil && row.ID == self {
				return q, nil
			}
			continue
		}
		if _, err := r.Lstat(q); err == nil {
			continue
		}
		return q, nil
	}
	return "", fmt.Errorf("no free path like %s", p)
}
