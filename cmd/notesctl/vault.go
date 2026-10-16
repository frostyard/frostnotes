package main

import (
	"fmt"
	"path/filepath"
	"strconv"

	"github.com/frostyard/clix"
	"github.com/frostyard/frostnotes/api"
	"github.com/spf13/cobra"
)

func newVaultCmd(opts *rootOptions) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "vault",
		Short: "List, register and forget vaults",
	}
	cmd.AddCommand(newVaultListCmd(opts), newVaultAddCmd(opts), newVaultRemoveCmd(opts))
	return cmd
}

func newVaultListCmd(opts *rootOptions) *cobra.Command {
	return &cobra.Command{
		Use:     "list",
		Aliases: []string{"ls"},
		Short:   "List registered vaults",
		Args:    cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			c, _, err := opts.dial(cmd.Context())
			if err != nil {
				return err
			}
			defer c.Close()
			vaults, err := c.Vault().List(cmd.Context(), &api.VaultListParams{})
			if err != nil {
				return err
			}
			if written, err := clix.OutputJSON(vaults); err != nil || written {
				return err
			}
			if len(vaults) == 0 {
				fmt.Fprintf(cmd.OutOrStdout(), "no vaults\n")
			}
			for _, vault := range vaults {
				fmt.Fprintf(cmd.OutOrStdout(), "%d\t%s\t%s\n", vault.ID, vault.Name, vault.Path)
			}
			return nil
		},
	}
}

func newVaultAddCmd(opts *rootOptions) *cobra.Command {
	var name string
	cmd := &cobra.Command{
		Use:   "add PATH",
		Short: "Register a vault directory",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			path, err := filepath.Abs(args[0])
			if err != nil {
				return fmt.Errorf("resolve vault path: %w", err)
			}
			params := &api.VaultAddParams{Path: path}
			if cmd.Flags().Changed("name") {
				params.Name = &name
			}
			c, _, err := opts.dial(cmd.Context())
			if err != nil {
				return err
			}
			defer c.Close()
			vault, err := c.Vault().Add(cmd.Context(), params)
			if err != nil {
				return err
			}
			if written, err := clix.OutputJSON(vault); err != nil || written {
				return err
			}
			fmt.Fprintf(cmd.OutOrStdout(), "added vault %d %q at %s\n", vault.ID, vault.Name, vault.Path)
			return nil
		},
	}
	cmd.Flags().StringVar(&name, "name", "", "Vault display name (default: directory base name)")
	return cmd
}

func newVaultRemoveCmd(opts *rootOptions) *cobra.Command {
	return &cobra.Command{
		Use:   "rm ID",
		Short: "Forget a registered vault",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			id, err := strconv.ParseInt(args[0], 10, 64)
			if err != nil {
				return fmt.Errorf("vault ID %q is not a number", args[0])
			}
			c, _, err := opts.dial(cmd.Context())
			if err != nil {
				return err
			}
			defer c.Close()
			if err := c.Vault().Remove(cmd.Context(), &api.VaultRemoveParams{ID: id}); err != nil {
				return err
			}
			fmt.Fprintf(cmd.OutOrStdout(), "removed vault %d\n", id)
			return nil
		},
	}
}
