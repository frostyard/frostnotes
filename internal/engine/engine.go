// Package engine implements the notesd API domains over the store and the
// vaults' actors (docs/design/overview.md).
package engine

import (
	"errors"

	"github.com/frostyard/frostnotes/api"
	"github.com/frostyard/frostnotes/internal/hub"
	"github.com/frostyard/frostnotes/internal/store"
	"github.com/frostyard/frostnotes/internal/syncer"
	"github.com/frostyard/frostnotes/internal/vault"
)

// Engine owns the domain services.
type Engine struct {
	db     *store.DB
	vaults *vault.Manager
	hub    *hub.Hub
	sync   *syncer.Syncer
}

// New returns an Engine over db, whose vaults the manager runs; it must
// have started. h serves the hub and log domains and s the sync domain
// (docs/specs/sync-protocol.md); either may be nil, and its domains then
// answer unavailable.
func New(db *store.DB, vaults *vault.Manager, h *hub.Hub, s *syncer.Syncer) *Engine {
	return &Engine{db: db, vaults: vaults, hub: h, sync: s}
}

// Vaults implements the vault domain.
func (e *Engine) Vaults() api.VaultService { return vaults{e.db, e.vaults} }

// Notes implements the note domain.
func (e *Engine) Notes() api.NoteService { return notes{e.db, e.vaults} }

// apiError maps store errors to API errors.
func apiError(err error, what string) error {
	switch {
	case err == nil:
		return nil
	case errors.Is(err, store.ErrNotFound):
		return api.NotFound("%s does not exist", what)
	case errors.Is(err, store.ErrConflict):
		return api.Conflict("%s is already registered", what)
	}
	return err
}
