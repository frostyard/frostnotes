package main

import (
	"fmt"
	"io"
	"os"
	"strconv"

	"github.com/frostyard/clix"
	"github.com/frostyard/frostnotes/api"
	"github.com/spf13/cobra"
)

func newWriteCmd(opts *rootOptions) *cobra.Command {
	var base, file string
	var create bool
	cmd := &cobra.Command{
		Use:   "write PATH",
		Short: "Write a note from stdin or a file, naming its base hash",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if create && cmd.Flags().Changed("base") {
				return fmt.Errorf("--create cannot be used with --base")
			}
			if !create && base == "" {
				return fmt.Errorf("--base is required unless --create is set")
			}
			var content []byte
			var err error
			if cmd.Flags().Changed("file") {
				content, err = os.ReadFile(file)
				if err != nil {
					return fmt.Errorf("read content from %q: %w", file, err)
				}
			} else {
				content, err = io.ReadAll(cmd.InOrStdin())
				if err != nil {
					return fmt.Errorf("read content from stdin: %w", err)
				}
			}
			return opts.withVault(cmd, func(c *api.Client, vault int64) error {
				var res *api.WriteResult
				if create {
					res, err = c.Note().Create(cmd.Context(), &api.NoteCreateParams{
						VaultID: vault, Path: args[0], Content: string(content),
					})
				} else {
					res, err = c.Note().Write(cmd.Context(), &api.NoteWriteParams{
						VaultID: vault, Path: args[0], Base: base, Content: string(content),
					})
				}
				if err != nil {
					return err
				}
				return printWriteResult(cmd, res, create)
			})
		},
	}
	cmd.Flags().StringVar(&base, "base", "", "Hash of the content this write started from")
	cmd.Flags().BoolVar(&create, "create", false, "Create a new note")
	cmd.Flags().StringVar(&file, "file", "", "Read content from this file instead of stdin")
	return cmd
}

func printWriteResult(cmd *cobra.Command, res *api.WriteResult, create bool) error {
	if written, err := clix.OutputJSON(res); err != nil || written {
		return err
	}
	action, suffix := "wrote", ""
	if create {
		action = "created"
	} else if res.Merged {
		action, suffix = "merged", conflictSuffix(res.Conflicts)
	}
	_, err := fmt.Fprintf(cmd.OutOrStdout(), "%s %s %s%s\n", action, res.Path, res.Hash, suffix)
	return err
}

func conflictSuffix(conflicts int64) string {
	switch conflicts {
	case 0:
		return ""
	case 1:
		return ", 1 conflict"
	default:
		return fmt.Sprintf(", %d conflicts", conflicts)
	}
}

func newMvCmd(opts *rootOptions) *cobra.Command {
	var updateLinks bool
	cmd := &cobra.Command{
		Use:   "mv PATH TO",
		Short: "Move a note and optionally update links to it",
		Args:  cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			return opts.withVault(cmd, func(c *api.Client, vault int64) error {
				params := &api.NoteMoveParams{VaultID: vault, Path: args[0], To: args[1]}
				if cmd.Flags().Changed("update-links") {
					params.UpdateLinks = &updateLinks
				}
				res, err := c.Note().Move(cmd.Context(), params)
				if err != nil {
					return err
				}
				if written, err := clix.OutputJSON(res); err != nil || written {
					return err
				}
				if _, err := fmt.Fprintf(cmd.OutOrStdout(), "moved %s to %s\n", args[0], res.Path); err != nil {
					return err
				}
				for _, path := range res.Rewritten {
					if _, err := fmt.Fprintf(cmd.OutOrStdout(), "rewrote links in %s\n", path); err != nil {
						return err
					}
				}
				return nil
			})
		},
	}
	cmd.Flags().BoolVar(&updateLinks, "update-links", false, "Rewrite links to the moved note")
	return cmd
}

func newRmCmd(opts *rootOptions) *cobra.Command {
	var base string
	cmd := &cobra.Command{
		Use:   "rm PATH",
		Short: "Delete a note, preserving its versions",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if base == "" {
				return fmt.Errorf("--base is required")
			}
			return opts.withVault(cmd, func(c *api.Client, vault int64) error {
				if err := c.Note().Delete(cmd.Context(), &api.NoteDeleteParams{
					VaultID: vault, Path: args[0], Base: base,
				}); err != nil {
					return err
				}
				if written, err := clix.OutputJSON(nil); err != nil || written {
					return err
				}
				_, err := fmt.Fprintf(cmd.OutOrStdout(), "deleted %s\n", args[0])
				return err
			})
		},
	}
	cmd.Flags().StringVar(&base, "base", "", "Current content hash of the note")
	return cmd
}

func newVersionsCmd(opts *rootOptions) *cobra.Command {
	var limit int64
	cmd := &cobra.Command{
		Use:   "versions PATH",
		Short: "List a note's versions, newest first",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return opts.withVault(cmd, func(c *api.Client, vault int64) error {
				params := &api.NoteVersionsParams{VaultID: vault, Path: args[0]}
				if cmd.Flags().Changed("limit") {
					params.Limit = &limit
				}
				versions, err := c.Note().Versions(cmd.Context(), params)
				if err != nil {
					return err
				}
				if written, err := clix.OutputJSON(versions); err != nil || written {
					return err
				}
				for _, version := range versions {
					if _, err := fmt.Fprintln(cmd.OutOrStdout(), formatVersion(version)); err != nil {
						return err
					}
				}
				return nil
			})
		},
	}
	cmd.Flags().Int64Var(&limit, "limit", 0, "Maximum number of versions (default: 50)")
	return cmd
}

func formatVersion(version api.Version) string {
	author := version.Author
	if author == "" {
		author = "-"
	}
	state := version.Hash[:min(12, len(version.Hash))]
	if version.Deleted {
		state = "deleted"
	} else if version.Conflict {
		state += " conflict"
	}
	return fmt.Sprintf("%d\t%s\t%s\t%s\t%s\t%s", version.ID,
		version.At.UTC().Format("2006-01-02T15:04:05Z"), version.Source, author, state, version.Path)
}

func newRevertCmd(opts *rootOptions) *cobra.Command {
	var base string
	cmd := &cobra.Command{
		Use:   "revert PATH VERSION",
		Short: "Restore a note's version, naming its current base hash unless deleted",
		Args:  cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			version, err := strconv.ParseInt(args[1], 10, 64)
			if err != nil {
				return fmt.Errorf("version %q is not an integer: %w", args[1], err)
			}
			return opts.withVault(cmd, func(c *api.Client, vault int64) error {
				res, err := c.Note().Revert(cmd.Context(), &api.NoteRevertParams{
					VaultID: vault, Path: args[0], Version: version, Base: base,
				})
				if err != nil {
					return err
				}
				if written, err := clix.OutputJSON(res); err != nil || written {
					return err
				}
				suffix := ""
				if res.Merged {
					suffix = ", merged" + conflictSuffix(res.Conflicts)
				}
				_, err = fmt.Fprintf(cmd.OutOrStdout(), "reverted %s to version %d: %s%s\n", res.Path, version, res.Hash, suffix)
				return err
			})
		},
	}
	cmd.Flags().StringVar(&base, "base", "", "Current content hash (omit for a deleted note)")
	return cmd
}
