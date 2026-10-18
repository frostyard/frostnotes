// Obsidian's block IDs for @lezer/markdown (docs/specs/markdown.md#block-ids).
// IDs end an inline section; extract.ts decides which blocks they identify.
import { tags as t } from "@lezer/highlight";
import type { MarkdownConfig } from "@lezer/markdown";

/** BlockId parses a ^block-id as a BlockId node. */
export const BlockId: MarkdownConfig = {
  defineNodes: [{ name: "BlockId", style: t.meta }],
  parseInline: [
    {
      name: "BlockId",
      parse(cx, next, pos) {
        if (next !== 94 || (pos !== cx.offset && !/\s/.test(cx.slice(pos - 1, pos)))) return -1; // ^
        const match = /^\^[A-Za-z0-9-]+\s*$/.exec(cx.slice(pos, cx.end));
        return match ? cx.addElement(cx.elt("BlockId", pos, pos + match[0].trimEnd().length)) : -1;
      },
    },
  ],
};
