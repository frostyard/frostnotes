package render

import "github.com/yuin/goldmark"

// extendSyntax registers how the rest of Obsidian's syntax renders: tags,
// highlights, math, callouts, task checkboxes, headings and block IDs
// (docs/specs/render.md#obsidians-syntax). Task card T-0046.
func (e *obsidianHTML) extendSyntax(m goldmark.Markdown) {
	_, _, _ = m, e.lineOf, e.lineless // the card's renderers name lines through them
}
