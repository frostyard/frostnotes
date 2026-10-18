package render

// CONTRACT TEST for task card T-0046 (docs/tasks). Do not edit.

import "testing"

// TestSyntaxFixtures checks every fixture of docs/specs/render.md.
func TestSyntaxFixtures(t *testing.T) { runFixtures(t, "base", "links", "syntax") }
