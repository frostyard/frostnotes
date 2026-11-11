// The system around the page (docs/design/app.md#window): a web link opens
// in a new tab, a file the browser shows opens in one by its content URL, a
// folder's path is typed, and the title is the page's.
import { useFiles } from "../data/files";
import { notify } from "../data/overlays";
import { extensionOf, fileType } from "../lib/fileTree";
import { useLodge } from "../lodge/lodge";
import type { Client } from "../rpc/gen/api";

/**
 * openExternal opens a web link (http, https, mailto) outside the app:
 * inside lodge the shell opens it, a lodge URL in its app and any other in
 * a new tab (lodge-client's open).
 */
export async function openExternal(url: string): Promise<void> {
  const shell = useLodge.getState().lodge;
  if (shell) {
    shell.open(url);
    return;
  }
  window.open(url, "_blank", "noopener,noreferrer");
}

/** browserShows reports whether a browser shows a file of path's type in a tab of its own: images, audio, video and PDF. */
export function browserShows(path: string): boolean {
  const type = fileType(path);
  return type === "image" || type === "audio" || type === "video" || type === "pdf";
}

/**
 * openOutsideLabel names a file's way out of the app, or is null where it
 * has none: Open in new browser tab for a file the browser shows, and
 * nothing for any other (docs/specs/ui.md#in-a-browser). A file menu's Open
 * in new tab is the workspace's.
 */
export function openOutsideLabel(path: string): string | null {
  return browserShows(path) ? "Open in new browser tab" : null;
}

/** openOutside opens a vault file outside the app, by its content URL in a new tab. */
export async function openOutside(c: Client, path: string): Promise<void> {
  const hash = useFiles.getState().files.get(path)?.hash;
  const url = hash === undefined ? undefined : c.transport.blobURL?.(hash, extensionOf(path));
  if (!url) {
    notify(`Cannot open ${path} here.`);
    return;
  }
  await openExternal(url);
}

/**
 * suppressNativeMenu keeps the webview's own context menu from showing
 * (docs/design/app.md#window): the app draws its own menus, and the
 * webview's would offer to download a link, save an image, or reload the
 * app. It returns the removal.
 */
export function suppressNativeMenu(
  target: Pick<Window, "addEventListener" | "removeEventListener"> = window,
): () => void {
  const stop = (e: Event) => e.preventDefault();
  target.addEventListener("contextmenu", stop);
  return () => target.removeEventListener("contextmenu", stop);
}

/** setWindowTitle names the window as a task bar shows it: the page's title. */
export function setWindowTitle(title: string): void {
  document.title = title;
}

/** windowTitle is the window's title (ADR-0024): the active note and the window's vault. */
export function windowTitle(note: string | null, vault: string | null): string {
  return [note, vault, "Frostnotes"].filter((part): part is string => part !== null && part !== "").join(" - ");
}
