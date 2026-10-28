// The Flatpak's setting for background activity, and what the desktop's
// Background portal last answered (src-tauri/src/background.rs;
// docs/design/app.md#the-flatpak, docs/adr/0033-a-setting-keeps-notesd-running-without-a-window.md).
// Only the Flatpak has such a setting: elsewhere notesd is a service, and
// the web app has neither.
import { invoke, isTauri } from "@tauri-apps/api/core";

/** PortalAnswer is the Background portal's last answer. */
export interface PortalAnswer {
  /** When the shell asked, in RFC 3339 UTC time. */
  at: string;
  /** Whether that request asked for autostart at login; false with the setting off. */
  autostartRequested: boolean;
  /** 0 granted, 1 cancelled, 2 other; null when the portal gave no answer. */
  response: number | null;
  /** The portal's background permission. */
  background: boolean | null;
  /** The portal's autostart permission. */
  autostart: boolean | null;
  /** Why the request failed, when it did. */
  error: string | null;
  /**
   * The change's number, as this request read the shell's counter under its
   * lock just before it read the setting; null when it never read it.
   */
  generation: number | null;
}

/** Background is the setting and the portal's last answer. */
export interface Background {
  /** On: notesd keeps syncing after the last window closes, and starts at login. */
  keepRunning: boolean;
  /** The portal's last answer; null before the first, or when it cannot be read. */
  portal: PortalAnswer | null;
  /**
   * This change's number: the counter as a `setBackground` call left it. An
   * answer that read the counter at or after it is about this setting. Null
   * from `readBackground`, and when the helper could not start.
   */
  generation: number | null;
}

/** readBackground reads the setting in the Flatpak, and is null anywhere else or when it cannot. */
export async function readBackground(): Promise<Background | null> {
  if (!isTauri()) return null;
  try {
    return await invoke<Background | null>("background_get");
  } catch {
    return null;
  }
}

/** setBackground stores the setting, asks the desktop again and answers the new state; null as readBackground. */
export async function setBackground(keepRunning: boolean): Promise<Background | null> {
  if (!isTauri()) return null;
  try {
    return await invoke<Background | null>("background_set", { keepRunning });
  } catch {
    return null;
  }
}
