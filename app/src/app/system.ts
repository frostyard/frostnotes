// The system around the page (docs/design/app.md#window): in the desktop
// app the shell's two checked commands (src-tauri/src/open.rs) open a web
// link in the system's browser and a vault file in its default app, the
// system's folder picker names a folder (ADR-0024), and the window's title
// is the system's; in a browser, a web link opens in a new tab, files do
// not open, a folder's path is typed, and the title is the page's.
import { invoke, isTauri } from "@tauri-apps/api/core";
import { getCurrentWindow } from "@tauri-apps/api/window";

import { notify } from "../data/overlays";
import { useNotes } from "../data/stores";

const message = (err: unknown) => (err instanceof Error ? err.message : String(err));

/** openExternal opens a web link (http, https, mailto) outside the app. */
export async function openExternal(url: string): Promise<void> {
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
