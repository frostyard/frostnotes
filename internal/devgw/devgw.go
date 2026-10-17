// Package devgw serves notesd's RPC over a WebSocket on loopback, so the UI can
// be developed in an ordinary browser against a real notesd
// (docs/specs/rpc-protocol.md, Transports). notesd serves it only with -devgw.
// Browsers do not apply CORS to WebSockets, so every connection must carry
// the token and come from an allowed Origin.
package devgw

import (
	"context"
	"crypto/subtle"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"net/netip"
	"slices"
	"time"

	"golang.org/x/net/websocket"

	"github.com/frostyard/frostnotes/internal/blobs"
)

// Origins are the pages allowed to connect: the Vite dev server.
var Origins = []string{"http://127.0.0.1:5173", "http://localhost:5173"}

// ErrNotLoopback means the gateway was asked to listen beyond this machine.
var ErrNotLoopback = errors.New("devgw: listen address must be loopback")

// Handler upgrades requests that carry token (as ?token=) and an Origin in
// origins, and hands each connection to serve.
func Handler(token string, origins []string, serve func(nc net.Conn)) http.Handler {
	return websocket.Server{
		// The library parses Origin only in its default handshake, so read
		// the header here.
		Handshake: func(_ *websocket.Config, req *http.Request) error {
			got := req.URL.Query().Get("token")
			if token == "" || subtle.ConstantTimeCompare([]byte(got), []byte(token)) != 1 {
				return errors.New("devgw: bad token")
			}
			if !slices.Contains(origins, req.Header.Get("Origin")) {
				return errors.New("devgw: origin not allowed")
			}
			return nil
		},
		Handler: func(ws *websocket.Conn) {
			ws.PayloadType = websocket.TextFrame
			serve(ws)
		},
	}
}

// ListenAndServe serves Handler on addr, which must be a loopback address,
// and BlobHandler over store at /blobs/, until ctx ends.
func ListenAndServe(ctx context.Context, addr, token string, serve func(nc net.Conn), store *blobs.Store, log *slog.Logger) error {
	ap, err := netip.ParseAddrPort(addr)
	if err != nil {
		return fmt.Errorf("devgw: %w", err)
	}
	if !ap.Addr().IsLoopback() {
		return fmt.Errorf("%w: %s", ErrNotLoopback, addr)
	}
	if len(token) < 16 {
		return errors.New("devgw: the token must be at least 16 characters")
	}
	mux := http.NewServeMux()
	mux.Handle("/blobs/", BlobHandler(token, store))
	mux.Handle("/", Handler(token, Origins, serve))
	srv := &http.Server{Addr: addr, Handler: mux, ReadHeaderTimeout: 10 * time.Second}
	stop := context.AfterFunc(ctx, func() { _ = srv.Close() })
	defer stop()
	log.Warn("dev gateway listening; for development only", "addr", addr)
	if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
		return err
	}
	return nil
}
