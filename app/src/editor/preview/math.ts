// Math, drawn with KaTeX, in live preview (docs/specs/ui.md#editor-and-reading-view).
// Inline and block widgets reveal their source when selected or clicked.
// Each formula is drawn once and cloned, its MathML attached a frame later
// (docs/design/app.md#live-preview).
import "katex/dist/katex.min.css";

import { syntaxTree } from "@codemirror/language";
import { Decoration, type EditorView, WidgetType } from "@codemirror/view";
import type { SyntaxNodeRef } from "@lezer/common";
import katex from "katex";

import type { DrawContext, PreviewAction, Provider } from "./framework";

// DRAWN_MAX is how many formulas' drawings a page keeps.
const DRAWN_MAX = 500;

/** Drawn is KaTeX's drawing of a formula: its element, and its MathML laid aside. */
interface Drawn {
  el: HTMLElement;
  mathml: Element | null;
}

// KaTeX's drawings by mode and TeX, oldest first: a widget clones one,
// since drawing a formula again costs more than copying it.
const drawings = new Map<string, Drawn>();

// drawing returns the formula's drawing, made once. A formula KaTeX
// refuses to draw shows its TeX and is not kept.
function drawing(tex: string, displayMode: boolean): Drawn {
  const key = `${displayMode ? "D" : "I"}${tex}`;
  const kept = drawings.get(key);
  if (kept) {
    drawings.delete(key);
    drawings.set(key, kept);
    return kept;
  }
  const el = document.createElement(displayMode ? "div" : "span");
  el.className = displayMode ? "cm-pv-math cm-pv-math-block" : "cm-pv-math";
  el.dataset.pv = "math";
  try {
    katex.render(tex, el, { displayMode, throwOnError: false });
  } catch {
    el.textContent = tex;
    return { el, mathml: null };
  }
  const mathml = el.querySelector(".katex-mathml");
  mathml?.remove();
  const drawn = { el, mathml };
  drawings.set(key, drawn);
  for (const oldest of drawings.keys()) {
    if (drawings.size <= DRAWN_MAX) break;
    drawings.delete(oldest);
  }
  return drawn;
}

// The MathML waiting for the next frame, with the KaTeX element it goes in.
let pending: [Element, Element][] = [];

function attachPending() {
  const batch = pending;
  pending = [];
  for (const [katexEl, mathml] of batch) if (katexEl.isConnected) katexEl.prepend(mathml.cloneNode(true));
}

// attachMathML puts a formula's MathML, which only assistive technology
// reads (KaTeX hides it), in place in the next frame: laying it out was
// about half of what a formula cost the frame that scrolled it in.
function attachMathML(katexEl: Element, mathml: Element) {
  if (typeof requestAnimationFrame !== "function") {
    katexEl.prepend(mathml.cloneNode(true));
    return;
  }
  if (pending.length === 0) requestAnimationFrame(attachPending);
  pending.push([katexEl, mathml]);
}

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
  // Display math at the default size: KaTeX's block of about three lines
  // (two with limits above and below), its margins and padding.
  override get estimatedHeight(): number {
    return this.displayMode ? 100 : -1;
  }
  toDOM(): HTMLElement {
    const drawn = drawing(this.tex, this.displayMode);
    const el = drawn.el.cloneNode(true) as HTMLElement;
    // KaTeX's element: .katex, inside .katex-display in display mode.
    const katexEl = this.displayMode ? el.firstElementChild?.firstElementChild : el.firstElementChild;
    if (drawn.mathml && katexEl) attachMathML(katexEl, drawn.mathml);
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
