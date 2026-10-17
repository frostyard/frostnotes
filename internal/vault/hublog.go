package vault

import (
	"context"
	"errors"

	"github.com/frostyard/frostnotes/api"
	"github.com/frostyard/frostnotes/internal/store"
)

// Push says which device's push a change is, for the hub's log
// (docs/specs/sync-protocol.md); it is nil for every other writer.
type Push struct {
	DeviceID  int64
	ReplicaID int64
	PushID    int64
	Base      int64 // the revision the device based its change on
}

// logRevision appends to the hub's log the revision a change to file
// fileID makes, in the change's own transaction, when this vault is served
// (ADR-0017); for a vault that is not served it does nothing. A put of a
// file the log does not have yet is its create, and a move of one is too;
// a delete or a conflict of such a file appends nothing. A conflict
// revision lost to the file's head as it stands.
func (v *Vault) logRevision(ctx context.Context, tx *store.Tx, op api.RevisionOp, fileID int64, path, hash string,
	size int64, who Author) error {
	if _, err := tx.HubVault(ctx, v.id); errors.Is(err, store.ErrNotFound) {
		return nil
	} else if err != nil {
		return err
	}
	f, err := tx.HubFile(ctx, v.id, fileID)
	known := err == nil
	if err != nil && !errors.Is(err, store.ErrNotFound) {
		return err
	}
	if !known {
		switch op {
		case api.RevisionOpPut, api.RevisionOpMove:
			op = api.RevisionOpCreate
		case api.RevisionOpDelete, api.RevisionOpConflict:
			return nil
		}
	}
	source := who.Source
	if source == "" {
		source = api.VersionSourceEditor
	}
	r := store.Revision{VaultID: v.id, FileID: fileID, Op: op, Path: path, Hash: hash, Size: size,
		Source: source, Author: who.Name}
	if known {
		r.Base = f.Revision
		if op == api.RevisionOpConflict {
			r.LostTo = f.Revision
		}
	}
	if p := who.Push; p != nil {
		r.DeviceID, r.ReplicaID, r.PushID, r.Base = p.DeviceID, p.ReplicaID, p.PushID, p.Base
	}
	_, err = tx.AppendRevision(ctx, r)
	return err
}

// Serve makes the vault a hub vault (hub.serve): it gets a sync ID, and
// each of its files, by path, a create revision, all in one transaction
// under the vault's lock, so no change slips between the files listed and
// the log begun.
func (v *Vault) Serve(ctx context.Context) (store.HubVault, error) {
	if err := v.lock(); err != nil {
		return store.HubVault{}, err
	}
	defer v.mu.Unlock()
	files, err := v.cfg.DB.Files(ctx, v.id)
	if err != nil {
		return store.HubVault{}, err
	}
	var served store.HubVault
	err = v.cfg.DB.Tx(ctx, func(tx *store.Tx) error {
		if _, err := tx.ServeVault(ctx, v.id); err != nil {
			return err
		}
		for _, f := range files {
			if _, err := tx.AppendRevision(ctx, store.Revision{VaultID: v.id, FileID: f.ID, Op: api.RevisionOpCreate,
				Path: f.Path, Hash: f.Hash, Size: f.Size, Source: api.VersionSourceOutside}); err != nil {
				return err
			}
		}
		served, err = tx.HubVault(ctx, v.id)
		return err
	})
	return served, err
}

// Resolve clears file fileID's conflict flag with a resolve revision
// (log.resolve). seq is the newest conflict revision the user has seen: a
// newer one is api.Conflict, so a conflict that arrived since is not
// dismissed unseen. A file that is not flagged needs nothing.
func (v *Vault) Resolve(ctx context.Context, fileID, seq int64, who Author) error {
	if err := v.lock(); err != nil {
		return err
	}
	defer v.mu.Unlock()
	hf, err := v.cfg.DB.HubFile(ctx, v.id, fileID)
	if errors.Is(err, store.ErrNotFound) {
		return api.NotFound("file %d is not in the vault's log", fileID)
	} else if err != nil {
		return err
	}
	if !hf.Conflicted {
		return nil
	}
	revs, err := v.cfg.DB.History(ctx, v.id, fileID, 0, 200)
	if err != nil {
		return err
	}
	var newest api.Revision
	for _, r := range revs {
		if r.Op == api.RevisionOpConflict {
			newest = r
			break
		}
	}
	if newest.Seq > seq {
		return api.Conflict("file %d has a conflict revision, %d, newer than %d: look at it first", fileID, newest.Seq, seq)
	}
	head, err := v.cfg.DB.Revision(ctx, v.id, hf.Revision)
	if err != nil {
		return err
	}
	hash := head.Hash
	if hash == "" {
		hash = newest.Hash // a deleted file keeps no content of its own
	}
	return v.cfg.DB.Tx(ctx, func(tx *store.Tx) error {
		r := store.Revision{VaultID: v.id, FileID: fileID, Op: api.RevisionOpResolve, Path: head.Path, Hash: hash,
			Size: head.Size, Base: hf.Revision, Source: who.Source, Author: who.Name}
		if p := who.Push; p != nil {
			r.DeviceID, r.ReplicaID = p.DeviceID, p.ReplicaID
		}
		_, err := tx.AppendRevision(ctx, r)
		return err
	})
}
