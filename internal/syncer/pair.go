package syncer

import (
	"bytes"
	"context"
	"crypto/tls"
	"encoding/json/v2"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"

	"golang.org/x/net/websocket"

	"github.com/frostyard/frostnotes/api"
	"github.com/frostyard/frostnotes/internal/hub"
	"github.com/frostyard/frostnotes/internal/secrets"
	"github.com/frostyard/frostnotes/internal/store"
)

// devicePairing is a pairing this device started, kept in memory until it
// is collected or expires.
type devicePairing struct {
	url, secret, hub string
	expires          time.Time
}

// checkURL accepts a hub URL whose scheme is https, or http on loopback
// (ADR-0016), and returns it without a trailing '/'.
func checkURL(raw string) (*url.URL, error) { return hub.CheckURL(raw) }

// pair starts a pairing with the hub at rawURL (sync.pair).
func (s *Syncer) pair(ctx context.Context, rawURL, name string) (*api.Pairing, error) {
	u, err := checkURL(rawURL)
	if err != nil {
		return nil, err
	}
	if err := s.probeSecrets(ctx); err != nil {
		return nil, api.Unavailable("this device cannot keep a hub token: %v", err)
	}
	if name == "" {
		name = s.cfg.Device
	}
	body, err := json.Marshal(map[string]string{"name": name})
	if err != nil {
		return nil, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, u.JoinPath("sync", "pair").String(), bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := s.cfg.HTTP.Do(req)
	if err != nil {
		return nil, api.Unavailable("reach the hub at %s: %v", u, err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusCreated {
		return nil, hubError(resp)
	}
	var got struct {
		Pairing   string    `json:"pairing"`
		Secret    string    `json:"secret"`
		Code      string    `json:"code"`
		Hub       string    `json:"hub"`
		ExpiresAt time.Time `json:"expiresAt"`
	}
	if err := json.UnmarshalRead(io.LimitReader(resp.Body, 1<<16), &got); err != nil {
		return nil, api.Unavailable("the hub at %s answered a pairing oddly: %v", u, err)
	}
	s.mu.Lock()
	s.pairings[got.Pairing] = &devicePairing{url: u.String(), secret: got.Secret, hub: got.Hub, expires: got.ExpiresAt}
	s.mu.Unlock()
	return &api.Pairing{Pairing: got.Pairing, Code: got.Code, Hub: got.Hub, URL: u.String(), ExpiresAt: got.ExpiresAt}, nil
}

// paired waits for a pairing to be approved (sync.paired); on approval it
// keeps the token in the secret store and records the hub.
func (s *Syncer) paired(ctx context.Context, id string) (*api.PairResult, error) {
	s.mu.Lock()
	p := s.pairings[id]
	s.mu.Unlock()
	if p == nil {
		return nil, api.NotFound("no pairing %q is waiting", id)
	}
	u, err := url.Parse(p.url)
	if err != nil {
		return nil, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u.JoinPath("sync", "pair", id).String(), nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+p.secret)
	resp, err := s.cfg.HTTP.Do(req)
	if err != nil {
		return nil, api.Unavailable("reach the hub at %s: %v", u, err)
	}
	defer func() { _ = resp.Body.Close() }()
	switch resp.StatusCode {
	case http.StatusGone, http.StatusNotFound:
		s.dropPairing(id)
		return &api.PairResult{State: api.PairingStateExpired, Hub: p.hub}, nil
	case http.StatusOK:
	default:
		return nil, hubError(resp)
	}
	var got struct {
		State  string `json:"state"`
		Token  string `json:"token"`
		Device int64  `json:"device"`
		Hub    string `json:"hub"`
	}
	if err := json.UnmarshalRead(io.LimitReader(resp.Body, 1<<16), &got); err != nil {
		return nil, api.Unavailable("the hub at %s answered a pairing oddly: %v", u, err)
	}
	if got.State != "approved" {
		return &api.PairResult{State: api.PairingStatePending, Hub: p.hub}, nil
	}
	s.dropPairing(id)
	if err := s.sameHub(ctx, got.Hub, p.url); err != nil {
		return nil, s.withdrawn(ctx, u, got.Token, err)
	}
	if err := s.cfg.Secrets.Set(ctx, secrets.HubToken(got.Hub), got.Token); err != nil {
		return nil, s.withdrawn(ctx, u, got.Token, fmt.Errorf("keep the device token: %w", err))
	}
	var row store.SyncHub
	if err := s.cfg.DB.Tx(ctx, func(tx *store.Tx) error {
		var err error
		row, err = tx.PutSyncHub(ctx, store.SyncHub{HubID: got.Hub, URL: p.url, DeviceID: got.Device})
		return err
	}); err != nil {
		if derr := s.cfg.Secrets.Delete(ctx, secrets.HubToken(got.Hub)); derr != nil {
			s.cfg.Log.Warn("delete a hub token a pairing cannot keep", "hub", got.Hub, "err", derr)
		}
		return nil, s.withdrawn(ctx, u, got.Token, err)
	}
	s.kickHub(row.ID)
	return &api.PairResult{State: api.PairingStateApproved, Hub: got.Hub}, nil
}

// probeSecrets checks that the secret store can keep a token before the hub
// hears of this device: the store's prompt comes first, and a device that
// cannot keep a token never asks to pair.
func (s *Syncer) probeSecrets(ctx context.Context) error {
	if err := s.cfg.Secrets.Set(ctx, secrets.ProbeKey, "probe"); err != nil {
		return err
	}
	return s.cfg.Secrets.Delete(ctx, secrets.ProbeKey)
}

// withdrawn has the hub forget the device it just handed a token to, and
// returns err saying what became of the device: the hub's list of devices no
// longer holds it, or the hub still lists it and its owner must revoke it.
func (s *Syncer) withdrawn(ctx context.Context, base *url.URL, token string, err error) error {
	if werr := s.withdraw(ctx, base, token); werr != nil {
		s.cfg.Log.Warn("withdraw a pairing the device cannot keep", "hub", base, "err", werr)
		return fmt.Errorf("%w; the hub may still list this device: revoke it on the hub (hub.revoke)", err)
	}
	return fmt.Errorf("%w; the hub forgot the device's pairing", err)
}

// withdraw asks the hub to forget the device its token names. It is best
// effort, and keeps its own deadline, so that a hub that does not answer
// does not hold the call.
func (s *Syncer) withdraw(ctx context.Context, base *url.URL, token string) error {
	ctx, cancel := context.WithTimeout(context.WithoutCancel(ctx), withdrawTimeout)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodDelete, base.JoinPath("sync", "device").String(), nil)
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Origin", hub.DeviceOrigin)
	resp, err := s.cfg.HTTP.Do(req)
	if err != nil {
		return err
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusNoContent {
		return hubError(resp)
	}
	return nil
}

// withdrawTimeout bounds a withdrawal: the pairing's answer is dropped
// anyway when it fails.
const withdrawTimeout = 10 * time.Second

// sameHub refuses a pairing whose hub claims the ID of a hub this device
// syncs vaults with at another URL: anyone can learn a hub's ID, so the
// claim alone would let a hub the user was led to take another's token
// and vaults (docs/specs/sync-protocol.md, Pairing). A hub that moved is
// paired again once its vaults have left it.
func (s *Syncer) sameHub(ctx context.Context, hubID, url string) error {
	hubs, err := s.cfg.DB.SyncHubs(ctx)
	if err != nil {
		return err
	}
	for _, h := range hubs {
		if h.HubID != hubID || h.URL == url {
			continue
		}
		remotes, err := s.cfg.DB.Remotes(ctx)
		if err != nil {
			return err
		}
		for _, r := range remotes {
			if r.Hub == h.ID {
				return api.Conflict("the hub at %s says it is hub %s, which this device syncs with at %s: "+
					"leave those vaults (sync.leave) before pairing with it at a new URL", url, hubID, h.URL)
			}
		}
	}
	return nil
}

func (s *Syncer) dropPairing(id string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.pairings, id)
}

// hubError turns a hub's refusal into an API error.
func hubError(resp *http.Response) error {
	line, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
	msg := strings.TrimSpace(string(line))
	switch resp.StatusCode {
	case http.StatusUnauthorized:
		return errRevoked
	case http.StatusTooManyRequests:
		return api.Unavailable("the hub: %s", msg)
	case http.StatusBadRequest, http.StatusRequestEntityTooLarge:
		return api.InvalidParams("the hub: %s", msg)
	}
	return api.Unavailable("the hub answered %s: %s", resp.Status, msg)
}

// session is one connection to a hub: its JSON-RPC client and the token
// and URL its blob transfers use.
type session struct {
	hub    store.SyncHub
	base   *url.URL
	token  string
	client *api.Client
}

// dial connects to hub row id's /sync WebSocket with its token and
// completes rpc.hello and events.subscribe. When the hub records another
// URL (log.hub), the device moves its row there and connects again, once.
func (s *Syncer) dial(ctx context.Context, hubRow int64) (*session, error) {
	sess, err := s.dialAt(ctx, hubRow)
	if err != nil {
		return nil, err
	}
	moved, err := s.follow(ctx, sess)
	if err != nil || moved {
		_ = sess.client.Close()
	}
	if err != nil {
		return nil, err
	}
	if moved {
		return s.dialAt(ctx, hubRow)
	}
	return sess, nil
}

// dialAt connects to hub row id at the URL it records now.
func (s *Syncer) dialAt(ctx context.Context, hubRow int64) (*session, error) {
	h, err := s.cfg.DB.SyncHub(ctx, hubRow)
	if err != nil {
		return nil, err
	}
	token, err := s.cfg.Secrets.Get(ctx, secrets.HubToken(h.HubID))
	if errors.Is(err, secrets.ErrNotFound) {
		return nil, errRevoked
	} else if err != nil {
		return nil, err
	}
	base, err := url.Parse(h.URL)
	if err != nil {
		return nil, err
	}
	ws := *base
	ws.Scheme = map[string]string{"https": "wss", "http": "ws"}[base.Scheme]
	cfg, err := websocket.NewConfig(ws.JoinPath("sync").String(), hub.DeviceOrigin)
	if err != nil {
		return nil, err
	}
	cfg.Header.Set("Authorization", "Bearer "+token)
	conn, err := s.dialSocket(ctx, cfg, base)
	if err != nil {
		if errors.Is(err, websocket.ErrBadStatus) {
			// The upgrade was refused before it began; a bad token is the
			// likely cause, which a blob request confirms.
			if probe := s.probe(ctx, base, token); probe != nil {
				return nil, probe
			}
		}
		return nil, fmt.Errorf("connect to %s: %w", h.URL, err)
	}
	conn.PayloadType = websocket.TextFrame
	client := api.NewClient(conn)
	if _, err := client.RPC().Hello(ctx, &api.RPCHelloParams{Protocol: api.Protocol, Client: s.cfg.Client}); err != nil {
		_ = client.Close()
		return nil, err
	}
	if _, err := client.Events().Subscribe(ctx, &api.EventsSubscribeParams{}); err != nil {
		_ = client.Close()
		return nil, err
	}
	return &session{hub: h, base: base, token: token, client: client}, nil
}

// probe asks the hub for a blob that cannot exist, to tell a refused token
// (401) from any other failure.
func (s *Syncer) probe(ctx context.Context, base *url.URL, token string) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodHead, base.JoinPath("sync", "blobs", strings.Repeat("0", 64)).String(), nil)
	if err != nil {
		return nil
	}
	req.Header.Set("Authorization", "Bearer "+token)
	resp, err := s.cfg.HTTP.Do(req)
	if err != nil {
		return nil
	}
	_ = resp.Body.Close()
	if resp.StatusCode == http.StatusUnauthorized {
		return errRevoked
	}
	return nil
}

// dialSocket opens the WebSocket over Config.Dial, with TLS for https.
func (s *Syncer) dialSocket(ctx context.Context, cfg *websocket.Config, base *url.URL) (*websocket.Conn, error) {
	host := base.Host
	if base.Port() == "" {
		host = net.JoinHostPort(base.Hostname(), map[string]string{"https": "443", "http": "80"}[base.Scheme])
	}
	nc, err := s.cfg.Dial(ctx, host)
	if err != nil {
		return nil, err
	}
	if base.Scheme == "https" {
		tc := tls.Client(nc, &tls.Config{ServerName: base.Hostname(), MinVersion: tls.VersionTLS12})
		if err := tc.HandshakeContext(ctx); err != nil {
			_ = nc.Close()
			return nil, err
		}
		nc = tc
	}
	if dl, ok := ctx.Deadline(); ok {
		_ = nc.SetDeadline(dl)
	} else {
		_ = nc.SetDeadline(time.Now().Add(30 * time.Second))
	}
	ws, err := websocket.NewClient(cfg, nc)
	if err != nil {
		_ = nc.Close()
		return nil, err
	}
	_ = nc.SetDeadline(time.Time{})
	return ws, nil
}
