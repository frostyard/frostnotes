// A tab group's strip (docs/specs/ui.md, Tab strip): its tabs, the New tab
// and Tab list buttons, and the commands they send. The workspace's
// containers decide what each command does, open the menus, and handle
// drops. Task T-0042 draws it; until then it is an empty strip.
import type { DragEvent } from "react";

/** TabItem is a tab as its strip shows it. */
export interface TabItem {
  /** Stable for the tab's life. */
  id: string;
  /** The note's title, or the view's ("New tab", "History of Garden plan"). */
  title: string;
  /** The group's active tab; exactly one tab of a group is. */
  active: boolean;
  pinned: boolean;
}

/** TabCommand is what the strip asks of the workspace. */
export type TabCommand =
  | { kind: "activate"; id: string }
  | { kind: "close"; id: string }
  | { kind: "newTab" }
  /** Open a tab's menu at (x, y), in client coordinates. */
  | { kind: "menu"; id: string; x: number; y: number }
  /** Open the tab list at (x, y), in client coordinates. */
  | { kind: "list"; x: number; y: number };

/** TabStripProps are a strip's inputs. */
export interface TabStripProps {
  /** The tab list's accessible name. */
  label: string;
  tabs: TabItem[];
  /** This group holds the window's active tab. */
  focused: boolean;
  /** The strip sits in the title bar row, so its empty space drags the window. */
  dragRegion: boolean;
  onCommand: (command: TabCommand) => void;
  /** A tab started to drag; the workspace's drag and drop takes it from there. */
  onDragStart: (id: string, event: DragEvent<HTMLElement>) => void;
}

/** TabStrip draws a tab group's strip. */
export function TabStrip(props: TabStripProps) {
  return <div role="tablist" aria-label={props.label} />;
}
