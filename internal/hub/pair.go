package hub

import (
	"crypto/rand"
	"crypto/subtle"
	"encoding/base64"
	"encoding/hex"
	"encoding/json/v2"
	"errors"
	"io"
	"net/http"
	"slices"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	"github.com/frostyard/frostnotes/api"
	"github.com/frostyard/frostnotes/internal/store"
)

// Pairing (ADR-0016, docs/specs/sync-protocol.md, Pairing): pending
// pairings live in the hub's memory for 10 minutes, at most 8 at once, and
// are approved only through the local socket.
const (
	pairingTTL     = 10 * time.Minute
	maxPending     = 8
	maxPairBody    = 1 << 10
	collectWait    = 30 * time.Second
	crockford      = "0123456789ABCDEFGHJKMNPQRSTVWXYZ"
	tokenPrefix    = "fnd_"
	maxDeviceName  = 64
	expiredKeptFor = pairingTTL // an expired pairing still answers 410 this long
)

type pairing struct {
	id         string
	secretHash string
	code       string // 8 characters of crockford, without the '-'
	name, addr string
	created    time.Time
	expires    time.Time
	approved   bool
	changed    chan struct{} // closed when approved
}

func randomHex(n int) string {
	b := make([]byte, n)
	_, _ = rand.Read(b) // crypto/rand.Read never fails
	return hex.EncodeToString(b)
}

func newCode() string {
	b := make([]byte, 8)
	_, _ = rand.Read(b)
	out := make([]byte, 8)
	for i, c := range b {
		out[i] = crockford[int(c)%len(crockford)]
	}
	return string(out)
}

// normalizeCode reads a code as a user may type it: any case, with spaces
// or '-', O for 0 and I or L for 1.
func normalizeCode(s string) string {
	var b strings.Builder
	for _, r := range strings.ToUpper(s) {
		switch r {
		case ' ', '-':
		case 'O':
			b.WriteByte('0')
		case 'I', 'L':
			b.WriteByte('1')
		default:
			b.WriteRune(r)
		}
	}
	return b.String()
}

func showCode(c string) string { return c[:4] + "-" + c[4:] }

func validName(name string) bool {
	if name == "" || len(name) > maxDeviceName || !utf8.ValidString(name) {
		return false
	}
	return !strings.ContainsFunc(name, unicode.IsControl)
}

// sweep forgets pairings that expired long enough ago. h.mu must be held.
func (h *Hub) sweep(now time.Time) {
	for id, p := range h.pairings {
		if now.After(p.expires.Add(expiredKeptFor)) {
			delete(h.pairings, id)
		}
	}
}

// startPairing answers POST /sync/pair.
func (h *Hub) startPairing(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Name string `json:"name"`
	}
	body, err := io.ReadAll(http.MaxBytesReader(w, r.Body, maxPairBody))
	if err != nil {
		http.Error(w, "a pairing request is at most 1 KiB", http.StatusRequestEntityTooLarge)
		return
	}
	if err := json.Unmarshal(body, &req, json.RejectUnknownMembers(true)); err != nil || !validName(req.Name) {
		http.Error(w, `the body must be {"name": "…"}, a name of 1 to 64 bytes`, http.StatusBadRequest)
		return
	}
	hubID, err := h.hubID(r)
	if err != nil {
		h.cfg.Log.Error("make the hub's ID", "err", err)
		http.Error(w, "the hub cannot pair now", http.StatusInternalServerError)
		return
	}
	now := h.cfg.Now()
	secret := make([]byte, 32)
	_, _ = rand.Read(secret)
	encoded := base64.RawURLEncoding.EncodeToString(secret)
	p := &pairing{id: randomHex(16), secretHash: TokenHash(encoded), code: newCode(), name: req.Name,
		addr: peerOf(r), created: now, expires: now.Add(pairingTTL), changed: make(chan struct{})}
	h.mu.Lock()
	h.sweep(now)
	pending := 0
	for _, q := range h.pairings {
		if !q.approved && now.Before(q.expires) {
			pending++
		}
	}
	if pending >= maxPending {
		h.mu.Unlock()
		http.Error(w, "too many devices are waiting to pair; try again later", http.StatusTooManyRequests)
		return
	}
	h.pairings[p.id] = p
	h.mu.Unlock()
	h.cfg.Log.Info("a device asks to pair", "name", p.name, "addr", p.addr)
	writeJSON(w, http.StatusCreated, map[string]any{"pairing": p.id, "secret": encoded, "code": showCode(p.code),
		"hub": hubID, "expiresAt": p.expires.UTC()})
}

