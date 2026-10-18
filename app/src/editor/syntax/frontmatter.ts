// A note's YAML frontmatter for @lezer/markdown (docs/specs/markdown.md#frontmatter):
// the "---" block at the very start, one node, so none of it parses as
// markdown (no setext heading, no links). The properties block (batch 3)
// draws it; until then it shows as source, muted.
import { tags as t } from "@lezer/highlight";
import type { MarkdownConfig } from "@lezer/markdown";

import { frontmatterEnd } from "./extract";

export const Frontmatter: MarkdownConfig = {
  defineNodes: [{ name: "Frontmatter", block: true, style: t.meta }],
  parseBlock: [
    {
      name: "Frontmatter",
      before: "HorizontalRule",
      parse(cx, line) {
        if (cx.lineStart !== 0 || !/^---[ \t]*$/.test(line.text)) return false;
        // Whether the block closes needs more than peekLine's one line of
        // lookahead: the context's input, there at runtime though not in
        // @lezer/markdown's types (pinned; the unclosed case's test guards
        // it). Without it, nothing is frontmatter.
        const input = (cx as unknown as { input?: { length: number; read(from: number, to: number): string } }).input;
        if (!input) return false;
        const text = input.read(0, Math.min(input.length, 1 << 20));
        const end = frontmatterEnd(text);
        if (end === 0) return false;
        const close = text.slice(0, end).replace(/\r?\n$/, "").length;
        while (cx.lineStart + line.text.length < close && cx.nextLine()) {
          // Every line up to the closing "---" is the frontmatter's.
        }
        cx.nextLine();
        cx.addElement(cx.elt("Frontmatter", 0, close));
        return true;
      },
    },
  ],
};
