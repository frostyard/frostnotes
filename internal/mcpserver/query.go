package mcpserver

import (
	"context"
	"encoding/json/v2"
	"fmt"
	"slices"
	"strings"

	"github.com/frostyard/frostnotes/api"
	sdk "github.com/modelcontextprotocol/go-sdk/mcp"
)

// queryNotes runs note.query for an agent (docs/specs/mcp.md#queries): an
// agent's filters as the source of a base with one table view, or a .base
// file's view. A view with a notice is an error, never empty files.
func (t *tools) queryNotes(ctx context.Context, _ *sdk.CallToolRequest, in queryIn) (*sdk.CallToolResult, queryOut, error) {
	if in.Limit == 0 {
		in.Limit = 200
	}
	if in.Limit < 1 || in.Limit > 1000 {
		return nil, queryOut{}, fmt.Errorf("limit must be between 1 and 1000")
	}
	if in.Filters == nil && in.Path == "" {
		return nil, queryOut{}, fmt.Errorf("give filters or path")
	}
	if in.Filters != nil && in.Path != "" {
		return nil, queryOut{}, fmt.Errorf("give filters or path, not both")
	}
	if in.View != "" && in.Path == "" {
		return nil, queryOut{}, fmt.Errorf("view needs path")
	}
	if len(in.Sort) > 0 && in.Path != "" {
		return nil, queryOut{}, fmt.Errorf("sort applies only with filters: a base's view sets its own order")
	}
	var properties []string
	for _, name := range in.Properties {
		if name == "" {
			return nil, queryOut{}, fmt.Errorf("a property name is empty")
		}
		if !slices.Contains(properties, name) {
			properties = append(properties, name)
		}
	}
	params := &api.NoteQueryParams{VaultID: t.o.VaultID, Limit: new(int64(in.Limit))}
	if in.Filters != nil {
		type view struct {
			Type  string      `json:"type"`
			Order []string    `json:"order,omitzero"`
			Sort  []querySort `json:"sort,omitzero"`
		}
		if len(in.Sort) == 0 {
			in.Sort = nil
		}
		source, err := json.Marshal(struct {
			Filters any    `json:"filters"`
			Views   []view `json:"views"`
		}{Filters: in.Filters, Views: []view{{Type: "table", Order: properties, Sort: in.Sort}}})
		if err != nil {
			return nil, queryOut{}, err
		}
		params.Source = new(string(source))
	} else {
		params.Path = &in.Path
		if in.View != "" {
			params.View = &in.View
		}
	}
	res, err := t.o.Client.Note().Query(ctx, params)
	if err != nil {
		return nil, queryOut{}, err
	}
	if res.Notice != nil && *res.Notice != "" {
		return nil, queryOut{}, fmt.Errorf("view %q: %s", res.View.Name, *res.Notice)
	}
	keys := make([]string, len(res.Columns))
	for i, col := range res.Columns {
		keys[i] = col.Key
	}
	columns := make([]int, len(properties))
	for i, name := range properties {
		column := i
		if in.Path != "" {
			column = slices.IndexFunc(keys, func(key string) bool {
				return key == name || strings.TrimPrefix(key, "note.") == strings.TrimPrefix(name, "note.")
			})
			if column < 0 {
				return nil, queryOut{}, fmt.Errorf("property %q is not a column of view %q; its columns are %s", name, res.View.Name, strings.Join(keys, ", "))
			}
		}
		if slices.Contains(res.Unshown, keys[column]) {
			return nil, queryOut{}, fmt.Errorf("property %q cannot be returned: formulas, this and unknown file properties are outside the subset", name)
		}
		columns[i] = column
	}
	out := queryOut{Files: []queryFile{}, Total: int(res.Total), Notice: Notice}
	for _, row := range res.Rows {
		values := make(map[string]any, len(properties))
		for i, name := range properties {
			var value any
			if err := json.Unmarshal(row.Values[columns[i]], &value); err != nil {
				return nil, queryOut{}, fmt.Errorf("decode %s's %s: %w", row.Path, name, err)
			}
			values[name] = value
		}
		out.Files = append(out.Files, queryFile{
			Path: row.Path, Title: row.Title, URL: t.url(row.Path),
			Properties: values, Writable: t.writable(row.Path),
		})
	}
	return nil, out, nil
}
