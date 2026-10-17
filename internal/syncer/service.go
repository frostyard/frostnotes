package syncer

import (
	"context"
	"errors"
	"slices"

	"github.com/frostyard/frostnotes/api"
	"github.com/frostyard/frostnotes/internal/store"
)

// Service returns the sync domain, for the device's local clients.
func (s *Syncer) Service() api.SyncService { return service{s} }

type service struct{ s *Syncer }

var _ api.SyncService = service{}

func (sv service) Pair(ctx context.Context, p *api.SyncPairParams) (*api.Pairing, error) {
	name := ""
	if p.Name != nil {
		name = *p.Name
	}
	return sv.s.pair(ctx, p.URL, name)
}

func (sv service) Paired(ctx context.Context, p *api.SyncPairedParams) (*api.PairResult, error) {
	return sv.s.paired(ctx, p.Pairing)
}

func (sv service) Hubs(ctx context.Context, _ *api.SyncHubsParams) ([]api.SyncHub, error) {
	hs, err := sv.s.cfg.DB.SyncHubs(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]api.SyncHub, 0, len(hs))
	for _, h := range hs {
		out = append(out, h.API())
	}
	return out, nil
}

// remote returns vault's remote, or notFound when it does not sync.
func (sv service) remote(ctx context.Context, vaultID int64) (store.Remote, error) {
	r, err := sv.s.cfg.DB.Remote(ctx, vaultID)
	if errors.Is(err, store.ErrNotFound) {
		return store.Remote{}, api.NotFound("vault %d does not sync with a hub", vaultID)
	}
	return r, err
}

// connect dials the hub vault's remote syncs with, for a one-off call.
func (sv service) connect(ctx context.Context, vaultID int64) (store.Remote, *session, error) {
	r, err := sv.remote(ctx, vaultID)
	if err != nil {
		return store.Remote{}, nil, err
	}
	sess, err := sv.s.dial(ctx, r.Hub)
	if err != nil {
		return store.Remote{}, nil, api.Unavailable("reach the hub: %v", err)
	}
	return r, sess, nil
}

func (sv service) Join(ctx context.Context, p *api.SyncJoinParams) (*api.SyncStatus, error) {
	u, err := checkURL(p.URL)
	if err != nil {
		return nil, err
	}
	if _, err := sv.s.cfg.Vaults.Vault(p.VaultID); err != nil {
		return nil, err
	}
	if _, err := sv.s.cfg.DB.Remote(ctx, p.VaultID); err == nil {
		return nil, api.Conflict("vault %d syncs with a hub already: sync.leave it first", p.VaultID)
	}
	h, err := sv.s.cfg.DB.SyncHubByURL(ctx, u.String())
	if errors.Is(err, store.ErrNotFound) {
		return nil, api.NotFound("this device has not paired with %s: pair first (notesctl hub join pairs)", u)
	} else if err != nil {
		return nil, err
	}
	if err := sv.s.join(ctx, p.VaultID, h, p.Name); err != nil {
		return nil, err
	}
	return sv.statusOf(p.VaultID), nil
}