func (h *Hub) hubID(r *http.Request) (string, error) {
	var id string
	err := h.cfg.DB.Tx(r.Context(), func(tx *store.Tx) error {
		var err error
		id, err = tx.HubID(r.Context())
		return err
	})
	return id, err
}

// collectPairing answers GET /sync/pair/{pairing}: it waits up to 30 s for
// approval, and hands an approved device its token once.
func (h *Hub) collectPairing(w http.ResponseWriter, r *http.Request) {
	h.mu.Lock()
	p, ok := h.pairings[r.PathValue("pairing")]
	h.mu.Unlock()
	if !ok || subtle.ConstantTimeCompare([]byte(TokenHash(bearer(r))), []byte(p.secretHash)) != 1 {
		http.Error(w, "no such pairing", http.StatusNotFound)
		return
	}
	timer := time.NewTimer(collectWait)
	defer timer.Stop()
	select {
	case <-p.changed:
	case <-timer.C:
	case <-r.Context().Done():
		return
	}
	h.mu.Lock()
	now := h.cfg.Now()
	switch {
	case h.pairings[p.id] != p:
		h.mu.Unlock()
		http.Error(w, "no such pairing", http.StatusNotFound) // collected meanwhile
		return
	case !p.approved && !now.Before(p.expires):
		h.mu.Unlock()
		http.Error(w, "the pairing expired before it was approved", http.StatusGone)
		return
	case !p.approved:
		h.mu.Unlock()
		writeJSON(w, http.StatusOK, map[string]any{"state": "pending"})
		return
	}
	delete(h.pairings, p.id) // a token is handed out once
	h.mu.Unlock()
	secret := make([]byte, 32)
	_, _ = rand.Read(secret)
	token := tokenPrefix + base64.RawURLEncoding.EncodeToString(secret)
	var d store.HubDevice
	var hubID string
	err := h.cfg.DB.Tx(r.Context(), func(tx *store.Tx) error {
		var err error
		if hubID, err = tx.HubID(r.Context()); err != nil {
			return err
		}
		d, err = tx.AddDevice(r.Context(), p.name, TokenHash(token))
		return err
	})
	if err != nil {
		h.cfg.Log.Error("record a paired device", "err", err)
		http.Error(w, "the hub cannot pair now", http.StatusInternalServerError)
		return
	}
	h.cfg.Log.Info("a device paired", "device", d.ID, "name", d.Name)
	writeJSON(w, http.StatusOK, map[string]any{"state": "approved", "token": token, "device": d.ID, "hub": hubID})
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	data, err := json.Marshal(v)
	if err != nil {
		http.Error(w, "encode the answer", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_, _ = w.Write(append(data, '\n'))
}

// pending lists the pairings that have not expired, oldest first.
func (h *Hub) pending() []api.PendingPairing {
	h.mu.Lock()
	defer h.mu.Unlock()
	now := h.cfg.Now()
	h.sweep(now)
	out := []api.PendingPairing{}
	for _, p := range h.pairings {
		if now.Before(p.expires) {
			out = append(out, api.PendingPairing{Code: showCode(p.code), Name: p.name, Addr: p.addr,
				CreatedAt: p.created, ExpiresAt: p.expires, Approved: p.approved})
		}
	}
	slices.SortFunc(out, func(a, b api.PendingPairing) int { return a.CreatedAt.Compare(b.CreatedAt) })
	return out
}

// approve approves the pending pairing with code.
func (h *Hub) approve(code string) (api.PendingPairing, error) {
	code = normalizeCode(code)
	h.mu.Lock()
	defer h.mu.Unlock()
	now := h.cfg.Now()
	for _, p := range h.pairings {
		if p.code == code && now.Before(p.expires) {
			if !p.approved {
				p.approved = true
				close(p.changed)
				h.cfg.Log.Info("pairing approved", "name", p.name)
			}
			return api.PendingPairing{Code: showCode(p.code), Name: p.name, Addr: p.addr, CreatedAt: p.created,
				ExpiresAt: p.expires, Approved: true}, nil
		}
	}
	return api.PendingPairing{}, api.NotFound("no pending pairing has code %s", code)
}

var errNoDevice = errors.New("hub: the request carries no device")
