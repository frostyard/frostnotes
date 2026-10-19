// What the sidebar panes share (docs/specs/ui.md, Sidebars and panes): how
// a click on a row says where to open it, the panes' sort orders by
// count, and their rows' and toolbars' looks.

/** PaneClick is a click on a pane's row: Ctrl opens in a new tab, Ctrl+Alt to the right, a middle click in a new tab. */
export interface PaneClick {
  ctrl: boolean;
  alt: boolean;
  middle: boolean;
}

/** paneClick reads a mouse event's modifiers and button. */
export function paneClick(e: { ctrlKey: boolean; altKey: boolean; button: number }): PaneClick {
  return { ctrl: e.ctrlKey, alt: e.altKey, middle: e.button === 1 };
}

/** CountSort is how the Tags and All properties panes sort: by name (A to Z) or by count (most first). */
export type CountSort = "name" | "count";

/** paneRow is a pane row's look: 28 high (44 under a coarse pointer), rounded 6, inset 8, the sidebar's row type. */
export const paneRow =
  "text-sidebar-row mx-2 flex h-7 min-w-0 items-center gap-1.5 rounded-md hover:bg-selection-inactive pointer-coarse:h-11";

/** paneToolbar is a pane's toolbar's look: 32 high, its 24 × 24 buttons centered, 4 apart. */
export const paneToolbar = "flex h-8 shrink-0 items-center justify-center gap-1";

/** paneEmpty is the look of a pane's text when it has nothing to list. */
export const paneEmpty = "px-4 py-3 text-[13px] leading-[18px] text-secondary";
