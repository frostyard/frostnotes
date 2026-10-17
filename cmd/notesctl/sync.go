package main

import (
	"fmt"
	"io"

	"github.com/frostyard/clix"
	"github.com/frostyard/frostnotes/api"
	"github.com/spf13/cobra"
)

func newSyncCmd(opts *rootOptions) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "sync",
		Short: "Inspect and manage vault sync with a hub",
	}
	cmd.AddCommand(newSyncStatusCmd(opts), newSyncConflictsCmd(opts),
		newSyncResolveCmd(opts), newSyncResetCmd(opts), newSyncLeaveCmd(opts),
		newSyncVerifyCmd(opts))
	return cmd
}

func newSyncStatusCmd(opts *rootOptions) *cobra.Command {
	return &cobra.Command{
		Use:   "status",
		Short: "Show sync status for all vaults or the selected vault",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			c, _, err := opts.dial(cmd.Context())
			if err != nil {
				return err
			}
			defer c.Close()
			params := &api.SyncStatusParams{}
			if cmd.Flags().Changed("vault") {
				vault, err := opts.vaultID(cmd.Context(), c)
				if err != nil {
					return err
				}
				params.VaultID = &vault
			}
			statuses, err := c.Sync().Status(cmd.Context(), params)
			if err != nil {
				return err
			}
			if written, err := clix.OutputJSON(statuses); err != nil || written {
				return err
			}
			if len(statuses) == 0 {
				_, err := fmt.Fprintln(cmd.OutOrStdout(), "no synced vaults")
				return err
			}
			for _, status := range statuses {
				if err := printSyncStatus(cmd.OutOrStdout(), status); err != nil {
					return err
				}
			}
			return nil
		},
	}
}

func printSyncStatus(out io.Writer, status api.SyncStatus) error {
	if _, err := fmt.Fprintf(out, "%d\t%s\t%s\t%s\t%d/%d\t%d pending\t%d conflicted\n",
		status.VaultID, status.HubVault, status.URL, status.State, status.Cursor,
		status.HubSeq, status.Pending, status.Conflicts); err != nil {
		return err
	}
	for _, refusal := range status.Refused {
		if _, err := fmt.Fprintf(out, "\trefused %s: %s\n", refusal.Path, refusal.Reason); err != nil {
			return err
		}
	}
	for _, path := range status.Skipped {
		if _, err := fmt.Fprintf(out, "\tskipped %s\n", path); err != nil {
			return err
		}
	}
	if status.Error != nil {
		_, err := fmt.Fprintf(out, "\terror: %s\n", *status.Error)
		return err
	}
	return nil
}

func newSyncConflictsCmd(opts *rootOptions) *cobra.Command {
	return &cobra.Command{
		Use:   "conflicts",
		Short: "List conflicted files and their revisions",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return opts.withVault(cmd, func(c *api.Client, vault int64) error {
				conflicts, err := c.Sync().Conflicts(cmd.Context(), &api.SyncConflictsParams{VaultID: vault})
				if err != nil {
					return err
				}
				if written, err := clix.OutputJSON(conflicts); err != nil || written {
					return err
				}
				if len(conflicts) == 0 {
					_, err := fmt.Fprintln(cmd.OutOrStdout(), "no conflicts")
					return err
				}
				for _, conflict := range conflicts {
					if _, err := fmt.Fprintln(cmd.OutOrStdout(), conflict.Path); err != nil {
						return err
					}
					for _, revision := range conflict.Revisions {
						if _, err := fmt.Fprintf(cmd.OutOrStdout(), "\t%d\t%s\t%s\t%s\n", revision.Seq,
							revision.At.UTC().Format("2006-01-02T15:04:05Z"), revision.DeviceName,
							formatRevisionAuthor(revision.Author)); err != nil {
							return err
						}
					}
				}
				return nil
			})
		},
	}
}

func newSyncResolveCmd(opts *rootOptions) *cobra.Command {
	return &cobra.Command{
		Use:   "resolve PATH",
		Short: "Clear a file's conflict flag on the hub",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return opts.withVault(cmd, func(c *api.Client, vault int64) error {
				if err := c.Sync().Resolve(cmd.Context(), &api.SyncResolveParams{VaultID: vault, Path: args[0]}); err != nil {
					return err
				}
				_, err := fmt.Fprintf(cmd.OutOrStdout(), "resolved %s\n", args[0])
				return err
			})
		},
	}
}

