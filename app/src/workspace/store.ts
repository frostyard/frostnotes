// The workspace store (docs/design/app.md#workspace; ADR-0021): the window's
// vault's workspace, kept in local storage per vault path and written 500
// ms after the last change. Moves and deletes reach every vault's saved
// workspace through the event feed, replays included, so tabs follow what
// happened while the app was closed.
import { create, type StoreApi, type UseBoundStore } from "zustand";

import type { EventFeed } from "../data/events";
import { type KeyValue, localStore } from "../data/storage";
import type { NotesState } from "../data/stores";
import type { Event } from "../rpc/gen/api";
import { emptyWorkspace, type Workspace } from "./model";
import { applyDelete, applyMove } from "./ops";
import { validate } from "./validate";

/** workspaceKey is the local-storage key of a vault's workspace. */
export function workspaceKey(vaultPath: string): string {
  return `frostnotes.workspace.v1:${vaultPath}`;
}

/** SAVE_DELAY is how long after the last change the workspace is written. */
export const SAVE_DELAY = 500;

/** WorkspaceState is the window's workspace and what changes it. */
export interface WorkspaceState {
  /** The vault whose workspace this is; null when the window shows none. */
  vaultPath: string | null;
  ws: Workspace;
  /** load shows a vault's stored workspace, writing the one shown before. */
  load(vaultPath: string | null): void;
  /** update applies an operation and writes the result soon. */
  update(op: (ws: Workspace) => Workspace): void;
  /** flush writes a pending change now. */
  flush(): void;
  /** reset forgets the window's vault's layout (Reset workspace layout). */
  reset(): void;
}

/** readWorkspace reads a vault's stored workspace. */
export function readWorkspace(storage: KeyValue, vaultPath: string): Workspace {
  const stored = storage.get(workspaceKey(vaultPath));
  if (stored === null) return emptyWorkspace();
  try {
    return validate(JSON.parse(stored));
  } catch {
    return emptyWorkspace();
  }
}

/** createWorkspaceStore makes a workspace store over storage. */
export function createWorkspaceStore(storage: KeyValue, delay = SAVE_DELAY): UseBoundStore<StoreApi<WorkspaceState>> {
  let timer: ReturnType<typeof setTimeout> | undefined;
  return create<WorkspaceState>((set, get) => {
    const write = () => {
      clearTimeout(timer);
      timer = undefined;
      const { vaultPath, ws } = get();
      if (vaultPath !== null) storage.set(workspaceKey(vaultPath), JSON.stringify(ws));
    };
    return {
      vaultPath: null,
      ws: emptyWorkspace(),
      load: (vaultPath) => {
        if (vaultPath === get().vaultPath) return;
        if (timer !== undefined) write();
        set({ vaultPath, ws: vaultPath === null ? emptyWorkspace() : readWorkspace(storage, vaultPath) });
      },
      update: (op) => {
        const ws = op(get().ws);
        if (ws === get().ws) return;
        set({ ws });
        clearTimeout(timer);
        timer = setTimeout(write, delay);
      },
      flush: () => {
        if (timer !== undefined) write();
      },
      reset: () => {
        clearTimeout(timer);
        timer = undefined;
        const { vaultPath } = get();
        if (vaultPath !== null) storage.remove(workspaceKey(vaultPath));
        set({ ws: emptyWorkspace() });
      },
    };
  });
}

/**
 * followEvent applies a move or a delete to the workspace of the vault it
 * happened in: the store's when that vault is shown, else the stored one.
 * pathOf names a vault by ID.
 */
export function followEvent(
  store: UseBoundStore<StoreApi<WorkspaceState>>,
  storage: KeyValue,
  e: Event,
  pathOf: (vaultId: number) => string | undefined,
): void {
  let vaultId: number;
  let op: (ws: Workspace) => Workspace;
  switch (e.event) {
    case "note.moved":
      vaultId = e.data.vaultId;
      op = (ws) => applyMove(ws, e.data.from, e.data.to);
      break;
    case "note.removed":
      vaultId = e.data.vaultId;
      op = (ws) => applyDelete(ws, e.data.path);
      break;
    default:
      return;
  }
  const path = pathOf(vaultId);
  if (path === undefined) return;
  if (path === store.getState().vaultPath) {
    store.getState().update(op);
    return;
  }
  if (storage.get(workspaceKey(path)) === null) return;
  storage.set(workspaceKey(path), JSON.stringify(op(readWorkspace(storage, path))));
}

/** useWorkspace is the window's workspace. */
const storage = localStore();
export const useWorkspace = createWorkspaceStore(storage);

/**
 * wireWorkspace keeps the window's workspace in step with the window's
 * vault and the event feed, and writes it when the page goes away. Call it
 * once, before Session connects, so replays reach it; it returns the
 * unwiring.
 */
export function wireWorkspace(feed: EventFeed, notes: UseBoundStore<StoreApi<NotesState>>): () => void {
  const pathOf = (id: number) => notes.getState().vaults.find((v) => v.id === id)?.path;
  const show = (s: NotesState) => useWorkspace.getState().load(s.vaultId === null ? null : (pathOf(s.vaultId) ?? null));
  show(notes.getState());
  const offNotes = notes.subscribe((s, prev) => {
    if (s.vaultId !== prev.vaultId || s.vaults !== prev.vaults) show(s);
  });
  const offFeed = feed.listen((e) => followEvent(useWorkspace, storage, e, pathOf));
  const flush = () => useWorkspace.getState().flush();
  globalThis.addEventListener?.("pagehide", flush);
  return () => {
    offNotes();
    offFeed();
    globalThis.removeEventListener?.("pagehide", flush);
  };
}
