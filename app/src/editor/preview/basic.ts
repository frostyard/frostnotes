// Live preview's first providers, from M0's spike (docs/specs/ui.md#editor-and-reading-view):
// emphasis and strikethrough marks hidden, headings in their sizes, links
// and wikilinks as links (an unresolved one dimmed) that follow on click,
// task checkboxes that toggle one character, code blocks on their color.
// Batch 3's cards add the rest, a provider each.
import { syntaxTree } from "@codemirror/language";
import type { EditorState, TransactionSpec } from "@codemirror/state";
import { Decoration, WidgetType } from "@codemirror/view";
import type { SyntaxNode } from "@lezer/common";

import { whereFor } from "../../workspace/ops";
import { type DrawContext, type Provider, previewEnv } from "./framework";

const SCHEME = /^[a-zA-Z][a-zA-Z0-9+.-]*:/;

/** marks hides emphasis and strikethrough marks away from the selection. */
export const marks: Provider = {
  id: "marks",
  kind: "inline",
  nodes: ["EmphasisMark", "StrikethroughMark"],
  draw(node, cx) {
    if (!cx.revealed(node.from)) cx.hide(node.from, node.to);
    return false;
  },
};

const headingLines = [1, 2, 3, 4, 5, 6].map((n) => Decoration.line({ class: `cm-pv-h${n}` }));

/** headings draws headings in their sizes, their #s hidden away from the selection. */
export const headings: Provider = {
  id: "headings",
  kind: "inline",
  nodes: ["ATXHeading1", "ATXHeading2", "ATXHeading3", "ATXHeading4", "ATXHeading5", "ATXHeading6", "HeaderMark"],
  draw(node, cx) {
    if (node.name !== "HeaderMark") {
      const deco = headingLines[Number(node.name.slice(-1)) - 1];
      if (deco) cx.line(node.from, deco);
      return;
    }
    const heading = node.node.parent;
    if (!heading?.name.startsWith("ATXHeading") || cx.revealed(node.from)) return false;
    const doc = cx.state.doc;
    if (node.from === heading.from)
      cx.hide(node.from, doc.sliceString(node.to, node.to + 1) === " " ? node.to + 1 : node.to);
    else cx.hide(doc.sliceString(node.from - 1, node.from) === " " ? node.from - 1 : node.from, node.to);
    return false;
  },
};

// linkMark is a link's shown text: data-pv sends its clicks to the provider.
function linkMark(provider: string, target: string, unresolved: boolean, href?: string) {
  return Decoration.mark({
    class: unresolved ? "cm-pv-link cm-pv-unresolved" : "cm-pv-link",
    attributes: { "data-pv": provider, "data-target": target, ...(href ? { "data-href": href } : {}) },
  });
}

/** wikilinks shows a wikilink's alias, or its target and subpath, as a link away from the selection. */
export const wikilinks: Provider = {
  id: "wikilink",
  kind: "inline",
  nodes: ["WikiLink"],
  draw(node, cx) {
    if (cx.revealed(node.from)) return false;
    const n = node.node;
    const doc = cx.state.doc;
    const target = n.getChild("WikiLinkTarget");
    const sub = n.getChild("WikiLinkSubpath");
    const alias = n.getChild("WikiLinkAlias");
    const name = target ? doc.sliceString(target.from, target.to).trim() : "";
    // An empty target ([[#Tasks]]) names the note itself, which is always there.
    const unresolved = name !== "" && cx.env.resolve(name) === null;
    const mark = linkMark("wikilink", name, unresolved);
    if (alias) {
      cx.hide(n.from, alias.from);
      cx.add(mark.range(alias.from, alias.to));
      cx.hide(alias.to, n.to);
      return false;
    }
    const first = target ?? sub;
    if (!first) return false;
    const last = sub ?? target ?? first;
    cx.hide(n.from, first.from);
    if (target) cx.add(mark.range(target.from, target.to));
    if (sub) {
      // [[Plan#Goals]] reads "Plan > Goals"; [[#Goals]] reads "Goals".
      cx.add(Decoration.replace({ widget: new TextWidget(target ? " > " : "") }).range(sub.from, sub.from + 1));
      cx.add(mark.range(sub.from + 1, sub.to));
    }
    cx.hide(last.to, n.to);
    return false;
  },
  click(el, _view, e) {
    const target = el.dataset.target ?? "";
    const resolve = _view.state.facet(previewEnv).resolve;
    return { kind: "command", command: { kind: "follow", target, path: resolve(target) ?? null, where: whereFor(e) } };
  },
};

