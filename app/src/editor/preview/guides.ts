// Indentation guides in live preview (docs/specs/ui.md#editor-and-reading-view).
// Mark indentation below each parent bullet without changing the text.
import { Decoration } from "@codemirror/view";

import type { Provider } from "./framework";

const guide = Decoration.mark({ class: "cm-pv-guide" });

const guides: Provider = {
  id: "guide",
  kind: "inline",
  nodes: ["ListItem"],
  draw(node, cx) {
    const item = node.node;
    if (!item.getChild("BulletList") && !item.getChild("OrderedList")) return;
    const mark = item.getChild("ListMark");
    if (!mark) return;
    const doc = cx.state.doc;
    const first = doc.lineAt(item.from);
    const column = mark.from - first.from;
    const last = doc.lineAt(item.to).number;
    for (let number = first.number + 1; number <= last; number++) {
      const line = doc.line(number);
      const indent = /^[ \t]*/.exec(line.text)?.[0].length ?? 0;
      if (indent > column) {
        const from = line.from + column;
        cx.add(guide.range(from, from + 1));
      }
    }
  },
};

/** GUIDES are the providers of indentation guides. */
export const GUIDES: readonly Provider[] = [guides];
