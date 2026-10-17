package mcpserver

import (
	"strings"
	"testing"
)

// Frontmatter written as one flow mapping holds every key on one line;
// setting one key there would rewrite the others, so it is refused.
func TestSetPropertiesRefusesAFlowMapping(t *testing.T) {
	note := "---\n{status: draft, owner: wren}\n---\nbody\n"
	if got, err := setProperties(note, map[string]any{"status": "done"}); err == nil || !strings.Contains(err.Error(), "flow") {
		t.Fatalf("setProperties on a flow mapping = %q, %v; want an error about it", got, err)
	}
}