func newSyncResetCmd(opts *rootOptions) *cobra.Command {
	return &cobra.Command{
		Use:   "reset",
		Short: "Reset a vault's sync by leaving and rejoining its hub",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return opts.withVault(cmd, func(c *api.Client, vault int64) error {
				status, err := c.Sync().Reset(cmd.Context(), &api.SyncResetParams{VaultID: vault})
				if err != nil {
					return err
				}
				_, err = fmt.Fprintf(cmd.OutOrStdout(), "reset vault %d: %s\n", vault, status.State)
				return err
			})
		},
	}
}

func newSyncLeaveCmd(opts *rootOptions) *cobra.Command {
	return &cobra.Command{
		Use:   "leave",
		Short: "Stop syncing a vault, keeping its files",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return opts.withVault(cmd, func(c *api.Client, vault int64) error {
				if err := c.Sync().Leave(cmd.Context(), &api.SyncLeaveParams{VaultID: vault}); err != nil {
					return err
				}
				_, err := fmt.Fprintf(cmd.OutOrStdout(), "vault %d no longer syncs; its files stay\n", vault)
				return err
			})
		},
	}
}

func newSyncVerifyCmd(opts *rootOptions) *cobra.Command {
	return &cobra.Command{
		Use:   "verify",
		Short: "Check a vault's files and pushes against its hub",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return opts.withVault(cmd, func(c *api.Client, vault int64) error {
				report, err := c.Sync().Verify(cmd.Context(), &api.SyncVerifyParams{VaultID: vault})
				if err != nil {
					return err
				}
				written, err := clix.OutputJSON(report)
				if err != nil {
					return err
				}
				if !written {
					if err := printVerifyReport(cmd.OutOrStdout(), report); err != nil {
						return err
					}
				}
				if !report.Ok {
					return fmt.Errorf("vault %d differs from its hub", vault)
				}
				return nil
			})
		},
	}
}

func printVerifyReport(out io.Writer, report *api.VerifyReport) error {
	for _, path := range report.Differing {
		if _, err := fmt.Fprintf(out, "differs\t%s\n", path); err != nil {
			return err
		}
	}
	for _, push := range report.Unrecorded {
		if _, err := fmt.Fprintf(out, "unrecorded\tpush %d\n", push); err != nil {
			return err
		}
	}
	for _, path := range report.Conflicts {
		if _, err := fmt.Fprintf(out, "conflicted\t%s\n", path); err != nil {
			return err
		}
	}
	if report.Ok {
		_, err := fmt.Fprintf(out, "ok: %d files match the hub\n", report.Files)
		return err
	}
	return nil
}

func newHistoryCmd(opts *rootOptions) *cobra.Command {
	var limit int64
	cmd := &cobra.Command{
		Use:   "history PATH",
		Short: "List a file's history on the hub, newest first",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return opts.withVault(cmd, func(c *api.Client, vault int64) error {
				params := &api.SyncHistoryParams{VaultID: vault, Path: args[0]}
				if cmd.Flags().Changed("limit") {
					params.Limit = &limit
				}
				revisions, err := c.Sync().History(cmd.Context(), params)
				if err != nil {
					return err
				}
				if written, err := clix.OutputJSON(revisions); err != nil || written {
					return err
				}
				for _, revision := range revisions {
					if _, err := fmt.Fprintln(cmd.OutOrStdout(), formatRevision(revision)); err != nil {
						return err
					}
				}
				return nil
			})
		},
	}
	cmd.Flags().Int64Var(&limit, "limit", 0, "Maximum number of revisions (default: 50, at most 200)")
	return cmd
}

func formatRevision(revision api.Revision) string {
	state := revision.Hash[:min(12, len(revision.Hash))]
	if revision.Op == api.RevisionOpDelete {
		state = "deleted"
	}
	if revision.Pruned {
		state += " pruned"
	}
	return fmt.Sprintf("%d\t%s\t%s\t%s\t%s\t%s\t%s", revision.Seq,
		revision.At.UTC().Format("2006-01-02T15:04:05Z"), revision.Op,
		revision.DeviceName, formatRevisionAuthor(revision.Author), state, revision.Path)
}

func formatRevisionAuthor(author string) string {
	if author == "" {
		return "-"
	}
	return author
}
