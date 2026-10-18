// Obsidian's embeds for @lezer/markdown (docs/specs/markdown.md#wikilinks-and-embeds).
// An embed wraps a wikilink with its leading exclamation mark.
import { tags as t } from "@lezer/highlight";
import type { MarkdownConfig } from "@lezer/markdown";

import { wikiLinkElement } from "./wikilink";

/** Embed parses ![[target]] as an Embed node around a WikiLink. */
export const Embed: MarkdownConfig = {
  defineNodes: [{ name: "Embed" }, { name: "EmbedMark", style: t.processingInstruction }],
  parseInline: [
    {
      name: "Embed",
      before: "Image",
      parse(cx, next, pos) {
        if (next !== 33) return -1; // !
        const link = wikiLinkElement(cx, pos + 1);
        return link ? cx.addElement(cx.elt("Embed", pos, link.to, [cx.elt("EmbedMark", pos, pos + 1), link])) : -1;
      },
    },
  ],
};
