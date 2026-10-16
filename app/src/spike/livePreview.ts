// Live preview for spike 1 (ADR-0007): decorations over the markdown source,
// never a transformation of it. Away from the selection it hides emphasis
// and strikethrough marks, heading #s and link and wikilink brackets (a
// wikilink with an alias shows only the alias), and draws a checkbox for a
// task; on the lines the selection touches the source shows as typed. A
// checkbox toggles by editing the one character between its brackets.
import { syntaxTree } from "@codemirror/language";
import type { EditorState, Range, TransactionSpec } from "@codemirror/state";
import {
  Decoration,
  type DecorationSet,
  type EditorView,
  ViewPlugin,
  type ViewUpdate,
  WidgetType,
} from "@codemirror/view";
import type { SyntaxNode } from "@lezer/common";

/** PreviewKind names each decoration live preview draws (in its spec, for tests and checks). */
export type PreviewKind = "hide" | "heading" | "link" | "wikilink" | "checkbox" | "code";

const hidden = Decoration.replace({ preview: "hide" });
const linkText = Decoration.mark({ class: "cm-pv-link", preview: "link" });
const wikiText = Decoration.mark({ class: "cm-pv-wikilink", preview: "wikilink" });
const headingLines = [1, 2, 3, 4, 5, 6].map((n) => Decoration.line({ class: `cm-pv-h${n}`, preview: "heading" }));
const codeLine = Decoration.line({ class: "cm-pv-code", preview: "code" });

class CheckboxWidget extends WidgetType {
  constructor(readonly checked: boolean) {
    super();
  }

  override eq(other: CheckboxWidget): boolean {
    return other.checked === this.checked;
  }

  toDOM(): HTMLElement {
    const box = document.createElement("input");
    box.type = "checkbox";
    box.className = "cm-pv-task";
    box.checked = this.checked;
    box.setAttribute("aria-label", this.checked ? "Done" : "To do");
    return box;
  }

  // The editor sees mousedown on the box, so livePreview's handler can toggle it.
  override ignoreEvent(): boolean {
    return false;
  }
}

const checkboxes = [false, true].map((checked) =>
  Decoration.replace({ widget: new CheckboxWidget(checked), preview: "checkbox" }),
);

interface Span {
  from: number;
  to: number;
}

/**
 * buildDecorations computes live preview over ranges (the visible ranges)
 * of state. Syntax shows on every line a selection range touches.
 */
