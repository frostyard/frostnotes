// Obsidian's wikilinks for @lezer/markdown (docs/specs/markdown.md#body):
// [[target]], [[target#subpath]], [[target|alias]] on one line, the alias
// after the first "|" (a "\|", as tables write it, counts as one). The
// parser only names ranges of the text; it never changes it (ADR-0007).
// An embed's "!" is the embeds extension's (embeds.ts), around this one's
// WikiLink.
import { tags as t } from "@lezer/highlight";
import type { Element, InlineContext, MarkdownConfig } from "@lezer/markdown";

const OPEN = 91; // [
const CLOSE = 93; // ]
const PIPE = 124; // |
const HASH = 35; // #
const BACKSLASH = 92; // \
const NEWLINE = 10;

/** wikiLinkElement is the WikiLink element whose "[[" is at pos, or null when none starts there. */
export function wikiLinkElement(cx: InlineContext, pos: number): Element | null {
  if (cx.char(pos) !== OPEN || cx.char(pos + 1) !== OPEN) return null;
  let pipe = -1;
  for (let i = pos + 2; i < cx.end; i++) {
    const c = cx.char(i);
    if (c === NEWLINE || c === OPEN) return null;
    if (c === PIPE && pipe < 0) pipe = cx.char(i - 1) === BACKSLASH ? i - 1 : i;
    if (c !== CLOSE) continue;
    if (cx.char(i + 1) !== CLOSE) return null;
    const end = pipe < 0 ? i : pipe;
    const after = pipe < 0 ? i : cx.char(pipe) === BACKSLASH ? pipe + 2 : pipe + 1;
    // A link names a target, a subpath or an alias: [[]] and [[ ]] name none.
    if (cx.slice(pos + 2, end).trim() === "" && cx.slice(after, i).trim() === "") return null;
    let hash = -1;
    for (let j = pos + 2; j < end; j++) {
      if (cx.char(j) === HASH) {
        hash = j;
        break;
      }
    }
    const children = [cx.elt("WikiLinkMark", pos, pos + 2)];
    if (hash !== pos + 2 && end > pos + 2) children.push(cx.elt("WikiLinkTarget", pos + 2, hash < 0 ? end : hash));
    if (hash >= 0) children.push(cx.elt("WikiLinkSubpath", hash, end));
    if (pipe >= 0) {
      children.push(cx.elt("WikiLinkPipe", pipe, after));
      if (after < i) children.push(cx.elt("WikiLinkAlias", after, i));
    }
    children.push(cx.elt("WikiLinkMark", i, i + 2));
    return cx.elt("WikiLink", pos, i + 2, children);
  }
  return null;
}

export const WikiLink: MarkdownConfig = {
  defineNodes: [
    { name: "WikiLink", style: t.link },
    { name: "WikiLinkMark", style: t.processingInstruction },
    { name: "WikiLinkTarget" },
    { name: "WikiLinkSubpath" },
    { name: "WikiLinkPipe", style: t.processingInstruction },
    { name: "WikiLinkAlias" },
  ],
  parseInline: [
    {
      name: "WikiLink",
      before: "Link",
      parse(cx, next, pos) {
        if (next !== OPEN) return -1;
        const link = wikiLinkElement(cx, pos);
        return link ? cx.addElement(link) : -1;
      },
    },
  ],
};
