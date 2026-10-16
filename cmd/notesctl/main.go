// Command notesctl is the command-line client for notesd: it speaks the
// same RPC API as the app (docs/specs/rpc-api.md).
package main

import (
	"os"

	"github.com/frostyard/clix"
)

// Set via ldflags at build time.
var (
	version = "dev"
	commit  = "none"
	date    = "unknown"
	builtBy = "local"
)

func main() {
	app := clix.App{Version: version, Commit: commit, Date: date, BuiltBy: builtBy}
	if err := app.Run(newRootCmd()); err != nil {
		os.Exit(1)
	}
}
