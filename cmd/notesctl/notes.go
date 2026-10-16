package main

import (
	"context"
	"fmt"
	"strconv"
	"strings"

	"github.com/frostyard/clix"
	"github.com/frostyard/frostnotes/api"
	"github.com/spf13/cobra"
)

// vaultID picks the vault --vault names, from vault.list.
func (o *rootOptions) vaultID(ctx context.Context, c *api.Client) (int64, error) {
	vaults, err := c.Vault().List(ctx, &api.VaultListParams{})
	if err != nil {
		return 0, err
	}
	if o.vault == "" {
		switch len(vaults) {
		case 0:
			return 0, fmt.Errorf("no vaults registered; use notesctl vault add")
		case 1:
			return vaults[0].ID, nil
		default:
			return 0, fmt.Errorf("several vaults registered; choose one with --vault")
		}
	}
	if id, err := strconv.ParseInt(o.vault, 10, 64); err == nil {
		for _, vault := range vaults {
			if vault.ID == id {
				return id, nil
			}
		}
	}
	var id int64
	matches := 0
	for _, vault := range vaults {
		if strings.EqualFold(vault.Name, o.vault) {
			id = vault.ID
			matches++
		}
	}
	switch matches {
	case 0:
		return 0, fmt.Errorf("no vault matches %q", o.vault)
	case 1:
		return id, nil
	default:
		return 0, fmt.Errorf("several vaults match %q; use --vault with an ID", o.vault)
	}
}

// withVault dials notesd, picks the vault and runs fn; it closes the client.
func (o *rootOptions) withVault(cmd *cobra.Command, fn func(c *api.Client, vault int64) error) error {
	c, _, err := o.dial(cmd.Context())
	if err != nil {
		return err
	}
	defer c.Close()
	vault, err := o.vaultID(cmd.Context(), c)
	if err != nil {
		return err
	}
	return fn(c, vault)
}

func newLsCmd(opts *rootOptions) *cobra.Command {
	var tag, prop string
	cmd := &cobra.Command{
		Use:   "ls [FOLDER]",
		Short: "List notes in a vault",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return opts.withVault(cmd, func(c *api.Client, vault int64) error {
				params := &api.NoteListParams{VaultID: vault}
				if len(args) == 1 {
					params.Folder = &args[0]
				}
				if cmd.Flags().Changed("tag") {
					params.Tag = &tag
				}
				if cmd.Flags().Changed("prop") {
					key, value, hasValue := strings.Cut(prop, "=")
					params.Property = &key
					if hasValue {
						params.Value = &value
					}
				}
				entries, err := c.Note().List(cmd.Context(), params)
				if err != nil {
					return err
				}
				if written, err := clix.OutputJSON(entries); err != nil || written {
					return err
				}
				for _, entry := range entries {
					if _, err := fmt.Fprintln(cmd.OutOrStdout(), entry.Path); err != nil {
						return err
					}
				}
				return nil
			})
		},
	}
	cmd.Flags().StringVar(&tag, "tag", "", "Only notes with this tag")
	cmd.Flags().StringVar(&prop, "prop", "", "Only notes with KEY or KEY=VALUE")
	return cmd
}

func newCatCmd(opts *rootOptions) *cobra.Command {
	return &cobra.Command{
		Use:   "cat PATH",
		Short: "Print a note's content exactly",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return opts.withVault(cmd, func(c *api.Client, vault int64) error {
				note, err := c.Note().Read(cmd.Context(), &api.NoteReadParams{VaultID: vault, Path: args[0]})
				if err != nil {
					return err
				}
				if written, err := clix.OutputJSON(note); err != nil || written {
					return err
				}
				_, err = fmt.Fprint(cmd.OutOrStdout(), note.Content)
				return err
			})
		},
	}
}

func newSearchCmd(opts *rootOptions) *cobra.Command {
	var limit int64
	cmd := &cobra.Command{
		Use:   "search QUERY...",
		Short: "Search notes in a vault",
		Args:  cobra.MinimumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return opts.withVault(cmd, func(c *api.Client, vault int64) error {
				params := &api.NoteSearchParams{VaultID: vault, Query: strings.Join(args, " ")}
				if cmd.Flags().Changed("limit") {
					params.Limit = &limit
				}
				hits, err := c.Note().Search(cmd.Context(), params)
				if err != nil {
					return err
				}
				if written, err := clix.OutputJSON(hits); err != nil || written {
					return err
				}
				for _, hit := range hits {
					if _, err := fmt.Fprintf(cmd.OutOrStdout(), "%s\t%s\n", hit.Path, formatSnippet(hit.Snippet)); err != nil {
						return err
					}
				}
				return nil
			})
		},
	}
	cmd.Flags().Int64Var(&limit, "limit", 0, "Maximum number of search results (default: 20)")
	return cmd
}

func formatSnippet(parts []api.SnippetPart) string {
	var snippet strings.Builder
	for _, part := range parts {
		if part.Match {
			snippet.WriteString("**")
		}
		snippet.WriteString(part.Text)
		if part.Match {
			snippet.WriteString("**")
		}
	}
	return strings.NewReplacer("\r\n", " ", "\n", " ", "\r", " ").Replace(snippet.String())
}

func newBacklinksCmd(opts *rootOptions) *cobra.Command {
	return &cobra.Command{
		Use:   "backlinks PATH",
		Short: "Show links to a file",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return opts.withVault(cmd, func(c *api.Client, vault int64) error {
				backlinks, err := c.Note().Backlinks(cmd.Context(), &api.NoteBacklinksParams{VaultID: vault, Path: args[0]})
				if err != nil {
					return err
				}
				if written, err := clix.OutputJSON(backlinks); err != nil || written {
					return err
				}
				for _, backlink := range backlinks {
					if _, err := fmt.Fprintf(cmd.OutOrStdout(), "%s:%d\t%s\n", backlink.Source, backlink.Link.Line, formatLink(backlink.Link)); err != nil {
						return err
					}
				}
				return nil
			})
		},
	}
}

func formatLink(link api.Link) string {
	target := link.Target + link.Subpath
	var text string
	if link.Kind == api.LinkKindMarkdown {
		text = "[" + link.Alias + "](" + target + ")"
	} else {
		if link.Alias != "" {
			target += "|" + link.Alias
		}
		text = "[[" + target + "]]"
	}
	if link.Embed {
		text = "!" + text
	}
	if link.Kind == api.LinkKindProperty {
		text = link.Key + ": " + text
	}
	return text
}
