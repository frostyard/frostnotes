// Package hub is notesd's hub role (ADR-0005, ADR-0016,
// docs/specs/sync-protocol.md): the HTTP endpoints devices sync through
// (pairing, the /sync WebSocket, blobs), the token check that guards them,
// the log domain devices call, and the hub domain for the hub's own local
// clients. Pushes become writes through the vaults' actors
// (vault.Vault.Push), so the hub merges with the same write path as every
// local writer.
package hub

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"log/slog"
	"net"
	"net/http"
	"strings"
	"sync"
	"time"

	"golang.org/x/net/websocket"

	"github.com/frostyard/frostnotes/api"
	"github.com/frostyard/frostnotes/internal/blobs"
	"github.com/frostyard/frostnotes/internal/blobxfer"
	"github.com/frostyard/frostnotes/internal/rpcserver"
	"github.com/frostyard/frostnotes/internal/store"
	"github.com/frostyard/frostnotes/internal/vault"
)

// DeviceOrigin is the only Origin a request to the hub may carry: a device
// sends it on its WebSocket (golang.org/x/net/websocket requires one), and
// no browser page can have it (docs/specs/sync-protocol.md, Endpoints).
const DeviceOrigin = "notesd://device"

// Config is what the hub needs. Zero fields take the defaults in brackets.
type Config struct {
	DB     *store.DB
	Blobs  *blobs.Store
	Vaults *vault.Manager
	RPC    *rpcserver.Server
	Log    *slog.Logger     // [discard]
	Now    func() time.Time // [time.Now]
	// Listen is the address notesd --hub serves, for hub.status; "" when
	// the hub serves none.
	Listen string
	// MaxBlob is the largest blob a device may upload [blobxfer.DefaultMaxSize].
	MaxBlob int64
}

// Hub serves devices. Its router, set with SetRouter, serves their
// WebSocket connections as caller device.
type Hub struct {
	cfg Config

	mu       sync.Mutex
	router   *api.Router
	pairings map[string]*pairing        // by pairing ID
	conns    map[int64]map[net.Conn]int // a device's live connections
}

// New returns a hub over cfg.
func New(cfg Config) *Hub {
	if cfg.Log == nil {
		cfg.Log = slog.New(slog.DiscardHandler)
	}
	if cfg.Now == nil {
		cfg.Now = time.Now
	}
	if cfg.MaxBlob == 0 {
		cfg.MaxBlob = blobxfer.DefaultMaxSize
	}
	return &Hub{cfg: cfg, pairings: map[string]*pairing{}, conns: map[int64]map[net.Conn]int{}}
}

// SetRouter sets the router device connections dispatch through; it is
// built from services that include the hub's own.
func (h *Hub) SetRouter(r *api.Router) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.router = r
}

// Handler serves the hub's endpoints (docs/specs/sync-protocol.md,
// Endpoints).
func (h *Hub) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("POST /sync/pair", h.startPairing)
	mux.HandleFunc("GET /sync/pair/{pairing}", h.collectPairing)
	mux.Handle("GET /sync", h.authenticated(h.syncSocket()))
	mux.Handle("/sync/blobs/{hash}", h.authenticated(blobxfer.Handler(h.cfg.Blobs, h.cfg.MaxBlob, h.cfg.DB.HasContent)))
	return refuseOrigins(mux)
}

// Serve serves Handler on ln until ctx ends.
func (h *Hub) Serve(ctx context.Context, ln net.Listener) error {
	srv := &http.Server{Handler: h.Handler(), ReadHeaderTimeout: 10 * time.Second,
		BaseContext: func(net.Listener) context.Context { return ctx }}
	stop := context.AfterFunc(ctx, func() { _ = srv.Close() })
	defer stop()
	h.cfg.Log.Info("hub listening", "addr", ln.Addr().String())
	if err := srv.Serve(ln); err != nil && !errors.Is(err, http.ErrServerClosed) {
		return err
	}
	return nil
}

// refuseOrigins answers 403 to a request whose Origin is not the device's.
func refuseOrigins(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if o := r.Header.Get("Origin"); o != "" && o != DeviceOrigin {
			http.Error(w, "the hub's sync endpoints are for devices, not browsers", http.StatusForbidden)
			return
		}
		next.ServeHTTP(w, r)
	})
}

// TokenHash is the SHA-256, hex, the hub keeps of a device token.
func TokenHash(token string) string {
	sum := sha256.Sum256([]byte(token))
	return hex.EncodeToString(sum[:])
}

type deviceKey struct{}

// deviceFrom returns the device a request or connection authenticated as.
func deviceFrom(ctx context.Context) (store.HubDevice, bool) {
	d, ok := ctx.Value(deviceKey{}).(store.HubDevice)
	return d, ok
}

// bearer returns the request's bearer token, or "".
func bearer(r *http.Request) string {
	token, ok := strings.CutPrefix(r.Header.Get("Authorization"), "Bearer ")
	if !ok {
		return ""
	}
	return strings.TrimSpace(token)
}

// authenticated admits a request whose bearer token is a device's, with the
// device in its context, and answers 401 to any other.
func (h *Hub) authenticated(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
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
		h.touch(r.Context(), d.ID)
		next.ServeHTTP(w, r.WithContext(context.WithValue(r.Context(), deviceKey{}, d)))
	})
}

// touch records a device's request, at most once a minute.
func (h *Hub) touch(ctx context.Context, id int64) {
	err := h.cfg.DB.Tx(ctx, func(tx *store.Tx) error {
		_, err := tx.TouchDevice(ctx, id, h.cfg.Now())
		return err
	})
	if err != nil {
		h.cfg.Log.Warn("record a device's request", "device", id, "err", err)
	}
}

// syncSocket serves JSON-RPC over the WebSocket as caller device, until
// the connection ends or the device is revoked.
func (h *Hub) syncSocket() http.Handler {
	return websocket.Server{
		// The authenticated wrapper checked the token, and refuseOrigins the
		// Origin, before the upgrade.
		Handshake: func(*websocket.Config, *http.Request) error { return nil },
		Handler: func(ws *websocket.Conn) {
			ws.PayloadType = websocket.TextFrame
			ctx := ws.Request().Context()
			d, _ := deviceFrom(ctx)
			h.mu.Lock()
			router := h.router
			if h.conns[d.ID] == nil {
				h.conns[d.ID] = map[net.Conn]int{}
			}
			h.conns[d.ID][ws]++
			h.mu.Unlock()
			defer h.forget(d.ID, ws)
			if router == nil {
				h.cfg.Log.Error("the hub has no router")
				return
			}
			h.cfg.RPC.ServeConn(ctx, ws, router, api.CallerDevice)
		},
	}
}

func (h *Hub) forget(id int64, c net.Conn) {
	h.mu.Lock()
	defer h.mu.Unlock()
	delete(h.conns[id], c)
	if len(h.conns[id]) == 0 {
		delete(h.conns, id)
	}
}

// disconnect closes every connection of device id.
func (h *Hub) disconnect(id int64) {
	h.mu.Lock()
	conns := h.conns[id]
	delete(h.conns, id)
	h.mu.Unlock()
	for c := range conns {
		_ = c.Close()
	}
}

// connected reports whether device id has a live connection.
func (h *Hub) connected(id int64) bool {
	h.mu.Lock()
	defer h.mu.Unlock()
	return len(h.conns[id]) > 0
}
