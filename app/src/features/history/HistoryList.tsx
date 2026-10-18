// The history's selectable versions and hub revisions (docs/specs/ui.md,
// History), with local times, source icons and version details.
import { Bot, Cloud, HardDrive, type LucideIcon, Pencil } from "lucide-react";

import { sizeText, whenText } from "../../lib/when";

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
export const SOURCE_ICONS: Readonly<Record<HistorySource, LucideIcon>> = {
  editor: Pencil,
  agent: Bot,
  outside: HardDrive,
  sync: Cloud,
};

function Entries({
  entries,
  selected,
  onSelect,
  now,
}: Pick<HistoryListProps, "selected" | "onSelect"> & {
  entries: readonly HistoryEntry[];
  now: Date;
}) {
  return (
    <ul className="px-2">
      {entries
        .toSorted((a, b) => Date.parse(b.at) - Date.parse(a.at))
        .map((entry) => {
          const Icon = SOURCE_ICONS[entry.source];
          const details = entry.deleted
            ? "Deleted"
            : [entry.author, entry.size === null ? "" : sizeText(entry.size)].filter(Boolean).join(" · ");
          return (
            <li key={entry.id}>
              <button
                type="button"
                aria-current={selected === entry.id ? "true" : undefined}
                data-entry={entry.id}
                onClick={() => onSelect(entry.id)}
                className={`flex h-11 w-full flex-col justify-center rounded-md px-2 text-left ${
                  selected === entry.id ? "bg-selection-sidebar" : "hover:bg-selection-inactive"
                }`}
              >
                <span className="flex items-center gap-2 text-sidebar-row">
                  {whenText(new Date(entry.at), now)}
                  {entry.conflict && (
                    <span className="rounded bg-badge px-1 text-keycap text-badge-text">Conflict</span>
                  )}
                </span>
                <span className="flex items-center gap-1.5 text-xs text-secondary">
                  <Icon size={12} aria-hidden="true" data-source={entry.source} className="shrink-0" />
                  {details}
                </span>
              </button>
            </li>
          );
        })}
    </ul>
  );
}

/** HistoryList draws the history's lists. */
export function HistoryList({ versions, revisions, selected, onSelect, now = new Date() }: HistoryListProps) {
  return (
    <nav aria-label="Versions" className="py-2">
      {versions.length > 0 ? (
        <Entries entries={versions} selected={selected} onSelect={onSelect} now={now} />
      ) : (
        <p className="px-4 text-sidebar-row text-secondary">No versions yet.</p>
      )}
      {revisions !== null && (
        <section aria-label="On the hub" className="mt-3">
          <h3 className="px-4 text-sidebar-section text-tertiary uppercase">On the hub</h3>
          {revisions.length > 0 ? (
            <Entries entries={revisions} selected={selected} onSelect={onSelect} now={now} />
          ) : (
            <p className="px-4 text-sidebar-row text-secondary">No revisions on the hub.</p>
          )}
        </section>
      )}
    </nav>
  );
}
