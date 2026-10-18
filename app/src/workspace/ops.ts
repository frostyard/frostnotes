// The workspace's operations (docs/design/app.md#workspace): pure functions
// from a workspace to a workspace, each a unit test. Empty splits
// collapse, and the last tab of the last group becomes an empty tab
// rather than closing.
import type { SortOrder } from "../lib/fileTree";
import {
  type ClosedTab,
  type Direction,
  groups,
  MAX_CLOSED,
  MAX_EXPANDED,
  MAX_HISTORY,
  MAX_RECENT,
  newTab,
  type PaneId,
  SIDEBAR_MAX,
  SIDEBAR_MIN,
  type Side,
  type Split,
  sameView,
  type Tab,
  type TabGroup,
  type ViewState,
  viewPath,
  type Workspace,
  type WorkspaceNode,
} from "./model";

/** OpenWhere is where a view opens: in the active tab, a new tab, or a new group to the right or below. */
export type OpenWhere = "here" | "tab" | "right" | "down";

/** Edge is a tab group's edge, where a dropped tab makes a new split. */
export type Edge = "left" | "right" | "top" | "bottom";

/** ClickInput is what of a click decides where it opens. */
export interface ClickInput {
  ctrlKey: boolean;
  altKey: boolean;
  button: number;
}

/**
 * whereFor returns where a click opens what it names, as Obsidian opens
 * links (docs/design/app.md#workspace): in the current tab, Ctrl in a new
 * tab, Ctrl+Alt to the right, a middle click in a new tab.
 */
export function whereFor(e: ClickInput): OpenWhere {
  if (e.ctrlKey && e.altKey) return "right";
  if (e.ctrlKey || e.button === 1) return "tab";
  return "here";
}

/** MoveTarget is where a moved tab goes: a place in a group's tabs, or a group's edge. */
export type MoveTarget = { group: string; index: number } | { group: string; edge: Edge };

// --- reading

/** activeGroup is the group holding the window's active tab. */
export function activeGroup(ws: Workspace): TabGroup {
  return findGroup(ws, ws.activeGroup) ?? (groups(ws.root)[0] as TabGroup);
}

/** activeTab is the window's active tab. */
export function activeTab(ws: Workspace): Tab {
  const g = activeGroup(ws);
  return g.tabs[g.active] as Tab;
}

/** findGroup returns the group with an ID, if any. */
export function findGroup(ws: Workspace, id: string): TabGroup | undefined {
  return groups(ws.root).find((g) => g.id === id);
}

/** findTab returns a tab, its group and its index, if any. */
export function findTab(ws: Workspace, tabId: string): { group: TabGroup; index: number; tab: Tab } | undefined {
  for (const group of groups(ws.root)) {
    const index = group.tabs.findIndex((t) => t.id === tabId);
    if (index >= 0) return { group, index, tab: group.tabs[index] as Tab };
  }
  return undefined;
}

// --- opening and navigating

/**
 * open shows view: in the active tab, which keeps its old view to go back
 * to, unless it is pinned; in a new tab after the active one; or in a new
 * group to the right of or below the active group.
 */
export function open(ws: Workspace, view: ViewState, where: OpenWhere = "here"): Workspace {
  const tab = activeTab(ws);
  if (where === "here" && !tab.pinned) return remember(navigate(ws, tab.id, view), view);
  if (where === "right" || where === "down") return remember(splitWith(ws, ws.activeGroup, view, where), view);
  const [id, next] = takeId(ws, "t");
  const g = activeGroup(next);
  return remember(insertTab(next, g.id, g.active + 1, newTab(id, view)), view);
}

/** openNewTab opens an empty tab after the active one. */
export function openNewTab(ws: Workspace): Workspace {
  return open(ws, { type: "empty" }, "tab");
}

/** setView changes a tab's view without history: its scroll, cursor or mode. */
export function setView(ws: Workspace, tabId: string, view: ViewState): Workspace {
  return mapTab(ws, tabId, (t) => ({ ...t, view }));
}

