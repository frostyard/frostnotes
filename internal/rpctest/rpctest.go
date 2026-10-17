// Package rpctest runs a real notesd API server (store, blobs, broker,
// vaults, engine and rpcserver) on a temporary socket, for tests of
// clients such as notesctl.
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

// Options configure a test server.
type Options struct {
	// Log receives the server's logs; nil discards them.
	Log *slog.Logger
	// DataDir holds the database; empty means a fresh temporary directory.
	// Reusing one after Stop restarts notesd on the same data.
	DataDir string
	// Now, when set, is the clock for stored timestamps.
	Now func() time.Time
	// Note, when set, serves the note domain in place of the engine's, so
	// a client's tests can run against a fake.
	Note api.NoteService
	// Hub, Sync and LogDomain, when set, serve the hub, sync and log
	// domains in place of the engine's, as Note does.
	Hub       api.HubService
	Sync      api.SyncService
	LogDomain api.LogService
	// Vault tunes the vaults' actors (intervals, batch size); its DB,
	// Blobs, Log, Publish and, when Now is set, Now are filled in.
	Vault vault.Config
	// HubListen serves the hub's endpoints on a loopback port, at
	// Server.HubURL, as notesd --hub does.
	HubListen bool
	// Syncer tunes the device's sync loops (quiet periods, backoff, the
	// device's name); its stores, broker, secrets, log and, when Now is
	// set, Now are filled in.
	Syncer syncer.Config
}

// Server is a running test server; it stops when the test ends.
type Server struct {
	Socket string
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
	if o.Now != nil {
		vc.Now = o.Now
	}
	srv.Vaults = vault.NewManager(vc)
	ctx, cancel := context.WithCancel(context.Background())
	if err := srv.Vaults.Start(ctx); err != nil {
		cancel()
		t.Fatal(err)
	}

	socket, cleanup, err := SocketPath()
	if err != nil {
		t.Fatal(err)
	}
	srv.Socket = socket
	ln, err := rpcserver.Listen(srv.Socket)
	if err != nil {
		t.Fatal(err)
	}
	rpc := rpcserver.New(rpcserver.Options{Name: Name, Broker: broker, Logger: log})
	srv.RPC = rpc
	srv.Secrets = secrets.NewFile(filepath.Join(data, "secrets.json"))
	parts := filepath.Join(data, "parts")
	if err := os.MkdirAll(parts, 0o700); err != nil {
		t.Fatal(err)
	}
	hc := hub.Config{DB: db, Blobs: srv.Blobs, Vaults: srv.Vaults, RPC: rpc, Log: log, Now: o.Now}
	var hubLn net.Listener
	if o.HubListen {
		if hubLn, err = net.Listen("tcp", "127.0.0.1:0"); err != nil {
			t.Fatal(err)
		}
		hc.Listen = hubLn.Addr().String()
		srv.HubURL = "http://" + hc.Listen
	}
	srv.Hub = hub.New(hc)
	sc := o.Syncer
	sc.DB, sc.Blobs, sc.Vaults, sc.Secrets, sc.Broker, sc.Publish, sc.Log = db, srv.Blobs, srv.Vaults, srv.Secrets, broker, broker.Publish, log
	sc.PartDir = parts
	if o.Now != nil {
		sc.Now = o.Now
	}
	srv.Syncer = syncer.New(sc)
	eng := engine.New(db, srv.Vaults, srv.Hub, srv.Syncer)
	note := o.Note
	if note == nil {
		note = eng.Notes()
	}
	services := api.Services{RPC: rpc, Events: rpc, Vault: eng.Vaults(), Note: note,
		Hub: eng.Hub(), Sync: eng.Sync(), Log: eng.Log()}
	if o.Hub != nil {
		services.Hub = o.Hub
	}
	if o.Sync != nil {
		services.Sync = o.Sync
	}
	if o.LogDomain != nil {
		services.Log = o.LogDomain
	}
	router, err := api.NewRouter(services)
	if err != nil {
		t.Fatal(err)
	}
	srv.Router = router
	srv.Hub.SetRouter(router)
	done := make(chan error, 1)
	go func() { done <- rpc.Serve(ctx, ln, router) }()
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
		cancel()
		for _, ch := range []chan error{done, hubDone} {
			select {
			case err := <-ch:
				if err != nil {
					t.Errorf("Serve: %v", err)
				}
			case <-time.After(5 * time.Second):
				t.Error("Serve did not stop")
			}
		}
		srv.Syncer.Wait()
		srv.Vaults.Wait()
		_ = db.Close()
		cleanup()
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
