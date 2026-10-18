// The status bar (docs/specs/ui.md#status-bar): the connection's state at
// the left while it is not ready; at the right the vault's sync state and
// conflicts, the active note's backlinks, its words and characters, and
// the editing mode. Task T-0050 replaces this stub, which shows the
// connection only.

import type { Connection } from "../../data/stores";
import type { SyncState } from "../../rpc/gen/api";

/** StatusSync is the vault's sync as the bar shows it. */
export interface StatusSync {
  state: SyncState;
  /** The icon's tooltip and name: "Synced 9:41", "Offline: <error>", the state's text. */
  text: string;
  conflicts: number;
}

/** StatusCounts are the active note's, or its selection's, counts. */
export interface StatusCounts {
  words: number;
  characters: number;
}

/** EditingMode is the active note's mode: live preview, source mode or reading. */
export type EditingMode = "live" | "source" | "read";

export interface StatusBarProps {
  connection: Connection;
  /** The vault's sync; null for a vault that does not sync. */
  sync: StatusSync | null;
  /** The active note's backlinks; null without an active note. */
  backlinks: number | null;
  counts: StatusCounts | null;
  mode: EditingMode | null;
  onConflicts: () => void;
  onBacklinks: () => void;
  /** Open the editing mode's menu at (x, y): the button's left and top, in client coordinates. */
  onMode: (x: number, y: number) => void;
}

/** StatusBar draws the window's status bar. */
export function StatusBar({ connection }: StatusBarProps) {
  return (
    <footer
      role="status"
      className="text-status flex h-6 shrink-0 items-center gap-4 border-t border-separator bg-toolbar px-3 text-secondary"
    >
      <span className="min-w-0 flex-1 truncate">
        {connection.state === "connecting" && "Connecting to notesd…"}
        {connection.state === "lost" && `Reconnecting to notesd… (${connection.reason})`}
      </span>
    </footer>
  );
}
