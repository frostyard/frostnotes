// A note's headings from its syntax tree (docs/specs/ui.md, Right sidebar
// panes: Outline), as lib/outline.ts nests them: ATX and setext headings,
// their text without the marks, where their line starts.
import { ensureSyntaxTree, syntaxTree } from "@codemirror/language";
import { EditorState } from "@codemirror/state";

import type { Heading } from "../lib/outline";
import { markdownSupport } from "./syntax/index";

const HEADING = /^(?:ATXHeading|SetextHeading)([1-6])$/;

/** headingsOf lists a document's headings in order, parsing all of it (or as much as 100 ms allows). */
export function headingsOf(state: EditorState): Heading[] {
  const tree = ensureSyntaxTree(state, state.doc.length, 100) ?? syntaxTree(state);
  const out: Heading[] = [];
  tree.iterate({
    enter: (node) => {
      const m = HEADING.exec(node.name);
      if (!m) return undefined;
      const line = state.doc.lineAt(node.from);
      const raw = node.name.startsWith("ATX")
        ? line.text.replace(/^\s{0,3}#{1,6}\s*/, "").replace(/\s+#+\s*$/, "")
        : line.text;
      out.push({ level: Number(m[1]), text: raw.trim(), from: line.from });
      return false;
    },
  });
  return out;
}

/** headingsOfText parses text alone, for a note no editor shows (the reading view). */
export function headingsOfText(text: string): Heading[] {
  return headingsOf(EditorState.create({ doc: text, extensions: markdownSupport() }));
}