// join records the remote and the agreed state of every local file the
// hub has had at its path, then starts the vault's loop.
func (s *Syncer) join(ctx context.Context, vaultID int64, h store.SyncHub, name string) error {
	sess, err := s.dial(ctx, h.ID)
	if err != nil {
		return api.Unavailable("reach the hub: %v", err)
	}
	defer func() { _ = sess.client.Close() }()
	vaults, err := sess.client.Log().Vaults(ctx, &api.LogVaultsParams{})
	if err != nil {
		return err
	}
	i := slices.IndexFunc(vaults, func(v api.LogVault) bool { return v.Name == name })
	if i < 0 {
		return api.NotFound("the hub serves no vault named %q", name)
	}
	rep, err := sess.client.Log().Join(ctx, &api.LogJoinParams{Vault: vaults[i].Sync})
	if err != nil {
		return err
	}
	files, err := s.cfg.DB.Files(ctx, vaultID)
	if err != nil {
		return err
	}
	var agreed []store.Agreed
	for batch := range slices.Chunk(files, queryBatch) {
		qs := make([]api.MatchQuery, 0, len(batch))
		for _, f := range batch {
			qs = append(qs, api.MatchQuery{Path: f.Path, Hash: f.Hash})
		}
		ms, err := sess.client.Log().Match(ctx, &api.LogMatchParams{Vault: rep.Vault, Files: qs})
		if err != nil {
			return err
		}
		for j, m := range ms {
			if m.File != nil && m.Revision != nil {
				f := batch[j]
				hash := f.Hash
				if m.RevisionHash != nil {
					hash = *m.RevisionHash // the copy differs from it: a put to push
				}
				agreed = append(agreed, store.Agreed{VaultID: vaultID, HubFile: *m.File, FileID: f.ID, Seq: *m.Revision,
					Revision: *m.Revision, Path: f.Path, Hash: hash})
			}
		}
	}
	err = s.cfg.DB.Tx(ctx, func(tx *store.Tx) error {
		if _, err := tx.AddRemote(ctx, store.Remote{VaultID: vaultID, Hub: h.ID, SyncID: rep.Vault, Name: name,
			ReplicaID: rep.ID, HubSeq: rep.Seq}); err != nil {
			return err
		}
		for _, a := range agreed {
			// Two local files matching one hub file keep the first.
			if err := tx.PutAgreed(ctx, a); err != nil && !errors.Is(err, store.ErrConflict) {
				return err
			}
		}
		return nil
	})
	if err != nil {
		return err
	}
	s.startLoop(vaultID)
	return nil
}

func (sv service) Leave(ctx context.Context, p *api.SyncLeaveParams) error {
	if _, err := sv.remote(ctx, p.VaultID); err != nil {
		return err
	}
	sv.s.stopLoop(p.VaultID)
	return sv.s.cfg.DB.Tx(ctx, func(tx *store.Tx) error { return tx.DeleteRemote(ctx, p.VaultID) })
}

func (sv service) statusOf(vaultID int64) *api.SyncStatus {
	if l := sv.s.loop(vaultID); l != nil {
		st := l.snapshot()
		return &st
	}
	return &api.SyncStatus{VaultID: vaultID, State: api.SyncStateOffline, Refused: []api.SyncRefusal{}, Skipped: []string{}}
}

func (sv service) Status(ctx context.Context, p *api.SyncStatusParams) ([]api.SyncStatus, error) {
	remotes, err := sv.s.cfg.DB.Remotes(ctx)
	if err != nil {
		return nil, err
	}
	out := []api.SyncStatus{}
	for _, r := range remotes {
		if p.VaultID != nil && r.VaultID != *p.VaultID {
			continue
		}
		out = append(out, *sv.statusOf(r.VaultID))
	}
	if p.VaultID != nil && len(out) == 0 {
		return nil, api.NotFound("vault %d does not sync with a hub", *p.VaultID)
	}
	return out, nil
}

// hubFileAt returns the hub file the local file at path is agreed with.
func (sv service) hubFileAt(ctx context.Context, vaultID int64, path string) (store.Agreed, error) {
	rows, err := sv.s.cfg.DB.AgreedRows(ctx, vaultID)
	if err != nil {
		return store.Agreed{}, err
	}
	if f, err := sv.s.cfg.DB.FileByPath(ctx, vaultID, path); err == nil {
		for _, a := range rows {
			if a.FileID == f.ID {
				return a, nil
			}
		}
	}
	for _, a := range rows {
		if a.Path == path {
			return a, nil
		}
	}
	return store.Agreed{}, api.NotFound("%s is not a file the hub knows", path)
}

// conflictsOf returns the conflict revisions after the last resolve.
func conflictsOf(revs []api.Revision) []api.Revision {
	out := []api.Revision{}
	for _, r := range revs { // newest first
		if r.Op == api.RevisionOpResolve {
			break
		}
		if r.Op == api.RevisionOpConflict {
			out = append(out, r)
		}
	}
	return out
}

