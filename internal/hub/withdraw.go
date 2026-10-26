package hub

import (
	"errors"
	"net/http"

	"github.com/frostyard/frostnotes/internal/store"
)

// withdrawDevice answers DELETE /sync/device (sync-protocol.md, Pairing): a
// device whose pairing was approved but which could not keep the token asks
// the hub to forget it, so that a failed pairing leaves no device behind.
//
// The request does not go through authenticated: that records the device as
// seen, and a device the hub must forget is one that never synced. A device
// that made a request is told to be revoked instead, since the hub's
// revisions may name it.
func (h *Hub) withdrawDevice(w http.ResponseWriter, r *http.Request) {
	token := bearer(r)
	d, err := h.cfg.DB.DeviceByToken(r.Context(), TokenHash(token))
	if token == "" || err != nil {
		if err != nil && !errors.Is(err, store.ErrNotFound) {
			h.cfg.Log.Warn("check a device token", "err", err)
		}
		w.Header().Set("WWW-Authenticate", "Bearer")
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}
	var deleted bool
	err = h.cfg.DB.Tx(r.Context(), func(tx *store.Tx) error {
		var err error
		deleted, err = tx.DeleteUnseenDevice(r.Context(), d.ID)
		return err
	})
	if err != nil {
		h.cfg.Log.Error("forget a device that withdrew its pairing", "device", d.ID, "err", err)
		http.Error(w, "the hub cannot answer now", http.StatusInternalServerError)
		return
	}
	if !deleted {
		http.Error(w, "the device has made requests; revoke it with hub.revoke", http.StatusConflict)
		return
	}
	h.cfg.Log.Info("a device withdrew its pairing", "device", d.ID)
	w.WriteHeader(http.StatusNoContent)
}
