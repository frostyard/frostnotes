package hub_test

import (
	"errors"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/frostyard/frostnotes/api"
	"github.com/frostyard/frostnotes/internal/store"
)

// A device that paired but cannot keep its token has the hub forget it, so
// a failed pairing leaves no device behind. A device that made a request
// stays: the hub's revisions may name it, so revoking it is the hub's owner's
// decision.
func TestADeviceWithdrawsItsPairing(t *testing.T) {
	h := newHub(t)
	ctx := t.Context()
	token, device := h.pair("laptop")
	if resp := h.do(http.MethodDelete, "/sync/device", token, nil, nil); resp.StatusCode != http.StatusNoContent {
		t.Fatalf("DELETE /sync/device = %s, want 204", resp.Status)
	}
	if ds, err := h.c.Hub().Devices(ctx, &api.HubDevicesParams{}); err != nil || len(ds) != 0 {
		t.Fatalf("devices after the withdrawal = %+v, %v; want none", ds, err)
	}
	if _, err := h.srv.DB.Device(ctx, device); !errors.Is(err, store.ErrNotFound) {
		t.Fatalf("the withdrawn device = %v, want ErrNotFound", err)
	}

	// One authenticated request is enough to keep the device.
	phone, phoneID := h.pair("phone")
	c, err := h.dial(phone)
	if err != nil {
		t.Fatal(err)
	}
	resp := h.do(http.MethodDelete, "/sync/device", phone, nil, nil)
	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusConflict || !strings.Contains(string(body), "hub.revoke") {
		t.Fatalf("a seen device's withdrawal = %s %q, want 409 naming hub.revoke", resp.Status, body)
	}
	if _, err := h.srv.DB.Device(ctx, phoneID); err != nil {
		t.Fatalf("the seen device = %v, want it to stay", err)
	}
	if _, err := c.Log().Vaults(ctx, &api.LogVaultsParams{}); err != nil {
		t.Fatalf("the seen device's connection broke: %v", err)
	}

	// A token that is nobody's is refused as at every other device
	// endpoint.
	for _, tok := range []string{"", "fnd_wrong"} {
		resp := h.do(http.MethodDelete, "/sync/device", tok, nil, nil)
		if resp.StatusCode != http.StatusUnauthorized || resp.Header.Get("WWW-Authenticate") != "Bearer" {
			t.Errorf("DELETE /sync/device with token %q = %s", tok, resp.Status)
		}
	}
}
