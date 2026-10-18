// The window's commands (docs/design/app.md#commands-and-keys): the
// workspace's, the sidebars', and creating a note. Each reads the stores
// when it runs, so the registry holds them once. The commands batch 2's
// prompts, settings and panes add join as those land.

import { isTauri } from "@tauri-apps/api/core";
import { getCurrentWindow } from "@tauri-apps/api/window";

import type { Command } from "../commands/registry";
import { useFiles } from "../data/files";
import { useNotes } from "../data/stores";
import { useVaultState } from "../data/vaultState";
import type { Client } from "../rpc/gen/api";
import type { ViewState } from "../workspace/model";
import {
  activeGroup,
  activeTab,
  back,
  close,
  closeOthers,
  cycle,
  forward,
  gotoLastTab,
  gotoTab,
  type OpenWhere,
  open,
  openNewTab,
  pin,
  reopenClosed,
  setView,
  split,
  toggleSidebar,
} from "../workspace/ops";
import { useWorkspace } from "../workspace/store";

const ws = () => useWorkspace.getState().ws;
const update = (op: Parameters<ReturnType<typeof useWorkspace.getState>["update"]>[0]) =>
  useWorkspace.getState().update(op);

/** newNotePath is where a new note goes: "Untitled.md", or "Untitled N.md" when taken, in the folder the settings name. */
export function newNotePath(taken: (path: string) => boolean, folder: string): string {
  const at = (name: string) => (folder === "" ? name : `${folder}/${name}`);
  for (let n = 0; ; n++) {
    const p = at(n === 0 ? "Untitled.md" : `Untitled ${n}.md`);
    if (!taken(p)) return p;
  }
}

// newNoteFolder is the folder a new note goes in, by the vault's settings
// (Obsidian's Default location for new notes).
function newNoteFolder(): string {
  const s = useVaultState.getState().settings;
  if (s?.newNoteLocation === "folder") return s.newNoteFolder;
  if (s?.newNoteLocation === "current") {
    const v = activeTab(ws()).view;
    if (v.type !== "empty") return v.path.includes("/") ? v.path.slice(0, v.path.lastIndexOf("/")) : "";
  }
  return "";
}

/** createNote creates an empty note where the settings say and opens it. */
export async function createNote(c: Client, where: OpenWhere): Promise<void> {
  const vaultId = useNotes.getState().vaultId;
  if (vaultId === null) return;
  const files = useFiles.getState().files;
  const path = newNotePath((p) => files.has(p), newNoteFolder());
  await c.note.create({ vaultId, path, content: "" });
  update((w) => open(w, { type: "note", path, mode: "edit" }, where));
}

const hasVault = () => useNotes.getState().vaultId !== null;
const noteView = (): Extract<ViewState, { type: "note" }> | undefined => {
  const v = activeTab(ws()).view;
  return v.type === "note" ? v : undefined;
};

/** workspaceCommands are the commands this window has now. */
export function workspaceCommands(c: Client): Command[] {
  const gotos: Command[] = Array.from({ length: 8 }, (_, i) => ({
    id: `workspace:goto-tab-${i + 1}`,
    name: `Go to tab ${i + 1}`,
    run: () => update((w) => gotoTab(w, i + 1)),
  }));
  return [
    { id: "file-explorer:new-file", name: "Create new note", applies: hasVault, run: () => createNote(c, "here") },
    {
      id: "file-explorer:new-file-in-new-pane",
      name: "Create new note to the right",
      applies: hasVault,
      run: () => createNote(c, "right"),
    },
    { id: "workspace:new-tab", name: "New tab", run: () => update(openNewTab) },
    { id: "workspace:close", name: "Close current tab", run: () => update((w) => close(w, activeTab(w).id)) },
    {
      id: "workspace:close-others",
      name: "Close all other tabs",
      applies: () => activeGroup(ws()).tabs.length > 1,
      run: () => update((w) => closeOthers(w, activeTab(w).id)),
    },
    {
      id: "workspace:undo-close-pane",
      name: "Undo close tab",
      applies: () => ws().closed.length > 0,
      run: () => update(reopenClosed),
    },
    { id: "workspace:next-tab", name: "Go to next tab", run: () => update((w) => cycle(w, 1)) },
    { id: "workspace:previous-tab", name: "Go to previous tab", run: () => update((w) => cycle(w, -1)) },
    ...gotos,
    { id: "workspace:goto-last-tab", name: "Go to last tab", run: () => update(gotoLastTab) },
    {
      id: "app:go-back",
      name: "Navigate back",
      applies: () => activeTab(ws()).back.length > 0,
      run: () => update(back),
    },
    {
      id: "app:go-forward",
      name: "Navigate forward",
      applies: () => activeTab(ws()).forward.length > 0,
      run: () => update(forward),
    },
    {
      id: "workspace:split-vertical",
      name: "Split right",
      run: () => update((w) => split(w, activeTab(w).id, "right")),
    },
    {
      id: "workspace:split-horizontal",
      name: "Split down",
      run: () => update((w) => split(w, activeTab(w).id, "down")),
    },
    { id: "workspace:toggle-pin", name: "Toggle pin", run: () => update((w) => pin(w, activeTab(w).id)) },
    { id: "app:toggle-left-sidebar", name: "Toggle left sidebar", run: () => update((w) => toggleSidebar(w, "left")) },
    {
      id: "app:toggle-right-sidebar",
      name: "Toggle right sidebar",
      run: () => update((w) => toggleSidebar(w, "right")),
    },
    {
      id: "markdown:toggle-preview",
      name: "Toggle reading view",
      applies: () => noteView() !== undefined,
      run: () =>
        update((w) => {
          const v = activeTab(w).view;
          return v.type === "note"
            ? setView(w, activeTab(w).id, { ...v, mode: v.mode === "edit" ? "read" : "edit" })
            : w;
        }),
    },
    {
      id: "workspace:close-window",
      name: "Close window",
      applies: () => isTauri(),
      run: () => getCurrentWindow().close(),
    },
    {
      id: "frostnotes:reset-workspace-layout",
      name: "Reset workspace layout",
      applies: hasVault,
      run: () => useWorkspace.getState().reset(),
    },
  ];
}
