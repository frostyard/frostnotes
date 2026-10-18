// The Outline pane (docs/specs/ui.md, Right sidebar panes): the active
// note's headings as lib/outline.ts nests them. Task T-0053 replaces this
// stub.
import type { Heading, OutlineRow } from "../../lib/outline";

export interface OutlinePaneProps {
  /** The rows (outlineRows); null when no note is open. */
  rows: readonly OutlineRow[] | null;
  /** The index in rows of the heading the cursor is under; -1 for none. */
  current: number;
  /** Fold or unfold the heading that starts at from. */
  onToggle: (from: number) => void;
  /** Take the editor to the heading. */
  onGo: (heading: Heading) => void;
}

/** OutlinePane draws the Outline pane. */
export function OutlinePane(_props: OutlinePaneProps) {
  return null;
}
