// Obsidian's callouts (docs/specs/markdown.md#callouts-highlights-and-footnotes):
// a blockquote whose first line starts with [!type]. Task T-0066 replaces
// this stub.
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
export function readCallout(_state: EditorState, _quote: SyntaxNode): Callout | null {
  return null;
}
