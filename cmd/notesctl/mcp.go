package main

import (
	"github.com/frostyard/frostnotes/api"
	"github.com/frostyard/frostnotes/internal/mcpserver"
	"github.com/spf13/cobra"
)

// newMCPCmd serves the vault to an agent over MCP on stdin and stdout
// (ADR-0012, docs/specs/mcp.md). Nothing else may reach stdout.
func newMCPCmd(opts *rootOptions) *cobra.Command {
	var readOnly bool
	cmd := &cobra.Command{
		Use:   "mcp",
		Short: "Serve the vault to an agent over MCP on stdin and stdout",
		Long: `Serve the vault to an agent over the Model Context Protocol on stdin
and stdout, for an MCP host such as Claude Code to launch. The tools search,
read and list notes and their backlinks, and, unless --read-only, create
notes, edit them and append to the daily note. Each write is recorded in the
note's history under the MCP client's name, and notesctl revert undoes it.`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return opts.withVault(cmd, func(c *api.Client, vault int64) error {
				o := mcpserver.Options{Client: c, VaultID: vault, ReadOnly: readOnly, Version: version}
				return mcpserver.Serve(cmd.Context(), o, cmd.InOrStdin(), cmd.OutOrStdout())
			})
		},
	}
	cmd.Flags().BoolVar(&readOnly, "read-only", false, "Offer only the tools that read")
	return cmd
}
