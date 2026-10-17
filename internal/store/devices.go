package store

import (
	"context"
	"errors"
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
func (h HubDevice) API(connected bool) api.HubDevice { return api.HubDevice{} }

// AddDevice records a paired device with paired_at = t.Now(). A token hash
// another device has is ErrConflict. Task card T-0026.
func (t *Tx) AddDevice(ctx context.Context, name, tokenHash string) (HubDevice, error) {
	return HubDevice{}, errors.New("store.AddDevice is not implemented yet (T-0026)")
}

// DeviceByToken returns the device whose token's SHA-256 is tokenHash, or
// ErrNotFound when none has it (revoked devices have none). Task card
// T-0026.
func (d *DB) DeviceByToken(ctx context.Context, tokenHash string) (HubDevice, error) {
	return HubDevice{}, errors.New("store.DeviceByToken is not implemented yet (T-0026)")
}

// Device returns device id, revoked or not, or ErrNotFound. Task card
// T-0026.
func (d *DB) Device(ctx context.Context, id int64) (HubDevice, error) {
	return HubDevice{}, errors.New("store.Device is not implemented yet (T-0026)")
}

// Devices returns every device, revoked ones included, by ID; a non-nil
// empty slice when none. Task card T-0026.
func (d *DB) Devices(ctx context.Context) ([]HubDevice, error) {
	return nil, errors.New("store.Devices is not implemented yet (T-0026)")
}

// RevokeDevice clears device id's token hash and sets revoked_at =
// t.Now(), once: revoking a revoked device changes nothing. ErrNotFound
// for a device that does not exist. Task card T-0026.
func (t *Tx) RevokeDevice(ctx context.Context, id int64) error {
	return errors.New("store.RevokeDevice is not implemented yet (T-0026)")
}

// TouchDevice sets device id's seen_at to at, unless seen_at is less than
// a minute before at, and reports whether it changed it. Task card T-0026.
func (t *Tx) TouchDevice(ctx context.Context, id int64, at time.Time) (bool, error) {
	return false, errors.New("store.TouchDevice is not implemented yet (T-0026)")
}
