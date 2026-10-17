// Command notesd is the frostnotes engine: a per-user daemon that owns
// vaults, the local store and the RPC socket the app and notesctl use
// (docs/design/overview.md).
package main

import (
	"context"
	"flag"
	"fmt"
	"log/slog"
	"net"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"

	"github.com/frostyard/frostnotes/api"
	"github.com/frostyard/frostnotes/internal/blobs"
	"github.com/frostyard/frostnotes/internal/blobxfer"
	"github.com/frostyard/frostnotes/internal/config"
	"github.com/frostyard/frostnotes/internal/devgw"
	"github.com/frostyard/frostnotes/internal/engine"
	"github.com/frostyard/frostnotes/internal/events"
	"github.com/frostyard/frostnotes/internal/hub"
	"github.com/frostyard/frostnotes/internal/rpcserver"
	"github.com/frostyard/frostnotes/internal/secrets"
	"github.com/frostyard/frostnotes/internal/store"
	"github.com/frostyard/frostnotes/internal/syncer"
	"github.com/frostyard/frostnotes/internal/vault"
)

// Set via ldflags at build time.
var (
	version = "dev"
	commit  = "none"
)

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	if err := run(ctx, os.Args[1:]); err != nil {
		fmt.Fprintln(os.Stderr, "notesd:", err)
		os.Exit(1)
	}
}

func run(ctx context.Context, args []string) error {
	fs := flag.NewFlagSet("notesd", flag.ContinueOnError)
	showVersion := fs.Bool("version", false, "print the version and exit")
	debug := fs.Bool("debug", false, "log at debug level")
	devgwAddr := fs.String("devgw", "", "also serve the API over a WebSocket on this loopback address, for UI development "+
		"in a browser; needs FROSTNOTES_DEVGW_TOKEN (at least 16 characters)")
	hubAddr := fs.String("hub", "", "also serve the hub's sync endpoints on this address (plain HTTP: put TLS in front "+
		"for devices on other machines; docs/specs/sync-protocol.md)")
	maxBlob := fs.Int64("hub-max-blob", blobxfer.DefaultMaxSize, "with -hub, the largest file a device may upload, in bytes")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *showVersion {
		fmt.Printf("notesd %s (%s)\n", version, commit)
		return nil
	}
	level := slog.LevelInfo
	if *debug {
		level = slog.LevelDebug
	}
	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: level}))

	paths, err := config.Resolve(os.Getenv)
	if err != nil {
		return err
	}
	for _, dir := range []string{paths.DataDir, paths.Blobs} {
		if err := os.MkdirAll(dir, 0o700); err != nil {
			return err
		}
	}
	db, err := store.Open(ctx, paths.DB)
	if err != nil {
		return err
	}
	defer func() { _ = db.Close() }()
	broker := events.NewBroker(db)
	db.OnCommit = broker.Publish

	// One actor per vault scans and watches it and makes its writes; the
	// manager also prunes versions daily (docs/design/vault.md).
	vaults := vault.NewManager(vault.Config{DB: db, Blobs: blobs.New(paths.Blobs), Log: logger, Publish: broker.Publish})
	vctx, stopVaults := context.WithCancel(ctx)
	defer func() { stopVaults(); vaults.Wait() }()
	if err := vaults.Start(vctx); err != nil {
		return err
	}

	ln, err := rpcserver.Listen(paths.Socket)
	if err != nil {
		return err
	}
	srv := rpcserver.New(rpcserver.Options{Name: "notesd " + version, Broker: broker, Logger: logger})
	// The device side of sync runs on every notesd; the hub's listener only
	// with -hub (docs/design/sync.md).
	keys, err := secrets.Open(ctx, filepath.Join(paths.DataDir, "secrets.json"), logger)
	if err != nil {
		return err
	}
	parts := filepath.Join(paths.DataDir, "parts")
	if err := os.MkdirAll(parts, 0o700); err != nil {
		return err
	}
	blobStore := blobs.New(paths.Blobs)
	h := hub.New(hub.Config{DB: db, Blobs: blobStore, Vaults: vaults, RPC: srv, Log: logger, Listen: *hubAddr, MaxBlob: *maxBlob})
	syncs := syncer.New(syncer.Config{DB: db, Blobs: blobStore, Vaults: vaults, Secrets: keys, Broker: broker,
		Publish: broker.Publish, Log: logger, PartDir: parts, Client: "notesd " + version})
	eng := engine.New(db, vaults, h, syncs)
	router, err := api.NewRouter(api.Services{RPC: srv, Events: srv, Vault: eng.Vaults(), Note: eng.Notes(),
		Hub: eng.Hub(), Sync: eng.Sync(), Log: eng.Log()})
	if err != nil {
		return err
	}
	h.SetRouter(router)
	sctx, stopSync := context.WithCancel(ctx)
	defer func() { stopSync(); syncs.Wait() }()
	if err := syncs.Start(sctx); err != nil {
		return err
	}
	if *hubAddr != "" {
		hubLn, err := net.Listen("tcp", *hubAddr)
		if err != nil {
			return fmt.Errorf("hub: %w", err)
		}
		go func() {
			if err := h.Serve(ctx, hubLn); err != nil {
				logger.Error("hub stopped", "err", err)
			}
		}()
	}
	if *devgwAddr != "" {
		// The gateway stands in for the app's bridge, so its connections
		// are local callers (docs/specs/rpc-protocol.md).
		serve := func(nc net.Conn) { srv.ServeConn(ctx, nc, router, api.CallerLocal) }
		go func() {
			if err := devgw.ListenAndServe(ctx, *devgwAddr, os.Getenv("FROSTNOTES_DEVGW_TOKEN"), serve, blobStore, logger); err != nil {
				logger.Error("dev gateway stopped", "err", err)
			}
		}()
	}
	logger.Info("notesd started", "version", version, "socket", paths.Socket, "db", paths.DB)

	err = srv.Serve(ctx, ln, router)
	logger.Info("notesd stopped")
	return err
}
