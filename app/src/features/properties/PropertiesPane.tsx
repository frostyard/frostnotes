// The All properties pane (docs/specs/ui.md, Right sidebar panes). Task
// T-0056 replaces this stub.
import type { LucideIcon } from "lucide-react";

import type { PropertyCount } from "../../rpc/gen/api";
import type { CountSort } from "../panes/common";

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
export const TYPE_ICONS: Readonly<Record<string, LucideIcon>> = {};

/** sortProperties sorts keys by name (A to Z, ignoring case) or by count (most first, ties by name). */
export function sortProperties(properties: readonly PropertyCount[], _sort: CountSort): PropertyCount[] {
  return [...properties];
}

/** PropertiesPane draws the All properties pane. */
export function PropertiesPane(_props: PropertiesPaneProps) {
  return null;
}
