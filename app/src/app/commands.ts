// The window's commands (docs/design/app.md#commands-and-keys): the
// workspace's, the sidebars', the active editor's, creating a note, and
// following a link an editor drew or the cursor is on, and Obsidian's
// formatting commands (editor/format.ts). Each reads the stores when it
// runs, so the registry holds them once.

import { syntaxTree } from "@codemirror/language";
import { openSearchPanel } from "@codemirror/search";
import type { EditorState, StateCommand } from "@codemirror/state";
import type { SyntaxNode } from "@lezer/common";
import { isTauri } from "@tauri-apps/api/core";
import { getCurrentWindow } from "@tauri-apps/api/window";

import type { Command } from "../commands/registry";
import { useFiles } from "../data/files";
import { ask, notify, openPrompt, openSettings } from "../data/overlays";
import { setRenaming, usePanes } from "../data/panes";
import { usePrefs } from "../data/prefs";
import { useNotes } from "../data/stores";
import { useVaultState } from "../data/vaultState";
import {
  deleteParagraph,
  insertLink,
  setHeading,
  toggleBold,
  toggleChecklist,
  toggleCode,
  toggleComment,
  toggleHighlight,
  toggleItalics,
  toggleStrikethrough,
} from "../editor/format";
import { type PreviewCommand, previewEnv } from "../editor/preview/framework";
import type { DocumentRegistry } from "../editor/registry";
import { findLinks } from "../editor/syntax/extract";
import type { Client } from "../rpc/gen/api";
import type { ViewState } from "../workspace/model";
import {
  activate,
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
import { revealInFiles } from "./menus";
import { searchFor } from "./panes/counts";

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
  const files = useFiles.getState().files;
  await createNoteAt(
    c,
    newNotePath((p) => files.has(p), newNoteFolder()),
    where,
  );
}

// createNoteAt creates an empty note at path and opens it, from tabId's tab when given.
async function createNoteAt(c: Client, path: string, where: OpenWhere, tabId?: string): Promise<void> {
  const vaultId = useNotes.getState().vaultId;
  if (vaultId === null) return;
  await c.note.create({ vaultId, path, content: "" });
  update((w) => open(tabId ? activate(w, tabId) : w, { type: "note", path, mode: "edit" }, where));
}

/**
 * openOrCreate opens the note named name (as typed, with or without
 * ".md"; folders it names are kept), creating it where new notes go when
 * it does not exist: the quick switcher's Shift+Enter.
 */
export async function openOrCreate(c: Client, name: string, where: OpenWhere): Promise<void> {
  const bare = name.trim().replace(/^\/+/, "");
  if (bare === "") return;
  const file = bare.toLowerCase().endsWith(".md") ? bare : `${bare}.md`;
  const folder = newNoteFolder();
  const path = file.includes("/") || folder === "" ? file : `${folder}/${file}`;
  if (useFiles.getState().files.has(path)) {
    update((w) => open(w, viewOf(path), where));
    return;
  }
  await createNoteAt(c, path, where);
}

/** viewOf is how a vault file opens: a note in the default view, anything else by its hash. */
export function viewOf(path: string): ViewState {
  return path.endsWith(".md") ? { type: "note", path, mode: usePrefs.getState().defaultView } : { type: "file", path };
}

const nameOf = (path: string) => path.slice(path.lastIndexOf("/") + 1).replace(/\.md$/, "");

/** copyPath puts a file's vault path on the clipboard. */
export async function copyPath(path: string): Promise<void> {
  try {
    await navigator.clipboard.writeText(path);
    notify("Copied the path.");
  } catch (err) {
    notify(`Cannot copy the path: ${err instanceof Error ? err.message : String(err)}`);
  }
}

/**
 * deleteFile deletes a file with its base, asking first while the Confirm
 * file deletion preference is on; notesd keeps its versions.
 */
export async function deleteFile(c: Client, path: string): Promise<void> {
  const vaultId = useNotes.getState().vaultId;
  const file = useFiles.getState().files.get(path);
  if (vaultId === null || !file) return;
  const name = nameOf(path);
  if (
    usePrefs.getState().confirmDelete &&
    !(await ask({
      title: `Delete "${name}"?`,
      text: "You can restore it from its version history.",
      confirmLabel: "Delete",
      danger: true,
    }))
  ) {
    return;
  }
  try {
    await c.note.delete({ vaultId, path, base: file.hash });
    notify(`Deleted ${name}.`);
  } catch (err) {
    notify(`Cannot delete ${name}: ${err instanceof Error ? err.message : String(err)}`);
  }
}

