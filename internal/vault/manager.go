package vault

import (
	"cmp"
	"context"
	"errors"
	"fmt"
	"maps"
	"slices"
	"sync"
	"time"

	"github.com/frostyard/frostnotes/api"
	"github.com/frostyard/frostnotes/internal/store"
)

// Manager runs one actor per registered vault, and the daily pass that
// prunes versions and blobs.
type Manager struct {
	cfg Config

	// reloadMu serializes Reload, Remove and Prune, so actors start and
	// stop one change at a time and none starts while Prune runs.
	reloadMu sync.Mutex

	mu     sync.Mutex
	ctx    context.Context
	vaults map[int64]*Vault
	wg     sync.WaitGroup
}

// NewManager returns a Manager; Start runs it.
func NewManager(cfg Config) *Manager {
	return &Manager{cfg: cfg.withDefaults(), vaults: map[int64]*Vault{}}
}

// Start opens every registered vault, scanning and watching each, and
// runs the prune pass now and every Config.PruneEvery, until ctx ends.
func (m *Manager) Start(ctx context.Context) error {
	m.mu.Lock()
	m.ctx = ctx
	m.mu.Unlock()
	if err := m.Reload(ctx); err != nil {
		return err
	}
	m.wg.Go(func() {
		for {
			if err := m.Prune(ctx); err != nil && ctx.Err() == nil {
				m.cfg.Log.Warn("prune versions", "err", err)
			}
			select {
			case <-ctx.Done():
				return
			case <-time.After(m.cfg.PruneEvery):
			}
		}
	})
	return nil
}

// Wait blocks until every actor has stopped after Start's context ended.
func (m *Manager) Wait() {
	m.wg.Wait()
	m.mu.Lock()
	vaults := slices.Collect(maps.Values(m.vaults))
	m.mu.Unlock()
	for _, v := range vaults {
		<-v.done
	}
}

// Reload starts an actor for each registered vault without one and stops
// the actors of vaults that are gone: vault.add and vault.remove call it.
func (m *Manager) Reload(ctx context.Context) error {
	m.reloadMu.Lock()
	defer m.reloadMu.Unlock()
	return m.reload(ctx)
}

// Remove forgets vault id: forget deletes its row, and the vault's actor
// first finishes the changes it is making and refuses new ones, so none
// writes a file the index can no longer record.
func (m *Manager) Remove(ctx context.Context, id int64, forget func() error) error {
	m.reloadMu.Lock()
	defer m.reloadMu.Unlock()
	m.mu.Lock()
	v := m.vaults[id]
	m.mu.Unlock()
	if v != nil {
		v.setRemoved(true)
	}
	if err := forget(); err != nil {
		if v != nil {
			v.setRemoved(false)
		}
		return err
	}
	return m.reload(ctx)
}

func (m *Manager) reload(ctx context.Context) error {
	list, err := m.cfg.DB.ListVaults(ctx)
	if err != nil {
		return fmt.Errorf("reload vaults: %w", err)
	}
	var stopping []*Vault
	m.mu.Lock()
	if m.ctx == nil {
		m.mu.Unlock()
		return errors.New("vault: Reload before Start")
	}
	live := map[int64]bool{}
	for _, sv := range list {
		live[sv.ID] = true
		if _, ok := m.vaults[sv.ID]; !ok {
			v := newVault(&m.cfg, sv)
			m.vaults[sv.ID] = v
			v.start(m.ctx)
		}
	}
	for id, v := range m.vaults {
		if !live[id] {
			stopping = append(stopping, v)
			delete(m.vaults, id)
		}
	}
	m.mu.Unlock()
	for _, v := range stopping {
		v.stop()
	}
	return nil
}

// Vault returns vault id's actor, or notFound.
func (m *Manager) Vault(id int64) (*Vault, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if v, ok := m.vaults[id]; ok {
		return v, nil
	}
	return nil, api.NotFound("vault %d does not exist", id)
}

// Prune deletes the versions older than Config.Keep, except each file's
// newest, thins a hub's log by ADR-0019, and removes the blobs that no
// version, file or kept revision names any more and that are a day old,
// with
// every vault's changes held off, and no vault starting, so none can name
// a blob it removes (docs/design/vault.md#versions); requests that only
// read the index go on. Temporary blobs older than an hour go too.
func (m *Manager) Prune(ctx context.Context) error {
	m.reloadMu.Lock()
	defer m.reloadMu.Unlock()
	m.mu.Lock()
	vaults := slices.SortedFunc(maps.Values(m.vaults), func(a, b *Vault) int { return cmp.Compare(a.id, b.id) })
	m.mu.Unlock()
	for _, v := range vaults {
		v.mu.Lock()
		defer v.mu.Unlock()
	}
	now := m.cfg.Now()
	var freed []string
	err := m.cfg.DB.Tx(ctx, func(tx *store.Tx) error {
		var err error
		if freed, err = tx.PruneVersions(ctx, now.Add(-m.cfg.Keep)); err != nil {
			return err
		}
		// A hub thins its log's content by age (ADR-0019).
		revs, err := tx.PruneRevisions(ctx, now, time.Local)
		freed = append(freed, revs...)
		return err
	})
	if err != nil {
		return err
	}
	for _, id := range freed {
		if err := m.cfg.Blobs.Remove(id); err != nil {
			return err
		}
	}
	// Blobs a crash left between storing them and the transaction that
	// would have named them.
	used, err := m.cfg.DB.UsedHashes(ctx)
	if err != nil {
		return err
	}
	ids, err := m.cfg.Blobs.IDs(ctx)
	if err != nil {
		return err
	}
	orphans := 0
	for _, id := range ids {
		if used[id] {
			continue
		}
		// A blob a device uploaded waits a day for the push that names it
		// (docs/specs/sync-protocol.md, Blobs).
		if at, err := m.cfg.Blobs.ModTime(id); err != nil || at.After(now.Add(-24*time.Hour)) {
			continue
		}
		if err := m.cfg.Blobs.Remove(id); err != nil {
			return err
		}
		orphans++
	}
	if _, err := m.cfg.Blobs.RemoveTemp(now.Add(-time.Hour)); err != nil {
		return err
	}
	if len(freed) > 0 || orphans > 0 {
		m.cfg.Log.Info("pruned versions", "blobs", len(freed), "orphans", orphans)
	}
	return nil
}
