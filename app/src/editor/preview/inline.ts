// Highlights, tags, inline code and footnote references in live preview (docs/specs/ui.md#editor-and-reading-view).
// Hide delimiters away from the selection and route tag and footnote clicks.
import { syntaxTree } from "@codemirror/language";
import { Decoration, WidgetType } from "@codemirror/view";

import type { Provider } from "./framework";

const highlights: Provider = {
  id: "highlight",
  kind: "inline",
  nodes: ["Highlight"],
  draw(node, cx) {
    if (cx.revealed(node.from, node.to)) return;
    for (const mark of node.node.getChildren("HighlightMark")) cx.hide(mark.from, mark.to);
  },
};

const tags: Provider = {
  id: "tag",
  kind: "inline",
  nodes: ["Hashtag"],
  draw(node, cx) {
    if (cx.revealed(node.from, node.to)) return;
    for (let parent = node.node.parent; parent; parent = parent.parent) {
      if (parent.name === "Link" || parent.name === "Image") return;
    }
    const name = cx.state.doc.sliceString(node.from + 1, node.to);
    cx.add(
      Decoration.mark({
        class: "cm-pv-tag",
        attributes: { "data-pv": "tag", "data-tag": name },
      }).range(node.from, node.to),
    );
  },
  click(el) {
    return { kind: "command", command: { kind: "search", query: `tag:#${el.dataset.tag ?? ""}` } };
  },
};

const code = Decoration.mark({ class: "cm-pv-inline-code" });

const inlineCode: Provider = {
  id: "inline-code",
  kind: "inline",
  nodes: ["InlineCode"],
  draw(node, cx) {
    if (cx.revealed(node.from, node.to)) return false;
    const marks = node.node.getChildren("CodeMark");
    const first = marks[0];
    const last = marks.at(-1);
    if (first && last && first !== last) {
      cx.hide(first.from, first.to);
      cx.hide(last.from, last.to);
      if (first.to < last.from) cx.add(code.range(first.to, last.from));
    }
    return false;
  },
};

class FootnoteWidget extends WidgetType {
  constructor(readonly label: string) {
    super();
  }
  override eq(other: FootnoteWidget): boolean {
    return other.label === this.label;
  }
  toDOM(): HTMLElement {
    const sup = document.createElement("sup");
    sup.className = "cm-pv-footnote";
    sup.dataset.pv = "footnote";
    sup.textContent = this.label;
    return sup;
  }
  override ignoreEvent(): boolean {
    return false;
  }
}

const footnotes: Provider = {
  id: "footnote",
  kind: "inline",
  nodes: ["FootnoteReference"],
  draw(node, cx) {
    if (cx.revealed(node.from, node.to)) return false;
    const label = node.node.getChild("FootnoteLabel");
    if (label) {
      const text = cx.state.doc.sliceString(label.from, label.to);
      cx.add(Decoration.replace({ widget: new FootnoteWidget(text) }).range(node.from, node.to));
    }
    return false;
  },
  click(el, view) {
    let target: number | undefined;
    syntaxTree(view.state).iterate({
      enter(node) {
        if (target !== undefined) return false;
        if (node.name !== "FootnoteDefinition") return;
        const label = node.node.getChild("FootnoteLabel");
        if (label && view.state.doc.sliceString(label.from, label.to) === el.textContent) target = node.from;
        return false;
      },
    });
    return {
      kind: "transaction",
      spec: { selection: { anchor: target ?? view.posAtDOM(el) + 2 }, scrollIntoView: true },
    };
  },
};

/** INLINE are the providers of highlights, tags, inline code and footnote references. */
export const INLINE: readonly Provider[] = [highlights, tags, inlineCode, footnotes];
