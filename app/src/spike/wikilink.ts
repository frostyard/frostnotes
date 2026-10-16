// Obsidian wikilinks for @lezer/markdown: [[target]], [[target#heading]] and
// [[target|alias]] on one line. The parser only names ranges of the text;
// it never changes it (ADR-0007).
import { tags as t } from "@lezer/highlight";
import type { MarkdownConfig } from "@lezer/markdown";

const OPEN = 91; // [
const CLOSE = 93; // ]
const PIPE = 124; // |
const NEWLINE = 10;

export const WikiLink: MarkdownConfig = {
  defineNodes: [
    { name: "WikiLink", style: t.link },
    { name: "WikiLinkMark", style: t.processingInstruction },
    { name: "WikiLinkTarget" },
    { name: "WikiLinkPipe", style: t.processingInstruction },
    { name: "WikiLinkAlias" },
  ],
  parseInline: [
    {
      name: "WikiLink",
      before: "Link",
      parse(cx, next, pos) {
        if (next !== OPEN || cx.char(pos + 1) !== OPEN) return -1;
        let pipe = -1;
        for (let i = pos + 2; i < cx.end; i++) {
          const c = cx.char(i);
          if (c === NEWLINE || c === OPEN) return -1;
          if (c === PIPE && pipe < 0) pipe = i;
          if (c !== CLOSE) continue;
          const targetEnd = pipe < 0 ? i : pipe;
          if (cx.char(i + 1) !== CLOSE || targetEnd === pos + 2) return -1;
          const children = [cx.elt("WikiLinkMark", pos, pos + 2), cx.elt("WikiLinkTarget", pos + 2, targetEnd)];
          if (pipe >= 0) {
            children.push(cx.elt("WikiLinkPipe", pipe, pipe + 1));
            if (pipe + 1 < i) children.push(cx.elt("WikiLinkAlias", pipe + 1, i));
          }
          children.push(cx.elt("WikiLinkMark", i, i + 2));
          return cx.addElement(cx.elt("WikiLink", pos, i + 2, children));
        }
        return -1;
      },
    },
  ],
};
