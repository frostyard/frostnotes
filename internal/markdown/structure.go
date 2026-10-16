package markdown

import "github.com/yuin/goldmark/ast"

// collectStructure adds the markdown links to vault files, headings, block
// IDs and tasks outside comments to d, and counts its words
// (docs/specs/markdown.md#body). Task card T-0012 implements it.
func collectStructure(d *Doc, root ast.Node, s *source) {}
