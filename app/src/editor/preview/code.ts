// Fenced code blocks in live preview (docs/specs/ui.md#editor-and-reading-view).
// Task T-0071 replaces this provider; until then a block's lines show on
// the code color.
import { Decoration } from "@codemirror/view";

import type { Provider } from "./framework";

const codeLine = Decoration.line({ class: "cm-pv-code" });

/** code draws a fenced code block's lines on the code color. */
export const code: Provider = {
  id: "code",
  kind: "inline",
  nodes: ["FencedCode"],
  draw(node, cx) {
    const doc = cx.state.doc;
    for (let pos = node.from; pos <= node.to; pos = doc.lineAt(pos).to + 1) cx.line(pos, codeLine);
    return false;
  },
};

/** CODE are the code blocks' providers. */
export const CODE: readonly Provider[] = [code];
