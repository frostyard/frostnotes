package markdown

import (
	"github.com/yuin/goldmark"
	"github.com/yuin/goldmark/ast"
)

// obsidian is the goldmark extension for the Obsidian syntax in a note's
// body: wikilinks and embeds, tags and math (docs/specs/markdown.md#body).
// Task card T-0011 adds its parsers.
type obsidian struct{}

// Extend registers the extension's parsers.
func (obsidian) Extend(goldmark.Markdown) {}

// findComments returns the body's %%…%% comments as sorted, disjoint
// ranges of note offsets. T-0011.
func findComments(root ast.Node, s *source) [][2]int { return nil }

// collectObsidian adds the wikilinks, embeds and inline tags outside
// comments to d. T-0011.
func collectObsidian(d *Doc, root ast.Node, s *source) {}
