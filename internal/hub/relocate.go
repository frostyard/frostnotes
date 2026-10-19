package hub

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json/v2"
	"errors"
	"io"
	"net"
	"net/http"
	"net/url"
	"strings"

	"github.com/frostyard/frostnotes/api"
	"github.com/frostyard/frostnotes/internal/store"
)

// The hub's recorded URL and the relocation proof
// (docs/specs/lodge-app.md#device, ADR-0026): devices learn the URL with
// log.hub, and a device that missed the move checks a new URL with
// POST /sync/hub, whose proof shows the hub holds its token's hash.

// CheckURL accepts a hub URL whose scheme is https, or http on loopback
// (ADR-0016), and returns it without a trailing '/' and with its host in
// lower case, so URLs that SameURL calls the same are equal strings.
func CheckURL(raw string) (*url.URL, error) {
	u, err := url.Parse(strings.TrimSuffix(raw, "/"))
	if err != nil || u.Host == "" || u.User != nil || u.RawQuery != "" || u.Fragment != "" || u.Opaque != "" {
		return nil, api.InvalidParams("%q is not a hub URL", raw)
	}
	u.Host = strings.ToLower(u.Host)
	switch u.Scheme {
	case "https":
	case "http":
		host := u.Hostname()
		ip := net.ParseIP(host)
		if host != "localhost" && (ip == nil || !ip.IsLoopback()) {
			return nil, api.InvalidParams("%s: a hub on another machine needs https:// (put TLS in front of it)", raw)
		}
	default:
		return nil, api.InvalidParams("%s: a hub URL is https://", raw)
	}
	return u, nil
}

// SameURL reports whether two hub URLs are the same: scheme and host
// compared without case, the path exactly, a trailing '/' ignored.
func SameURL(a, b string) bool {
	ua, err := url.Parse(strings.TrimSuffix(a, "/"))
	if err != nil {
		return false
	}
	ub, err := url.Parse(strings.TrimSuffix(b, "/"))
	if err != nil {
		return false
	}
	return strings.EqualFold(ua.Scheme, ub.Scheme) && strings.EqualFold(ua.Host, ub.Host) &&
		ua.EscapedPath() == ub.EscapedPath() && ua.User == nil && ub.User == nil &&
		ua.RawQuery == "" && ub.RawQuery == "" && ua.Fragment == "" && ub.Fragment == ""
}

// NonceSize is the bytes of a relocation nonce; it travels as 43 base64url
// characters, unpadded.
const NonceSize = 32

// maxRelocateBody is the largest POST /sync/hub body.
const maxRelocateBody = 1 << 10

// Proof is a relocation proof: HMAC-SHA256, keyed with the 32 bytes of the
// SHA-256 of the device's token, of "frostnotes relocate\n" + hub ID +
// "\n" + url + "\n" + nonce, in unpadded base64url. The hub computes it
// from the hash it keeps; the device from its own token (TokenKey).
func Proof(key []byte, hubID, url, nonce string) string {
	m := hmac.New(sha256.New, key)
	_, _ = io.WriteString(m, "frostnotes relocate\n"+hubID+"\n"+url+"\n"+nonce)
	return base64.RawURLEncoding.EncodeToString(m.Sum(nil))
}

// TokenKey is a relocation proof's key for a device token: its SHA-256,
// the bytes TokenHash spells in hex.
func TokenKey(token string) []byte {
	sum := sha256.Sum256([]byte(token))
	return sum[:]
}

// Relocation is POST /sync/hub's answer.
type Relocation struct {
	Hub   string `json:"hub"`
	URL   string `json:"url"`
	Proof string `json:"proof"`
}

// RelocationRequest is POST /sync/hub's body.
type RelocationRequest struct {
	Device int64  `json:"device"`
	Nonce  string `json:"nonce"`
}

// validNonce reports whether s is 32 bytes in unpadded base64url.
func validNonce(s string) bool {
	b, err := base64.RawURLEncoding.Strict().DecodeString(s)
	return err == nil && len(b) == NonceSize
}

// relocation answers POST /sync/hub: the hub's ID, its recorded URL and
// the proof for a paired device's nonce. It takes no credential: the
// proof reveals only what an HMAC under the token's hash reveals.
func (h *Hub) relocation(w http.ResponseWriter, r *http.Request) {
	body, err := io.ReadAll(http.MaxBytesReader(w, r.Body, maxRelocateBody))
	if err != nil {
		http.Error(w, "a relocation request is at most 1 KiB", http.StatusRequestEntityTooLarge)
		return
	}
	var req RelocationRequest
	if err := json.Unmarshal(body, &req, json.RejectUnknownMembers(true)); err != nil || req.Device < 1 || !validNonce(req.Nonce) {
		http.Error(w, `the body must be {"device": <ID>, "nonce": "<32 bytes, base64url>"}`, http.StatusBadRequest)
		return
	}
	d, err := h.cfg.DB.Device(r.Context(), req.Device)
	if errors.Is(err, store.ErrNotFound) || err == nil && (!d.RevokedAt.IsZero() || d.TokenHash == "") {
		http.Error(w, "no such device", http.StatusNotFound)
		return
	}
	if err != nil {
		h.cfg.Log.Error("read a device for a relocation", "err", err)
		http.Error(w, "the hub cannot answer now", http.StatusInternalServerError)
		return
	}
	key, err := hex.DecodeString(d.TokenHash)
	if err != nil || len(key) != sha256.Size {
		h.cfg.Log.Error("a device's token hash is not SHA-256", "device", d.ID)
		http.Error(w, "the hub cannot answer now", http.StatusInternalServerError)
		return
	}
	hubID, err := h.cfg.DB.HubIdentity(r.Context())
	if err != nil {
		h.cfg.Log.Error("read the hub's ID", "err", err)
		http.Error(w, "the hub cannot answer now", http.StatusInternalServerError)
		return
	}
	u, err := h.cfg.DB.HubURL(r.Context())
	if err != nil {
		h.cfg.Log.Error("read the hub's URL", "err", err)
		http.Error(w, "the hub cannot answer now", http.StatusInternalServerError)
		return
	}
	if u == "" || hubID == "" {
		http.Error(w, "the hub records no URL for its devices", http.StatusConflict)
		return
	}
	h.cfg.Log.Debug("a device asks for a relocation proof", "device", d.ID)
	w.Header().Set("Content-Type", "application/json")
	_ = json.MarshalWrite(w, Relocation{Hub: hubID, URL: u, Proof: Proof(key, hubID, u, req.Nonce)})
}
