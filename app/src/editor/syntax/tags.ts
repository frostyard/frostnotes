// Obsidian's inline tags for @lezer/markdown (docs/specs/markdown.md#tags).
// Tags start at an inline section's start or after white space, and their
// Unicode names follow notesd's tagParser without accepting all digits.
import { tags as t } from "@lezer/highlight";
import type { MarkdownConfig } from "@lezer/markdown";

const NAME = /^[\p{L}\p{M}\p{Nd}_/-]+/u;
const DIGITS = /^\p{Nd}+$/u;

/** Hashtag parses a body's tags as Hashtag nodes. */
export const Hashtag: MarkdownConfig = {
  defineNodes: [{ name: "Hashtag", style: t.labelName }],
  parseInline: [
    {
      name: "Hashtag",
      parse(cx, next, pos) {
        if (next !== 35 || (pos !== cx.offset && !/\s/.test(cx.slice(pos - 1, pos)))) return -1;
        const name = NAME.exec(cx.slice(pos + 1, cx.end))?.[0];
        if (!name || DIGITS.test(name)) return -1;
        return cx.addElement(cx.elt("Hashtag", pos, pos + 1 + name.length));
      },
    },
  ],
};
