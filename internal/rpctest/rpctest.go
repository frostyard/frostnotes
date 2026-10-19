// Package rpctest runs a real notesd API server (store, blobs, broker,
// vaults, engine and rpcserver) on a temporary socket, for tests of
// clients such as notesctl. The socket, the serving goroutine and its
// stop are lodge's kit's (github.com/frostyard/lodge/kit/rpctest).
package rpctest

import (
	"context"
	"log/slog"
	"net"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	kitrpctest "github.com/frostyard/lodge/kit/rpctest"

	"github.com/frostyard/frostnotes/api"
	"github.com/frostyard/frostnotes/internal/blobs"
	"github.com/frostyard/frostnotes/internal/engine"
	"github.com/frostyard/frostnotes/internal/events"
	"github.com/frostyard/frostnotes/internal/hub"
	"github.com/frostyard/frostnotes/internal/rpcserver"
	"github.com/frostyard/frostnotes/internal/secrets"
	"github.com/frostyard/frostnotes/internal/store"
	"github.com/frostyard/frostnotes/internal/syncer"
	"github.com/frostyard/frostnotes/internal/vault"
)

// Name is what the test server reports from rpc.hello.
const Name = "notesd rpctest"

// Options configure a test server. Their Services, when set, edit the
// engine's services before the router is built, so a client's tests can
// serve a fake domain.
type Options struct {
	kitrpctest.Options[api.Services]
	// Log receives the server's logs; nil discards them.
	Log *slog.Logger
	// DataDir holds the database; empty means a fresh temporary directory.
	// Reusing one after Stop restarts notesd on the same data.
	DataDir string
	// Now, when set, is the clock for stored timestamps.
	Now func() time.Time
	// Vault tunes the vaults' actors (intervals, batch size); its DB,
	// Blobs, Log, Publish and, when Now is set, Now are filled in.
	Vault vault.Config
	// HubListen serves the hub's endpoints on a loopback port, at
	// Server.HubURL, as notesd --hub does; HubAddr, when set, is the
	// address, so a restarted hub keeps its URL.
	HubListen bool
	HubAddr   string
	// Crash is the simulation's crash points (vault.Config.Crash and its
	// syncer and hub counterparts).
	Crash func(point string) bool
	// Syncer tunes the device's sync loops (quiet periods, backoff, the
	// device's name); its stores, broker, secrets, log and, when Now is
	// set, Now are filled in.
	Syncer syncer.Config
}

// Server is a running test server; it stops when the test ends.
type Server struct {
	Socket string
	Run    *kitrpctest.Run
	DB     *store.DB
	Blobs  *blobs.Store
	Broker *events.Broker
	Vaults *vault.Manager
	RPC    *rpcserver.Server
	// Router serves every domain; pass it to RPC.ServeConn to test a
	// connection of another caller kind.
	Router *api.Router
	// Hub and Syncer are this notesd's hub role and sync loops; Secrets
	// keeps its device tokens, in a file. HubURL is set with HubListen.
	Hub     *hub.Hub
	Syncer  *syncer.Syncer
	Secrets secrets.Store
	HubURL  string

	stop func()
}

// Start runs a server with default options.
func Start(t testing.TB) *Server { return StartWith(t, Options{}) }

