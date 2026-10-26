package store

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"github.com/frostyard/frostnotes/api"
)

// HubDevice is a hub_devices row: a paired device (ADR-0016).
type HubDevice struct {
	ID        int64
	Name      string
	TokenHash string // "" once revoked
	PairedAt  time.Time
	SeenAt    time.Time // zero until its first request
	RevokedAt time.Time // zero unless revoked
}

// API converts d to its wire form; connected says whether it is connected
// now, which only the hub's listener knows. Task card T-0026.
func (h HubDevice) API(connected bool) api.HubDevice {
	out := api.HubDevice{ID: h.ID, Name: h.Name, PairedAt: h.PairedAt,
		Connected: connected, Revoked: !h.RevokedAt.IsZero()}
	if !h.SeenAt.IsZero() {
		out.SeenAt = &h.SeenAt
	}
	return out
}

// AddDevice records a paired device with paired_at = t.Now(). A token hash
// another device has is ErrConflict. Task card T-0026.
func (t *Tx) AddDevice(ctx context.Context, name, tokenHash string) (HubDevice, error) {
	h, err := scanHubDevice(t.QueryRowContext(ctx, `INSERT INTO hub_devices (name, token_hash, paired_at)
		VALUES (?, ?, ?) RETURNING `+deviceColumns, name, tokenHash, FormatTime(t.Now())))
	if IsUniqueViolation(err) {
		return HubDevice{}, fmt.Errorf("add device: %w", ErrConflict)
	}
	return h, err
}

// DeviceByToken returns the device whose token's SHA-256 is tokenHash, or
// ErrNotFound when none has it (revoked devices have none). Task card
// T-0026.
func (d *DB) DeviceByToken(ctx context.Context, tokenHash string) (HubDevice, error) {
	return scanHubDevice(d.db.QueryRowContext(ctx,
		`SELECT `+deviceColumns+` FROM hub_devices WHERE token_hash = ?`, tokenHash))
}

// Device returns device id, revoked or not, or ErrNotFound. Task card
// T-0026.
func (d *DB) Device(ctx context.Context, id int64) (HubDevice, error) {
	return scanHubDevice(d.db.QueryRowContext(ctx,
		`SELECT `+deviceColumns+` FROM hub_devices WHERE id = ?`, id))
}

// Devices returns every device, revoked ones included, by ID; a non-nil
// empty slice when none. Task card T-0026.
func (d *DB) Devices(ctx context.Context) ([]HubDevice, error) {
	rows, err := d.db.QueryContext(ctx, `SELECT `+deviceColumns+` FROM hub_devices ORDER BY id`)
	if err != nil {
		return nil, fmt.Errorf("list devices: %w", err)
	}
	defer rows.Close()
	out := []HubDevice{}
	for rows.Next() {
		h, err := scanHubDevice(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, h)
	}
	return out, hubReadError("list devices", rows.Err())
}

// RevokeDevice clears device id's token hash and sets revoked_at =
// t.Now(), once: revoking a revoked device changes nothing. ErrNotFound
// for a device that does not exist. Task card T-0026.
func (t *Tx) RevokeDevice(ctx context.Context, id int64) error {
	var revoked sql.NullString
	err := t.QueryRowContext(ctx, `SELECT revoked_at FROM hub_devices WHERE id = ?`, id).Scan(&revoked)
	if err != nil {
		return hubReadError("revoke device", err)
	}
	if revoked.Valid {
		return nil
	}
	_, err = t.ExecContext(ctx, `UPDATE hub_devices SET token_hash = NULL, revoked_at = ? WHERE id = ?`,
		FormatTime(t.Now()), id)
	return hubReadError("revoke device", err)
}

// DeleteUnseenDevice deletes device id when no request has touched it and
// it is not revoked, and reports whether it deleted it: a pairing the
// device could not keep its token for. A device that made a request stays,
// since the hub's revisions may name it.
func (t *Tx) DeleteUnseenDevice(ctx context.Context, id int64) (bool, error) {
	res, err := t.ExecContext(ctx, `DELETE FROM hub_devices WHERE id = ? AND seen_at IS NULL AND revoked_at IS NULL`, id)
	if err != nil {
		return false, fmt.Errorf("delete an unseen device: %w", err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return false, fmt.Errorf("delete an unseen device: %w", err)
	}
	return n > 0, nil
}

// TouchDevice sets device id's seen_at to at, unless seen_at is less than
// a minute before at, and reports whether it changed it. Task card T-0026.
func (t *Tx) TouchDevice(ctx context.Context, id int64, at time.Time) (bool, error) {
	res, err := t.ExecContext(ctx, `UPDATE hub_devices SET seen_at = ?
		WHERE id = ? AND (seen_at IS NULL OR seen_at <= ?)`, FormatTime(at), id, FormatTime(at.Add(-time.Minute)))
	if err != nil {
		return false, fmt.Errorf("touch device: %w", err)
	}
	n, err := res.RowsAffected()
	return n > 0, hubReadError("touch device", err)
}

const deviceColumns = `id, name, COALESCE(token_hash, ''), paired_at, seen_at, revoked_at`

func scanHubDevice(sc interface{ Scan(...any) error }) (HubDevice, error) {
	var h HubDevice
	var paired, seen, revoked sql.NullString
	if err := sc.Scan(&h.ID, &h.Name, &h.TokenHash, &paired, &seen, &revoked); err != nil {
		return HubDevice{}, hubReadError("read device", err)
	}
	for _, field := range []struct {
		stored sql.NullString
		target *time.Time
	}{{paired, &h.PairedAt}, {seen, &h.SeenAt}, {revoked, &h.RevokedAt}} {
		if field.stored.Valid {
			at, err := ParseTime(field.stored.String)
			if err != nil {
				return HubDevice{}, fmt.Errorf("read device: %w", err)
			}
			*field.target = at
		}
	}
	return h, nil
}
