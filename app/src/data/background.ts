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
   * The moment, in milliseconds since the epoch, at which this request read
   * the setting, under the helper's lock; null when it never asked.
   */
  askedMs: number | null;
}

/** Background is the setting and the portal's last answer. */
export interface Background {
  /** On: notesd keeps syncing after the last window closes, and starts at login. */
  keepRunning: boolean;
  /** The portal's last answer; null before the first, or when it cannot be read. */
  portal: PortalAnswer | null;
  /**
   * The moment, in milliseconds since the epoch, at which a `setBackground`
   * call wrote the setting: an answer that read the setting after it has a
   * later `askedMs`. Null from `readBackground`, and when the helper could
   * not start.
   */
  since: number | null;
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
