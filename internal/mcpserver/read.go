package mcpserver

import (
	"context"
	"fmt"

	sdk "github.com/modelcontextprotocol/go-sdk/mcp"
)

// The read tools (docs/specs/mcp.md#read-tools). Task card T-0021 builds
// them; until then each answers that it is not built.

func (t *tools) searchNotes(context.Context, *sdk.CallToolRequest, searchIn) (*sdk.CallToolResult, searchOut, error) {
	return nil, searchOut{}, fmt.Errorf("search_notes is not built yet (task card T-0021)")
}

func (t *tools) readNote(context.Context, *sdk.CallToolRequest, readIn) (*sdk.CallToolResult, noteOut, error) {
	return nil, noteOut{}, fmt.Errorf("read_note is not built yet (task card T-0021)")
}

func (t *tools) listNotes(context.Context, *sdk.CallToolRequest, listIn) (*sdk.CallToolResult, listOut, error) {
	return nil, listOut{}, fmt.Errorf("list_notes is not built yet (task card T-0021)")
}

func (t *tools) getBacklinks(context.Context, *sdk.CallToolRequest, backlinksIn) (*sdk.CallToolResult, backlinksOut, error) {
	return nil, backlinksOut{}, fmt.Errorf("get_backlinks is not built yet (task card T-0021)")
}
