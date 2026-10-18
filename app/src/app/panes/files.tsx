// The Files pane's container (docs/specs/ui.md, Files pane): its rows from
// the files store and the workspace's sort and open folders, and what its
// commands do: open, create, rename and move through notesd, with links
// updated as the vault's setting says (asking when it is off).
import { useMemo } from "react";

import { refreshFolders, useFiles } from "../../data/files";
import { ask, notify, openMenu, openMove } from "../../data/overlays";
import { setRenaming, usePanes } from "../../data/panes";
import { usePrefs } from "../../data/prefs";
import { useClient } from "../../data/session";
import { useNotes } from "../../data/stores";
import { useVaultState } from "../../data/vaultState";
import { type FilesCommand, FilesPane } from "../../features/explorer/FilesPane";
import type { MenuItem } from "../../features/menu/Menu";
import { type SortOrder, treeRows } from "../../lib/fileTree";
import type { Client } from "../../rpc/gen/api";
import { activeTab, open, setFileSort, setFolderOpen, setFoldersOpen } from "../../workspace/ops";
import { useWorkspace } from "../../workspace/store";
import { copyPath, createNote, deleteFile, newNotePath } from "../commands";
import { openFile } from "./open";

const SORTS: { id: SortOrder; label: string }[] = [
  { id: "alphabetical", label: "File name (A to Z)" },
  { id: "alphabeticalReverse", label: "File name (Z to A)" },
  { id: "byModifiedTime", label: "Modified time (new to old)" },
  { id: "byModifiedTimeReverse", label: "Modified time (old to new)" },
];

const update = (op: Parameters<ReturnType<typeof useWorkspace.getState>["update"]>[0]) =>
  useWorkspace.getState().update(op);
const parentOf = (p: string) => (p.includes("/") ? p.slice(0, p.lastIndexOf("/")) : "");
const nameOf = (p: string) => p.slice(p.lastIndexOf("/") + 1);
const join = (folder: string, name: string) => (folder === "" ? name : `${folder}/${name}`);
const message = (err: unknown) => (err instanceof Error ? err.message : String(err));

/** FilesContainer draws the Files pane over the window's vault. */
export function FilesContainer() {
  const client = useClient();
  const files = useFiles((s) => s.files);
  const folders = useFiles((s) => s.folders);
  const loaded = useFiles((s) => s.loaded);
  const pane = useWorkspace((s) => s.ws.files);
  const active = useWorkspace((s) => {
    const v = activeTab(s.ws).view;
    return v.type === "empty" ? null : v.path;
  });
  const showUnsupported = usePrefs((s) => s.showUnsupported);
  const guides = usePrefs((s) => s.indentGuides);
  const renaming = usePanes((s) => s.renaming);
  const rows = useMemo(
    () =>
      treeRows(
        [...files.values()].map((f) => ({ path: f.path, modifiedAt: Date.parse(f.modifiedAt) || 0 })),
        folders,
        { sort: pane.sort, expanded: new Set(pane.expanded), showUnsupported },
      ),
    [files, folders, pane, showUnsupported],
  );
  return (
    <FilesPane
      rows={rows}
      active={active}
      anyExpanded={pane.expanded.length > 0}
      indentGuides={guides}
      renaming={renaming}
      loaded={loaded}
      onCommand={(c) => void run(client, c, folders)}
    />
  );
}

async function run(c: Client, cmd: FilesCommand, folders: readonly string[]): Promise<void> {
  const vaultId = useNotes.getState().vaultId;
  if (vaultId === null) return;
  try {
    switch (cmd.kind) {
      case "open":
        return openFile(cmd.path, cmd.click);
      case "toggle":
        return update((w) => setFolderOpen(w, cmd.path, !w.files.expanded.includes(cmd.path)));
      case "newNote":
        return await createNote(c, "here");
      case "newFolder":
        return await newFolder(c, vaultId, "");
      case "sortMenu":
        return openMenu({
          label: "Sort order",
          x: cmd.x,
          y: cmd.y,
          items: SORTS.map((s) => ({
            kind: "item",
            id: s.id,
            label: s.label,
            checked: useWorkspace.getState().ws.files.sort === s.id,
          })),
          onChoose: (id) => update((w) => setFileSort(w, id as SortOrder)),
        });
      case "collapseAll":
        return update((w) => setFoldersOpen(w, []));
      case "expandAll":
        return update((w) => setFoldersOpen(w, [...folders]));
      case "menu":
        return openFilesMenu(c, vaultId, cmd.path, cmd.folder, cmd.x, cmd.y);
      case "rename":
        return setRenaming(cmd.path);
      case "cancelRename":
        return setRenaming(null);
      case "renamed":
        setRenaming(null);
        if (cmd.name.trim() === "") return; // a blank name keeps the old one
        return await rename(c, vaultId, cmd.path, cmd.name.trim(), folders);
      case "drop":
        return await drop(c, vaultId, cmd.paths, cmd.folder, folders);
    }
  } catch (err) {
    notify(message(err));
  }
}

// newFolder makes "Untitled" (or "Untitled N") in folder and starts renaming it.
async function newFolder(c: Client, vaultId: number, folder: string): Promise<void> {
  const taken = new Set(useFiles.getState().folders);
  let path = join(folder, "Untitled");
  for (let n = 1; taken.has(path); n++) path = join(folder, `Untitled ${n}`);
  await c.note.mkdir({ vaultId, path });
  await refreshFolders(c, vaultId);
  if (folder !== "") update((w) => setFolderOpen(w, folder, true));
  setRenaming(path);
}

