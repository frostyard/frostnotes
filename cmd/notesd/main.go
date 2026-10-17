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
	"syscall"

	"github.com/frostyard/frostnotes/api"
	"github.com/frostyard/frostnotes/internal/blobs"
	"github.com/frostyard/frostnotes/internal/config"
	"github.com/frostyard/frostnotes/internal/devgw"
	"github.com/frostyard/frostnotes/internal/engine"
	"github.com/frostyard/frostnotes/internal/events"
	"github.com/frostyard/frostnotes/internal/rpcserver"
	"github.com/frostyard/frostnotes/internal/store"
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
	eng := engine.New(db, vaults)
	router, err := api.NewRouter(api.Services{RPC: srv, Events: srv, Vault: eng.Vaults(), Note: eng.Notes(),
		Hub: eng.Hub(), Sync: eng.Sync(), Log: eng.Log()})
	if err != nil {
		return err
	}
	if *devgwAddr != "" {
		// The gateway stands in for the app's bridge, so its connections
		// are local callers (docs/specs/rpc-protocol.md).
		serve := func(nc net.Conn) { srv.ServeConn(ctx, nc, router, api.CallerLocal) }
		go func() {
			if err := devgw.ListenAndServe(ctx, *devgwAddr, os.Getenv("FROSTNOTES_DEVGW_TOKEN"), serve, logger); err != nil {
				logger.Error("dev gateway stopped", "err", err)
			}
		}()
	}
	logger.Info("notesd started", "version", version, "socket", paths.Socket, "db", paths.DB)

	err = srv.Serve(ctx, ln, router)
	logger.Info("notesd stopped")
	return err
}
