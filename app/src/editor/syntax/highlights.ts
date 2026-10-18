// Obsidian's ==highlights== for @lezer/markdown
// (docs/specs/markdown.md#callouts-highlights-and-footnotes). Delimiters
// follow GFM strikethrough's flanking rules, preserving nested syntax.
import { tags as t } from "@lezer/highlight";
import type { MarkdownConfig } from "@lezer/markdown";

const HIGHLIGHT_DELIM = { resolve: "Highlight", mark: "HighlightMark" };
const PUNCTUATION = /[\p{S}\p{P}]/u;

/** Highlight parses ==text== as a Highlight node. */
export const Highlight: MarkdownConfig = {
  defineNodes: [
    { name: "Highlight", style: { "Highlight/...": t.special(t.content) } },
    { name: "HighlightMark", style: t.processingInstruction },
  ],
  parseInline: [
    {
      name: "Highlight",
      after: "Emphasis",
      parse(cx, next, pos) {
        if (next !== 61 || cx.char(pos + 1) !== 61 || cx.char(pos + 2) === 61) return -1;
        const before = cx.slice(pos - 1, pos);
        const after = cx.slice(pos + 2, pos + 3);
        const sBefore = /\s|^$/.test(before);
        const sAfter = /\s|^$/.test(after);
        const pBefore = PUNCTUATION.test(before);
        const pAfter = PUNCTUATION.test(after);
        return cx.addDelimiter(
          HIGHLIGHT_DELIM,
          pos,
          pos + 2,
          !sAfter && (!pAfter || sBefore || pBefore),
          !sBefore && (!pBefore || sAfter || pAfter),
        );
      },
    },
  ],
};
