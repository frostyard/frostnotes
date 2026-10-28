package main

import (
	"context"
	"os"

	"github.com/frostyard/frostnotes/api"
	"github.com/frostyard/frostnotes/internal/config"
	"github.com/spf13/cobra"
)

// rootOptions are the persistent flags every subcommand shares.
type rootOptions struct {
	socket string
	vault  string
}

func newRootCmd() *cobra.Command {
	opts := &rootOptions{}
	root := &cobra.Command{
		Use:           "notesctl",
		Short:         "Control and inspect notesd, the frostnotes engine",
		SilenceUsage:  true,
		SilenceErrors: true,
	}
	root.PersistentFlags().StringVar(&opts.socket, "socket", "", "notesd socket (default: $FROSTNOTES_SOCKET, else $XDG_RUNTIME_DIR/frostnotes/notesd.sock)")
	root.PersistentFlags().StringVar(&opts.vault, "vault", "", "The vault, by name or ID (default: the only one)")
	root.AddCommand(
		newHelloCmd(opts),
		newShutdownCmd(opts),
		newVaultCmd(opts),
		newHubCmd(opts),
		newSyncCmd(opts),
		newHistoryCmd(opts),
		newLsCmd(opts),
		newCatCmd(opts),
		newSearchCmd(opts),
		newBacklinksCmd(opts),
		newWriteCmd(opts),
		newMvCmd(opts),
		newRmCmd(opts),
		newVersionsCmd(opts),
		newRevertCmd(opts),
		newMCPCmd(opts),
	)
	return root
}

// socketPath is the socket to dial: --socket, else $FROSTNOTES_SOCKET, else
// the default for this machine (in the Flatpak, the app's shared one).
func (o *rootOptions) socketPath() (string, error) {
	if o.socket != "" {
		return o.socket, nil
	}
	paths, err := config.Resolve(os.Getenv)
	if err != nil {
		return "", err
	}
	return paths.Socket, nil
}

// dial connects to notesd and completes rpc.hello. The caller closes the client.
func (o *rootOptions) dial(ctx context.Context) (*api.Client, *api.Hello, error) {
	socket, err := o.socketPath()
	if err != nil {
		return nil, nil, err
	}
	return api.Dial(ctx, socket, "notesctl "+version)
}
