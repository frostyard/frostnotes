// The system around the page (docs/design/app.md#window): in the desktop
// app the shell's two checked commands (src-tauri/src/open.rs) open a web
// link in the system's browser and a vault file in its default app, the
// system's folder picker names a folder (ADR-0024), and the window's title
// is the system's; in a browser, a web link opens in a new tab, a file the
// browser shows opens in one by its content URL, a folder's path is typed,
// and the title is the page's.
import { invoke, isTauri } from "@tauri-apps/api/core";
import { getCurrentWindow } from "@tauri-apps/api/window";

import { useFiles } from "../data/files";
import { notify } from "../data/overlays";
import { useNotes } from "../data/stores";
import { extensionOf, fileType } from "../lib/fileTree";
import { useLodge } from "../lodge/lodge";
import type { Client } from "../rpc/gen/api";

const message = (err: unknown) => (err instanceof Error ? err.message : String(err));

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
  if (!isTauri()) {
    window.open(url, "_blank", "noopener,noreferrer");
    return;
  }
  try {
    await invoke("open_url", { url });
  } catch (err) {
    notify(`Cannot open ${url}: ${message(err)}`);
  }
}

/** openInDefaultApp opens a vault file with the system's application for it. */
export async function openInDefaultApp(vaultId: number, path: string): Promise<void> {
  const root = useNotes.getState().vaults.find((v) => v.id === vaultId)?.path;
  if (!isTauri() || root === undefined) {
    notify("Open in default app works in the desktop app.");
    return;
  }
  try {
    await invoke("open_file", { root, path });
  } catch (err) {
    notify(`Cannot open ${path}: ${message(err)}`);
  }
}

/** browserShows reports whether a browser shows a file of path's type in a tab of its own: images, audio, video and PDF. */
export function browserShows(path: string): boolean {
  const type = fileType(path);
  return type === "image" || type === "audio" || type === "video" || type === "pdf";
}

/**
 * openOutsideLabel names a file's way out of the app, or is null where it
 * has none: Open in default app in the desktop app; in a browser, Open in
 * new browser tab for a file the browser shows, and nothing for any other
 * (docs/specs/ui.md#in-a-browser). A file menu's Open in new tab is the
 * workspace's.
 */
export function openOutsideLabel(path: string): string | null {
  if (isTauri()) return "Open in default app";
  return browserShows(path) ? "Open in new browser tab" : null;
}

/** openOutside opens a vault file outside the app: with its default app, or in a browser by its content URL in a new tab. */
export async function openOutside(c: Client, vaultId: number, path: string): Promise<void> {
  if (isTauri()) return openInDefaultApp(vaultId, path);
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

/** canPickFolders reports whether the system has a folder picker here: the desktop app's. */
export function canPickFolders(): boolean {
  return isTauri();
}

/** pickFolder asks the system's folder picker for a folder: its path, or null when cancelled or where there is no picker. */
export async function pickFolder(title: string): Promise<string | null> {
  if (!canPickFolders()) return null;
  const { open } = await import("@tauri-apps/plugin-dialog");
  const picked = await open({ directory: true, multiple: false, title });
  return typeof picked === "string" ? picked : null;
}

/** setWindowTitle names the window as a task bar shows it: the system's title in the desktop app, the page's in a browser. */
export function setWindowTitle(title: string): void {
  document.title = title;
  if (isTauri()) void getCurrentWindow().setTitle(title);
}

/** windowTitle is the window's title (ADR-0024): the active note and the window's vault. */
export function windowTitle(note: string | null, vault: string | null): string {
  return [note, vault, "Frostnotes"].filter((part): part is string => part !== null && part !== "").join(" - ");
}
