// The workspace (docs/design/app.md#workspace; docs/specs/ui.md#workspace):
// the split tree, each tab group with its strip and view header, the
// splitters between them, and dragging tabs between groups. Groups whose
// top edge is the workspace's draw their strips in the title bar row.

import { Splitter } from "@frostyard/lodge-ui/react";
import { createContext, type DragEvent, type ReactNode, useContext, useEffect, useRef, useState } from "react";
import { useFiles } from "../data/files";
import { usePrefs } from "../data/prefs";
import { useClient } from "../data/session";
import { useDocuments } from "../editor/registry";
import { NarrowHeader } from "../features/mobile/NarrowHeader";
import { TabStrip } from "../features/workspace/TabStrip";
import { ViewHeader } from "../features/workspace/ViewHeader";
import { type MergeSide, type Split, type TabGroup, tabTitle, type WorkspaceNode } from "../workspace/model";
import {
  activate,
  activateGroup,
  activeGroup,
  back,
  close,
  type Edge,
  findTab,
  forward,
  type MoveTarget,
  moveTab,
  open,
  openNewTab,
  resize,
  setView,
} from "../workspace/ops";
import { useWorkspace } from "../workspace/store";
import { mergeDone, previewCommand } from "./commands";
import { openMoreOptions, openTabList, openTabMenu, revealInFiles } from "./menus";
import { ViewBoundary } from "./ViewBoundary";
import { ViewContent } from "./ViewContent";

/** Zone is where a dragged tab would land in a group. */
type Zone = { kind: "index"; index: number; x: number } | { kind: "edge"; edge: Edge } | { kind: "end" };

interface Drag {
  tab: string | null;
  setTab: (id: string | null) => void;
}

const DragContext = createContext<Drag>({ tab: null, setTab: () => {} });

/** Narrow is how a narrow window's view header opens its drawers. */
export interface Narrow {
  onLeft: () => void;
  onRight: () => void;
}

/**
 * WorkspaceView draws the window's workspace; reserve keeps the top-right
 * strip clear of the window controls. In a narrow window it draws the
 * active group's active tab alone, under the narrow view header; the
 * other groups stay in the stored workspace (docs/specs/ui.md#narrow-windows).
 */
export function WorkspaceView(props: { reserve: number; narrow?: Narrow }) {
  const root = useWorkspace((s) => s.ws.root);
  const active = useWorkspace((s) => activeGroup(s.ws));
  const [tab, setTab] = useState<string | null>(null);
  useEffect(() => {
    const end = () => setTab(null);
    window.addEventListener("dragend", end);
    window.addEventListener("drop", end);
    return () => {
      window.removeEventListener("dragend", end);
      window.removeEventListener("drop", end);
    };
  }, []);
  return (
    <DragContext.Provider value={{ tab, setTab }}>
      <main aria-label="Workspace" className="flex min-w-0 flex-1 bg-window">
        {props.narrow ? (
          <GroupView node={active} group={active} top rightmost reserve={0} narrow={props.narrow} />
        ) : (
          <NodeView node={root} top rightmost reserve={props.reserve} />
        )}
      </main>
    </DragContext.Provider>
  );
}

interface NodeProps {
  node: WorkspaceNode;
  /** Its top edge is the workspace's. */
  top: boolean;
  /** Its right edge is the workspace's. */
  rightmost: boolean;
  reserve: number;
}

function NodeView(props: NodeProps) {
  return props.node.kind === "split" ? (
    <SplitView {...props} split={props.node} />
  ) : (
    <GroupView {...props} group={props.node} />
  );
}

/** MIN_PANE is the least a split's child shrinks to, in pixels. */
const MIN_PANE = 160;

function SplitView(props: NodeProps & { split: Split }) {
  const { split } = props;
  const box = useRef<HTMLDivElement>(null);
  const start = useRef<{ sizes: number[]; total: number } | null>(null);
  const update = useWorkspace((s) => s.update);
  const row = split.direction === "row";
  const children: ReactNode[] = [];
  split.children.forEach((child, i) => {
    if (i > 0) {
      children.push(
        <Splitter
          key={`splitter-${child.id}`}
          orientation={row ? "vertical" : "horizontal"}
          label="Resize"
          value={100 * split.sizes.slice(0, i).reduce((a, b) => a + b, 0)}
          onStart={() => {
            const r = box.current?.getBoundingClientRect();
            start.current = { sizes: [...split.sizes], total: (row ? r?.width : r?.height) || 1 };
          }}
          onMove={(delta) => {
            const s = start.current;
            if (!s) return;
            const a = s.sizes[i - 1] ?? 0;
            const b = s.sizes[i] ?? 0;
            const min = Math.min(MIN_PANE / s.total, (a + b) / 2);
            const d = Math.max(min - a, Math.min(b - min, delta / s.total));
            const sizes = [...s.sizes];
            sizes[i - 1] = a + d;
            sizes[i] = b - d;
            update((w) => resize(w, split.id, sizes));
          }}
        />,
      );
    }
    children.push(
      <div key={child.id} className="flex min-h-0 min-w-0" style={{ flex: `${split.sizes[i] ?? 1} 1 0px` }}>
        <NodeView
          node={child}
          top={props.top && (row || i === 0)}
          rightmost={props.rightmost && (!row || i === split.children.length - 1)}
          reserve={props.reserve}
        />
      </div>,
    );
  });
  return (
    <div ref={box} className={`flex min-h-0 min-w-0 flex-1 ${row ? "flex-row" : "flex-col"}`}>
      {children}
    </div>
  );
}

