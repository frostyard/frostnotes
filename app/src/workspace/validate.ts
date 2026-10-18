// Validation of a stored workspace (ADR-0021): what an older or newer app
// wrote, or what a hand edit left, is read for what it holds that this app
// understands, and the rest is dropped, so a stored layout never wedges
// the window. Nothing here throws.
import type { SortOrder } from "../lib/fileTree";
import {
  type ClosedTab,
  emptyWorkspace,
  type FilesPaneState,
  groups,
  LEFT_WIDTH,
  MAX_CLOSED,
  MAX_EXPANDED,
  MAX_HISTORY,
  MAX_RECENT,
  type MergeSide,
  PANES,
  type PaneId,
  RIGHT_WIDTH,
  SIDEBAR_MAX,
  SIDEBAR_MIN,
  type Side,
  type Sidebar,
  type Tab,
  type TabGroup,
  type ViewState,
  type Workspace,
  type WorkspaceNode,
} from "./model";

type Obj = Record<string, unknown>;
const isObj = (v: unknown): v is Obj => typeof v === "object" && v !== null && !Array.isArray(v);
const isStr = (v: unknown): v is string => typeof v === "string";
const isPath = (v: unknown): v is string => isStr(v) && v !== "" && !v.startsWith("/");
const isInt = (v: unknown): v is number => Number.isSafeInteger(v);

/** validate reads a stored workspace, keeping what it understands; it returns emptyWorkspace when nothing is. */
export function validate(input: unknown): Workspace {
  if (!isObj(input) || input.version !== 1) return emptyWorkspace();
  const ids = new Set<string>();
  const root = node(input.root, ids);
  if (!root) return emptyWorkspace();
  const all = groups(root);
  const activeGroup =
    isStr(input.activeGroup) && all.some((g) => g.id === input.activeGroup)
      ? input.activeGroup
      : (all[0] as TabGroup).id;
  const closed: ClosedTab[] = [];
  if (Array.isArray(input.closed)) {
    for (const c of input.closed.slice(-MAX_CLOSED)) {
      if (!isObj(c) || !isStr(c.group) || !isInt(c.index) || c.index < 0) continue;
      const t = tab(c.tab, ids);
      if (t) closed.push({ tab: t, group: c.group, index: c.index });
    }
  }
  const recent = Array.isArray(input.recent) ? [...new Set(input.recent.filter(isPath))].slice(0, MAX_RECENT) : [];
  const highest = Math.max(0, ...[...ids].map((id) => Number(id.slice(1))).filter(Number.isSafeInteger));
  const nextId = isInt(input.nextId) && input.nextId > highest ? input.nextId : highest + 1;
  return {
    version: 1,
    root,
    activeGroup,
    left: sidebar(input.left, "left", LEFT_WIDTH),
    right: sidebar(input.right, "right", RIGHT_WIDTH),
    closed,
    recent,
    files: filesPane(input.files),
    nextId,
  };
}

const SORTS: readonly SortOrder[] = ["alphabetical", "alphabeticalReverse", "byModifiedTime", "byModifiedTimeReverse"];

// filesPane reads the Files pane's state; a missing or strange one is the first one.
function filesPane(v: unknown): FilesPaneState {
  if (!isObj(v)) return { sort: "alphabetical", expanded: [] };
  const sort = SORTS.find((s) => s === v.sort) ?? "alphabetical";
  const expanded = Array.isArray(v.expanded) ? [...new Set(v.expanded.filter(isPath))].slice(0, MAX_EXPANDED) : [];
  return { sort, expanded };
}

