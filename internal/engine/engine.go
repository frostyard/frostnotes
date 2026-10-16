// Package engine implements the notesd API domains over the store
// (docs/design/overview.md).
package engine

import (
	"errors"

	"github.com/frostyard/frostnotes/api"
	"github.com/frostyard/frostnotes/internal/store"
)

// Engine owns the domain services.
type Engine struct{ db *store.DB }

// New returns an Engine over db.
func New(db *store.DB) *Engine { return &Engine{db: db} }

// Vaults implements the vault domain.
func (e *Engine) Vaults() api.VaultService { return vaults{e.db} }

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