export function buildDecorations(state: EditorState, ranges: readonly Span[]): DecorationSet {
  const doc = state.doc;
  const sel = state.selection.ranges.map((r) => ({ from: doc.lineAt(r.from).from, to: doc.lineAt(r.to).to }));
  const lineActive = (pos: number) => sel.some((r) => r.from <= pos && pos <= r.to);
  const touches = (from: number, to: number) => state.selection.ranges.some((r) => r.from <= to && r.to >= from);
  const out: Range<Decoration>[] = [];
  const seen = new Set<string>();
  const lines = new Set<number>();
  const line = (deco: Decoration, pos: number) => {
    const start = doc.lineAt(pos).from;
    if (!lines.has(start)) {
      lines.add(start);
      out.push(deco.range(start));
    }
  };
  // Hidden ranges stay within one line: a plugin may not replace line breaks.
  const hide = (from: number, to: number) => {
    if (from < to && doc.lineAt(from).number === doc.lineAt(to).number) out.push(hidden.range(from, to));
  };

  const tree = syntaxTree(state);
  for (const { from, to } of ranges) {
    tree.iterate({
      from,
      to,
      enter: (ref) => {
        // A node across two visible ranges is entered twice; draw it once.
        if (ranges.length > 1) {
          const key = `${ref.name}@${ref.from}`;
          if (seen.has(key)) return false;
          seen.add(key);
        }
        switch (ref.name) {
          case "ATXHeading1":
          case "ATXHeading2":
          case "ATXHeading3":
          case "ATXHeading4":
          case "ATXHeading5":
          case "ATXHeading6": {
            const deco = headingLines[Number(ref.name.slice(-1)) - 1];
            if (deco) line(deco, ref.from);
            return;
          }
          case "HeaderMark": {
            const heading = ref.node.parent;
            if (!heading?.name.startsWith("ATXHeading") || lineActive(ref.from)) return false;
            if (ref.from === heading.from) {
              hide(ref.from, doc.sliceString(ref.to, ref.to + 1) === " " ? ref.to + 1 : ref.to);
            } else {
              hide(doc.sliceString(ref.from - 1, ref.from) === " " ? ref.from - 1 : ref.from, ref.to);
            }
            return false;
          }
          case "EmphasisMark":
          case "StrikethroughMark":
            if (!lineActive(ref.from)) hide(ref.from, ref.to);
            return false;
          case "Link":
            inlineLink(ref.node);
            return;
          case "WikiLink":
            wikiLink(ref.node);
            return false;
          case "Task":
            task(ref.node);
            return;
          case "FencedCode":
            for (let pos = Math.max(ref.from, from); pos <= Math.min(ref.to, to); pos = doc.lineAt(pos).to + 1) {
              line(codeLine, pos);
            }
            return false;
          default:
            return;
        }
      },
    });
  }
  return Decoration.set(out, true);

  // [text](url): only the text shows, as a link.
  function inlineLink(link: SyntaxNode) {
    const [open, close] = link.getChildren("LinkMark");
    if (!open || !close || open.from !== link.from || !link.getChild("URL") || lineActive(link.from)) return;
    hide(open.from, open.to);
    out.push(linkText.range(open.to, close.from));
    hide(close.from, link.to);
  }

  // [[target]] shows the target, [[target|alias]] the alias.
  function wikiLink(link: SyntaxNode) {
    if (lineActive(link.from)) return;
    const shown = link.getChild("WikiLinkAlias") ?? link.getChild("WikiLinkTarget");
    if (!shown) return;
    hide(link.from, shown.from);
    out.push(wikiText.range(shown.from, shown.to));
    hide(shown.to, link.to);
  }

  // - [ ] and - [x] become a checkbox unless the selection is on them.
  function task(node: SyntaxNode) {
    const marker = node.getChild("TaskMarker");
    const bullet = node.parent?.getChild("ListMark");
    if (!marker || !bullet || touches(bullet.from, marker.to)) return;
    const checked = doc.sliceString(marker.from + 1, marker.from + 2) !== " ";
    const deco = checkboxes[checked ? 1 : 0];
    if (deco) out.push(deco.range(bullet.from, marker.to));
  }
}

/**
 * toggleTask returns the edit that toggles the task whose list item holds
 * pos: the character between the marker's brackets becomes "x" or " ".
 * Nothing else in the document changes.
 */
export function toggleTask(state: EditorState, pos: number): TransactionSpec | null {
  let node: SyntaxNode | null = syntaxTree(state).resolveInner(pos, 1);
  while (node && node.name !== "Task" && node.name !== "ListItem") node = node.parent;
  const task = node?.name === "ListItem" ? node.getChild("Task") : node;
  const marker = task?.getChild("TaskMarker");
  if (!marker) return null;
  const at = marker.from + 1;
  const insert = state.doc.sliceString(at, at + 1) === " " ? "x" : " ";
  return { changes: { from: at, to: at + 1, insert }, userEvent: "input.toggle" };
}

/** livePreview draws buildDecorations over the visible ranges and toggles checkboxes. */
export const livePreview = ViewPlugin.fromClass(
  class {
    decorations: DecorationSet;

    constructor(view: EditorView) {
      this.decorations = buildDecorations(view.state, view.visibleRanges);
    }

    update(u: ViewUpdate) {
      if (u.docChanged || u.viewportChanged || u.selectionSet || syntaxTree(u.startState) !== syntaxTree(u.state)) {
        this.decorations = buildDecorations(u.state, u.view.visibleRanges);
      }
    }
  },
  {
    decorations: (v) => v.decorations,
    eventHandlers: {
      mousedown(e, view) {
        const box = e.target;
        if (!(box instanceof HTMLInputElement) || !box.classList.contains("cm-pv-task")) return false;
        const spec = toggleTask(view.state, view.posAtDOM(box));
        if (!spec) return false;
        e.preventDefault();
        view.dispatch(spec);
        return true;
      },
    },
  },
);
