// The workspace model (docs/design/app.md#workspace): a tree of splits and
// tab groups, two sidebars of panes, the tabs closed lately and the files
// opened lately. Operations on it are pure functions (ops.ts); the
// workspace store keeps one per vault in local storage (store.ts).

/** NoteMode is how a note tab shows its note. */
export type NoteMode = "edit" | "read";

/**
 * MergeSide is the other side of a merge view: a version of the file, a
 * hub revision, or the text the editor held when notesd refused its base
 * (in memory only: a stored workspace drops it).
 */
export type MergeSide = { kind: "version"; version: number } | { kind: "revision"; seq: number } | { kind: "held" };

/** ViewState is what a tab shows. */
export type ViewState =
  | { type: "empty" }
  | { type: "note"; path: string; mode: NoteMode; scroll?: number; cursor?: number }
  | { type: "history"; path: string }
  | { type: "merge"; path: string; other: MergeSide }
  | { type: "file"; path: string };

/** Tab is a tab: its view and the views before and after it. */
export interface Tab {
  id: string;
  view: ViewState;
  /** Views to go back to, the nearest last; at most MAX_HISTORY. */
  back: ViewState[];
  /** Views to go forward to, the nearest last; at most MAX_HISTORY. */
  forward: ViewState[];
  pinned: boolean;
}

/** TabGroup is a group of tabs and its active one. */
export interface TabGroup {
  kind: "tabs";
  id: string;
  /** At least one. */
  tabs: Tab[];
  active: number;
}

/** Direction is how a split lays out its children: side by side (row) or stacked (column). */
export type Direction = "row" | "column";

/** Split lays out at least two children with sizes as fractions that sum to 1. */
export interface Split {
  kind: "split";
  id: string;
  direction: Direction;
  children: WorkspaceNode[];
  sizes: number[];
}

export type WorkspaceNode = Split | TabGroup;

/** PaneId names a sidebar pane. */
export type PaneId = "files" | "search" | "backlinks" | "outgoing" | "tags" | "properties" | "outline";

/** Side is a sidebar. */
export type Side = "left" | "right";

/** Sidebar is a sidebar: its panes, the selected one, its width and whether it is open. */
export interface Sidebar {
  panes: PaneId[];
  active: number;
  width: number;
  open: boolean;
}

/** ClosedTab is a closed tab and where it was, to reopen it. */
export interface ClosedTab {
  tab: Tab;
  group: string;
  index: number;
}

/** Workspace is one vault's layout. */
export interface Workspace {
  version: 1;
  root: WorkspaceNode;
  /** The tab group holding the window's active tab. */
  activeGroup: string;
  left: Sidebar;
  right: Sidebar;
  /** The tabs closed lately, the last last; at most MAX_CLOSED. */
  closed: ClosedTab[];
  /** The files opened lately, the newest first; at most MAX_RECENT. */
  recent: string[];
  /** The next number for an ID: IDs are "t<n>", "g<n>" and "s<n>". */
  nextId: number;
}

export const MAX_HISTORY = 50;
export const MAX_CLOSED = 20;
export const MAX_RECENT = 50;

/** Sidebar widths (docs/specs/ui.md#layout). */
export const SIDEBAR_MIN = 200;
export const SIDEBAR_MAX = 600;
export const LEFT_WIDTH = 260;
export const RIGHT_WIDTH = 300;

/** PANES are each sidebar's panes, in order (docs/specs/ui.md#sidebars-and-panes). */
export const PANES: Record<Side, readonly PaneId[]> = {
  left: ["files", "search"],
  right: ["backlinks", "outgoing", "tags", "properties", "outline"],
};

/** emptyWorkspace is a vault's first layout: one empty tab, both sidebars open on their first pane. */
export function emptyWorkspace(): Workspace {
  return {
    version: 1,
    root: { kind: "tabs", id: "g1", tabs: [newTab("t2", { type: "empty" })], active: 0 },
    activeGroup: "g1",
    left: { panes: [...PANES.left], active: 0, width: LEFT_WIDTH, open: true },
    right: { panes: [...PANES.right], active: 0, width: RIGHT_WIDTH, open: true },
    closed: [],
    recent: [],
    nextId: 3,
  };
}

/** newTab is a tab showing view, with no history. */
export function newTab(id: string, view: ViewState): Tab {
  return { id, view, back: [], forward: [], pinned: false };
}

/** viewPath is the file a view shows, if any. */
export function viewPath(view: ViewState): string | undefined {
  return view.type === "empty" ? undefined : view.path;
}

/** sameView reports whether two views show the same thing, ignoring scroll and cursor. */
export function sameView(a: ViewState, b: ViewState): boolean {
  if (a.type !== b.type) return false;
  switch (a.type) {
    case "empty":
      return true;
    case "note":
      return b.type === "note" && a.path === b.path && a.mode === b.mode;
    case "merge":
      return b.type === "merge" && a.path === b.path && JSON.stringify(a.other) === JSON.stringify(b.other);
    default:
      return viewPath(a) === viewPath(b);
  }
}

/** groups lists the tab groups under node, left to right and top to bottom. */
export function groups(node: WorkspaceNode): TabGroup[] {
  return node.kind === "tabs" ? [node] : node.children.flatMap(groups);
}

/** tabTitle is a tab's title as its strip shows it. */
export function tabTitle(view: ViewState): string {
  switch (view.type) {
    case "empty":
      return "New tab";
    case "note":
      return baseName(view.path).replace(/\.md$/, "");
    case "file":
      return baseName(view.path);
    case "history":
      return `History of ${baseName(view.path).replace(/\.md$/, "")}`;
    case "merge":
      return `Merge ${baseName(view.path).replace(/\.md$/, "")}`;
  }
}

function baseName(p: string): string {
  return p.slice(p.lastIndexOf("/") + 1);
}