const WEB = /^(https?|mailto):/i;

/**
 * followLink follows a link an editor drew, from its tab: a vault file
 * opens; a web link opens outside the app; an unresolved note link creates
 * the note, as Obsidian does: under its own folders when it names any,
 * else where new notes go.
 */
export async function followLink(
  c: Client,
  cmd: Extract<PreviewCommand, { kind: "follow" }>,
  tabId: string,
): Promise<void> {
  if (cmd.path !== null) {
    const path = cmd.path;
    const view: ViewState = path.endsWith(".md") ? { type: "note", path, mode: "edit" } : { type: "file", path };
    update((w) => open(activate(w, tabId), view, cmd.where));
    return;
  }
  if (WEB.test(cmd.target)) {
    // The desktop app hands web links to the system browser (tauri-plugin-opener, with the menus).
    window.open(cmd.target, "_blank", "noopener,noreferrer");
    return;
  }
  const target = cmd.target.trim().replace(/^\/+/, "");
  if (target === "" || /^[a-zA-Z][a-zA-Z0-9+.-]*:/.test(target)) return;
  const name = target.endsWith(".md") ? target : `${target}.md`;
  const folder = newNoteFolder();
  await createNoteAt(c, name.includes("/") || folder === "" ? name : `${folder}/${name}`, cmd.where, tabId);
}

/** previewCommand does what a view of a note asks from its tab: follows a link, searches (a tag), or copies (a code block). */
export async function previewCommand(c: Client, cmd: PreviewCommand, tabId: string): Promise<void> {
  switch (cmd.kind) {
    case "follow":
      await followLink(c, cmd, tabId);
      return;
    case "search":
      searchFor(cmd.query);
      return;
    case "copy":
      try {
        await navigator.clipboard.writeText(cmd.text);
        notify("Copied to the clipboard.");
      } catch {
        notify("Cannot copy to the clipboard.");
      }
  }
}

const hasVault = () => useNotes.getState().vaultId !== null;
const noteView = (): Extract<ViewState, { type: "note" }> | undefined => {
  const v = activeTab(ws()).view;
  return v.type === "note" ? v : undefined;
};

/**
 * mergeDone finishes a merge: the note's conflict notice goes, the file's
 * conflict flag on its hub clears when the vault syncs (sync.resolve), and
 * the merge tab closes.
 */
export async function mergeDone(c: Client, docs: DocumentRegistry, path: string, closeTab: () => void): Promise<void> {
  const vaultId = useNotes.getState().vaultId;
  if (vaultId !== null) {
    const doc = docs.find(vaultId, path);
    if (doc?.notice?.kind === "conflict" || doc?.notice?.kind === "refused") doc.dismiss();
    if (useVaultState.getState().sync) {
      try {
        await c.sync.resolve({ vaultId, path });
      } catch {
        // A file the hub has no flag for, or no hub to reach: nothing to clear.
      }
    }
  }
  closeTab();
}

/** linkAt is the link at pos in a view's note: its target and, for a web link, its URL. */
export function linkAt(state: EditorState, pos: number): { target: string; href?: string } | null {
  const line = state.doc.lineAt(pos);
  const link = findLinks(state, line.from, line.to).find((l) => l.from <= pos && pos <= l.to);
  if (link) return { target: link.target };
  for (let n: SyntaxNode | null = syntaxTree(state).resolveInner(pos, 0); n; n = n.parent) {
    if (n.name !== "Link" && n.name !== "Autolink" && n.name !== "URL") continue;
    const url = n.name === "URL" ? n : n.getChild("URL");
    const href = url ? state.sliceDoc(url.from, url.to).replace(/^<|>$/g, "") : "";
    return /^[a-zA-Z][a-zA-Z0-9+.-]*:/.test(href) ? { target: href, href } : null;
  }
  return null;
}