/** back shows the active tab's previous view. */
export function back(ws: Workspace): Workspace {
  const tab = activeTab(ws);
  const prev = tab.back.at(-1);
  if (!prev) return ws;
  return mapTab(ws, tab.id, (t) => ({
    ...t,
    view: prev,
    back: t.back.slice(0, -1),
    forward: capped([...t.forward, t.view]),
  }));
}

/** forward shows the active tab's next view. */
export function forward(ws: Workspace): Workspace {
  const tab = activeTab(ws);
  const next = tab.forward.at(-1);
  if (!next) return ws;
  return mapTab(ws, tab.id, (t) => ({
    ...t,
    view: next,
    forward: t.forward.slice(0, -1),
    back: capped([...t.back, t.view]),
  }));
}

// navigate shows view in a tab, keeping its old view to go back to; an
// empty tab's view is not kept.
function navigate(ws: Workspace, tabId: string, view: ViewState): Workspace {
  return mapTab(ws, tabId, (t) => {
    if (sameView(t.view, view)) return { ...t, view };
    const back = t.view.type === "empty" ? t.back : capped([...t.back, t.view]);
    return { ...t, view, back, forward: [] };
  });
}

// remember puts a file the view shows first among the recent files.
function remember(ws: Workspace, view: ViewState): Workspace {
  const p = viewPath(view);
  if (p === undefined || view.type === "history" || view.type === "merge") return ws;
  return { ...ws, recent: [p, ...ws.recent.filter((r) => r !== p)].slice(0, MAX_RECENT) };
}

// --- activating

/** activate makes a tab its group's active tab and its group the active group. */
export function activate(ws: Workspace, tabId: string): Workspace {
  const at = findTab(ws, tabId);
  if (!at) return ws;
  return { ...mapGroup(ws, at.group.id, (g) => ({ ...g, active: at.index })), activeGroup: at.group.id };
}

/** activateGroup makes a group the active group. */
export function activateGroup(ws: Workspace, groupId: string): Workspace {
  return findGroup(ws, groupId) ? { ...ws, activeGroup: groupId } : ws;
}

/** cycle activates the next (1) or previous (-1) tab of the active group, wrapping. */
export function cycle(ws: Workspace, delta: 1 | -1): Workspace {
  const g = activeGroup(ws);
  const n = g.tabs.length;
  return mapGroup(ws, g.id, (x) => ({ ...x, active: (x.active + delta + n) % n }));
}

/** gotoTab activates the active group's nth tab, counting from 1; past the end, nothing. */
export function gotoTab(ws: Workspace, n: number): Workspace {
  const g = activeGroup(ws);
  return n >= 1 && n <= g.tabs.length ? mapGroup(ws, g.id, (x) => ({ ...x, active: n - 1 })) : ws;
}

/** gotoLastTab activates the active group's last tab. */
export function gotoLastTab(ws: Workspace): Workspace {
  return gotoTab(ws, activeGroup(ws).tabs.length);
}

/** pin pins or unpins a tab; without pinned it toggles. */
export function pin(ws: Workspace, tabId: string, pinned?: boolean): Workspace {
  return mapTab(ws, tabId, (t) => ({ ...t, pinned: pinned ?? !t.pinned }));
}

// --- closing

/** close closes a tab, keeping it to reopen. */
export function close(ws: Workspace, tabId: string): Workspace {
  const at = findTab(ws, tabId);
  if (!at) return ws;
  const closed: ClosedTab = { tab: at.tab, group: at.group.id, index: at.index };
  return { ...removeTab(ws, tabId), closed: [...ws.closed, closed].slice(-MAX_CLOSED) };
}

/** closeOthers closes a group's unpinned tabs but one. */
export function closeOthers(ws: Workspace, tabId: string): Workspace {
  const at = findTab(ws, tabId);
  if (!at) return ws;
  const others = at.group.tabs.filter((t) => t.id !== tabId && !t.pinned);
  return activate(
    others.reduce((x, t) => close(x, t.id), ws),
    tabId,
  );
}

/** closeToTheRight closes a group's unpinned tabs after one. */
export function closeToTheRight(ws: Workspace, tabId: string): Workspace {
  const at = findTab(ws, tabId);
  if (!at) return ws;
  return at.group.tabs
    .slice(at.index + 1)
    .filter((t) => !t.pinned)
    .reduce((x, t) => close(x, t.id), ws);
}

