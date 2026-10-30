package synctest

import (
	"context"
	"errors"
	"fmt"
	"maps"
	"os"
	"path/filepath"
	"slices"
	"time"

	"github.com/frostyard/frostnotes/api"
)

// The scripted API: scenario tests (plan 0004, Phase 6) drive a world
// step by step, naming paths, and the oracle keeps its books through it.
// A write that gains a token has written it; one that drops a token has
// removed it on purpose. Each call returns the error of the notesd call it
// makes, benign or not: a script knows what it expects.

// Token returns a new token for content a script writes. It counts as
// written once a write that holds it succeeds.
func (w *World) Token() string { return w.oracle.token() }

// books records a script's change from old to new content at p on d.
func (w *World) books(d *Node, p, old, new string) {
	w.oracle.step = -1
	gone := tokenRE.FindAllString(old, -1)
	now := tokenRE.FindAllString(new, -1)
	for _, tok := range now {
		if !slices.Contains(gone, tok) && !w.oracle.inserted[tok] {
			w.oracle.insert(tok)
			w.oracle.wrote(tok, d.Name, p+" (script)")
		}
	}
	for _, tok := range gone {
		if !slices.Contains(now, tok) {
			w.oracle.removed[tok] = true
		}
	}
	w.paths[p] = true
}

// Write makes the note at p on device d hold content, through notesd: a
// write with the base it reads first, or a create when there is no note at
// p.
func (w *World) Write(d *Node, p, content string) error {
	ctx := w.t.Context()
	note, err := d.C.Note().Read(ctx, &api.NoteReadParams{VaultID: d.Vault, Path: p})
	var ae *api.Error
	if errors.As(err, &ae) && ae.Code == api.CodeNotFound {
		if _, err := d.C.Note().Create(ctx, &api.NoteCreateParams{VaultID: d.Vault, Path: p, Content: content}); err != nil {
			return err
		}
		w.books(d, p, "", content)
		return nil
	}
	if err != nil {
		return err
	}
	if _, err := d.C.Note().Write(ctx, &api.NoteWriteParams{VaultID: d.Vault, Path: p, Base: note.Hash, Content: content}); err != nil {
		return err
	}
	w.books(d, p, note.Content, content)
	return nil
}

// WriteFrom writes content to the note at p on device d with an older
// base, the hash of a version it read before: notesd merges it into what
// the note holds now. Only the tokens content gains are written; a merge
// that conflicts keeps the content as a conflict version.
func (w *World) WriteFrom(d *Node, p, base, content string) error {
	ctx := w.t.Context()
	res, err := d.C.Note().Write(ctx, &api.NoteWriteParams{VaultID: d.Vault, Path: p, Base: base, Content: content})
	if err != nil {
		return err
	}
	old, _ := d.Srv.Blobs.Get(base)
	w.books(d, p, string(old), content)
	had := tokenRE.FindAllString(string(old), -1)
	for _, tok := range tokenRE.FindAllString(content, -1) {
		if !slices.Contains(had, tok) {
			w.oracle.conflict(tok, d, res)
		}
	}
	return nil
}

// WriteDisk writes content at p on device d's disk, as another program
// would, and has notesd index it.
func (w *World) WriteDisk(d *Node, p string, content []byte) error {
	old := Files(d)[p]
	if err := writeFile(d, p, string(content), false); err != nil {
		return err
	}
	w.books(d, p, old, string(content))
	return w.index(d, p)
}

// Move moves the file at from to to on device d through notesd, updating
// the links that name it.
func (w *World) Move(d *Node, from, to string) error {
	links := true
	if _, err := d.C.Note().Move(w.t.Context(), &api.NoteMoveParams{VaultID: d.Vault, Path: from, To: to,
		UpdateLinks: &links}); err != nil {
		return err
	}
	w.paths[to] = true
	return nil
}

// MoveDisk renames from to to on device d's disk, as a file manager
// would, and has notesd index both paths.
func (w *World) MoveDisk(d *Node, from, to string) error {
	full := filepath.Join(d.Dir, filepath.FromSlash(to))
	if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
		return err
	}
	if err := os.Rename(filepath.Join(d.Dir, filepath.FromSlash(from)), full); err != nil {
		return err
	}
	w.paths[to] = true
	if err := w.index(d, from); err != nil {
		return err
	}
	return w.index(d, to)
}

// Delete deletes the file at p on device d through notesd, with the base
// it reads; the tokens it held are removed on purpose.
func (w *World) Delete(d *Node, p string) error {
	ctx := w.t.Context()
	note, err := d.C.Note().Read(ctx, &api.NoteReadParams{VaultID: d.Vault, Path: p})
	if err != nil {
		return err
	}
	if err := d.C.Note().Delete(ctx, &api.NoteDeleteParams{VaultID: d.Vault, Path: p, Base: note.Hash}); err != nil {
		return err
	}
	w.books(d, p, note.Content, "")
	return nil
}

// Offline cuts device d's link to the hub.
func (w *World) Offline(d *Node) { d.link.cut() }

// Online heals device d's link and wakes its loop.
func (w *World) Online(d *Node) {
	d.link.heal()
	d.Srv.Syncer.Kick(d.Vault)
}

// Settle heals every link and waits for a quiet point: no device has
// anything to push, and each has pulled all the hub's log holds.
func (w *World) Settle() error { return w.settle() }

// SettleOnline waits for a quiet point of the devices whose links are up,
// leaving cut links cut.
func (w *World) SettleOnline() error { return w.settleAfter(Step{}) }

