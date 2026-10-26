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
	"crypto/rand"
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
	// Crash, when set, is asked after a push is applied and recorded and
	// before its answer ("pushed"); true drops the device's connection
	// instead of answering, as a crash or a lost link would. The simulation
	// sets it (ADR-0020).
	Crash func(point string) bool
}

// Hub serves devices. Its router, set with SetRouter, serves their
// WebSocket connections as caller device.
type Hub struct {
	cfg Config

	mu       sync.Mutex
	router   *api.Router
	pairings map[string]*pairing            // by pairing ID
	conns    map[int64]map[net.Conn]int     // a device's live connections
	uploads  map[int64]map[string]time.Time // what each device uploaded, and when
	// logs guards whole push batches and their answers against other log
	// calls on the same vault. mu guards this map; each lock guards the log.
	logs map[int64]*sync.RWMutex
	// decoy keys the relocation proofs of devices that do not exist
	// (proofKey); random for each process.
	decoy []byte
	// limit bounds what a peer asks without a device token (limited).
	limit limiter
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
	decoy := make([]byte, sha256.Size)
	_, _ = rand.Read(decoy)
	return &Hub{cfg: cfg, pairings: map[string]*pairing{}, conns: map[int64]map[net.Conn]int{}, uploads: map[int64]map[string]time.Time{},
		logs: map[int64]*sync.RWMutex{}, decoy: decoy}
}

// logLock keeps a push's ordered moves and answers together: another
// push, pull or match must not see a partially applied swap or parked
// path. Different vaults can still sync independently.
func (h *Hub) logLock(vaultID int64) *sync.RWMutex {
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.logs[vaultID] == nil {
		h.logs[vaultID] = &sync.RWMutex{}
	}
	return h.logs[vaultID]
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
	mux.HandleFunc("POST /sync/pair", h.limited(h.startPairing))
	mux.HandleFunc("GET /sync/pair/{pairing}", h.collectPairing)
	mux.HandleFunc("POST /sync/hub", h.limited(h.relocation))
	mux.HandleFunc("DELETE /sync/device", h.withdrawDevice)
	mux.Handle("GET /sync", h.authenticated(h.syncSocket()))
	mux.Handle("/sync/blobs/{hash}", h.authenticated(blobxfer.Handler(h.cfg.Blobs, h.cfg.MaxBlob, h.cfg.DB.HasContent, h.uploaded)))
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

type peerKey struct{}

// Peer is the node a request came from as a listener in front of the hub
// saw it (lodge's Lodge-* headers): Name is shown beside a pending
// pairing, to tell pairings apart, and Key, its node ID, bounds how many
// pairings one node may have waiting. Neither admits anything
// (docs/specs/lodge-app.md#device).
type Peer struct {
	Name string
	Key  string
}

// WithPeer returns ctx naming the peer a request came from.
func WithPeer(ctx context.Context, p Peer) context.Context {
	return context.WithValue(ctx, peerKey{}, p)
}

// peerOf is the peer a request came from: the one WithPeer named, else the
// connection's remote address, with no key.
func peerOf(r *http.Request) Peer {
	if p, ok := r.Context().Value(peerKey{}).(Peer); ok {
		return p
	}
	return Peer{Name: r.RemoteAddr}
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

// uploadKept is how long a device's upload counts as its own for its
// pushes: the day the hub keeps a blob no revision names (ADR-0019).
const uploadKept = 24 * time.Hour

// uploaded records that the request's device uploaded hash.
func (h *Hub) uploaded(ctx context.Context, hash string) {
	d, ok := deviceFrom(ctx)
	if !ok {
		return
	}
	now := h.cfg.Now()
	h.mu.Lock()
	defer h.mu.Unlock()
	mine := h.uploads[d.ID]
	if mine == nil {
		mine = map[string]time.Time{}
		h.uploads[d.ID] = mine
	}
	for k, at := range mine {
		if now.Sub(at) > uploadKept {
			delete(mine, k)
		}
	}
	mine[hash] = now
}

// mayName reports whether device d may name content hash in a push to hub
// vault vaultID: its log names it already, or d uploaded it. Content the
// hub holds for anything else is not d's to name (docs/specs/sync-protocol.md,
// Blobs).
func (h *Hub) mayName(ctx context.Context, d int64, vaultID int64, hash string) (bool, error) {
	h.mu.Lock()
	at, ok := h.uploads[d][hash]
	h.mu.Unlock()
	if ok && h.cfg.Now().Sub(at) <= uploadKept {
		return true, nil
	}
	return h.cfg.DB.VaultHasContent(ctx, vaultID, hash)
}

// maxConns is how many sync connections one device may hold at once; a
// device's loop holds one per synced vault.
const maxConns = 64

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
			if h.cfg.Crash != nil {
				h.cfg.Crash("connecting") // a test's moment between the token check and what follows
			}
			h.mu.Lock()
			router := h.router
			if len(h.conns[d.ID]) >= maxConns {
				h.mu.Unlock()
				h.cfg.Log.Warn("a device has too many connections", "device", d.ID)
				return
			}
			if h.conns[d.ID] == nil {
				h.conns[d.ID] = map[net.Conn]int{}
			}
			h.conns[d.ID][ws]++
			h.mu.Unlock()
			defer h.forget(d.ID, ws)
			// A revocation between the token check and the line above
			// closed the device's connections without this one: the
			// device is checked again now that a revocation would close it.
			if now, err := h.cfg.DB.Device(ctx, d.ID); err != nil || now.TokenHash != d.TokenHash || !now.RevokedAt.IsZero() {
				return
			}
			if router == nil {
				h.cfg.Log.Error("the hub has no router")
				return
			}
			h.cfg.RPC.ServeConn(ctx, ws, router, api.Identity{Caller: api.CallerDevice})
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
