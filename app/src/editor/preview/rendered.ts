// Callouts, tables and note embeds, drawn as notesd renders them, in live preview (docs/specs/ui.md#editor-and-reading-view).
// Whole blocks delegate their HTML to the preview environment and reveal
// their source when selected; links and callout titles keep their actions.
import { Decoration, type EditorView, WidgetType } from "@codemirror/view";
import type { SyntaxNodeRef } from "@lezer/common";

import { whereFor } from "../../workspace/ops";
import { readCallout } from "../syntax/callouts";
import { type DrawContext, type PreviewAction, type Provider, previewEnv } from "./framework";

/** RenderedWidget shows a block through the environment's sanitized renderer. */
export class RenderedWidget extends WidgetType {
  constructor(
    readonly provider: string,
    readonly markdown: string,
    readonly className: string,
  ) {
    super();
  }

  override eq(other: RenderedWidget): boolean {
    return this.provider === other.provider && this.markdown === other.markdown && this.className === other.className;
  }

  toDOM(view: EditorView): HTMLElement {
    const el = document.createElement("div");
    el.className = `cm-pv-rendered rendered ${this.className}`;
    el.dataset.pv = this.provider;
    view.state.facet(previewEnv).render(el, this.markdown);
    return el;
  }

  override ignoreEvent(): boolean {
    return false;
  }
}

function drawBlock(node: SyntaxNodeRef, cx: DrawContext, provider: string, className: string): false {
  const doc = cx.state.doc;
  const from = doc.lineAt(node.from).from;
  const to = doc.lineAt(node.to).to;
  const widget = new RenderedWidget(provider, doc.sliceString(from, to), className);
  cx.add(Decoration.replace({ widget, block: true }).range(from, to));
  return false;
}

function startsLine(node: SyntaxNodeRef, cx: DrawContext): boolean {
  const doc = cx.state.doc;
  return doc.sliceString(doc.lineAt(node.from).from, node.from).trim() === "";
}

/** renderedClick follows links, searches tags, folds callouts, or reveals the block's source. */
export function renderedClick(el: HTMLElement, view: EditorView, event: MouseEvent): PreviewAction {
  const target = event.target instanceof Element ? event.target : null;
  const tag = target?.closest<HTMLAnchorElement>("a.tag[data-tag]");
  if (tag) return { kind: "command", command: { kind: "search", query: `tag:#${tag.dataset.tag ?? ""}` } };
  const link = target?.closest<HTMLAnchorElement>("a.internal-link, a.file-embed, a.external-link");
  if (link) {
    const external = link.classList.contains("external-link");
    return {
      kind: "command",
      command: {
        kind: "follow",
        target: external ? (link.getAttribute("href") ?? "") : ((link.dataset.href ?? "").split("#")[0] ?? ""),
        path: external ? null : (link.dataset.path ?? null),
        where: whereFor(event),
      },
    };
  }
  const title = target?.closest<HTMLElement>(".callout[data-fold] > .callout-title");
  if (title?.parentElement) {
    const callout = title.parentElement;
    callout.dataset.fold = callout.dataset.fold === "-" ? "+" : "-";
    return { kind: "handled" };
  }
  return { kind: "transaction", spec: { selection: { anchor: view.posAtDOM(el) } } };
}

/** RENDERED are the providers of callouts, tables and note embeds. */
export const RENDERED: readonly Provider[] = [
  {
    id: "callout",
    kind: "block",
    nodes: ["Blockquote"],
    draw(node, cx) {
      if (!startsLine(node, cx) || cx.revealed(node.from, node.to) || !readCallout(cx.state, node.node)) return;
      return drawBlock(node, cx, "callout", "cm-pv-callout");
    },
    click: renderedClick,
  },
  {
    id: "table",
    kind: "block",
    nodes: ["Table"],
    draw(node, cx) {
      if (!startsLine(node, cx) || cx.revealed(node.from, node.to)) return;
      return drawBlock(node, cx, "table", "cm-pv-table");
    },
    click: renderedClick,
  },
  {
    id: "embed",
    kind: "block",
    nodes: ["Embed"],
    draw(node, cx) {
      const doc = cx.state.doc;
      const line = doc.lineAt(node.from);
      if (cx.revealed(line.from, line.to) || line.text.trim() !== doc.sliceString(node.from, node.to)) return;
      const targetNode = node.node.getChild("WikiLink")?.getChild("WikiLinkTarget");
      const target = targetNode ? doc.sliceString(targetNode.from, targetNode.to).trim() : "";
      if (!cx.env.resolve(target)?.endsWith(".md")) return;
      return drawBlock(node, cx, "embed", "cm-pv-embed");
    },
    click: renderedClick,
  },
];
