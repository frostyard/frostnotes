// GFM's footnotes for @lezer/markdown
// (docs/specs/markdown.md#callouts-highlights-and-footnotes). Definitions
// consume one line, with ordinary inline syntax after the label.
import { tags as t } from "@lezer/highlight";
import type { BlockContext, LeafBlock, MarkdownConfig } from "@lezer/markdown";

const REFERENCE = /^\[\^[^\s[\]]+\]/;

/** Footnote parses footnote references and definitions. */
export const Footnote: MarkdownConfig = {
  defineNodes: [
    { name: "FootnoteReference", style: t.labelName },
    { name: "FootnoteDefinition", block: true },
    { name: "FootnoteMark", style: t.processingInstruction },
    { name: "FootnoteLabel", style: t.labelName },
  ],
  parseInline: [
    {
      name: "FootnoteReference",
      before: "Link",
      parse(cx, next, pos) {
        if (next !== 91) return -1; // [
        const match = REFERENCE.exec(cx.slice(pos, cx.end));
        if (!match) return -1;
        const end = pos + match[0].length;
        return cx.addElement(
          cx.elt("FootnoteReference", pos, end, [
            cx.elt("FootnoteMark", pos, pos + 2),
            cx.elt("FootnoteLabel", pos + 2, end - 1),
            cx.elt("FootnoteMark", end - 1, end),
          ]),
        );
      },
    },
  ],
  parseBlock: [
    {
      name: "FootnoteDefinition",
      before: "LinkReference",
      leaf(_cx, leaf) {
        const match = REFERENCE.exec(leaf.content);
        if (!match || leaf.content[match[0].length] !== ":") return null;
        const labelEnd = leaf.start + match[0].length - 1;
        function finish(cx: BlockContext, block: LeafBlock): boolean {
          const start = block.start;
          cx.addLeafElement(
            block,
            cx.elt("FootnoteDefinition", start, start + block.content.length, [
              cx.elt("FootnoteMark", start, start + 2),
              cx.elt("FootnoteLabel", start + 2, labelEnd),
              cx.elt("FootnoteMark", labelEnd, labelEnd + 2),
              ...cx.parser.parseInline(block.content.slice(labelEnd + 2 - start), labelEnd + 2),
            ]),
          );
          return true;
        }
        return {
          nextLine(cx, _line, block) {
            return finish(cx, block);
          },
          finish,
        };
      },
    },
  ],
};
