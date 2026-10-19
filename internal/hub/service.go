package hub

import (
	"context"
	"errors"

	"github.com/frostyard/frostnotes/api"
	"github.com/frostyard/frostnotes/internal/store"
)

// Service returns the hub domain, for the hub's own local clients.
func (h *Hub) Service() api.HubService { return hubService{h} }

type hubService struct{ h *Hub }

var _ api.HubService = hubService{}

func (s hubService) Status(ctx context.Context, _ *api.HubStatusParams) (*api.HubStatus, error) {
	id, err := s.h.cfg.DB.HubIdentity(ctx)
	if err != nil {
		return nil, err
	}
	vs, err := s.h.cfg.DB.HubVaults(ctx)
	if err != nil {
		return nil, err
	}
	ds, err := s.h.cfg.DB.Devices(ctx)
	if err != nil {
		return nil, err
	}
	u, err := s.h.cfg.DB.HubURL(ctx)
	if err != nil {
		return nil, err
	}
	st := &api.HubStatus{Hub: id, Vaults: int64(len(vs))}
	if s.h.cfg.Listen != "" {
		listen := s.h.cfg.Listen
		st.Listen = &listen
	}
	if u != "" {
		st.URL = &u
	}
	for _, d := range ds {
		if d.RevokedAt.IsZero() {
			st.Devices++
		}
		if s.h.connected(d.ID) {
			st.Connected++
		}
	}
	return st, nil
}

func (s hubService) Serve(ctx context.Context, p *api.HubServeParams) (*api.HubVault, error) {
	v, err := s.h.cfg.Vaults.Vault(p.VaultID)
	if err != nil {
		return nil, err
	}
	hv, err := v.Serve(ctx)
	if errors.Is(err, store.ErrConflict) {
		return nil, api.Conflict("vault %d is served already", p.VaultID)
	}
	if err != nil {
		return nil, err
	}
	out := hv.API()
	return &out, nil
}

func (s hubService) Vaults(ctx context.Context, _ *api.HubVaultsParams) ([]api.HubVault, error) {
	vs, err := s.h.cfg.DB.HubVaults(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]api.HubVault, 0, len(vs))
	for _, v := range vs {
		out = append(out, v.API())
	}
	return out, nil
}

func (s hubService) Pending(context.Context, *api.HubPendingParams) ([]api.PendingPairing, error) {
	return s.h.pending(), nil
}

func (s hubService) Approve(_ context.Context, p *api.HubApproveParams) (*api.PendingPairing, error) {
	pp, err := s.h.approve(p.Code)
	if err != nil {
		return nil, err
	}
	return &pp, nil
}

func (s hubService) Devices(ctx context.Context, _ *api.HubDevicesParams) ([]api.HubDevice, error) {
	ds, err := s.h.cfg.DB.Devices(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]api.HubDevice, 0, len(ds))
	for _, d := range ds {
		out = append(out, d.API(s.h.connected(d.ID)))
	}
	return out, nil
}

// SetURL records the URL devices should use (hub.setURL); they move to it
// at their next connection (log.hub).
func (s hubService) SetURL(ctx context.Context, p *api.HubSetURLParams) (*api.HubStatus, error) {
	u := ""
	if p.URL != "" {
		parsed, err := CheckURL(p.URL)
		if err != nil {
			return nil, err
		}
		u = parsed.String()
	}
	if err := s.h.cfg.DB.Tx(ctx, func(tx *store.Tx) error { return tx.SetHubURL(ctx, u) }); err != nil {
		return nil, err
	}
	s.h.cfg.Log.Info("the hub's URL for devices", "url", u)
	return s.Status(ctx, &api.HubStatusParams{})
}

func (s hubService) Revoke(ctx context.Context, p *api.HubRevokeParams) error {
	err := s.h.cfg.DB.Tx(ctx, func(tx *store.Tx) error { return tx.RevokeDevice(ctx, p.Device) })
	if errors.Is(err, store.ErrNotFound) {
		return api.NotFound("device %d does not exist", p.Device)
	}
	if err != nil {
		return err
	}
	s.h.disconnect(p.Device)
	s.h.cfg.Log.Info("device revoked", "device", p.Device)
	return nil
}
