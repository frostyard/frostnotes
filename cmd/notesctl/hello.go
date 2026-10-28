package main

import (
	"fmt"

	"github.com/frostyard/clix"
	"github.com/spf13/cobra"
)

// newHelloCmd builds the hello command: the smallest end-to-end check
// that notesd is running.
func newHelloCmd(opts *rootOptions) *cobra.Command {
	var build bool
	cmd := &cobra.Command{
		Use:   "hello",
		Short: "Check that notesd is running and print its identity",
		Long: `Check that notesd answers and print who it is. With --build, print only the
build it runs from: in the Flatpak, the app-commit of its instance (an empty
line outside it). It exits non-zero when notesd does not answer.`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			c, h, err := opts.dial(cmd.Context())
			if err != nil {
				return err
			}
			defer c.Close()
			if build {
				b := ""
				if h.Build != nil {
					b = *h.Build
				}
				fmt.Fprintln(cmd.OutOrStdout(), b)
				return nil
			}
			if written, err := clix.OutputJSON(h); err != nil || written {
				return err
			}
			fmt.Fprintf(cmd.OutOrStdout(), "%s (protocol %d)\n", h.Server, h.Protocol)
			return nil
		},
	}
	cmd.Flags().BoolVar(&build, "build", false, "print only the build notesd runs from (the Flatpak's app-commit), an empty line when it has none")
	return cmd
}
