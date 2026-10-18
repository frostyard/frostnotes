// The All properties pane (docs/specs/ui.md, Right sidebar panes): sorted
// frontmatter keys with type icons, counts, and callbacks for choosing a key.
import {
  ArrowUpNarrowWide,
  Binary,
  Calendar,
  Clock,
  Forward,
  List,
  type LucideIcon,
  SquareCheck,
  Tags,
  Text,
} from "lucide-react";

import type { PropertyCount } from "../../rpc/gen/api";
import { IconButton } from "../chrome/IconButton";
import { type CountSort, paneEmpty, paneRow, paneToolbar } from "../panes/common";

export interface PropertiesPaneProps {
  /** The vault's keys; null while they load. */
  properties: readonly PropertyCount[] | null;
  sort: CountSort;
  /** Open the sort order's menu at (x, y): the button's left and bottom. */
  onSortMenu: (x: number, y: number) => void;
  /** List the notes with the key. */
  onChoose: (key: string) => void;
}

/** TYPE_ICONS are the property types' icons; a key with no type is text. */
export const TYPE_ICONS: Readonly<Record<string, LucideIcon>> = {
  text: Text,
  multitext: List,
  number: Binary,
  checkbox: SquareCheck,
  date: Calendar,
  datetime: Clock,
  aliases: Forward,
  tags: Tags,
};

/** sortProperties sorts keys by name (A to Z, ignoring case) or by count (most first, ties by name). */
export function sortProperties(properties: readonly PropertyCount[], sort: CountSort): PropertyCount[] {
  return [...properties].sort(
    (a, b) => (sort === "count" ? b.count - a.count : 0) || a.key.toLowerCase().localeCompare(b.key.toLowerCase()),
  );
}

/** PropertiesPane draws the All properties pane. */
export function PropertiesPane({ properties, sort, onSortMenu, onChoose }: PropertiesPaneProps) {
  return (
    <>
      <div className={paneToolbar}>
        <IconButton
          label="Change sort order"
          size={24}
          icon={<ArrowUpNarrowWide size={16} aria-hidden="true" />}
          onClick={(event) => {
            const { left, bottom } = event.currentTarget.getBoundingClientRect();
            onSortMenu(left, bottom);
          }}
        />
      </div>
      {properties !== null &&
        (properties.length === 0 ? (
          <p className={paneEmpty}>No properties found.</p>
        ) : (
          <ul aria-label="All properties">
            {sortProperties(properties, sort).map(({ key, type, count }) => {
              const Icon = (Object.hasOwn(TYPE_ICONS, type) ? TYPE_ICONS[type] : undefined) ?? Text;
              return (
                <li key={key}>
                  <button
                    type="button"
                    className={`${paneRow} w-[calc(100%-1rem)] px-2 text-left`}
                    data-type={type || "text"}
                    onClick={() => onChoose(key)}
                  >
                    <Icon size={14} className="shrink-0 text-secondary" aria-hidden="true" />
                    <span className="min-w-0 flex-1 truncate">{key}</span>
                    <span className="text-status shrink-0 text-secondary">{count}</span>
                  </button>
                </li>
              );
            })}
          </ul>
        ))}
    </>
  );
}