/** closeAll closes a group's unpinned tabs. */
export function closeAll(ws: Workspace, groupId: string): Workspace {
  const g = findGroup(ws, groupId);
  if (!g) return ws;
  return g.tabs.filter((t) => !t.pinned).reduce((x, t) => close(x, t.id), ws);
}

/** reopenClosed reopens the tab closed last where it was, or after the active tab when its group is gone. */
export function reopenClosed(ws: Workspace): Workspace {
  const last = ws.closed.at(-1);
  if (!last) return ws;
  const rest = { ...ws, closed: ws.closed.slice(0, -1) };
  const g = findGroup(rest, last.group);
  return g
    ? insertTab(rest, g.id, Math.min(last.index, g.tabs.length), last.tab)
    : insertTab(rest, activeGroup(rest).id, activeGroup(rest).active + 1, last.tab);
}

// --- splitting and moving

/** split opens a copy of a tab's view in a new group to its right or below its group. */
export function split(ws: Workspace, tabId: string, where: "right" | "down"): Workspace {
  const at = findTab(ws, tabId);
  if (!at) return ws;
  return splitWith(ws, at.group.id, at.tab.view, where);
}

/** moveTab moves a tab into a group at an index, or to a group's edge as a new group. */
export function moveTab(ws: Workspace, tabId: string, target: MoveTarget): Workspace {
  const from = findTab(ws, tabId);
  const to = findGroup(ws, target.group);
  if (!from || !to) return ws;
  if ("edge" in target) {
    if (from.group.id === to.id && from.group.tabs.length === 1) return ws;
    const removed = removeTab(ws, tabId, false);
    const [gid, a] = takeId(removed, "g");
    const [sid, b] = takeId(a, "s");
    const group: TabGroup = { kind: "tabs", id: gid, tabs: [from.tab], active: 0 };
    return { ...b, root: beside(b.root, to.id, group, target.edge, sid), activeGroup: gid };
  }
  let index = target.index;
  if (from.group.id === to.id) {
    if (index > from.index) index--;
    const tabs = from.group.tabs.filter((t) => t.id !== tabId);
    tabs.splice(Math.max(0, Math.min(index, tabs.length)), 0, from.tab);
    return activate(
      mapGroup(ws, to.id, (g) => ({ ...g, tabs })),
      tabId,
    );
  }
  const removed = removeTab(ws, tabId, false);
  const dest = findGroup(removed, to.id);
  if (!dest) return ws;
  return insertTab(removed, dest.id, Math.min(index, dest.tabs.length), from.tab);
}

/** resize sets a split's sizes, normalized to sum to 1, none under 5%. */
export function resize(ws: Workspace, splitId: string, sizes: number[]): Workspace {
  return {
    ...ws,
    root: mapNode(ws.root, splitId, (n) =>
      n.kind === "split" && sizes.length === n.children.length ? { ...n, sizes: normalize(sizes) } : n,
    ),
  };
}

// splitWith opens view in a new group to the right of or below a group.
function splitWith(ws: Workspace, groupId: string, view: ViewState, where: "right" | "down"): Workspace {
  const [tid, a] = takeId(ws, "t");
  const [gid, b] = takeId(a, "g");
  const [sid, c] = takeId(b, "s");
  const group: TabGroup = { kind: "tabs", id: gid, tabs: [newTab(tid, view)], active: 0 };
  return { ...c, root: beside(c.root, groupId, group, where === "right" ? "right" : "bottom", sid), activeGroup: gid };
}

// beside puts node at a group's edge: in its parent split when that runs
// the same way, sharing the group's size, else in a new split around it.
function beside(root: WorkspaceNode, groupId: string, node: WorkspaceNode, edge: Edge, splitId: string): WorkspaceNode {
  const direction: Direction = edge === "left" || edge === "right" ? "row" : "column";
  const after = edge === "right" || edge === "bottom";
  const visit = (n: WorkspaceNode): WorkspaceNode => {
    if (n.kind === "tabs") {
      if (n.id !== groupId) return n;
      return { kind: "split", id: splitId, direction, children: after ? [n, node] : [node, n], sizes: [0.5, 0.5] };
    }
    const i = n.children.findIndex((c) => c.kind === "tabs" && c.id === groupId);
    if (i >= 0 && n.direction === direction) {
      const children = [...n.children];
      const sizes = [...n.sizes];
      const half = (sizes[i] ?? 0) / 2;
      children.splice(after ? i + 1 : i, 0, node);
      sizes.splice(i, 1, half, half);
      return { ...n, children, sizes };
    }
    return { ...n, children: n.children.map(visit) };
  };
  return visit(root);
}