function GroupView(props: NodeProps & { group: TabGroup; narrow?: Narrow }) {
  const { group } = props;
  const update = useWorkspace((s) => s.update);
  const focused = useWorkspace((s) => s.ws.activeGroup === group.id);
  const viewHeader = usePrefs((s) => s.viewHeader);
  const files = useFiles((s) => s.files);
  const drag = useContext(DragContext);
  const client = useClient();
  const docs = useDocuments();
  const [zone, setZone] = useState<Zone | null>(null);
  const strip = useRef<HTMLDivElement>(null);
  const tab = group.tabs[group.active] ?? group.tabs[0];
  if (!tab) return null;
  const view = tab.view;

  const zoneAt = (e: DragEvent<HTMLElement>): Zone => {
    const s = strip.current?.getBoundingClientRect();
    if (s && e.clientY < s.bottom) {
      const tabs = [...(strip.current?.querySelectorAll('[role="tab"]') ?? [])].map((el) => el.getBoundingClientRect());
      const index = tabs.filter((r) => r.left + r.width / 2 < e.clientX).length;
      const edge = index < tabs.length ? tabs[index]?.left : tabs.at(-1)?.right;
      return { kind: "index", index, x: (edge ?? s.left) - s.left };
    }
    const r = e.currentTarget.getBoundingClientRect();
    const top = s ? s.bottom : r.top;
    const x = (e.clientX - r.left) / r.width;
    const y = (e.clientY - top) / Math.max(1, r.bottom - top);
    const edges: [Edge, number][] = [
      ["left", x],
      ["right", 1 - x],
      ["top", y],
      ["bottom", 1 - y],
    ];
    const [edge, d] = edges.reduce((a, b) => (b[1] < a[1] ? b : a));
    return d < 0.25 ? { kind: "edge", edge } : { kind: "end" };
  };

  const onDragOver = (e: DragEvent<HTMLDivElement>) => {
    if (drag.tab === null) return;
    e.preventDefault();
    e.dataTransfer.dropEffect = "move";
    setZone(zoneAt(e));
  };
  const onDrop = (e: DragEvent<HTMLDivElement>) => {
    const moving = drag.tab;
    if (moving === null) return;
    e.preventDefault();
    const z = zoneAt(e);
    const target: MoveTarget =
      z.kind === "edge"
        ? { group: group.id, edge: z.edge }
        : { group: group.id, index: z.kind === "index" ? z.index : group.tabs.length };
    update((w) => moveTab(w, moving, target));
    setZone(null);
    drag.setTab(null);
  };

  const path = view.type === "empty" ? undefined : view.path;
  return (
    // biome-ignore lint/a11y/noStaticElementInteractions: pointer-down here only makes the group the active one; its contents carry their own roles
    <div
      className="relative flex min-h-0 min-w-0 flex-1 flex-col"
      onPointerDownCapture={() => !focused && update((w) => activateGroup(w, group.id))}
      onDragOver={onDragOver}
      onDragLeave={(e) => {
        if (!e.currentTarget.contains(e.relatedTarget as Node | null)) setZone(null);
      }}
      onDrop={onDrop}
    >
      {!props.narrow && (
        <div
          ref={strip}
          data-tauri-drag-region={props.top ? true : undefined}
          className="relative shrink-0 overflow-hidden border-b border-separator bg-toolbar"
          style={{ paddingRight: props.top && props.rightmost ? props.reserve : 0 }}
        >
          <TabStrip
            label="Tabs"
            tabs={group.tabs.map((t, i) => ({
              id: t.id,
              title: tabTitle(t.view),
              active: i === group.active,
              pinned: t.pinned,
            }))}
            focused={focused}
            dragRegion={props.top}
            onCommand={(c) => {
              switch (c.kind) {
                case "activate":
                  update((w) => activate(w, c.id));
                  break;
                case "close":
                  update((w) => close(w, c.id));
                  break;
                case "newTab":
                  update((w) => openNewTab(activateGroup(w, group.id)));
                  break;
                case "menu":
                  openTabMenu(c.id, c.x, c.y);
                  break;
                case "list":
                  openTabList(group.id, c.x, c.y);
                  break;
              }
            }}
            onDragStart={(id, e) => {
              e.dataTransfer.effectAllowed = "move";
              e.dataTransfer.setData("text/plain", tabTitle(group.tabs.find((t) => t.id === id)?.view ?? view));
              drag.setTab(id);
            }}
          />
          {zone?.kind === "index" && (
            <div className="pointer-events-none absolute inset-y-2 w-0.5 bg-accent" style={{ left: zone.x }} />
          )}
        </div>
      )}
      {props.narrow && (
        <NarrowHeader
          title={tabTitle(view)}
          path={path}
          mode={view.type === "note" ? view.mode : undefined}
          onFolder={(folder) => {
            revealInFiles(`${folder}/`);
            props.narrow?.onLeft();
          }}
          onLeft={props.narrow.onLeft}
          onRight={props.narrow.onRight}
          onToggleMode={() =>
            update((w) =>
              view.type === "note" ? setView(w, tab.id, { ...view, mode: view.mode === "edit" ? "read" : "edit" }) : w,
            )
          }
          onMore={(x, y) => openMoreOptions(client, tab.id, view, x, y)}
        />
      )}
      {viewHeader && !props.narrow && (
        <ViewHeader
          path={path}
          title={tabTitle(view)}
          canBack={tab.back.length > 0}
          canForward={tab.forward.length > 0}
          mode={view.type === "note" ? view.mode : undefined}
          onBack={() => update((w) => back(activate(w, tab.id)))}
          onForward={() => update((w) => forward(activate(w, tab.id)))}
          onFolder={(folder) => revealInFiles(`${folder}/`)}
          onToggleMode={() =>
            update((w) =>
              view.type === "note" ? setView(w, tab.id, { ...view, mode: view.mode === "edit" ? "read" : "edit" }) : w,
            )
          }
          onMore={(x, y) => openMoreOptions(client, tab.id, view, x, y)}
        />
      )}
      <div className="relative min-h-0 flex-1 overflow-auto">
        <ViewBoundary view={`${tab.id}:${view.type}:${path ?? ""}`} label={path ?? tabTitle(view)}>
          <ViewContent
            tabId={tab.id}
            view={view}
            exists={path === undefined || files.has(path)}
            active={focused}
            onOpen={(p, where) =>
              update((w) => open(activate(w, tab.id), { type: "note", path: p, mode: "edit" }, where))
            }
            onFollow={(c) => void previewCommand(client, c, tab.id)}
            onCompare={(notice) => {
              if (path === undefined || (notice.kind !== "conflict" && notice.kind !== "refused")) return;
              // A notesd that does not name the conflict version: the
              // history lists it.
              if (notice.kind === "conflict" && notice.version === undefined) {
                update((w) => open(activate(w, tab.id), { type: "history", path }, "tab"));
                return;
              }
              const other: MergeSide =
                notice.kind === "conflict" ? { kind: "version", version: notice.version ?? 0 } : { kind: "held" };
              update((w) => open(activate(w, tab.id), { type: "merge", path, other }, "tab"));
            }}
            onClose={() => update((w) => close(w, tab.id))}
            onCompareWith={(p, other) =>
              update((w) => open(activate(w, tab.id), { type: "merge", path: p, other }, "tab"))
            }
            onMergeDone={(p) => void mergeDone(client, docs, p, () => update((w) => close(w, tab.id)))}
            onCursor={(pos, p) =>
              update((w) => {
                const t = findTab(w, tab.id)?.tab.view;
                return t?.type === "note" && t.path === p ? setView(w, tab.id, { ...t, cursor: pos }) : w;
              })
            }
          />
        </ViewBoundary>
        {zone && zone.kind !== "index" && <DropTint zone={zone} />}
      </div>
    </div>
  );
}

// DropTint shows where a dropped tab lands: the whole group, or the half
// at an edge that becomes a new group.
function DropTint({ zone }: { zone: Exclude<Zone, { kind: "index" }> }) {
  const at: Record<string, string> = {
    end: "inset-0",
    left: "inset-y-0 left-0 w-1/2",
    right: "inset-y-0 right-0 w-1/2",
    top: "inset-x-0 top-0 h-1/2",
    bottom: "inset-x-0 bottom-0 h-1/2",
  };
  return (
    <div
      data-testid="drop-tint"
      className={`pointer-events-none absolute bg-accent/15 ${at[zone.kind === "edge" ? zone.edge : "end"]}`}
    />
  );
}
