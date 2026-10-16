package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/frostyard/frostnotes/api"
)

// Vault is a vaults row.
type Vault struct {
	ID        int64
	Name      string
	Path      string
	CreatedAt time.Time
}

// API converts v to its wire form.
func (v Vault) API() api.Vault {
	return api.Vault{ID: v.ID, Name: v.Name, Path: v.Path, CreatedAt: v.CreatedAt}
}

const vaultColumns = `id, name, path, created_at`

func scanVault(sc interface{ Scan(...any) error }) (Vault, error) {
	var v Vault
	var created string
	if err := sc.Scan(&v.ID, &v.Name, &v.Path, &created); err != nil {
		return Vault{}, err
	}
	t, err := ParseTime(created)
	if err != nil {
		return Vault{}, err
	}
	v.CreatedAt = t
	return v, nil
}

// InsertVault registers a vault with CreatedAt = tx.Now() and emits
// api.VaultAdded. A path already registered returns ErrConflict.
func (t *Tx) InsertVault(ctx context.Context, name, path string) (Vault, error) {
	created := t.Now().UTC().Truncate(time.Millisecond)
	row := t.QueryRowContext(ctx, `INSERT INTO vaults (name, path, created_at) VALUES (?, ?, ?)
		RETURNING `+vaultColumns, name, path, FormatTime(created))
	v, err := scanVault(row)
	if IsUniqueViolation(err) {
		return Vault{}, fmt.Errorf("vault %s: %w", path, ErrConflict)
	}
	if err != nil {
		return Vault{}, fmt.Errorf("insert vault: %w", err)
	}
	if err := t.Emit(ctx, api.VaultAdded{Vault: v.API()}); err != nil {
		return Vault{}, err
	}
	return v, nil
}

// DeleteVault forgets vault id and emits api.VaultRemoved. A missing vault
// returns ErrNotFound.
func (t *Tx) DeleteVault(ctx context.Context, id int64) error {
	res, err := t.ExecContext(ctx, `DELETE FROM vaults WHERE id = ?`, id)
	if err != nil {
		return fmt.Errorf("delete vault %d: %w", id, err)
	}
	if n, err := res.RowsAffected(); err != nil {
		return fmt.Errorf("delete vault %d: %w", id, err)
	} else if n == 0 {
		return fmt.Errorf("vault %d: %w", id, ErrNotFound)
	}
	return t.Emit(ctx, api.VaultRemoved{ID: id})
}

// GetVault returns one vault, or ErrNotFound.
func (d *DB) GetVault(ctx context.Context, id int64) (Vault, error) {
	v, err := scanVault(d.db.QueryRowContext(ctx, `SELECT `+vaultColumns+` FROM vaults WHERE id = ?`, id))
	if errors.Is(err, sql.ErrNoRows) {
		return Vault{}, fmt.Errorf("vault %d: %w", id, ErrNotFound)
	}
	if err != nil {
		return Vault{}, fmt.Errorf("get vault %d: %w", id, err)
	}
	return v, nil
}

// ListVaults returns every vault ordered by name, then ID; none is an empty
// slice.
func (d *DB) ListVaults(ctx context.Context) ([]Vault, error) {
	rows, err := d.db.QueryContext(ctx, `SELECT `+vaultColumns+` FROM vaults ORDER BY name COLLATE NOCASE, id`)
	if err != nil {
		return nil, fmt.Errorf("list vaults: %w", err)
	}
	defer rows.Close()
	out := []Vault{}
	for rows.Next() {
		v, err := scanVault(rows)
		if err != nil {
			return nil, fmt.Errorf("list vaults: %w", err)
		}
		out = append(out, v)
	}
	return out, rows.Err()
}