// --- sidebars

/** toggleSidebar opens or closes a sidebar. */
export function toggleSidebar(ws: Workspace, side: Side): Workspace {
  return { ...ws, [side]: { ...ws[side], open: !ws[side].open } };
}

/** setSidebarWidth sets a sidebar's width, between SIDEBAR_MIN and SIDEBAR_MAX. */
export function setSidebarWidth(ws: Workspace, side: Side, width: number): Workspace {
  const w = Math.round(Math.max(SIDEBAR_MIN, Math.min(SIDEBAR_MAX, width)));
  return { ...ws, [side]: { ...ws[side], width: w } };
}

/** selectPane shows a pane in its sidebar, opening the sidebar. */
export function selectPane(ws: Workspace, side: Side, pane: PaneId): Workspace {
  const active = ws[side].panes.indexOf(pane);
  return active < 0 ? ws : { ...ws, [side]: { ...ws[side], active, open: true } };
}

// --- the Files pane

/** setFileSort sets the Files pane's sort order. */
export function setFileSort(ws: Workspace, sort: SortOrder): Workspace {
  return { ...ws, files: { ...ws.files, sort } };
}

/** setFolderOpen opens or closes a folder in the Files pane. */
export function setFolderOpen(ws: Workspace, folder: string, open: boolean): Workspace {
  const rest = ws.files.expanded.filter((p) => p !== folder);
  const expanded = open ? [...rest, folder].slice(-MAX_EXPANDED) : rest;
  return { ...ws, files: { ...ws.files, expanded } };
}

/** setFoldersOpen opens exactly folders: Expand all with every folder, Collapse all with none. */
export function setFoldersOpen(ws: Workspace, folders: readonly string[]): Workspace {
  return { ...ws, files: { ...ws.files, expanded: [...new Set(folders)].slice(0, MAX_EXPANDED) } };
}

/** revealPath opens every folder holding path, so its row shows. */
export function revealPath(ws: Workspace, path: string): Workspace {
  const parts = path.split("/").slice(0, -1);
  const folders = parts.map((_, i) => parts.slice(0, i + 1).join("/"));
  return folders.reduce((w, f) => (w.files.expanded.includes(f) ? w : setFolderOpen(w, f, true)), ws);
}

// --- following the vault

/** applyMove follows a file or folder moved from one path to another, in every view, history and list. */
export function applyMove(ws: Workspace, from: string, to: string): Workspace {
  const move = (p: string) => (p === from ? to : p.startsWith(`${from}/`) ? to + p.slice(from.length) : p);
  const view = (v: ViewState): ViewState => {
    const p = viewPath(v);
    return p === undefined || move(p) === p ? v : ({ ...v, path: move(p) } as ViewState);
  };
  const tab = (t: Tab): Tab => ({ ...t, view: view(t.view), back: t.back.map(view), forward: t.forward.map(view) });
  return {
    ...mapTabs(ws, tab),
    closed: ws.closed.map((c) => ({ ...c, tab: tab(c.tab) })),
    recent: ws.recent.map(move),
    files: { ...ws.files, expanded: ws.files.expanded.map(move) },
  };
}

/**
 * applyDelete forgets a deleted file in history and the recent files; a
 * tab showing it stays, saying so, until it is closed or the file comes
 * back.
 */
export function applyDelete(ws: Workspace, path: string): Workspace {
  const keep = (v: ViewState) => viewPath(v) !== path;
  return {
    ...mapTabs(ws, (t) => ({ ...t, back: t.back.filter(keep), forward: t.forward.filter(keep) })),
    recent: ws.recent.filter((r) => r !== path),
  };
}

// --- the tree

