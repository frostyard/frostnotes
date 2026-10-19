// Package lodgeapp serves notesd as lodge's notes app
// (docs/specs/lodge-app.md, ADR-0026) over lodge/kit, which owns the app
// socket, its peer check and the Lodge-* headers: /health, /api as caller
// session, /content/blobs/ for lodge-content, and the hub's device routes
// under /device/. /mcp comes in plan 0006's Phase 4.
package lodgeapp

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"fmt"
	"io/fs"
	"log/slog"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	kit "github.com/frostyard/lodge/kit/lodgeapp"

	"github.com/frostyard/frostnotes/api"
	"github.com/frostyard/frostnotes/internal/blobhttp"
	"github.com/frostyard/frostnotes/internal/blobs"
	"github.com/frostyard/frostnotes/internal/hub"
	"github.com/frostyard/frostnotes/internal/rpcserver"
)

// ID is notes' app id: its manifest's, its socket's name and its URLs'
// segment.
const ID = "notes"

// Limits of a blob answer (docs/specs/lodge-app.md#limits).
const (
	MaxRange = 8 << 20
	MaxWhole = 64 << 20
)

// Config is what the app socket serves.
type Config struct {
	RPC    *rpcserver.Server
	Router *api.Router
	Blobs  *blobs.Store
	// Hub serves the device routes; nil answers them 404.
	Hub *hub.Hub
	// Version is notesd's, for /health.
	Version string
	// ContentKey names files in content URLs (blobhttp.ContentMAC); a
	// session learns it from rpc.hello. LoadContentKey keeps it.
	ContentKey []byte
	Log        *slog.Logger
}

// Server returns the kit's server for notes' routes. Without a content key
// it makes one for this process: content URLs are never unkeyed.
func Server(cfg Config) *kit.Server {
	if cfg.ContentKey == nil {
		cfg.ContentKey = make([]byte, 32)
		_, _ = rand.Read(cfg.ContentKey)
	}
	s := &kit.Server{
		Health: func(context.Context) kit.Health {
			return kit.Health{OK: true, Version: cfg.Version, Protocol: api.Protocol}
		},
		API: func(ctx context.Context, nc net.Conn, c kit.Caller) {
			cfg.RPC.ServeConn(ctx, nc, cfg.Router, Identity(c, cfg.ContentKey))
		},
		Content: blobhttp.Handler(cfg.Blobs, blobhttp.Options{Prefix: "/content/blobs/",
			MaxRange: MaxRange, MaxWhole: MaxWhole, Download: true, Key: cfg.ContentKey}),
		Logger: cfg.Log,
	}
	if cfg.Hub != nil {
		s.Device = Device(cfg.Hub.Handler())
	}
	return s
}

// Serve listens on notes' app socket (LODGE_ROOT and LODGE_UID for
// development, else /run/lodge/apps/notes.sock, or the socket systemd
// passed) and serves it until ctx ends.
func Serve(ctx context.Context, cfg Config) error {
	env, err := kit.LoadEnv()
	if err != nil {
		return err
	}
	ln, err := env.Listen(ID)
	if err != nil {
		return err
	}
	if cfg.Log != nil {
		cfg.Log.Info("lodge app socket", "path", env.SocketPath(ID), "lodge uid", env.LodgeUID)
	}
	return Server(cfg).Serve(ctx, ln)
}

// Identity is the connection identity of an /api session: caller session,
// named by lodge's headers, whose writes are "<node> (web)", and which
// learns the content key from rpc.hello.
func Identity(c kit.Caller, contentKey []byte) api.Identity {
	return api.Identity{Caller: api.CallerSession, Login: c.Login, DisplayName: c.DisplayName, Node: c.NodeName,
		ContentKey: base64.RawURLEncoding.EncodeToString(contentKey)}
}

// contentKeyFile is the content key's file in notesd's data directory.
const contentKeyFile = "content.key"

// LoadContentKey returns the content key kept in dir, making it (32 bytes
// from crypto/rand, mode 0600) on first use, so content URLs stay the
// same across restarts and browsers can cache files.
func LoadContentKey(dir string) ([]byte, error) {
	path := filepath.Join(dir, contentKeyFile)
	key, err := os.ReadFile(path)
	if err == nil {
		if len(key) != 32 {
			return nil, fmt.Errorf("%s holds %d bytes, not 32", path, len(key))
		}
		return key, nil
	}
	if !errors.Is(err, fs.ErrNotExist) {
		return nil, err
	}
	key = make([]byte, 32)
	_, _ = rand.Read(key)
	f, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if err != nil {
		return nil, fmt.Errorf("make the content key: %w", err)
	}
	if _, err := f.Write(key); err != nil {
		_ = f.Close()
		return nil, fmt.Errorf("write the content key: %w", err)
	}
	if err := f.Close(); err != nil {
		return nil, fmt.Errorf("write the content key: %w", err)
	}
	return key, nil
}

// Device serves the hub's endpoints (sync-protocol.md's paths) under
// /device/, as lodge forwards /device/notes/…: the device token still
// authenticates each request, and lodge's peer headers only name the
// node beside a pending pairing.
func Device(h http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		rest, ok := strings.CutPrefix(r.URL.Path, "/device")
		if !ok || (rest != "/sync" && !strings.HasPrefix(rest, "/sync/")) {
			http.NotFound(w, r)
			return
		}
		ctx := hub.WithPeer(r.Context(), peer(r.Context()))
		r2 := r.Clone(ctx)
		r2.URL.Path, r2.URL.RawPath = rest, ""
		h.ServeHTTP(w, r2)
	})
}

// peer is the node lodge named for a device request: its name, with its
// login when lodge sent one ("phone (ana@example.com)", "phone", or ""),
// and its node ID as the key that bounds its pending pairings.
func peer(ctx context.Context) hub.Peer {
	c, ok := kit.CallerFrom(ctx)
	if !ok {
		return hub.Peer{}
	}
	name := c.NodeName
	if name != "" && c.Login != "" {
		name += " (" + c.Login + ")"
	}
	return hub.Peer{Name: name, Key: c.NodeID}
}
