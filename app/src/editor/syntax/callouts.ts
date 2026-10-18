// Obsidian's callouts (docs/specs/markdown.md#callouts-highlights-and-footnotes):
// read a blockquote's first-line marker without changing how its body parses.
import type { EditorState } from "@codemirror/state";
import type { SyntaxNode } from "@lezer/common";

/** Callout is what a callout's first line says. */
export interface Callout {
  /** The type, lowercased: "note", "warning". */
  type: string;
  /** "+" or "-" when foldable (open or folded), else "". */
  fold: "" | "+" | "-";
  /** The title as written, trimmed; "" when none. */
  title: string;
  /** Where "[!type]" and its fold sign start and end. */
  markFrom: number;
  markTo: number;
}

/** readCallout reads the callout a Blockquote node is, or null when it is a plain quote. */
export function readCallout(state: EditorState, quote: SyntaxNode): Callout | null {
  if (quote.name !== "Blockquote") return null;
  const mark = quote.getChild("QuoteMark");
  if (!mark) return null;
  const line = state.doc.lineAt(mark.to);
  const afterMark = state.doc.sliceString(mark.to, line.to);
  const text = afterMark.replace(/^[ \t]*/, "");
  const match = /^\[!([A-Za-z0-9_-]+)\]([+-]?)(?:[ \t]+(.*?))?[ \t]*$/.exec(text);
  if (!match) return null;
  const [, type = "", sign, title = ""] = match;
  const fold = sign === "+" || sign === "-" ? sign : "";
  const markFrom = mark.to + afterMark.length - text.length;
  return {
    type: type.toLowerCase(),
    fold,
    title: title.trim(),
    markFrom,
    markTo: markFrom + type.length + 3 + fold.length,
  };
}