// StartWith runs a server with a fresh database, on a socket of its own
// (SocketPath).
func StartWith(t testing.TB, o Options) *Server {
	t.Helper()
	log := o.Log
	if log == nil {
		log = slog.New(slog.DiscardHandler)
	}
	data := o.DataDir
	if data == "" {
		data = t.TempDir()
	}
	db, err := store.Open(context.Background(), filepath.Join(data, "notesd.db"))
	if err != nil {
		t.Fatal(err)
	}
	if o.Now != nil {
		db.Now = o.Now
	}
	broker := events.NewBroker(db)
	db.OnCommit = broker.Publish
	srv := &Server{DB: db, Blobs: blobs.New(filepath.Join(data, "blobs")), Broker: broker}
	vc := o.Vault
	vc.DB, vc.Blobs, vc.Log, vc.Publish = db, srv.Blobs, log, broker.Publish
	vc.Crash = o.Crash
	if o.Now != nil {
		vc.Now = o.Now
	}
	srv.Vaults = vault.NewManager(vc)
	ctx, cancel := context.WithCancel(context.Background())
	if err := srv.Vaults.Start(ctx); err != nil {
		cancel()
		t.Fatal(err)
	}

	rpc := rpcserver.New(rpcserver.Options{Name: Name, Broker: broker, Logger: log})
	srv.RPC = rpc
	srv.Secrets = secrets.NewFile(filepath.Join(data, "secrets.json"))
	parts := filepath.Join(data, "parts")
	if err := os.MkdirAll(parts, 0o700); err != nil {
		t.Fatal(err)
	}
	hc := hub.Config{DB: db, Blobs: srv.Blobs, Vaults: srv.Vaults, RPC: rpc, Log: log, Now: o.Now, Crash: o.Crash}
	var hubLn net.Listener
	if o.HubListen {
		addr := o.HubAddr
		if addr == "" {
			addr = "127.0.0.1:0"
		}
		if hubLn, err = net.Listen("tcp", addr); err != nil {
			t.Fatal(err)
		}
		hc.Listen = hubLn.Addr().String()
		srv.HubURL = "http://" + hc.Listen
	}
	srv.Hub = hub.New(hc)
	sc := o.Syncer
	sc.DB, sc.Blobs, sc.Vaults, sc.Secrets, sc.Broker, sc.Publish, sc.Log = db, srv.Blobs, srv.Vaults, srv.Secrets, broker, broker.Publish, log
	sc.PartDir = parts
	sc.Crash = o.Crash
	if o.Now != nil {
		sc.Now = o.Now
	}
	srv.Syncer = syncer.New(sc)
	eng := engine.New(db, srv.Vaults, srv.Hub, srv.Syncer)
	services := api.Services{RPC: rpc, Events: rpc, Vault: eng.Vaults(), Note: eng.Notes(),
		Hub: eng.Hub(), Sync: eng.Sync(), Log: eng.Log()}
	o.Edit(&services)
	router, err := api.NewRouter(services)
	if err != nil {
		t.Fatal(err)
	}
	srv.Router = router
	srv.Hub.SetRouter(router)
	srv.Run = kitrpctest.Serve(t, func(ctx context.Context, ln net.Listener) error { return rpc.Serve(ctx, ln, router) })
	srv.Socket = srv.Run.Socket
	hubDone := make(chan error, 1)
	if hubLn != nil {
		go func() { hubDone <- srv.Hub.Serve(ctx, hubLn) }()
	} else {
		hubDone <- nil
	}
	if err := srv.Syncer.Start(ctx); err != nil {
		cancel()
		t.Fatal(err)
	}
	srv.stop = sync.OnceFunc(func() {
		srv.Run.Stop()
		cancel()
		select {
		case err := <-hubDone:
			if err != nil {
				t.Errorf("the hub's Serve: %v", err)
			}
		case <-time.After(5 * time.Second):
			t.Error("the hub's Serve did not stop")
		}
		srv.Syncer.Wait()
		srv.Vaults.Wait()
		_ = db.Close()
	})
	t.Cleanup(srv.stop)
	return srv
}

// Stop shuts the server down now, as a crash between transactions would:
// the socket closes and the database is closed. The test's cleanup does the
// same; Stop is for restarting on the same DataDir.
func (s *Server) Stop() { s.stop() }

// Dial connects and completes rpc.hello; the client closes when the test ends.
func (s *Server) Dial(t testing.TB) *api.Client {
	t.Helper()
	c, _, err := api.Dial(context.Background(), s.Socket, "rpctest")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = c.Close() })
	return c
}

// DialAs connects as a connection its listener authenticated as id (a
// session or an agent behind lodge, a device) over an in-process pipe,
// and completes rpc.hello; the client closes when the test ends.
func (s *Server) DialAs(t testing.TB, id api.Identity) *api.Client {
	t.Helper()
	c := api.NewClient(s.Run.Pipe(t, func(ctx context.Context, nc net.Conn) { s.RPC.ServeConn(ctx, nc, s.Router, id) }))
	t.Cleanup(func() { _ = c.Close() })
	if _, err := c.RPC().Hello(t.Context(), &api.RPCHelloParams{Protocol: api.Protocol, Client: "rpctest"}); err != nil {
		t.Fatal(err)
	}
	return c
}
