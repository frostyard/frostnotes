// The status bar (docs/specs/ui.md#status-bar): the connection's state at
// the left while it is not ready; at the right the vault's sync state and
// conflicts, the active note's backlinks, its words and characters, and
// the editing mode. The container supplies the counts and handles actions.

import { BookOpen, CircleAlert, Cloud, CloudOff, CodeXml, Pencil, RefreshCw } from "lucide-react";
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

const syncIcons = { idle: Cloud, syncing: RefreshCw, offline: CloudOff, revoked: CircleAlert, reset: CircleAlert };
const editingModes = {
  live: { name: "Live Preview", icon: Pencil },
  source: { name: "Source mode", icon: CodeXml },
  read: { name: "Reading view", icon: BookOpen },
};
const buttonClass = "rounded px-1 hover:bg-selection-inactive hover:text-primary";

function countText(count: number, noun: string): string {
  return `${count.toLocaleString("en-US")} ${noun}${count === 1 ? "" : "s"}`;
}

/** StatusBar draws the window's status bar. */
export function StatusBar({
  connection,
  sync,
  backlinks,
  counts,
  mode,
  onConflicts,
  onBacklinks,
  onMode,
}: StatusBarProps) {
  const SyncIcon = sync === null ? null : syncIcons[sync.state];
  const editingMode = mode === null ? null : editingModes[mode];
  const ModeIcon = editingMode?.icon;
  return (
    <footer
      role="status"
      className="text-status flex h-6 shrink-0 items-center gap-4 border-t border-separator bg-toolbar px-3 text-secondary"
    >
      <span className="min-w-0 flex-1 truncate">
        {connection.state === "connecting" && "Connecting to notesd…"}
        {connection.state === "lost" && `Reconnecting to notesd… (${connection.reason})`}
      </span>
      {sync !== null && SyncIcon !== null && (
        <>
          <span role="img" aria-label={sync.text} title={sync.text} data-sync-state={sync.state}>
            <SyncIcon size={14} className={sync.state === "syncing" ? "animate-spin" : undefined} />
          </span>
          {sync.conflicts > 0 && (
            <button type="button" className={buttonClass} onClick={onConflicts}>
              {countText(sync.conflicts, "conflict")}
            </button>
          )}
        </>
      )}
      {backlinks !== null && (
        <button type="button" className={buttonClass} onClick={onBacklinks}>
          {countText(backlinks, "backlink")}
        </button>
      )}
      {counts !== null && (
        <>
          <span>{countText(counts.words, "word")}</span>
          <span>{countText(counts.characters, "character")}</span>
        </>
      )}
      {editingMode !== null && ModeIcon && (
        <button
          type="button"
          className={buttonClass}
          aria-haspopup="menu"
          aria-label={`Editing mode: ${editingMode.name}`}
          title={editingMode.name}
          onClick={(event) => {
            const { left, top } = event.currentTarget.getBoundingClientRect();
            onMode(left, top);
          }}
        >
          <ModeIcon size={14} />
        </button>
      )}
    </footer>
  );
}
