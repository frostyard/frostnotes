package mcpserver

import (
	"context"
	"fmt"
	"strings"

	"github.com/frostyard/frostnotes/api"
	sdk "github.com/modelcontextprotocol/go-sdk/mcp"
)

// The write tools (docs/specs/mcp.md#write-tools) attribute every write to
// the agent and name the version used to compute each edit.

func (t *tools) createNote(ctx context.Context, req *sdk.CallToolRequest, in createIn) (*sdk.CallToolResult, createOut, error) {
	if !strings.HasSuffix(in.Path, ".md") {
		return nil, createOut{}, fmt.Errorf("path must end in .md")
	}
	source, name := t.agentWrite(req)
	result, err := t.o.Client.Note().Create(ctx, &api.NoteCreateParams{
		VaultID: t.o.VaultID, Path: in.Path, Content: in.Content, Source: source, Author: name,
	})
	if err != nil {
		return nil, createOut{}, err
	}
	return nil, createOut{Path: result.Path, Hash: result.Hash}, nil
}

func (t *tools) editNote(ctx context.Context, req *sdk.CallToolRequest, in editIn) (*sdk.CallToolResult, editOut, error) {
	notes := t.o.Client.Note()
	note, err := notes.Read(ctx, &api.NoteReadParams{VaultID: t.o.VaultID, Path: in.Path})
	if err != nil {
		return nil, editOut{}, err
	}
	content, err := applyEdit(note.Content, in)
	if err != nil {
		return nil, editOut{}, err
	}
	source, name := t.agentWrite(req)
	result, err := notes.Write(ctx, &api.NoteWriteParams{
		VaultID: t.o.VaultID, Path: note.Path, Base: note.Hash, Content: content, Source: source, Author: name,
	})
	if err != nil {
		return nil, editOut{}, err
	}
	return nil, editOut{Path: result.Path, Hash: result.Hash, Merged: result.Merged, Conflicts: int(result.Conflicts)}, nil
}

func (t *tools) appendToDailyNote(ctx context.Context, req *sdk.CallToolRequest, in dailyIn) (*sdk.CallToolResult, dailyOut, error) {
	if in.Text == "" {
		return nil, dailyOut{}, fmt.Errorf("text must not be empty")
	}
	source, name := t.agentWrite(req)
	params := &api.NoteDailyParams{VaultID: t.o.VaultID, Source: source, Author: name}
	if in.Date != "" {
		params.Date = &in.Date
	}
	notes := t.o.Client.Note()
	daily, err := notes.Daily(ctx, params)
	if err != nil {
		return nil, dailyOut{}, err
	}
	result, err := notes.Write(ctx, &api.NoteWriteParams{
		VaultID: t.o.VaultID, Path: daily.Note.Path, Base: daily.Note.Hash,
		Content: appendText(daily.Note.Content, in.Text), Source: source, Author: name,
	})
	if err != nil {
		return nil, dailyOut{}, err
	}
	return nil, dailyOut{Path: result.Path, Hash: result.Hash, Created: daily.Created, Merged: result.Merged}, nil
}
