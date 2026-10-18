// The history's lists (docs/specs/ui.md, History). Task T-0061 replaces
// this stub.
import type { LucideIcon } from "lucide-react";

/** HistorySource is who made a version: the editor, an agent, another program, or sync. */
export type HistorySource = "editor" | "agent" | "outside" | "sync";

/** HistoryEntry is a row of the history. */
export interface HistoryEntry {
  /** Unique across both lists. */
  id: string;
  /** RFC 3339. */
  at: string;
  source: HistorySource;
  /** The author, or a device's name; may be empty. */
  author: string;
  /** In bytes; null when unknown. */
  size: number | null;
  conflict: boolean;
  deleted: boolean;
}

export interface HistoryListProps {
  versions: readonly HistoryEntry[];
  /** The hub's revisions; null for a vault that does not sync. */
  revisions: readonly HistoryEntry[] | null;
  selected: string | null;
  onSelect: (id: string) => void;
  /** What "today" is; the current time by default. */
  now?: Date;
}

/** SOURCE_ICONS are the sources' icons. */
export const SOURCE_ICONS: Readonly<Partial<Record<HistorySource, LucideIcon>>> = {};

/** HistoryList draws the history's lists. */
export function HistoryList(_props: HistoryListProps) {
  return null;
}
