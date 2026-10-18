// Quotes, lists, rules and done tasks in live preview (docs/specs/ui.md#editor-and-reading-view).
// Quote bars and done text stay visible; syntax draws away from selected lines.
import { Decoration, WidgetType } from "@codemirror/view";

import type { Provider } from "./framework";

class SpanWidget extends WidgetType {
  constructor(readonly className: string) {
    super();
  }
  override eq(other: SpanWidget): boolean {
    return other.className === this.className;
  }
  toDOM(): HTMLElement {
    const span = document.createElement("span");
    span.className = this.className;
    return span;
  }
}

const quoteLine = Decoration.line({ class: "cm-pv-quote" });
const ruleLine = Decoration.line({ class: "cm-pv-hr-line" });
const bullet = Decoration.replace({ widget: new SpanWidget("cm-pv-bullet") });
const rule = Decoration.replace({ widget: new SpanWidget("cm-pv-hr") });
const done = Decoration.mark({ class: "cm-pv-task-done" });

const quotes: Provider = {
  id: "quote",
  kind: "inline",
  nodes: ["Blockquote"],
  draw(node, cx) {
    const doc = cx.state.doc;
    const last = doc.lineAt(node.to).number;
    for (let number = doc.lineAt(node.from).number; number <= last; number++) {
      cx.line(doc.line(number).from, quoteLine);
    }
  },
};

const quoteMarks: Provider = {
  id: "quote-mark",
  kind: "inline",
  nodes: ["QuoteMark"],
  draw(node, cx) {
    if (!cx.revealed(node.from)) {
      const space = cx.state.doc.sliceString(node.to, node.to + 1) === " ";
      cx.hide(node.from, node.to + (space ? 1 : 0));
    }
  },
};

const bullets: Provider = {
  id: "bullet",
  kind: "inline",
  nodes: ["ListMark"],
  draw(node, cx) {
    const item = node.node.parent;
    if (
      item?.name === "ListItem" &&
      item.parent?.name === "BulletList" &&
      !item.getChild("Task") &&
      !cx.revealed(node.from)
    ) {
      cx.add(bullet.range(node.from, node.to));
    }
  },
};

const rules: Provider = {
  id: "rule",
  kind: "inline",
  nodes: ["HorizontalRule"],
  draw(node, cx) {
    if (cx.revealed(node.from)) return;
    cx.line(node.from, ruleLine);
    cx.add(rule.range(node.from, node.to));
  },
};

const doneTasks: Provider = {
  id: "task-done",
  kind: "inline",
  nodes: ["Task"],
  draw(node, cx) {
    const marker = node.node.getChild("TaskMarker");
    if (!marker) return;
    const doc = cx.state.doc;
    const checked = doc.sliceString(marker.from + 1, marker.from + 2);
    if (checked !== "x" && checked !== "X") return;
    const from = marker.to + 1;
    const to = doc.lineAt(marker.from).to;
    if (from < to) cx.add(done.range(from, to));
  },
};

/** LISTS are the providers of quotes, lists, rules and done tasks. */
export const LISTS: readonly Provider[] = [quotes, quoteMarks, bullets, rules, doneTasks];
