// Math, drawn with KaTeX, in live preview (docs/specs/ui.md#editor-and-reading-view).
// Inline and block widgets reveal their source when selected or clicked.
import "katex/dist/katex.min.css";

import { syntaxTree } from "@codemirror/language";
import { Decoration, type EditorView, WidgetType } from "@codemirror/view";
import type { SyntaxNodeRef } from "@lezer/common";
import katex from "katex";

import type { DrawContext, PreviewAction, Provider } from "./framework";

class MathWidget extends WidgetType {
  constructor(
    readonly tex: string,
    readonly displayMode: boolean,
  ) {
    super();
  }
  override eq(other: MathWidget): boolean {
    return this.tex === other.tex && this.displayMode === other.displayMode;
  }
  toDOM(): HTMLElement {
    const el = document.createElement(this.displayMode ? "div" : "span");
    el.className = this.displayMode ? "cm-pv-math cm-pv-math-block" : "cm-pv-math";
    el.dataset.pv = "math";
    try {
      katex.render(this.tex, el, { displayMode: this.displayMode, throwOnError: false });
    } catch {
      el.textContent = this.tex;
    }
    return el;
  }
  override ignoreEvent(): boolean {
    return false;
  }
}

function drawMath(node: SyntaxNodeRef, cx: DrawContext, displayMode: boolean): false {
  if (cx.revealed(node.from, node.to)) return false;
  const marks = node.node.getChildren(displayMode ? "BlockMathMark" : "InlineMathMark");
  const open = marks[0];
  const close = marks.at(-1);
  if (!open || !close || open === close) return false;
  const doc = cx.state.doc;
  const widget = new MathWidget(doc.sliceString(open.to, close.from).trim(), displayMode);
  const from = displayMode ? doc.lineAt(node.from).from : node.from;
  const to = displayMode ? doc.lineAt(node.to).to : node.to;
  cx.add(Decoration.replace({ widget, block: displayMode }).range(from, to));
  return false;
}

function clickMath(el: HTMLElement, view: EditorView): PreviewAction | null {
  const pos = view.posAtDOM(el);
  let anchor: number | undefined;
  // A block widget starts at its line's start, before any indentation.
  syntaxTree(view.state).iterate({
    from: pos,
    to: view.state.doc.lineAt(pos).to,
    enter(node) {
      if (anchor !== undefined) return false;
      if (node.from >= pos && (node.name === "InlineMath" || node.name === "BlockMath")) {
        anchor = node.node.getChild(`${node.name}Mark`)?.to;
        return false;
      }
    },
  });
  return anchor === undefined ? null : { kind: "transaction", spec: { selection: { anchor } } };
}

/** MATH are the providers of inline and block math. */
export const MATH: readonly Provider[] = [
  {
    id: "math",
    kind: "inline",
    nodes: ["InlineMath"],
    draw: (node, cx) => drawMath(node, cx, false),
    click: clickMath,
  },
  {
    id: "math",
    kind: "block",
    nodes: ["BlockMath"],
    draw: (node, cx) => drawMath(node, cx, true),
    click: clickMath,
  },
];
