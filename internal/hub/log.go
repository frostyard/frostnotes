package hub

import (
	"context"
	"errors"
	"regexp"

	"github.com/frostyard/frostnotes/api"
	"github.com/frostyard/frostnotes/internal/store"
	"github.com/frostyard/frostnotes/internal/vault"
)

// Limits of one log call (docs/specs/sync-protocol.md, Limits and timings).
const (
	maxPushItems  = 100
	maxBatch      = 1000
	maxPullLimit  = 1000
	maxHistoryRev = 200
)

var hashRE = regexp.MustCompile(`^[0-9a-f]{64}$`)

// Log returns the log domain, for paired devices.
func (h *Hub) Log() api.LogService { return logService{h} }

type logService struct{ h *Hub }

var _ api.LogService = logService{}

// served returns the served vault with sync ID syncID.
func (s logService) served(ctx context.Context, syncID string) (store.HubVault, error) {
	v, err := s.h.cfg.DB.HubVaultBySync(ctx, syncID)
	if errors.Is(err, store.ErrNotFound) {
		return store.HubVault{}, api.NotFound("the hub serves no vault %q", syncID)
	}
	return v, err
}

// replica returns replica id of the caller's device in vault v.
func (s logService) replica(ctx context.Context, v store.HubVault, id int64) (store.Replica, store.HubDevice, error) {
	d, ok := deviceFrom(ctx)
	if !ok {
		return store.Replica{}, store.HubDevice{}, errNoDevice
	}
	rep, err := s.h.cfg.DB.Replica(ctx, id)
	if errors.Is(err, store.ErrNotFound) || (err == nil && (rep.DeviceID != d.ID || rep.VaultID != v.VaultID)) {
		return store.Replica{}, store.HubDevice{}, api.NotFound("this device has no replica %d of the vault", id)
	}
	return rep, d, err
}

func (s logService) Vaults(ctx context.Context, _ *api.LogVaultsParams) ([]api.LogVault, error) {
	vs, err := s.h.cfg.DB.HubVaults(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]api.LogVault, 0, len(vs))
	for _, v := range vs {
		out = append(out, v.Log())
	}
	return out, nil
}

func (s logService) Join(ctx context.Context, p *api.LogJoinParams) (*api.Replica, error) {
	v, err := s.served(ctx, p.Vault)
	if err != nil {
		return nil, err
	}
	d, ok := deviceFrom(ctx)
	if !ok {
		return nil, errNoDevice
	}
	var rep store.Replica
	if err := s.h.cfg.DB.Tx(ctx, func(tx *store.Tx) error {
		var err error
		rep, err = tx.AddReplica(ctx, d.ID, v.VaultID)
		return err
	}); err != nil {
		return nil, err
	}
	return &api.Replica{ID: rep.ID, Vault: v.SyncID, Seq: v.Seq}, nil
}

func (s logService) Missing(ctx context.Context, p *api.LogMissingParams) ([]string, error) {
	if len(p.Hashes) > maxBatch {
		return nil, api.InvalidParams("at most %d hashes at once", maxBatch)
	}
	if _, err := s.served(ctx, p.Vault); err != nil {
		return nil, err
	}
	out := []string{}
	for _, h := range p.Hashes {
		if !hashRE.MatchString(h) {
			return nil, api.InvalidParams("%q is not a hash", h)
		}
		ok, err := s.h.cfg.Blobs.Has(h)
		if err != nil {
			return nil, err
		}
		if !ok {
			out = append(out, h)
		}
	}
	return out, nil
}

// pushSource is the version source a device's change is recorded with on
// the hub: what it was on the device, never sync.
func pushSource(src api.VersionSource) api.VersionSource {
	switch src {
	case api.VersionSourceEditor, api.VersionSourceAgent, api.VersionSourceOutside:
		return src
	}
	return api.VersionSourceOutside
}

