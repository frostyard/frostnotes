package syncer

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/rand"
	"encoding/base64"
	"encoding/json/v2"
	"errors"
	"io"
	"net/http"

	"github.com/frostyard/frostnotes/api"
	"github.com/frostyard/frostnotes/internal/hub"
	"github.com/frostyard/frostnotes/internal/secrets"
	"github.com/frostyard/frostnotes/internal/store"
)

// A hub that moves (docs/specs/lodge-app.md#device): at each connection a
// device asks log.hub for the URL the hub records and moves to it, and a
// device that missed the move checks a new URL with the hub's relocation
// proof (sync.relocate). Either way the device keeps its token, replicas,
// agreed states and cursors: only its hub row's URL changes.

// follow asks the hub for the URL it records (log.hub) and, when that
// differs from the device's, moves the hub's row there once the host at
// that URL proves it is this hub, as sync.relocate requires: the device's
// next connection sends its token there, so a mistyped URL, or a hub
// database someone changed, must not collect it. It reports whether it
// moved. A hub from before M4 has no log.hub, and a URL the device would
// not pair with is not followed.
func (s *Syncer) follow(ctx context.Context, sess *session) (bool, error) {
	got, err := sess.client.Log().Hub(ctx, &api.LogHubParams{})
	if errors.Is(err, api.ErrMethodNotFound) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	if got.URL == nil || hub.SameURL(*got.URL, sess.hub.URL) {
		return false, nil
	}
	if got.Hub != sess.hub.HubID {
		s.cfg.Log.Warn("the hub names another hub ID; not following its URL", "hub", sess.hub.HubID, "named", got.Hub)
		return false, nil
	}
	u, err := checkURL(*got.URL)
	if err != nil {
		s.cfg.Log.Warn("the hub records a URL this device does not use", "hub", sess.hub.HubID, "url", *got.URL, "err", err)
		return false, nil
	}
	if err := s.proven(ctx, sess.hub, sess.token, u.String()); err != nil {
		s.cfg.Log.Warn("not following the hub's URL: the host there does not prove it is this hub", "hub", sess.hub.HubID,
			"url", u.String(), "err", err)
		return false, nil
	}
	err = s.cfg.DB.Tx(ctx, func(tx *store.Tx) error {
		_, err := tx.SetSyncHubURL(ctx, sess.hub.ID, u.String())
		return err
	})
	if errors.Is(err, store.ErrConflict) {
		s.cfg.Log.Warn("not following the hub's URL", "hub", sess.hub.HubID, "err", err)
		return false, nil
	}
	if err != nil {
		return false, err
	}
	s.cfg.Log.Info("the hub moved", "hub", sess.hub.HubID, "from", sess.hub.URL, "to", u.String())
	return true, nil
}

// relocate moves the hub paired at from to the URL to, when the hub there
// proves itself (sync.relocate): it answers with this hub's ID, records
// to as its URL, and holds this device's token's hash. The token itself
// is never sent.
func (s *Syncer) relocate(ctx context.Context, from, to string) (*api.SyncHub, error) {
	old, err := checkURL(from)
	if err != nil {
		return nil, err
	}
	h, err := s.cfg.DB.SyncHubByURL(ctx, old.String())
	if errors.Is(err, store.ErrNotFound) {
		return nil, api.NotFound("this device has not paired with %s", old)
	} else if err != nil {
		return nil, err
	}
	u, err := checkURL(to)
	if err != nil {
		return nil, err
	}
	token, err := s.cfg.Secrets.Get(ctx, secrets.HubToken(h.HubID))
	if errors.Is(err, secrets.ErrNotFound) {
		return nil, api.Conflict("this device holds no token for hub %s: pair again", h.HubID)
	} else if err != nil {
		return nil, err
	}
	if err := s.proven(ctx, h, token, u.String()); err != nil {
		return nil, err
	}
	var row store.SyncHub
	err = s.cfg.DB.Tx(ctx, func(tx *store.Tx) error {
		row, err = tx.SetSyncHubURL(ctx, h.ID, u.String())
		return err
	})
	if errors.Is(err, store.ErrConflict) {
		return nil, api.Conflict("another hub is paired at %s", u)
	} else if err != nil {
		return nil, err
	}
	s.cfg.Log.Info("relocated a hub", "hub", h.HubID, "from", h.URL, "to", row.URL)
	s.kickHub(row.ID)
	out := row.API()
	return &out, nil
}

// proven checks that the host at url is hub h, by its relocation proof:
// it answers a fresh nonce with h's ID, records url as its own, and holds
// the hash of token. The token itself is never sent. A failed check is
// conflict.
func (s *Syncer) proven(ctx context.Context, h store.SyncHub, token, url string) error {
	nonce := make([]byte, hub.NonceSize)
	_, _ = rand.Read(nonce)
	req := hub.RelocationRequest{Device: h.DeviceID, Nonce: base64.RawURLEncoding.EncodeToString(nonce)}
	got, err := s.askRelocation(ctx, url, req)
	if err != nil {
		return err
	}
	switch {
	case got.Hub != h.HubID:
		return api.Conflict("the hub at %s is not hub %s", url, h.HubID)
	case !hub.SameURL(got.URL, url):
		return api.Conflict("the hub at %s records another URL for its devices: it is not that hub's address", url)
	case !hmac.Equal([]byte(got.Proof), []byte(hub.Proof(hub.TokenKey(token), h.HubID, got.URL, req.Nonce))):
		return api.Conflict("the hub at %s does not hold this device's token", url)
	}
	return nil
}

// askRelocation posts req to the hub at base's /sync/hub.
func (s *Syncer) askRelocation(ctx context.Context, base string, req hub.RelocationRequest) (*hub.Relocation, error) {
	u, err := checkURL(base)
	if err != nil {
		return nil, err
	}
	body, err := json.Marshal(req)
	if err != nil {
		return nil, err
	}
	hr, err := http.NewRequestWithContext(ctx, http.MethodPost, u.JoinPath("sync", "hub").String(), bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	hr.Header.Set("Content-Type", "application/json")
	resp, err := s.cfg.HTTP.Do(hr)
	if err != nil {
		return nil, api.Unavailable("reach the hub at %s: %v", u, err)
	}
	defer func() { _ = resp.Body.Close() }()
	switch resp.StatusCode {
	case http.StatusOK:
	case http.StatusNotFound:
		return nil, api.Conflict("the hub at %s does not know this device", u)
	case http.StatusConflict:
		return nil, api.Conflict("the hub at %s records no URL for its devices: run notesctl hub url on the hub", u)
	default:
		line, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
		return nil, api.Unavailable("the hub at %s answered %s: %s", u, resp.Status, bytes.TrimSpace(line))
	}
	var got hub.Relocation
	if err := json.UnmarshalRead(io.LimitReader(resp.Body, 1<<12), &got); err != nil {
		return nil, api.Conflict("the hub at %s answered a relocation oddly: %v", u, err)
	}
	return &got, nil
}
