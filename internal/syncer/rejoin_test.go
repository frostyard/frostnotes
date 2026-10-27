package syncer_test

import (
	"errors"
	"net"
	"testing"

	"github.com/frostyard/frostnotes/api"
	"github.com/frostyard/frostnotes/internal/secrets"
)

// pairOnly pairs d with the hub h, without joining a vault, and returns
// the hub's ID.
func pairOnly(t *testing.T, h, d *node, name string) string {
	t.Helper()
	ctx := t.Context()
	p, err := d.c.Sync().Pair(ctx, &api.SyncPairParams{URL: h.srv.HubURL, Name: &name})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := h.c.Hub().Approve(ctx, &api.HubApproveParams{Code: p.Code}); err != nil {
		t.Fatal(err)
	}
	if r, err := d.c.Sync().Paired(ctx, &api.SyncPairedParams{Pairing: p.Pairing}); err != nil || r.State != api.PairingStateApproved {
		t.Fatalf("Paired = %+v, %v", r, err)
	}
	return p.Hub
}

// joinHubVault joins d's vault to the hub's only vault.
func joinHubVault(t *testing.T, h, d *node) (*api.SyncStatus, error) {
	t.Helper()
	vs, err := h.c.Hub().Vaults(t.Context(), &api.HubVaultsParams{})
	if err != nil {
		t.Fatal(err)
	}
	return d.c.Sync().Join(t.Context(), &api.SyncJoinParams{VaultID: d.vault, URL: h.srv.HubURL, Name: vs[0].Name})
}

func TestJoinWithARevokedTokenIsRevoked(t *testing.T) {
	h := newHub(t, map[string]string{"n.md": fiveLines})
	d := start(t, deviceOptions("d"), vaultDir(t, nil))
	pairOnly(t, h, d, "d")
	ds, err := h.c.Hub().Devices(t.Context(), &api.HubDevicesParams{})
	if err != nil || len(ds) != 1 {
		t.Fatalf("Devices = %v, %v", ds, err)
	}
	if err := h.c.Hub().Revoke(t.Context(), &api.HubRevokeParams{Device: ds[0].ID}); err != nil {
		t.Fatal(err)
	}
	if _, err := joinHubVault(t, h, d); !errors.Is(err, api.ErrRevoked) {
		t.Fatalf("Join with a revoked token = %v, want revoked", err)
	}
	if st, err := d.c.Sync().Status(t.Context(), &api.SyncStatusParams{}); err != nil || len(st) != 0 {
		t.Fatalf("a refused join left sync state: %v, %v", st, err)
	}
}

func TestJoinWithoutATokenIsRevoked(t *testing.T) {
	h := newHub(t, map[string]string{"n.md": fiveLines})
	d := start(t, deviceOptions("d"), vaultDir(t, nil))
	hubID := pairOnly(t, h, d, "d")
	if err := d.srv.Secrets.Delete(t.Context(), secrets.HubToken(hubID)); err != nil {
		t.Fatal(err)
	}
	if _, err := joinHubVault(t, h, d); !errors.Is(err, api.ErrRevoked) {
		t.Fatalf("Join without a token = %v, want revoked", err)
	}
}

func TestJoinAnUnreachableHubIsUnavailable(t *testing.T) {
	h := newHub(t, map[string]string{"n.md": fiveLines})
	p := &partition{conns: map[net.Conn]bool{}}
	o := deviceOptions("d")
	o.Syncer.Dial = p.dial
	d := start(t, o, vaultDir(t, nil))
	pairOnly(t, h, d, "d")
	p.cut()
	_, err := joinHubVault(t, h, d)
	if !errors.Is(err, api.ErrUnavailable) || errors.Is(err, api.ErrRevoked) {
		t.Fatalf("Join of an unreachable hub = %v, want unavailable", err)
	}
}

func TestJoinNamesTheHubVaultAndItsURL(t *testing.T) {
	h := newHub(t, map[string]string{"n.md": fiveLines})
	d := start(t, deviceOptions("d"), vaultDir(t, nil))
	pairOnly(t, h, d, "d")
	st, err := joinHubVault(t, h, d)
	if err != nil {
		t.Fatal(err)
	}
	vs, err := h.c.Hub().Vaults(t.Context(), &api.HubVaultsParams{})
	if err != nil {
		t.Fatal(err)
	}
	if st.HubVault != vs[0].Name || st.URL != h.srv.HubURL {
		t.Fatalf("Join status names %q at %q, want %q at %q", st.HubVault, st.URL, vs[0].Name, h.srv.HubURL)
	}
}