func (s logService) Push(ctx context.Context, p *api.LogPushParams) ([]api.PushAnswer, error) {
	if len(p.Items) > maxPushItems {
		return nil, api.InvalidParams("at most %d items in one push", maxPushItems)
	}
	hv, err := s.served(ctx, p.Vault)
	if err != nil {
		return nil, err
	}
	rep, d, err := s.replica(ctx, hv, p.Replica)
	if err != nil {
		return nil, err
	}
	v, err := s.h.cfg.Vaults.Vault(hv.VaultID)
	if err != nil {
		return nil, err
	}
	// A repeat gets the answer it got before; the new items apply in an
	// order that frees each path before a move onto it, parking a file of
	// each ring of moves first (vault.PushOrder).
	prior := make([]*store.PushAnswer, len(p.Items))
	var fresh []int
	var changes []vault.PushChange
	for i, it := range p.Items {
		a, seen, err := s.answered(ctx, rep, it.Push)
		if err != nil {
			return nil, err
		}
		if seen {
			prior[i] = &a
			continue
		}
		fresh = append(fresh, i)
		changes = append(changes, changeOf(it))
	}
	order, park, err := v.PushOrder(ctx, changes)
	if err != nil {
		return nil, err
	}
	type parking struct{ from, to string }
	parked := map[int]parking{}
	for _, k := range park {
		from, to, err := v.Park(ctx, changes[k], s.author(d, rep, p.Items[fresh[k]]))
		if err != nil {
			return nil, err
		}
		parked[k] = parking{from, to}
	}
	for _, k := range order {
		i := fresh[k]
		a, err := s.apply(ctx, v, rep, d, p.Items[i], changes[k])
		if err != nil {
			return nil, err
		}
		prior[i] = &a
	}
	for k, pk := range parked {
		if err := v.Unpark(ctx, changes[k].FileID, pk.from, pk.to, s.author(d, rep, p.Items[fresh[k]])); err != nil {
			return nil, err
		}
	}
	out := make([]api.PushAnswer, 0, len(p.Items))
	for i, it := range p.Items {
		a, err := s.answer(ctx, hv, it, *prior[i])
		if err != nil {
			return nil, err
		}
		out = append(out, a)
	}
	if s.h.cfg.Crash != nil && s.h.cfg.Crash("pushed") {
		s.h.disconnect(d.ID)
		return nil, errors.New("hub: the answer was lost, for the simulation")
	}
	return out, nil
}

// answered returns the answer push already got from replica rep, if any.
func (s logService) answered(ctx context.Context, rep store.Replica, push int64) (store.PushAnswer, bool, error) {
	var a store.PushAnswer
	var seen bool
	err := s.h.cfg.DB.Tx(ctx, func(tx *store.Tx) error {
		got, err := tx.PushAnswer(ctx, rep.ID, push)
		if errors.Is(err, store.ErrNotFound) {
			return nil
		}
		a, seen = got, err == nil
		return err
	})
	return a, seen, err
}

func changeOf(it api.PushItem) vault.PushChange {
	c := vault.PushChange{Base: it.Base, BaseHash: it.BaseHash, Path: it.Path}
	if it.File != nil {
		c.FileID = *it.File
	}
	if it.Hash != nil {
		c.Hash = *it.Hash
	}
	if it.Deleted != nil {
		c.Deleted = *it.Deleted
	}
	if it.Conflict != nil {
		c.Conflict = *it.Conflict
	}
	return c
}

func (s logService) author(d store.HubDevice, rep store.Replica, it api.PushItem) vault.Author {
	return vault.Author{Source: pushSource(it.Source), Name: it.Author,
		Push: &vault.Push{DeviceID: d.ID, ReplicaID: rep.ID, PushID: it.Push, Base: it.Base}}
}

// apply applies one new item and records its answer.
func (s logService) apply(ctx context.Context, v *vault.Vault, rep store.Replica, d store.HubDevice, it api.PushItem,
	c vault.PushChange) (store.PushAnswer, error) {
	out, err := v.Push(ctx, c, s.author(d, rep, it))
	if err != nil {
		return store.PushAnswer{}, err
	}
	a := store.PushAnswer{ReplicaID: rep.ID, PushID: it.Push, FileID: out.FileID, Status: out.Status,
		Reason: out.Reason, Conflicts: out.Conflicts, BasePruned: out.BasePruned}
	err = s.h.cfg.DB.Tx(context.WithoutCancel(ctx), func(tx *store.Tx) error { return tx.RecordPush(ctx, a) })
	return a, err
}

