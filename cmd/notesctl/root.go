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
		newVaultCmd(opts),
		newLsCmd(opts),
		newCatCmd(opts),
		newSearchCmd(opts),
		newBacklinksCmd(opts),
	)
	return root
}

// dial connects to notesd and completes rpc.hello. The caller closes the client.
func (o *rootOptions) dial(ctx context.Context) (*api.Client, *api.Hello, error) {
	socket := o.socket
	if socket == "" {
		paths, err := config.Resolve(os.Getenv)
		if err != nil {
			return nil, nil, err
		}
		socket = paths.Socket
	}
	return api.Dial(ctx, socket, "notesctl "+version)
}