function takeId(ws: Workspace, prefix: "t" | "g" | "s"): [string, Workspace] {
  return [`${prefix}${ws.nextId}`, { ...ws, nextId: ws.nextId + 1 }];
}

function capped(views: ViewState[]): ViewState[] {
  return views.slice(-MAX_HISTORY);
}

function normalize(sizes: number[]): number[] {
  const clamped = sizes.map((s) => (Number.isFinite(s) ? Math.max(0.05, s) : 0.05));
  const sum = clamped.reduce((a, b) => a + b, 0);
  return clamped.map((s) => s / sum);
}

// mapNode replaces the node with an ID by fn's result, or removes it when
// fn gives null; a split left with one child becomes that child.
function mapNode(root: WorkspaceNode, id: string, fn: (n: WorkspaceNode) => WorkspaceNode | null): WorkspaceNode {
  const visit = (n: WorkspaceNode): WorkspaceNode | null => {
    if (n.id === id) return fn(n);
    if (n.kind === "tabs") return n;
    const kept: { child: WorkspaceNode; size: number }[] = [];
    n.children.forEach((c, i) => {
      const child = visit(c);
      if (child) kept.push({ child, size: n.sizes[i] ?? 0 });
    });
    if (kept.length === 1) return (kept[0] as { child: WorkspaceNode }).child;
    if (kept.length === 0) return null;
    const split: Split = { ...n, children: kept.map((k) => k.child), sizes: normalize(kept.map((k) => k.size)) };
    return split;
  };
  return visit(root) ?? root;
}

function mapGroup(ws: Workspace, id: string, fn: (g: TabGroup) => TabGroup): Workspace {
  return { ...ws, root: mapNode(ws.root, id, (n) => (n.kind === "tabs" ? fn(n) : n)) };
}

function mapTab(ws: Workspace, tabId: string, fn: (t: Tab) => Tab): Workspace {
  const at = findTab(ws, tabId);
  if (!at) return ws;
  return mapGroup(ws, at.group.id, (g) => ({ ...g, tabs: g.tabs.map((t) => (t.id === tabId ? fn(t) : t)) }));
}

function mapTabs(ws: Workspace, fn: (t: Tab) => Tab): Workspace {
  const visit = (n: WorkspaceNode): WorkspaceNode =>
    n.kind === "tabs" ? { ...n, tabs: n.tabs.map(fn) } : { ...n, children: n.children.map(visit) };
  return { ...ws, root: visit(ws.root) };
}

// insertTab puts a tab into a group at an index and activates it.
function insertTab(ws: Workspace, groupId: string, index: number, tab: Tab): Workspace {
  const next = mapGroup(ws, groupId, (g) => {
    const tabs = [...g.tabs];
    tabs.splice(index, 0, tab);
    return { ...g, tabs, active: index };
  });
  return { ...next, activeGroup: groupId };
}

// removeTab takes a tab out of its group. A group left empty goes, and
// the active group passes to the one in its place; the last group keeps
// an empty tab instead, unless keepLast is false (a move out of it).
function removeTab(ws: Workspace, tabId: string, keepLast = true): Workspace {
  const at = findTab(ws, tabId);
  if (!at) return ws;
  const { group, index } = at;
  if (group.tabs.length > 1) {
    const tabs = group.tabs.filter((t) => t.id !== tabId);
    const active = index < group.active || group.active === tabs.length ? group.active - 1 : group.active;
    return mapGroup(ws, group.id, (g) => ({ ...g, tabs, active: Math.max(0, active) }));
  }
  const all = groups(ws.root);
  if (all.length === 1) {
    if (!keepLast) return ws;
    const [id, next] = takeId(ws, "t");
    return mapGroup(next, group.id, (g) => ({ ...g, tabs: [newTab(id, { type: "empty" })], active: 0 }));
  }
  const position = all.findIndex((g) => g.id === group.id);
  const root = mapNode(ws.root, group.id, () => null);
  const left = groups(root);
  const activeGroupId =
    ws.activeGroup === group.id ? (left[Math.min(position, left.length - 1)] as TabGroup).id : ws.activeGroup;
  return { ...ws, root, activeGroup: activeGroupId };
}
