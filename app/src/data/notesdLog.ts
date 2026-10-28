// What the desktop app's shell reads of notesd's log, to say why notesd is
// not running (src-tauri/src/notesd_log.rs; docs/design/app.md#the-flatpak).
// Only the Flatpak has such a log: elsewhere, and in the web app, there is
// nothing to read.
import { invoke, isTauri } from "@tauri-apps/api/core";

/** NotesdLog is the end of notesd.log. */
export interface NotesdLog {
  /** The log's path. */
  path: string;
  /** Its last line with text; null when the log is empty or missing. */
  lastLine: string | null;
  /** That line is the launcher's: the Flatpak portal did not start notesd. */
  portalRefused: boolean;
}

/** readNotesdLog reads notesd.log's end in the Flatpak, and is null anywhere else or when it cannot. */
export async function readNotesdLog(): Promise<NotesdLog | null> {
  if (!isTauri()) return null;
  try {
    return await invoke<NotesdLog | null>("notesd_log");
  } catch {
    return null;
  }
}