func (sv service) Conflicts(ctx context.Context, p *api.SyncConflictsParams) ([]api.SyncConflict, error) {
	r, sess, err := sv.connect(ctx, p.VaultID)
	if err != nil {
		return nil, err
	}
	defer func() { _ = sess.client.Close() }()
	rows, err := sv.s.cfg.DB.AgreedRows(ctx, p.VaultID)
	if err != nil {
		return nil, err
	}
	limit := int64(maxHistory)
	out := []api.SyncConflict{}
	for _, a := range rows {
		if !a.Conflicted {
			continue
		}
		revs, err := sess.client.Log().History(ctx, &api.LogHistoryParams{Vault: r.SyncID, File: a.HubFile, Limit: &limit})
		if err != nil {
			return nil, err
		}
		out = append(out, api.SyncConflict{Path: a.Path, File: a.HubFile, Revisions: conflictsOf(revs)})
	}
	return out, nil
}

const maxHistory = 200

func (sv service) Resolve(ctx context.Context, p *api.SyncResolveParams) error {
	r, sess, err := sv.connect(ctx, p.VaultID)
	if err != nil {
		return err
	}
	defer func() { _ = sess.client.Close() }()
	a, err := sv.hubFileAt(ctx, p.VaultID, p.Path)
	if err != nil {
		return err
	}
	limit := int64(maxHistory)
	revs, err := sess.client.Log().History(ctx, &api.LogHistoryParams{Vault: r.SyncID, File: a.HubFile, Limit: &limit})
	if err != nil {
		return err
	}
	var seq int64
	if cs := conflictsOf(revs); len(cs) > 0 {
		seq = cs[0].Seq
	}
	f, err := sess.client.Log().Resolve(ctx, &api.LogResolveParams{Vault: r.SyncID, File: a.HubFile, Seq: seq})
	if err != nil {
		return err
	}
	if err := sv.s.cfg.DB.Tx(ctx, func(tx *store.Tx) error {
		if a.Revision == f.Revision {
			a.Seq, a.Conflicted = f.Seq, f.Conflicted
			return tx.PutAgreed(ctx, a)
		}
		return nil
	}); err != nil {
		return err
	}
	sv.s.Kick(p.VaultID)
	return nil
}

func (sv service) Reset(ctx context.Context, p *api.SyncResetParams) (*api.SyncStatus, error) {
	r, err := sv.remote(ctx, p.VaultID)
	if err != nil {
		return nil, err
	}
	h, err := sv.s.cfg.DB.SyncHub(ctx, r.Hub)
	if err != nil {
		return nil, err
	}
	sv.s.stopLoop(p.VaultID)
	if err := sv.s.cfg.DB.Tx(ctx, func(tx *store.Tx) error { return tx.DeleteRemote(ctx, p.VaultID) }); err != nil {
		return nil, err
	}
	if err := sv.s.join(ctx, p.VaultID, h, r.Name); err != nil {
		return nil, err
	}
	return sv.statusOf(p.VaultID), nil
}

func (sv service) Verify(ctx context.Context, p *api.SyncVerifyParams) (*api.VerifyReport, error) {
	r, sess, err := sv.connect(ctx, p.VaultID)
	if err != nil {
		return nil, err
	}
	defer func() { _ = sess.client.Close() }()
	return sv.s.verify(ctx, p.VaultID, r, sess)
}

func (sv service) History(ctx context.Context, p *api.SyncHistoryParams) ([]api.Revision, error) {
	r, sess, err := sv.connect(ctx, p.VaultID)
	if err != nil {
		return nil, err
	}
	defer func() { _ = sess.client.Close() }()
	a, err := sv.hubFileAt(ctx, p.VaultID, p.Path)
	if err != nil {
		return nil, err
	}
	return sess.client.Log().History(ctx, &api.LogHistoryParams{Vault: r.SyncID, File: a.HubFile, Limit: p.Limit})
}

