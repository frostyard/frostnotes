package mcpserver

import (
	"context"
	"encoding/json/v2"
	"fmt"
	"strings"
	"time"

	"github.com/frostyard/frostnotes/api"
	sdk "github.com/modelcontextprotocol/go-sdk/mcp"
)

// The read tools adapt notesd's note domain (docs/specs/mcp.md#read-tools).

func (t *tools) searchNotes(ctx context.Context, _ *sdk.CallToolRequest, in searchIn) (*sdk.CallToolResult, searchOut, error) {
	if in.Limit == 0 {
		in.Limit = 20
	}
	if in.Limit < 1 || in.Limit > 100 {
		return nil, searchOut{}, fmt.Errorf("limit must be between 1 and 100")
	}
	hits, err := t.o.Client.Note().Search(ctx, &api.NoteSearchParams{
		VaultID: t.o.VaultID, Query: in.Query, Limit: new(int64(in.Limit)),
	})
	if err != nil {
		return nil, searchOut{}, err
	}
	out := searchOut{Hits: []searchHit{}, Notice: Notice}
	for _, hit := range hits {
		var snippet strings.Builder
		for _, part := range hit.Snippet {
			if part.Match {
				snippet.WriteString("**")
			}
			snippet.WriteString(part.Text)
			if part.Match {
				snippet.WriteString("**")
			}
		}
		out.Hits = append(out.Hits, searchHit{Path: hit.Path, Title: hit.Title, Snippet: snippet.String()})
	}
	return nil, out, nil
}

func (t *tools) readNote(ctx context.Context, _ *sdk.CallToolRequest, in readIn) (*sdk.CallToolResult, noteOut, error) {
	note, err := t.o.Client.Note().Read(ctx, &api.NoteReadParams{VaultID: t.o.VaultID, Path: in.Path})
	if err != nil {
		return nil, noteOut{}, err
	}
	properties := map[string]any{}
	if len(note.Props) > 0 {
		if err := json.Unmarshal(note.Props, &properties); err != nil {
			return nil, noteOut{}, fmt.Errorf("decode note properties: %w", err)
		}
	}
	if properties == nil {
		properties = map[string]any{}
	}
	out := noteOut{
		Path: note.Path, Title: note.Title, Hash: note.Hash,
		Modified:   note.ModifiedAt.UTC().Format(time.RFC3339),
		Properties: properties,
		Tags:       append([]string{}, note.Tags...),
		Aliases:    append([]string{}, note.Aliases...),
		Content:    note.Content, Notice: Notice,
	}
	return nil, out, nil
}

func (t *tools) listNotes(ctx context.Context, _ *sdk.CallToolRequest, in listIn) (*sdk.CallToolResult, listOut, error) {
	if in.Limit == 0 {
		in.Limit = 200
	}
	if in.Limit < 1 || in.Limit > 1000 {
		return nil, listOut{}, fmt.Errorf("limit must be between 1 and 1000")
	}
	params := &api.NoteListParams{VaultID: t.o.VaultID}
	if in.Folder != "" {
		params.Folder = &in.Folder
	}
	if in.Tag != "" {
		params.Tag = &in.Tag
	}
	if in.Property != "" {
		params.Property = &in.Property
	}
	if in.Value != "" {
		params.Value = &in.Value
	}
	notes, err := t.o.Client.Note().List(ctx, params)
	if err != nil {
		return nil, listOut{}, err
	}
	out := listOut{Notes: []listEntry{}, Total: len(notes), Notice: Notice}
	for _, note := range notes[:min(in.Limit, len(notes))] {
		out.Notes = append(out.Notes, listEntry{
			Path: note.Path, Title: note.Title, Modified: note.ModifiedAt.UTC().Format(time.RFC3339),
		})
	}
	return nil, out, nil
}

func (t *tools) getBacklinks(ctx context.Context, _ *sdk.CallToolRequest, in backlinksIn) (*sdk.CallToolResult, backlinksOut, error) {
	links, err := t.o.Client.Note().Backlinks(ctx, &api.NoteBacklinksParams{VaultID: t.o.VaultID, Path: in.Path})
	if err != nil {
		return nil, backlinksOut{}, err
	}
	out := backlinksOut{Backlinks: []backlink{}, Notice: Notice}
	for _, link := range links {
		out.Backlinks = append(out.Backlinks, backlink{
			Source: link.Source, Line: int(link.Link.Line), Target: link.Link.Target,
			Alias: link.Link.Alias, Embed: link.Link.Embed,
		})
	}
	return nil, out, nil
}