/** links shows a markdown link's text as a link away from the selection. */
export const links: Provider = {
  id: "link",
  kind: "inline",
  nodes: ["Link"],
  draw(node, cx) {
    const n = node.node;
    const [open, close] = n.getChildren("LinkMark");
    const url = n.getChild("URL");
    if (!open || !close || !url || open.from !== n.from || cx.revealed(n.from)) return;
    const doc = cx.state.doc;
    let href = doc.sliceString(url.from, url.to);
    if (href.startsWith("<") && href.endsWith(">")) href = href.slice(1, -1);
    const external = SCHEME.test(href) || href.startsWith("//");
    let target = "";
    if (!external) {
      const path = href.split("#")[0] ?? "";
      try {
        target = decodeURIComponent(path);
      } catch {
        target = path;
      }
    }
    const unresolved = !external && target !== "" && cx.env.resolve(target) === null;
    cx.hide(open.from, open.to);
    cx.add(linkMark("link", target, unresolved, external ? href : undefined).range(open.to, close.from));
    cx.hide(close.from, n.to);
  },
  click(el, view, e) {
    const href = el.dataset.href;
    if (href) return { kind: "command", command: { kind: "follow", target: href, path: null, where: whereFor(e) } };
    const target = el.dataset.target ?? "";
    const path = target === "" ? null : (view.state.facet(previewEnv).resolve(target) ?? null);
    return { kind: "command", command: { kind: "follow", target, path, where: whereFor(e) } };
  },
};

class TextWidget extends WidgetType {
  constructor(readonly text: string) {
    super();
  }
  override eq(other: TextWidget): boolean {
    return other.text === this.text;
  }
  toDOM(): HTMLElement {
    const span = document.createElement("span");
    span.className = "cm-pv-link";
    span.textContent = this.text;
    return span;
  }
}

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
    box.dataset.pv = "task";
    box.setAttribute("aria-label", this.checked ? "Done" : "To do");
    return box;
  }
  // The editor sees mousedown on the box, so the framework sends it to the provider.
  override ignoreEvent(): boolean {
    return false;
  }
}

const checkboxes = [false, true].map((checked) => Decoration.replace({ widget: new CheckboxWidget(checked) }));

/** tasks draws a task's "- [ ]" as a checkbox unless the selection is on it; a click toggles it. */
export const tasks: Provider = {
  id: "task",
  kind: "inline",
  nodes: ["Task"],
  draw(node, cx) {
    task(node.node, cx);
  },
  click(el, view) {
    const spec = toggleTask(view.state, view.posAtDOM(el));
    return spec ? { kind: "transaction", spec } : null;
  },
};

function task(node: SyntaxNode, cx: DrawContext) {
  const marker = node.getChild("TaskMarker");
  const bullet = node.parent?.getChild("ListMark");
  if (!marker || !bullet || cx.touches(bullet.from, marker.to)) return;
  const checked = cx.state.doc.sliceString(marker.from + 1, marker.from + 2) !== " ";
  const deco = checkboxes[checked ? 1 : 0];
  if (deco) cx.add(deco.range(bullet.from, marker.to));
}

/**
 * toggleTask returns the edit that toggles the task whose list item holds
 * pos: the character between the marker's brackets becomes "x" or " ".
 * Nothing else in the document changes.
 */
export function toggleTask(state: EditorState, pos: number): TransactionSpec | null {
  let node: SyntaxNode | null = syntaxTree(state).resolveInner(pos, 1);
  while (node && node.name !== "Task" && node.name !== "ListItem") node = node.parent;
  const t = node?.name === "ListItem" ? node.getChild("Task") : node;
  const marker = t?.getChild("TaskMarker");
  if (!marker) return null;
  const at = marker.from + 1;
  const insert = state.doc.sliceString(at, at + 1) === " " ? "x" : " ";
  return { changes: { from: at, to: at + 1, insert }, userEvent: "input.toggle" };
}

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

/** BASIC are the first providers, in drawing order. */
export const BASIC: readonly Provider[] = [headings, marks, links, wikilinks, tasks, code];
