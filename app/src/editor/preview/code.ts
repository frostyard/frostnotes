// Fenced code blocks in live preview (docs/specs/ui.md#editor-and-reading-view).
// Code lines stay colored; away from the selection, fences give way to
// a language label and a button that asks the app to copy the code.
import { syntaxTree } from "@codemirror/language";
import { Decoration, WidgetType } from "@codemirror/view";
import type { SyntaxNode } from "@lezer/common";

import type { Provider } from "./framework";

class CodeHeaderWidget extends WidgetType {
  constructor(readonly language: string) {
    super();
  }
  override eq(other: CodeHeaderWidget): boolean {
    return other.language === this.language;
  }
  toDOM(): HTMLElement {
    const header = document.createElement("span");
    header.className = "cm-pv-code-header";
    const language = document.createElement("span");
    language.className = "cm-pv-code-lang";
    language.textContent = this.language;
    const copy = document.createElement("button");
    copy.className = "cm-pv-code-copy";
    copy.type = "button";
    copy.setAttribute("aria-label", "Copy code");
    copy.textContent = "Copy";
    copy.dataset.pv = "code";
    header.append(language, copy);
    return header;
  }
  override ignoreEvent(): boolean {
    return false;
  }
}

/** code draws code lines and, away from the selection, a language and copy header. */
export const code: Provider = {
  id: "code",
  kind: "inline",
  nodes: ["FencedCode"],
  draw(node, cx) {
    const doc = cx.state.doc;
    const first = doc.lineAt(node.from).number;
    const last = doc.lineAt(node.to).number;
    for (let number = first; number <= last; number++) {
      const classes = ["cm-pv-code"];
      if (number === first) classes.push("cm-pv-code-first");
      if (number === last) classes.push("cm-pv-code-last");
      cx.line(doc.line(number).from, Decoration.line({ class: classes.join(" ") }));
    }
    if (cx.revealed(node.from, node.to)) return false;
    const [open, close] = node.node.getChildren("CodeMark");
    if (!open) return false;
    const info = node.node.getChild("CodeInfo");
    const language = info ? doc.sliceString(info.from, info.to) : "";
    const openingLine = doc.lineAt(open.from);
    cx.add(Decoration.replace({ widget: new CodeHeaderWidget(language) }).range(open.from, openingLine.to));
    if (close && doc.lineAt(close.from).number > openingLine.number) cx.hide(close.from, doc.lineAt(close.from).to);
    return false;
  },
  click(el, view) {
    let node: SyntaxNode | null = syntaxTree(view.state).resolveInner(view.posAtDOM(el), 1);
    while (node && node.name !== "FencedCode") node = node.parent;
    if (!node) return null;
    const text = node
      .getChildren("CodeText")
      .map((part) => view.state.doc.sliceString(part.from, part.to))
      .join("");
    return { kind: "command", command: { kind: "copy", text } };
  },
};

/** CODE are the code blocks' providers. */
export const CODE: readonly Provider[] = [code];
