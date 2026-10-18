package render

// CONTRACT TEST for task card T-0045 (docs/tasks). Do not edit.

import "testing"

// TestLinkFixtures checks docs/specs/render.md's base and links fixtures.
func TestLinkFixtures(t *testing.T) { runFixtures(t, "base", "links") }
