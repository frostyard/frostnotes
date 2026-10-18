// Obsidian's math for @lezer/markdown (docs/specs/markdown.md#body).
// Math is opaque to the editor's other inline parsers, as it is to notesd.
import { tags as t } from "@lezer/highlight";
import type { Line, MarkdownConfig } from "@lezer/markdown";

function inlineMathClose(line: string, width: number): number {
  if (width === 2) return line.indexOf("$$", 2);
  if (line.length < 2 || /[ \t]/.test(line.charAt(1))) return -1;
  for (let i = 1; i < line.length; i++) {
    if (line[i] === "\\") {
      i++;
    } else if (line[i] === "$" && line[i - 1] !== " " && line[i - 1] !== "\t") {
      return i;
    }
  }
  return -1;
}

function startsBlockMath(line: Line): boolean {
  return line.indent - line.baseIndent <= 3 && line.text.startsWith("$$", line.pos);
}

/** MathSyntax parses $inline$ math and $$ blocks. */
export const MathSyntax: MarkdownConfig = {
  defineNodes: [
    { name: "InlineMath", style: t.special(t.string) },
    { name: "InlineMathMark", style: t.processingInstruction },
    { name: "BlockMath", block: true, style: t.special(t.string) },
    { name: "BlockMathMark", style: t.processingInstruction },
  ],
  parseInline: [
    {
      name: "InlineMath",
      parse(cx, next, pos) {
        if (next !== 36) return -1;
        const line = cx.slice(pos, cx.end).split(/[\r\n]/, 1)[0] ?? "";
        const width = line.startsWith("$$") ? 2 : 1;
        const close = inlineMathClose(line, width);
        if (close < 0) return -1;
        const to = pos + close + width;
        return cx.addElement(
          cx.elt("InlineMath", pos, to, [
            cx.elt("InlineMathMark", pos, pos + width),
            cx.elt("InlineMathMark", pos + close, to),
          ]),
        );
      },
    },
  ],
  parseBlock: [
    {
      name: "BlockMath",
      before: "FencedCode",
      parse(cx, line) {
        if (!startsBlockMath(line)) return false;
        const from = cx.lineStart + line.pos;
        const marks = [cx.elt("BlockMathMark", from, from + 2)];
        let close = line.text.indexOf("$$", line.pos + 2);
        // Line.depth exists at runtime, though @lezer/markdown omits it
        // from its types. It tells us whether all containers still match.
        while (close < 0 && cx.nextLine() && (line as Line & { depth: number }).depth >= cx.depth) {
          marks.push(...line.markers);
          close = line.text.indexOf("$$");
        }
        if (close >= 0) {
          const start = cx.lineStart + close;
          marks.push(cx.elt("BlockMathMark", start, start + 2));
          cx.nextLine();
        }
        cx.addElement(cx.elt("BlockMath", from, cx.prevLineEnd(), marks));
        return true;
      },
      endLeaf(_cx, line) {
        return startsBlockMath(line);
      },
    },
  ],
};
