// Opening things outside the app (docs/design/app.md#window): in the
// desktop app the shell's two checked commands (src-tauri/src/open.rs) open
// a web link in the system's browser and a vault file in its default app;
// in a browser, a web link opens in a new tab and files do not open.
import { invoke, isTauri } from "@tauri-apps/api/core";

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
