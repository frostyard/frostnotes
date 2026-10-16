// Command notesd is the frostnotes engine: a per-user daemon that owns
// vaults, the local store and the RPC socket the app and notesctl use
// (docs/design/overview.md).
package main

import (
	"context"
	"flag"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"syscall"

	"github.com/frostyard/frostnotes/api"
	"github.com/frostyard/frostnotes/internal/config"
	"github.com/frostyard/frostnotes/internal/engine"
	"github.com/frostyard/frostnotes/internal/events"
	"github.com/frostyard/frostnotes/internal/rpcserver"
	"github.com/frostyard/frostnotes/internal/store"
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

	ln, err := rpcserver.Listen(paths.Socket)
	if err != nil {
		return err
	}
	srv := rpcserver.New(rpcserver.Options{Name: "notesd " + version, Broker: broker, Logger: logger})
	eng := engine.New(db)
	router, err := api.NewRouter(api.Services{RPC: srv, Events: srv, Vault: eng.Vaults()})
	if err != nil {
		return err
	}
	logger.Info("notesd started", "version", version, "socket", paths.Socket, "db", paths.DB)
	err = srv.Serve(ctx, ln, router)
	logger.Info("notesd stopped")
	return err
}