// verify compares the vault with its hub's heads, and its answered pushes
// with the hub's records (M2's exit evidence).
func (s *Syncer) verify(ctx context.Context, vaultID int64, r store.Remote, sess *session) (*api.VerifyReport, error) {
	heads := map[string]api.LogFile{}
	var after int64
	limit := int64(1000)
	for {
		page, err := sess.client.Log().Pull(ctx, &api.LogPullParams{Vault: r.SyncID, After: after, Limit: &limit})
		if err != nil {
			return nil, err
		}
		for _, f := range page.Files {
			if !f.Deleted {
				heads[f.Path] = f
			}
		}
		after = page.Next
		if !page.More {
			break
		}
	}
	files, err := s.cfg.DB.Files(ctx, vaultID)
	if err != nil {
		return nil, err
	}
	rep := &api.VerifyReport{Differing: []string{}, Unrecorded: []int64{}, Conflicts: []string{}}
	local := map[string]bool{}
	for _, f := range files {
		local[f.Path] = true
		rep.Files++
		if h, ok := heads[f.Path]; !ok || h.Hash != f.Hash {
			rep.Differing = append(rep.Differing, f.Path)
		} else if h.Conflicted {
			rep.Conflicts = append(rep.Conflicts, f.Path)
		}
	}
	for p := range heads {
		if !local[p] && vaultPathHeld(s, vaultID, p) {
			rep.Differing = append(rep.Differing, p)
		}
	}
	slices.Sort(rep.Differing)
	slices.Sort(rep.Conflicts)
	pushes, err := s.cfg.DB.Pushes(ctx, vaultID)
	if err != nil {
		return nil, err
	}
	var ids []int64
	byID := map[int64]store.LocalPush{}
	for _, p := range pushes {
		if p.Status != api.PushStatusRefused {
			ids = append(ids, p.PushID)
			byID[p.PushID] = p
		}
	}
	for batch := range slices.Chunk(ids, queryBatch) {
		recs, err := sess.client.Log().Pushes(ctx, &api.LogPushesParams{Vault: r.SyncID, Replica: r.ReplicaID, Pushes: batch})
		if err != nil {
			return nil, err
		}
		for _, rec := range recs {
			ok, err := s.recorded(ctx, sess, r, byID[rec.Push], rec)
			if err != nil {
				return nil, err
			}
			if !ok {
				rep.Unrecorded = append(rep.Unrecorded, rec.Push)
			}
		}
	}
	rep.Ok = len(rep.Differing) == 0 && len(rep.Unrecorded) == 0
	return rep, nil
}

// vaultPathHeld reports whether the device would hold a head at p: not one
// its vault ignores.
func vaultPathHeld(s *Syncer, vaultID int64, p string) bool {
	v, err := s.cfg.Vaults.Vault(vaultID)
	return err == nil && !v.Ignores(p)
}

// recorded reports whether the hub holds what push p sent: a revision it
// made with the pushed content (or deletion), or, for a push that changed
// nothing, a revision of the file with that content.
func (s *Syncer) recorded(ctx context.Context, sess *session, r store.Remote, p store.LocalPush, rec api.PushRecord) (bool, error) {
	if len(rec.Revisions) > 0 {
		return true, nil // the push made revisions: its content, merged, or kept as a conflict
	}
	file := p.HubFile
	if file == 0 {
		a, err := s.cfg.DB.AgreedByFile(ctx, p.FileID)
		if err != nil {
			return false, nil
		}
		file = a.HubFile
	}
	if p.Hash == "" {
		return true, nil // a delete of a file deleted already changes nothing
	}
	limit := int64(maxHistory)
	revs, err := sess.client.Log().History(ctx, &api.LogHistoryParams{Vault: r.SyncID, File: file, Limit: &limit})
	if err != nil {
		var ae *api.Error
		if errors.As(err, &ae) && ae.Code == api.CodeNotFound {
			return false, nil
		}
		return false, err
	}
	return slices.ContainsFunc(revs, func(rv api.Revision) bool { return rv.Hash == p.Hash }), nil
}

// ReadRevision answers unavailable until M3's Phase 4 builds it
// (docs/plans/0005-m3-desktop-app.md).
func (sv service) ReadRevision(context.Context, *api.SyncReadRevisionParams) (*api.RevisionContent, error) {
	return nil, api.Unavailable("sync.readRevision is not built yet")
}
