package main

import (
	"encoding/json/jsontext"
	"encoding/json/v2"
	"fmt"
	"io"
	"strings"

	"github.com/frostyard/clix"
	"github.com/frostyard/frostnotes/api"
	"github.com/spf13/cobra"
)

func newQueryCmd(opts *rootOptions) *cobra.Command {
	var view, from string
	var limit int64
	cmd := &cobra.Command{
		Use:   "query PATH",
		Short: "Evaluate a view of an Obsidian base",
		Long: `Evaluate a view of a base (a .base file) against the vault: the subset of
Obsidian Bases that notesd supports (docs/specs/bases.md). PATH is the
vault path of the .base file; - reads the base's YAML from standard input.
--from names the note that embeds the base, which sets this.`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return opts.withVault(cmd, func(c *api.Client, vault int64) error {
				params := &api.NoteQueryParams{VaultID: vault}
				if args[0] == "-" {
					data, err := io.ReadAll(cmd.InOrStdin())
					if err != nil {
						return fmt.Errorf("read the base from stdin: %w", err)
					}
					source := string(data)
					params.Source = &source
				} else {
					params.Path = &args[0]
				}
				if cmd.Flags().Changed("view") {
					params.View = &view
				}
				if cmd.Flags().Changed("from") {
					params.From = &from
				}
				if cmd.Flags().Changed("limit") {
					params.Limit = &limit
				}
				res, err := c.Note().Query(cmd.Context(), params)
				if err != nil {
					return err
				}
				if written, err := clix.OutputJSON(res); err != nil || written {
					return err
				}
				return printQuery(cmd.OutOrStdout(), res)
			})
		},
	}
	cmd.Flags().StringVar(&view, "view", "", "The view's name (default: the first view)")
	cmd.Flags().Int64Var(&limit, "limit", 0, "At most this many rows, 1 to 1000 (default 1000)")
	cmd.Flags().StringVar(&from, "from", "", "The note that embeds the base: it sets this")
	return cmd
}

// printQuery writes a result as a table: the display names, then a line for
// each row with its values separated by tabs, with a "## VALUE" line before
// each group, and a count.
func printQuery(w io.Writer, res *api.QueryResult) error {
	if res.Notice != nil {
		_, err := fmt.Fprintf(w, "notice: %s\n", *res.Notice)
		return err
	}
	var b strings.Builder
	names := make([]string, len(res.Columns))
	for i, c := range res.Columns {
		names[i] = cell(c.DisplayName)
	}
	b.WriteString(strings.Join(names, "\t") + "\n")
	groups := res.Groups
	left := 0 // rows left in the current group
	for _, row := range res.Rows {
		for left == 0 && len(groups) > 0 {
			g := groups[0]
			groups = groups[1:]
			left = int(g.Count)
			b.WriteString(strings.TrimRight("## "+showJSON(g.Value), " ") + "\n")
		}
		left = max(left-1, 0)
		cells := make([]string, len(row.Values))
		for i, v := range row.Values {
			cells[i] = showJSON(v)
		}
		b.WriteString(strings.Join(cells, "\t") + "\n")
	}
	fmt.Fprintf(&b, "%d of %d rows\n", len(res.Rows), res.Total)
	_, err := io.WriteString(w, b.String())
	return err
}

// showJSON writes a value as a table cell: text as it is, a list joined
// with ", ", null as nothing.
func showJSON(v jsontext.Value) string {
	var x any
	if len(v) == 0 || json.Unmarshal(v, &x) != nil {
		return ""
	}
	return cell(showAny(x))
}

func showAny(x any) string {
	switch x := x.(type) {
	case nil:
		return ""
	case string:
		return x
	case []any:
		items := make([]string, len(x))
		for i, item := range x {
			items[i] = showAny(item)
		}
		return strings.Join(items, ", ")
	}
	out, err := json.Marshal(x)
	if err != nil {
		return ""
	}
	return string(out)
}

// cell keeps a value on its line and in its column.
func cell(s string) string {
	return strings.NewReplacer("\t", " ", "\n", " ", "\r", " ").Replace(s)
}