// answer is the wire answer to item it, from its recorded answer, with the
// file's state now.
func (s logService) answer(ctx context.Context, hv store.HubVault, it api.PushItem, prior store.PushAnswer) (api.PushAnswer, error) {
	a := api.PushAnswer{Push: it.Push, Status: prior.Status, Conflicts: prior.Conflicts, BasePruned: prior.BasePruned}
	if prior.Reason != "" {
		reason := prior.Reason
		a.Reason = &reason
	}
	if prior.FileID != 0 {
		f, err := s.h.cfg.DB.LogFile(ctx, hv.VaultID, prior.FileID)
		if err != nil {
			return api.PushAnswer{}, err
		}
		a.File = &f
	}
	return a, nil
}

func (s logService) Pull(ctx context.Context, p *api.LogPullParams) (*api.LogPage, error) {
	hv, err := s.served(ctx, p.Vault)
	if err != nil {
		return nil, err
	}
	limit := 0
	if p.Limit != nil {
		if *p.Limit > maxPullLimit {
			return nil, api.InvalidParams("a pull takes at most %d files", maxPullLimit)
		}
		limit = int(*p.Limit)
	}
	page, err := s.h.cfg.DB.Pull(ctx, hv.VaultID, p.After, limit)
	return &page, err
}

func (s logService) Match(ctx context.Context, p *api.LogMatchParams) ([]api.LogMatch, error) {
	if len(p.Files) > maxBatch {
		return nil, api.InvalidParams("at most %d files at once", maxBatch)
	}
	hv, err := s.served(ctx, p.Vault)
	if err != nil {
		return nil, err
	}
	return s.h.cfg.DB.Match(ctx, hv.VaultID, p.Files)
}

func (s logService) History(ctx context.Context, p *api.LogHistoryParams) ([]api.Revision, error) {
	hv, err := s.served(ctx, p.Vault)
	if err != nil {
		return nil, err
	}
	var before int64
	if p.Before != nil {
		before = *p.Before
	}
	limit := 0
	if p.Limit != nil {
		if *p.Limit > maxHistoryRev {
			return nil, api.InvalidParams("history takes at most %d revisions", maxHistoryRev)
		}
		limit = int(*p.Limit)
	}
	revs, err := s.h.cfg.DB.History(ctx, hv.VaultID, p.File, before, limit)
	if errors.Is(err, store.ErrNotFound) {
		return nil, api.NotFound("the vault has no file %d", p.File)
	}
	return revs, err
}

func (s logService) Pushes(ctx context.Context, p *api.LogPushesParams) ([]api.PushRecord, error) {
	if len(p.Pushes) > maxBatch {
		return nil, api.InvalidParams("at most %d pushes at once", maxBatch)
	}
	hv, err := s.served(ctx, p.Vault)
	if err != nil {
		return nil, err
	}
	if _, _, err := s.replica(ctx, hv, p.Replica); err != nil {
		return nil, err
	}
	return s.h.cfg.DB.PushRecords(ctx, hv.VaultID, p.Replica, p.Pushes)
}

func (s logService) Resolve(ctx context.Context, p *api.LogResolveParams) (*api.LogFile, error) {
	hv, err := s.served(ctx, p.Vault)
	if err != nil {
		return nil, err
	}
	d, ok := deviceFrom(ctx)
	if !ok {
		return nil, errNoDevice
	}
	v, err := s.h.cfg.Vaults.Vault(hv.VaultID)
	if err != nil {
		return nil, err
	}
	who := vault.Author{Source: api.VersionSourceEditor, Name: d.Name, Push: &vault.Push{DeviceID: d.ID}}
	if err := v.Resolve(ctx, p.File, p.Seq, who); err != nil {
		return nil, err
	}
	f, err := s.h.cfg.DB.LogFile(ctx, hv.VaultID, p.File)
	return &f, err
}