// node reads a split or tab group, or nothing; IDs must be unique.
function node(v: unknown, ids: Set<string>): WorkspaceNode | null {
  if (!isObj(v) || !isStr(v.id) || !/^[tgs][0-9]+$/.test(v.id) || ids.has(v.id)) return null;
  if (v.kind === "tabs") {
    if (!v.id.startsWith("g") || !Array.isArray(v.tabs)) return null;
    const tabs = v.tabs.map((t) => tab(t, ids)).filter((t): t is Tab => t !== null);
    if (tabs.length === 0) return null;
    ids.add(v.id);
    const active = isInt(v.active) && v.active >= 0 && v.active < tabs.length ? v.active : 0;
    return { kind: "tabs", id: v.id, tabs, active };
  }
  if (v.kind !== "split" || !v.id.startsWith("s") || (v.direction !== "row" && v.direction !== "column")) return null;
  if (!Array.isArray(v.children)) return null;
  ids.add(v.id);
  const kept: { child: WorkspaceNode; size: number }[] = [];
  v.children.forEach((c, i) => {
    const child = node(c, ids);
    const size = Array.isArray(v.sizes) ? v.sizes[i] : undefined;
    if (child) kept.push({ child, size: typeof size === "number" && size > 0 && Number.isFinite(size) ? size : 0 });
  });
  if (kept.length === 0) return null;
  if (kept.length === 1) return (kept[0] as { child: WorkspaceNode }).child;
  const fallback = 1 / kept.length;
  const sizes = kept.map((k) => (k.size > 0 ? k.size : fallback));
  const sum = sizes.reduce((a, b) => a + b, 0);
  return {
    kind: "split",
    id: v.id,
    direction: v.direction,
    children: kept.map((k) => k.child),
    sizes: sizes.map((s) => s / sum),
  };
}

function tab(v: unknown, ids: Set<string>): Tab | null {
  if (!isObj(v) || !isStr(v.id) || !/^t[0-9]+$/.test(v.id) || ids.has(v.id)) return null;
  const current = view(v.view);
  if (!current) return null;
  ids.add(v.id);
  const stack = (s: unknown) =>
    Array.isArray(s)
      ? s
          .map(view)
          .filter((x): x is ViewState => x !== null)
          .slice(-MAX_HISTORY)
      : [];
  return { id: v.id, view: current, back: stack(v.back), forward: stack(v.forward), pinned: v.pinned === true };
}

function view(v: unknown): ViewState | null {
  if (!isObj(v)) return null;
  switch (v.type) {
    case "empty":
      return { type: "empty" };
    case "note": {
      if (!isPath(v.path)) return null;
      const out: ViewState = { type: "note", path: v.path, mode: v.mode === "read" ? "read" : "edit" };
      if (isInt(v.scroll) && v.scroll >= 0) out.scroll = v.scroll;
      if (isInt(v.cursor) && v.cursor >= 0) out.cursor = v.cursor;
      return out;
    }
    case "history":
    case "file":
      return isPath(v.path) ? { type: v.type, path: v.path } : null;
    case "merge": {
      const other = mergeSide(v.other);
      return isPath(v.path) && other ? { type: "merge", path: v.path, other } : null;
    }
    default:
      return null;
  }
}

function mergeSide(v: unknown): MergeSide | null {
  if (!isObj(v)) return null;
  if (v.kind === "version" && isInt(v.version)) return { kind: "version", version: v.version };
  if (v.kind === "revision" && isInt(v.seq)) return { kind: "revision", seq: v.seq };
  return null;
}

function sidebar(v: unknown, side: Side, width: number): Sidebar {
  const all = PANES[side];
  if (!isObj(v)) return { panes: [...all], active: 0, width, open: true };
  // The panes are always this app's; a known stored pane stays selected.
  const stored = Array.isArray(v.panes) ? v.panes : [];
  const selected = isInt(v.active) ? stored[v.active] : undefined;
  const active = Math.max(0, all.indexOf(selected as PaneId));
  const w = isInt(v.width) ? Math.max(SIDEBAR_MIN, Math.min(SIDEBAR_MAX, v.width)) : width;
  return { panes: [...all], active, width: w, open: v.open !== false };
}
