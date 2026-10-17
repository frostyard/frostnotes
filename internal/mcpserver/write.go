package mcpserver

import (
	"context"
	"fmt"

	sdk "github.com/modelcontextprotocol/go-sdk/mcp"
)

// The write tools (docs/specs/mcp.md#write-tools). Task card T-0023 builds
// them on the edits of T-0022; until then each answers that it is not
// built.

func (t *tools) createNote(context.Context, *sdk.CallToolRequest, createIn) (*sdk.CallToolResult, createOut, error) {
	return nil, createOut{}, fmt.Errorf("create_note is not built yet (task card T-0023)")
}

func (t *tools) editNote(context.Context, *sdk.CallToolRequest, editIn) (*sdk.CallToolResult, editOut, error) {
	return nil, editOut{}, fmt.Errorf("edit_note is not built yet (task card T-0023)")
}

func (t *tools) appendToDailyNote(context.Context, *sdk.CallToolRequest, dailyIn) (*sdk.CallToolResult, dailyOut, error) {
	return nil, dailyOut{}, fmt.Errorf("append_to_daily_note is not built yet (task card T-0023)")
}
