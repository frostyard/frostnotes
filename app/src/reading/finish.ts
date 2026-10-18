// What the reading view and live preview's rendered blocks fill in after
// sanitized HTML is in place (docs/design/app.md#reading-view).
import "katex/dist/katex.min.css";

import katex from "katex";

/**
 * finish fills in what the HTML names but does not load: each data-blob
 * element's URL, math drawn with KaTeX, and remote images unless the
 * preference loads them. A task's checkbox, which notesd renders disabled
 * (docs/specs/render.md), is enabled: a click toggles its line.
 */
export function finish(el: HTMLElement, blobURL: (hash: string, ext: string) => string, remoteImages: boolean): void {
  for (const box of el.querySelectorAll<HTMLInputElement>('input[type="checkbox"][data-line]')) box.disabled = false;
  for (const node of el.querySelectorAll<HTMLElement>("[data-blob]")) {
    const blob = node.dataset.blob ?? "";
    const dot = blob.lastIndexOf(".");
    const url = blobURL(dot < 0 ? blob : blob.slice(0, dot), dot < 0 ? "" : blob.slice(dot + 1));
    if (node instanceof HTMLImageElement || node instanceof HTMLMediaElement) node.src = url;
  }
  if (!remoteImages) {
    for (const img of el.querySelectorAll<HTMLImageElement>("img[src]:not([data-blob])")) img.removeAttribute("src");
  }
  for (const node of el.querySelectorAll<HTMLElement>(".math[data-tex]")) {
    katex.render(node.dataset.tex ?? "", node, { displayMode: node.tagName === "DIV", throwOnError: false });
  }
}