// Heads returns the hub's files, by path.
func (w *World) Heads() map[string]string { return Files(w.Hub) }

// Files returns node n's vault files on disk, hidden ones aside, by path.
func Files(n *Node) map[string]string { return files(n) }

// HubFile returns the hub's log entry for the live file at p.
func (w *World) HubFile(p string) (api.LogFile, bool) {
	all, err := w.hubFiles(w.t.Context())
	if err != nil {
		return api.LogFile{}, false
	}
	for _, f := range all {
		if f.Path == p && !f.Deleted {
			return f, true
		}
	}
	return api.LogFile{}, false
}

// History returns the hub's revisions of the live file at p, newest
// first.
func (w *World) History(p string) ([]api.Revision, error) {
	f, ok := w.HubFile(p)
	if !ok {
		return nil, fmt.Errorf("the hub has no file at %s", p)
	}
	return w.Hub.Srv.DB.History(w.t.Context(), w.Hub.Vault, f.File, 0, 200)
}

// Status returns device d's sync status.
func (w *World) Status(d *Node) (api.SyncStatus, error) {
	st, err := d.C.Sync().Status(w.t.Context(), &api.SyncStatusParams{VaultID: &d.Vault})
	if err != nil {
		return api.SyncStatus{}, err
	}
	return st[0], nil
}

// Revoke revokes device d on the hub and waits until d's sync says so.
func (w *World) Revoke(d *Node) error {
	ctx := w.t.Context()
	ds, err := w.Hub.C.Hub().Devices(ctx, &api.HubDevicesParams{})
	if err != nil {
		return err
	}
	for _, hd := range ds {
		if hd.Name == d.Name && !hd.Revoked {
			if err := w.Hub.C.Hub().Revoke(ctx, &api.HubRevokeParams{Device: hd.ID}); err != nil {
				return err
			}
		}
	}
	d.link.heal()
	d.Srv.Syncer.Kick(d.Vault)
	return w.waitState(d, api.SyncStateRevoked)
}

// waitState waits, by d's sync.changed events, until d's sync is in state.
func (w *World) waitState(d *Node, state api.SyncState) error {
	ctx, cancel := context.WithTimeout(w.t.Context(), 30*time.Second)
	defer cancel()
	c, _, err := api.Dial(ctx, d.Srv.Socket, "synctest state")
	if err != nil {
		return err
	}
	defer func() { _ = c.Close() }()
	if _, err := c.Events().Subscribe(ctx, &api.EventsSubscribeParams{}); err != nil {
		return err
	}
	if st, err := w.Status(d); err == nil && st.State == state {
		return nil
	}
	for {
		select {
		case ev := <-c.Notifications():
			if e, err := api.DecodeEvent(ev.Event, ev.Data); err == nil {
				if ch, ok := e.(api.SyncChanged); ok && ch.Status.VaultID == d.Vault && ch.Status.State == state {
					return nil
				}
			}
		case <-ctx.Done():
			return fmt.Errorf("%s's sync did not become %s", d.Name, state)
		}
	}
}

// PairAgain pairs device d with the hub again, as its user does after a
// revocation; its vault then syncs again.
func (w *World) PairAgain(d *Node) error {
	ctx := w.t.Context()
	name := d.Name
	p, err := d.C.Sync().Pair(ctx, &api.SyncPairParams{URL: w.Hub.Srv.HubURL, Name: &name})
	if err != nil {
		return err
	}
	if _, err := w.Hub.C.Hub().Approve(ctx, &api.HubApproveParams{Code: p.Code}); err != nil {
		return err
	}
	if r, err := d.C.Sync().Paired(ctx, &api.SyncPairedParams{Pairing: p.Pairing}); err != nil || r.State != api.PairingStateApproved {
		return fmt.Errorf("pair %s again: %+v, %w", d.Name, r, err)
	}
	d.Srv.Syncer.Kick(d.Vault)
	return nil
}

// Reset drops device d's sync state and joins again (sync.reset).
func (w *World) Reset(d *Node) error {
	_, err := d.C.Sync().Reset(w.t.Context(), &api.SyncResetParams{VaultID: d.Vault})
	return err
}

// RestartHub stops the hub's notesd and starts it again on its data.
func (w *World) RestartHub() {
	w.Hub.stop()
	w.Hub.start(w.t)
}

// Snapshot returns a copy of device d's vault files, for AddDevice to
// join with later: an older copy of the vault.
func (w *World) Snapshot(d *Node) map[string]string { return maps.Clone(files(d)) }

// AddDevice starts a device named name whose vault holds vault, pairs it
// with the hub and joins the hub's vault, as a user does with a copy of
// their notes. Tokens in vault the oracle has not seen are written by it.
func (w *World) AddDevice(name string, vault map[string]string) (*Node, error) {
	d := newNode(w.t, name, w.o, false, "")
	for p, c := range vault {
		if err := writeFile(d, p, c, true); err != nil {
			return nil, err
		}
		w.books(d, p, "", c)
	}
	d.start(w.t)
	d.probe(w.t)
	if err := w.pair(d); err != nil {
		return nil, err
	}
	w.Devices = append(w.Devices, d)
	return d, nil
}

// RemoveDevice stops device d and takes it out of the world. Settle first:
// what d has not pushed goes with it.
func (w *World) RemoveDevice(d *Node) {
	d.stop()
	w.Devices = slices.DeleteFunc(w.Devices, func(n *Node) bool { return n == d })
}
