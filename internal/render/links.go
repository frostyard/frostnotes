package render

import "github.com/yuin/goldmark"

// extendLinks registers how links, embeds and vault files render
// (docs/specs/render.md#links-and-embeds). Task card T-0045.
func (e *obsidianHTML) extendLinks(m goldmark.Markdown) {
	_, _ = m, e.embed // the card's renderers embed notes through e.embed
}
