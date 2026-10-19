package engine

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"github.com/frostyard/frostnotes/api"
	"github.com/frostyard/frostnotes/internal/store"
	"github.com/frostyard/frostnotes/internal/vault"
)

type vaults struct {
	db *store.DB
	m  *vault.Manager
}

func (v vaults) List(ctx context.Context, _ *api.VaultListParams) ([]api.Vault, error) {
	list, err := v.db.ListVaults(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]api.Vault, 0, len(list))
	for _, vault := range list {
		out = append(out, vault.API())
	}
	return out, nil
}

// Add registers a directory. The path is resolved through symlinks, so one
// directory cannot be registered twice under two names, and a vault may
// neither contain nor lie inside another: their files would be indexed and
// synced twice. With create it makes the directory first, once it knows
// the vault may be registered.
func (v vaults) Add(ctx context.Context, p *api.VaultAddParams) (*api.Vault, error) {
	if !filepath.IsAbs(p.Path) {
		return nil, api.InvalidParams("vault path %q is not absolute", p.Path)
	}
	create := p.Create != nil && *p.Create
	path, err := vaultPath(p.Path, create)
	if err != nil {
		return nil, err
	}
	name := filepath.Base(path)
	if p.Name != nil {
		name = strings.TrimSpace(*p.Name)
	}
	if name == "" {
		return nil, api.InvalidParams("vault name is empty")
	}
	var out store.Vault
	err = v.db.Tx(ctx, func(tx *store.Tx) error {
		existing, err := v.db.ListVaults(ctx)
		if err != nil {
			return err
		}
		for _, e := range existing {
			if within(path, e.Path) || within(e.Path, path) {
				return api.Conflict("vault %q overlaps vault %q at %s", path, e.Name, e.Path)
			}
		}
		if create {
			if err := os.Mkdir(path, 0o755); errors.Is(err, fs.ErrExist) {
				return api.Conflict("%s already exists", path)
			} else if err != nil {
				return api.InvalidParams("make the vault's folder: %v", err)
			}
		}
		out, err = tx.InsertVault(ctx, name, path)
		if err != nil && create {
			_ = os.Remove(path) // the folder notesd just made, still empty
		}
		return err
	})
	if err != nil {
		return nil, apiError(err, "vault "+path)
	}
	if err := v.m.Reload(ctx); err != nil { // scan and watch it
		return nil, err
	}
	r := out.API()
	return &r, nil
}

func (v vaults) Remove(ctx context.Context, p *api.VaultRemoveParams) error {
	err := v.m.Remove(ctx, p.ID, func() error {
		return v.db.Tx(ctx, func(tx *store.Tx) error { return tx.DeleteVault(ctx, p.ID) })
	})
	return apiError(err, fmt.Sprintf("vault %d", p.ID))
}

func (v vaults) Status(ctx context.Context, p *api.VaultStatusParams) (*api.ScanStatus, error) {
	actor, err := v.m.Vault(p.ID)
	if err != nil {
		return nil, err
	}
	s, err := actor.Status(ctx)
	return &s, err
}

func (v vaults) Reindex(ctx context.Context, p *api.VaultReindexParams) (*api.ScanStatus, error) {
	actor, err := v.m.Vault(p.ID)
	if err != nil {
		return nil, err
	}
	s, err := actor.Reindex(ctx)
	return &s, err
}

// vaultPath resolves a vault's path through symlinks: the directory's
// own, or for one to create, its parent's, which must be a directory,
// while the directory itself must not exist yet.
func vaultPath(p string, create bool) (string, error) {
	if !create {
		path, err := filepath.EvalSymlinks(p)
		if err != nil {
			return "", api.InvalidParams("vault path %q: %v", p, err)
		}
		if fi, err := os.Stat(path); err != nil || !fi.IsDir() {
			return "", api.InvalidParams("vault path %q is not a directory", p)
		}
		return path, nil
	}
	clean := filepath.Clean(p)
	base := filepath.Base(clean)
	if base == string(filepath.Separator) || base == "." {
		return "", api.InvalidParams("vault path %q names no folder to make", p)
	}
	parent, err := filepath.EvalSymlinks(filepath.Dir(clean))
	if err != nil {
		return "", api.InvalidParams("the folder to hold the vault: %v", err)
	}
	if fi, err := os.Stat(parent); err != nil || !fi.IsDir() {
		return "", api.InvalidParams("%s is not a folder", filepath.Dir(clean))
	}
	path := filepath.Join(parent, base)
	if _, err := os.Lstat(path); err == nil {
		return "", api.Conflict("%s already exists", path)
	}
	return path, nil
}

// within reports whether path is dir or lies inside it.
func within(path, dir string) bool {
	rel, err := filepath.Rel(dir, path)
	return err == nil && rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator))
}

func (v vaults) Settings(_ context.Context, p *api.VaultSettingsParams) (*api.VaultSettings, error) {
	actor, err := v.m.Vault(p.ID)
	if err != nil {
		return nil, err
	}
	s, err := vault.LoadSettings(actor.Root())
	if err != nil {
		return nil, fmt.Errorf("vault %d's settings: %w", p.ID, err)
	}
	out := s.API()
	return &out, nil
}

func (v vaults) Configure(ctx context.Context, p *api.VaultConfigureParams) (*api.VaultSettings, error) {
	actor, err := v.m.Vault(p.ID)
	if err != nil {
		return nil, err
	}
	s, err := actor.Configure(ctx, p)
	if err != nil {
		return nil, err
	}
	out := s.API()
	return &out, nil
}
