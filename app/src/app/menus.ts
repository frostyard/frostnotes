// The window's menus (docs/specs/ui.md, Tab strip, View header, Status
// bar, Sidebars and panes): what each lists and what choosing an item
// does. Each opens through the overlays store at a point; app/Overlays.tsx
// draws it.
import { openMenu, openMove } from "../data/overlays";
import { setRenaming } from "../data/panes";
import { usePrefs } from "../data/prefs";
import type { MenuItem } from "../features/menu/Menu";
import type { Client, SyncConflict } from "../rpc/gen/api";
import { tabTitle, type ViewState } from "../workspace/model";
import {
  activate,
  close,
  closeAll,
  closeOthers,
  closeToTheRight,
  findGroup,
  findTab,
  open,
  pin,
  revealPath,
  selectPane,
  setView,
  split,
} from "../workspace/ops";
import { useWorkspace } from "../workspace/store";
import { copyPath, deleteFile } from "./commands";

const update = (op: Parameters<ReturnType<typeof useWorkspace.getState>["update"]>[0]) =>
  useWorkspace.getState().update(op);
const item = (id: string, label: string, extra: Partial<Extract<MenuItem, { kind: "item" }>> = {}): MenuItem => ({
  kind: "item",
  id,
  label,
  ...extra,
});
const SEP: MenuItem = { kind: "separator" };

/** openTabMenu opens a tab's context menu at (x, y). */
export function openTabMenu(tabId: string, x: number, y: number): void {
  const found = findTab(useWorkspace.getState().ws, tabId);
  if (!found) return;
  const { group, index, tab } = found;
  openMenu({
    label: "Tab",
    x,
    y,
    items: [
      item("close", "Close"),
      item("close-others", "Close others", { disabled: group.tabs.length < 2 }),
      item("close-right", "Close tabs to the right", { disabled: index === group.tabs.length - 1 }),
      item("close-all", "Close all"),
      SEP,
      item("pin", tab.pinned ? "Unpin" : "Pin"),
      SEP,
      item("split-right", "Split right"),
      item("split-down", "Split down"),
    ],
    onChoose: (id) => {
      switch (id) {
        case "close":
          return update((w) => close(w, tabId));
        case "close-others":
          return update((w) => closeOthers(w, tabId));
        case "close-right":
          return update((w) => closeToTheRight(w, tabId));
        case "close-all":
          return update((w) => closeAll(w, group.id));
        case "pin":
          return update((w) => pin(w, tabId));
        case "split-right":
          return update((w) => split(w, tabId, "right"));
        case "split-down":
          return update((w) => split(w, tabId, "down"));
      }
    },
  });
}

/** openTabList opens the list of a group's tabs at (x, y), the active one checked. */
export function openTabList(groupId: string, x: number, y: number): void {
  const group = findGroup(useWorkspace.getState().ws, groupId);
  if (!group) return;
  openMenu({
    label: "Tab list",
    x,
    y,
    items: group.tabs.map((t, i) => item(t.id, tabTitle(t.view), { checked: i === group.active })),
    onChoose: (id) => update((w) => activate(w, id)),
  });
}

/** revealInFiles shows the Files pane with path's row in view, its folders open; a folder path ends with "/". */
export function revealInFiles(path?: string): void {
  update((w) => selectPane(path === undefined ? w : revealPath(w, path), "left", "files"));
}

/** openMoreOptions opens a view's More options at (x, y). */
export function openMoreOptions(c: Client, tabId: string, view: ViewState, x: number, y: number): void {
  const file = view.type === "empty" ? null : view.path;
  const items: MenuItem[] = [];
  if (file !== null) items.push(item("history", "Open version history"), SEP);
  items.push(item("split-right", "Split right"), item("split-down", "Split down"));
  if (file !== null) {
    items.push(
      SEP,
      item("rename", "Rename…", { keys: "F2" }),
      item("move", "Move file to…"),
      item("copy-path", "Copy path"),
      item("reveal", "Reveal in Files"),
      item("open-default", "Open in default app", { disabled: true }),
      SEP,
      item("delete", "Delete file"),
    );
  }
  openMenu({
    label: "More options",
    x,
    y,
    items,
    onChoose: (id) => {
      switch (id) {
        case "history":
          if (file !== null) update((w) => open(activate(w, tabId), { type: "history", path: file }, "tab"));
          return;
        case "split-right":
          return update((w) => split(w, tabId, "right"));
        case "split-down":
          return update((w) => split(w, tabId, "down"));
        case "copy-path":
          if (file !== null) void copyPath(file);
          return;
        case "move":
          if (file !== null) openMove(file, false);
          return;
        case "reveal":
          if (file !== null) revealInFiles(file);
          return;
        case "rename":
          if (file !== null) {
            revealInFiles(file);
            setRenaming(file);
          }
          return;
        case "delete":
          if (file !== null) void deleteFile(c, file);
          return;
      }
    },
  });
}

/** openEditingModeMenu opens the status bar's editing mode menu above (x, y). */
export function openEditingModeMenu(tabId: string | null, x: number, y: number): void {
  const ws = useWorkspace.getState().ws;
  const view = tabId ? findTab(ws, tabId)?.tab.view : undefined;
  const reading = view?.type === "note" && view.mode === "read";
  const mode = usePrefs.getState().editingMode;
  openMenu({
    label: "Editing mode",
    x,
    y: y - 3 * 28 - 10,
    items: [
      item("live", "Live Preview", { checked: !reading && mode === "live" }),
      item("source", "Source mode", { checked: !reading && mode === "source" }),
      item("read", "Reading view", { checked: reading }),
    ],
    onChoose: (id) => {
      if (id === "live" || id === "source") usePrefs.getState().setPrefs({ editingMode: id });
      if (!tabId || view?.type !== "note") return;
      update((w) => setView(w, tabId, { ...view, mode: id === "read" ? "read" : "edit" }));
    },
  });
}

/** openConflictsMenu lists the vault's flagged files; choosing one compares it with its latest conflict revision. */
export function openConflictsMenu(conflicts: SyncConflict[], x: number, y: number): void {
  openMenu({
    label: "Conflicts",
    x,
    y,
    items: conflicts.map((c) => item(c.path, c.path.replace(/\.md$/, ""))),
    onChoose: (path) => {
      const seq = conflicts.find((c) => c.path === path)?.revisions.at(-1)?.seq;
      update((w) =>
        open(
          w,
          seq === undefined ? { type: "history", path } : { type: "merge", path, other: { kind: "revision", seq } },
          "tab",
        ),
      );
    },
  });
}