// newNoteIn creates an empty note in folder and opens it.
async function newNoteIn(c: Client, vaultId: number, folder: string): Promise<void> {
  const files = useFiles.getState().files;
  const path = newNotePath((p) => files.has(p), folder);
  await c.note.create({ vaultId, path, content: "" });
  update((w) => open(setFolderOpen(w, folder, true), { type: "note", path, mode: "edit" }, "here"));
}

// updateLinks says whether a move rewrites links: the vault's setting, or the user's answer.
async function updateLinks(name: string): Promise<boolean> {
  if (useVaultState.getState().settings?.updateLinks) return true;
  return ask({
    title: "Update links?",
    text: `Update the links to "${name}" so they still lead to it?`,
    confirmLabel: "Update",
    cancelLabel: "Don't update",
  });
}

/**
 * renameNote renames a note from its inline title: the same folder, the
 * new name, asking about links as a move does. A name taken says so.
 */
export async function renameNote(c: Client, vaultId: number, path: string, title: string): Promise<void> {
  const slash = path.lastIndexOf("/");
  const to = `${slash < 0 ? "" : path.slice(0, slash + 1)}${title}.md`;
  if (to === path) return;
  if (useFiles.getState().files.has(to)) {
    notify(`${to} exists already.`);
    return;
  }
  try {
    await move(c, vaultId, path, to, false);
  } catch (err) {
    notify(`Cannot rename ${path}: ${err instanceof Error ? err.message : String(err)}`);
  }
}

/** move moves a file or folder to a new path, its links updated as the vault's setting says. */
export async function move(c: Client, vaultId: number, from: string, to: string, folder: boolean): Promise<void> {
  if (from === to) return;
  const links = await updateLinks(nameOf(from).replace(/\.md$/, ""));
  if (folder) {
    await c.note.moveFolder({ vaultId, path: from, to, updateLinks: links });
    await refreshFolders(c, vaultId);
  } else {
    await c.note.move({ vaultId, path: from, to, updateLinks: links });
  }
}

// rename renames a row in its folder: a file keeps its extension.
async function rename(
  c: Client,
  vaultId: number,
  path: string,
  name: string,
  folders: readonly string[],
): Promise<void> {
  const folder = folders.includes(path);
  const base = nameOf(path);
  const dot = base.lastIndexOf(".");
  const ext = !folder && dot > 0 ? base.slice(dot) : "";
  await move(c, vaultId, path, join(parentOf(path), name + ext), folder);
}

// drop moves dragged rows into folder, skipping what is there already or would go into itself.
async function drop(
  c: Client,
  vaultId: number,
  paths: string[],
  folder: string,
  folders: readonly string[],
): Promise<void> {
  for (const p of paths) {
    if (parentOf(p) === folder || p === folder || folder.startsWith(`${p}/`)) continue;
    await move(c, vaultId, p, join(folder, nameOf(p)), folders.includes(p));
  }
}

// openFilesMenu opens a row's context menu, or the root's (path null).
function openFilesMenu(c: Client, vaultId: number, path: string | null, folder: boolean, x: number, y: number): void {
  const item = (id: string, label: string, extra: Partial<Extract<MenuItem, { kind: "item" }>> = {}): MenuItem => ({
    kind: "item",
    id,
    label,
    ...extra,
  });
  const sep: MenuItem = { kind: "separator" };
  const items: MenuItem[] = folder
    ? [
        item("new-note", "New note"),
        item("new-folder", "New folder"),
        ...(path === null
          ? []
          : [
              sep,
              item("rename", "Rename…", { keys: "F2" }),
              item("move", "Move folder to…"),
              sep,
              item("delete", "Delete"),
            ]),
      ]
    : [
        item("open-tab", "Open in new tab"),
        item("open-right", "Open to the right"),
        sep,
        item("rename", "Rename…", { keys: "F2" }),
        item("move", "Move file to…"),
        sep,
        item("copy-path", "Copy path"),
        item("history", "Open version history"),
        item("open-default", "Open in default app", { disabled: true }),
        sep,
        item("delete", "Delete"),
      ];
  const at = path ?? "";
  openMenu({
    label: folder ? "Folder" : "File",
    x,
    y,
    items,
    onChoose: (id) => {
      const act = async () => {
        switch (id) {
          case "new-note":
            return newNoteIn(c, vaultId, at);
          case "new-folder":
            return newFolder(c, vaultId, at);
          case "rename":
            return setRenaming(at);
          case "move":
            return openMove(at, folder);
          case "open-tab":
            return openFile(at, { ctrl: true, alt: false, middle: false });
          case "open-right":
            return openFile(at, { ctrl: true, alt: true, middle: false });
          case "copy-path":
            return copyPath(at);
          case "history":
            return update((w) => open(w, { type: "history", path: at }, "tab"));
          case "delete":
            return folder ? deleteFolder(c, vaultId, at) : deleteFile(c, at);
        }
      };
      act().catch((err: unknown) => notify(message(err)));
    },
  });
}

// deleteFolder deletes a folder and its files, naming each file's base, after asking.
async function deleteFolder(c: Client, vaultId: number, path: string): Promise<void> {
  const files = [...useFiles.getState().files.values()].filter((f) => f.path.startsWith(`${path}/`));
  const name = nameOf(path);
  if (
    usePrefs.getState().confirmDelete &&
    !(await ask({
      title: `Delete "${name}"?`,
      text: `Its ${files.length} files can be restored from their version history.`,
      confirmLabel: "Delete",
      danger: true,
    }))
  ) {
    return;
  }
  const res = await c.note.deleteFolder({ vaultId, path, files: files.map((f) => ({ path: f.path, base: f.hash })) });
  await refreshFolders(c, vaultId);
  notify(res.kept.length > 0 ? `Deleted ${name}, but kept ${res.kept.length} changed files.` : `Deleted ${name}.`);
}
