package mcpserver

import "errors"

// The edits of edit_note (docs/specs/mcp.md#edits), on a note's content
// as note.read returns it. Task card T-0022 builds them; until then each
// fails.

var errEditNotBuilt = errors.New("edits are not built yet (task card T-0022)")

// applyEdit returns content with in's one edit applied: old_text and
// new_text, append, prepend, or properties. No edit, or more than one, is
// an error.
func applyEdit(content string, in editIn) (string, error) { return "", errEditNotBuilt }

// replaceOnce replaces the one occurrence of old in content with new; old
// occurring zero times or more than once is an error that says how often.
func replaceOnce(content, old, new string) (string, error) { return "", errEditNotBuilt }

// appendText adds text at the end of content, on a line of its own and
// ending in "\n".
func appendText(content, text string) string { return "" }

// prependText adds text at the start of content's body, after its
// frontmatter, ending in "\n".
func prependText(content, text string) string { return "" }

// setProperties sets each key of props in content's frontmatter, removing
// those whose value is nil, and leaves every other byte as it is.
func setProperties(content string, props map[string]any) (string, error) {
	return "", errEditNotBuilt
}
