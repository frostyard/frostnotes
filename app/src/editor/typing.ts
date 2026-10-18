// What the Editor settings add to typing and the gutter
// (docs/specs/ui.md#settings-batch-2): folding by indentation (Fold
// indent; headings fold with the markdown language itself), and wrapping
// a selection in Markdown's marks as they are typed (Auto-pair Markdown
// syntax).
import { foldService } from "@codemirror/language";
import { EditorSelection, type EditorState, type Extension } from "@codemirror/state";
import { EditorView } from "@codemirror/view";

const indentOf = (text: string, tabSize: number) => {
  let n = 0;
  for (const c of text) {
    if (c === " ") n++;
    else if (c === "\t") n += tabSize - (n % tabSize);
    else return n;
  }
  return -1; // A blank line has no indentation of its own.
};

/**
 * indentFoldRange is what folds at the line from–to: the lines after it
 * that are indented deeper, up to the last of them (blank lines between
 * them included); null when none is deeper.
 */
export function indentFoldRange(state: EditorState, from: number, to: number): { from: number; to: number } | null {
  const doc = state.doc;
  const line = doc.lineAt(from);
  const base = indentOf(line.text, state.tabSize);
  if (base < 0) return null;
  let end = -1;
  for (let n = line.number + 1; n <= doc.lines; n++) {
    const next = doc.line(n);
    const depth = indentOf(next.text, state.tabSize);
    if (depth < 0) continue;
    if (depth <= base) break;
    end = next.to;
  }
  return end > to ? { from: to, to: end } : null;
}

/** indentFolding folds lines by their indentation. */
export const indentFolding: Extension = foldService.of(indentFoldRange);

/** MARKS are the characters a selection is wrapped in as they are typed. */
const MARKS = new Set(["*", "_", "~", "=", "`"]);

/** markdownPairs wraps each selected range in a Markdown mark typed over it. */
export const markdownPairs: Extension = EditorView.inputHandler.of((view, from, to, text) => {
  const sel = view.state.selection;
  if (!MARKS.has(text) || sel.ranges.some((r) => r.empty) || from !== sel.main.from || to !== sel.main.to) return false;
  view.dispatch(
    view.state.changeByRange((r) => ({
      changes: [
        { from: r.from, insert: text },
        { from: r.to, insert: text },
      ],
      range: EditorSelection.range(r.from + 1, r.to + 1),
    })),
    { userEvent: "input.type" },
  );
  return true;
});
