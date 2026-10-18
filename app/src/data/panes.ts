// The sidebar panes' state beyond the workspace (docs/specs/ui.md,
// Sidebars and panes): the search's query, which other panes and commands
// set; the Tags and All properties panes' sorts; the row being renamed in
// the Files pane; the Outline's folded headings, by note. In memory only:
// a new window starts them afresh.
import { create } from "zustand";

import type { CountSort } from "../features/panes/common";

export interface PanesState {
  searchQuery: string;
  /** Counts requests to focus the search field. */
  searchFocus: number;
  tagSort: CountSort;
  propertySort: CountSort;
  /** The Files pane's row being renamed. */
  renaming: string | null;
  /** The Outline's folded headings, by note path, by where each starts. */
  folded: Readonly<Record<string, readonly number[]>>;
}

export const usePanes = create<PanesState>(() => ({
  searchQuery: "",
  searchFocus: 0,
  tagSort: "name",
  propertySort: "name",
  renaming: null,
  folded: {},
}));

/** setSearch sets the search's query, focusing its field when asked. */
export function setSearch(query: string, focus = false): void {
  usePanes.setState((s) => ({ searchQuery: query, searchFocus: focus ? s.searchFocus + 1 : s.searchFocus }));
}

export function setRenaming(path: string | null): void {
  usePanes.setState({ renaming: path });
}

/** toggleFolded folds or unfolds the heading starting at from in the note at path. */
export function toggleFolded(path: string, from: number): void {
  usePanes.setState((s) => {
    const now = s.folded[path] ?? [];
    const next = now.includes(from) ? now.filter((f) => f !== from) : [...now, from];
    return { folded: { ...s.folded, [path]: next } };
  });
}