/** editorCommands are the commands of the active tab's editor. */
export function editorCommands(c: Client, docs: DocumentRegistry): Command[] {
  const editor = () => {
    const tab = activeTab(ws());
    return tab.view.type === "note" && tab.view.mode === "edit" ? docs.editors.get(tab.id) : undefined;
  };
  const has = () => editor() !== undefined;
  // format runs one of Obsidian's formatting commands on the active editor.
  const format = (id: string, name: string, cmd: StateCommand): Command => ({
    id,
    name,
    applies: has,
    run: () => {
      const e = editor();
      if (!e) return;
      cmd(e.view);
      e.view.focus();
    },
  });
  // follow follows the link under the cursor, resolved from the note's folder.
  const follow = (id: string, name: string, where: OpenWhere): Command => ({
    id,
    name,
    applies: () => {
      const e = editor();
      return e !== undefined && linkAt(e.view.state, e.view.state.selection.main.head) !== null;
    },
    run: async () => {
      const e = editor();
      const vaultId = useNotes.getState().vaultId;
      const link = e && linkAt(e.view.state, e.view.state.selection.main.head);
      // A link to the note itself ([[#Heading]]) leads nowhere else.
      if (!e || !link || link.target === "" || vaultId === null) return;
      const tabId = activeTab(ws()).id;
      let path: string | null = null;
      if (!link.href) {
        const known = e.view.state.facet(previewEnv).resolve(link.target);
        path =
          known !== undefined
            ? known
            : ((await c.note.resolve({ vaultId, from: e.doc.path, targets: [link.target] }))[0]?.path ?? null);
      }
      await followLink(c, { kind: "follow", target: link.target, path, where }, tabId);
    },
  });
  return [
    format("editor:toggle-bold", "Toggle bold", toggleBold),
    format("editor:toggle-italics", "Toggle italics", toggleItalics),
    format("editor:toggle-strikethrough", "Toggle strikethrough", toggleStrikethrough),
    format("editor:toggle-highlight", "Toggle highlight", toggleHighlight),
    format("editor:toggle-code", "Toggle code", toggleCode),
    format("editor:toggle-comments", "Toggle comment", toggleComment),
    format("editor:insert-link", "Insert Markdown link", insertLink),
    format("editor:toggle-checklist-status", "Toggle checkbox status", toggleChecklist),
    format("editor:delete-paragraph", "Delete paragraph", deleteParagraph),
    ...[1, 2, 3, 4, 5, 6].map((n) => format(`editor:set-heading-${n}`, `Set heading ${n}`, setHeading(n))),
    format("editor:set-heading-0", "Remove heading", setHeading(0)),
    follow("editor:follow-link", "Follow link under cursor", "here"),
    follow("editor:open-link-in-new-leaf", "Open link under cursor in new tab", "tab"),
    follow("editor:open-link-in-new-split", "Open link under cursor to the right", "right"),
    // A new window is a new tab until the app has more windows.
    follow("editor:open-link-in-new-window", "Open link under cursor in new window", "tab"),
    {
      id: "global-search:open",
      name: "Search in all files",
      applies: hasVault,
      run: () => {
        // The editor's selection becomes the query, quoted when it holds a space.
        const e = editor();
        const sel = e ? e.view.state.sliceDoc(e.view.state.selection.main.from, e.view.state.selection.main.to) : "";
        const q = sel.trim();
        searchFor(q === "" ? usePanes.getState().searchQuery : /\s/.test(q) ? `"${q}"` : q);
      },
    },
    {
      id: "workspace:edit-file-title",
      name: "Rename file",
      applies: () => activeTab(ws()).view.type === "note" || activeTab(ws()).view.type === "file",
      run: () => {
        const v = activeTab(ws()).view;
        if (v.type !== "note" && v.type !== "file") return;
        revealInFiles(v.path);
        setRenaming(v.path);
      },
    },
    { id: "editor:save-file", name: "Save current file", applies: has, run: () => editor()?.doc.save() },
    {
      id: "editor:open-search",
      name: "Search current file",
      applies: has,
      run: () => {
        const e = editor();
        if (e) openSearchPanel(e.view);
      },
    },
    {
      id: "editor:open-search-replace",
      name: "Search & replace in current file",
      applies: has,
      run: () => {
        const e = editor();
        if (!e) return;
        openSearchPanel(e.view);
        e.view.dom.querySelector<HTMLInputElement>(".cm-search input[name=replace]")?.focus();
      },
    },
  ];
}

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
    { id: "switcher:open", name: "Open quick switcher", applies: hasVault, run: () => openPrompt("switcher") },
    { id: "command-palette:open", name: "Open command palette", run: () => openPrompt("palette") },
    { id: "app:open-settings", name: "Open settings", run: () => openSettings() },
    { id: "app:open-help", name: "Open help", run: () => openSettings("hotkeys") },
    {
      id: "sync:view-version-history",
      name: "Open version history",
      applies: () => {
        const v = activeTab(ws()).view;
        return v.type === "note" || v.type === "file";
      },
      run: () =>
        update((w) => {
          const v = activeTab(w).view;
          return v.type === "note" || v.type === "file" ? open(w, { type: "history", path: v.path }, "tab") : w;
        }),
    },
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
