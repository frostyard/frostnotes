// Embedded images, audio and video in live preview (docs/specs/ui.md#editor-and-reading-view).
// Vault media and opted-in web images replace their source away from the selection.
import { Decoration, type EditorView, WidgetType } from "@codemirror/view";

import { fileType } from "../../lib/fileTree";
import type { PreviewAction, PreviewEnv, Provider } from "./framework";

type MediaKind = "image" | "audio" | "video";
interface Size {
  width?: string;
  height?: string;
}

function imageSize(text: string): Size | undefined {
  const match = /^(\d+)(?:x(\d+))?$/.exec(text);
  return match ? { width: match[1], height: match[2] } : undefined;
}

class MediaWidget extends WidgetType {
  constructor(
    readonly kind: MediaKind,
    readonly url: string,
    readonly alt: string,
    readonly size: Size = {},
  ) {
    super();
  }

  override eq(other: MediaWidget): boolean {
    return (
      this.kind === other.kind &&
      this.url === other.url &&
      this.alt === other.alt &&
      this.size.width === other.size.width &&
      this.size.height === other.size.height
    );
  }

  toDOM(): HTMLElement {
    const el = document.createElement(this.kind === "image" ? "img" : this.kind);
    el.className = "cm-pv-media";
    el.dataset.pv = "media";
    el.setAttribute("src", this.url);
    if (this.kind === "image") {
      el.setAttribute("alt", this.alt);
      if (this.size.width !== undefined) el.setAttribute("width", this.size.width);
      if (this.size.height !== undefined) el.setAttribute("height", this.size.height);
    } else {
      el.setAttribute("controls", "");
    }
    return el;
  }

  override ignoreEvent(): boolean {
    return this.kind !== "image";
  }
}

function reveal(el: HTMLElement, view: EditorView): PreviewAction {
  return { kind: "transaction", spec: { selection: { anchor: view.posAtDOM(el) + 2 } } };
}

const embeds: Provider = {
  id: "media",
  kind: "inline",
  nodes: ["Embed"],
  draw(node, cx) {
    if (cx.revealed(node.from)) return;
    const link = node.node.getChild("WikiLink");
    const target = link?.getChild("WikiLinkTarget");
    if (!target) return;
    const doc = cx.state.doc;
    const path = cx.env.resolve(doc.sliceString(target.from, target.to).trim());
    if (typeof path !== "string") return;
    const kind = fileType(path);
    if (kind !== "image" && kind !== "audio" && kind !== "video") return;
    const url = cx.env.fileURL(path);
    if (url === null) return;
    const aliasNode = link?.getChild("WikiLinkAlias");
    const alias = aliasNode ? doc.sliceString(aliasNode.from, aliasNode.to).trim() : "";
    const size = kind === "image" ? imageSize(alias) : undefined;
    const alt = size || !alias ? path.slice(path.lastIndexOf("/") + 1) : alias;
    cx.add(Decoration.replace({ widget: new MediaWidget(kind, url, alt, size) }).range(node.from, node.to));
    return false;
  },
  click: reveal,
};

function imageURL(href: string, env: PreviewEnv): string | null {
  if (/^[a-zA-Z][a-zA-Z0-9+.-]*:/.test(href) || href.startsWith("//")) {
    return /^https?:/i.test(href) && env.remoteImages() ? href : null;
  }
  let decoded = href;
  try {
    decoded = decodeURIComponent(href);
  } catch {
    // A stray percent sign is a literal part of the vault target.
  }
  const path = env.resolve(decoded.split("#")[0] ?? "");
  return typeof path === "string" && fileType(path) === "image" ? env.fileURL(path) : null;
}

const images: Provider = {
  id: "image",
  kind: "inline",
  nodes: ["Image"],
  draw(node, cx) {
    if (cx.revealed(node.from)) return;
    const doc = cx.state.doc;
    // Inline replacements cannot span lines, even when Markdown's image text does.
    if (doc.lineAt(node.from).number !== doc.lineAt(node.to).number) return;
    const [open, close] = node.node.getChildren("LinkMark");
    const urlNode = node.node.getChild("URL");
    if (!open || !close || !urlNode) return;
    let href = doc.sliceString(urlNode.from, urlNode.to);
    if (href.startsWith("<") && href.endsWith(">")) href = href.slice(1, -1);
    const url = imageURL(href, cx.env);
    if (url === null) return;
    const text = doc.sliceString(open.to, close.from);
    const pipe = text.lastIndexOf("|");
    const size = pipe < 0 ? undefined : imageSize(text.slice(pipe + 1));
    const alt = size ? text.slice(0, pipe).trim() : text;
    cx.add(Decoration.replace({ widget: new MediaWidget("image", url, alt, size) }).range(node.from, node.to));
    return false;
  },
  click: reveal,
};

/** MEDIA are the providers of embedded images, audio and video. */
export const MEDIA: readonly Provider[] = [embeds, images];
