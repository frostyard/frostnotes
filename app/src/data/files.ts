// The files store (docs/design/app.md#data): the window's vault's files and
// folders from note.files, kept current by note.changed, note.moved and
// note.removed. Events at or before the listing's starting point are in
// the listing already; those that arrive while it loads wait for it.
import { create } from "zustand";

import type { Client, Event, FileKind, VaultFile, VaultFiles } from "../rpc/gen/api";

/** FilesState is one vault's listing. */
export interface FilesState {
  /** The vault listed; null when the window shows none. */
  vaultId: number | null;
  /** Every file, by path. */
  files: ReadonlyMap<string, VaultFile>;
  /** Every folder below the root, empty ones included, sorted. */
  folders: readonly string[];
  /** The listing has arrived (or failed: see error). */
  loaded: boolean;
  /** Why the listing failed. */
  error?: string;
  /** The last sequence number the listing reflects: events up to it change nothing. */
  since: number;
  /** Events that arrived while the listing loaded. */
  queued: readonly Event[];
  /** Which load the state is from: an older load's answer is dropped. */
  generation: number;
  /** Counts changes to the set of paths (a file added, moved or removed), not to contents: link resolution's cache key. */
  pathsVersion: number;
}

/** emptyFiles is the state of vault before its listing arrives. */
export function emptyFiles(vaultId: number | null, since = 0, generation = 0): FilesState {
  return {
    vaultId,
    files: new Map(),
    folders: [],
    loaded: vaultId === null,
    since,
    queued: [],
    generation,
    pathsVersion: 0,
  };
}

export const useFiles = create<FilesState>(() => emptyFiles(null));

const kindOf = (p: string): FileKind => (p.endsWith(".md") ? "note" : "attachment");

function withParents(folders: readonly string[], p: string): readonly string[] {
  const add: string[] = [];
  for (let i = p.lastIndexOf("/"); i > 0; i = p.lastIndexOf("/", i - 1)) {
    const d = p.slice(0, i);
    if (!folders.includes(d)) add.push(d);
  }
  return add.length === 0 ? folders : [...folders, ...add].sort(byPath);
}

/** byPath orders paths as notesd does, by code unit. */
export function byPath(a: string, b: string): number {
  return a < b ? -1 : a > b ? 1 : 0;
}

/** fromListing is the state of a vault's listing. */
export function fromListing(vaultId: number, listing: VaultFiles, since: number, generation = 0): FilesState {
  return {
    vaultId,
    files: new Map(listing.files.map((f) => [f.path, f])),
    folders: [...listing.folders].sort(byPath),
    loaded: true,
    since,
    queued: [],
    generation,
    pathsVersion: 0,
  };
}

/**
 * applyFileEvent returns s after a note event, or s itself when the event
 * changes nothing: another vault's, one the listing reflects, or one it
 * already applied. A changed file's time is now, as the event carries
 * none, and a new file's size waits for the next listing.
 */
export function applyFileEvent(s: FilesState, e: Event, now: () => string): FilesState {
  if (e.seq !== undefined && e.seq <= s.since) return s;
  switch (e.event) {
    case "note.changed": {
      const { vaultId, path, hash } = e.data;
      if (vaultId !== s.vaultId) return s;
      const old = s.files.get(path);
      if (old?.hash === hash) return s;
      const files = new Map(s.files);
      files.set(path, { path, kind: kindOf(path), hash, size: old?.size ?? 0, modifiedAt: now() });
      return { ...s, files, folders: withParents(s.folders, path), pathsVersion: s.pathsVersion + (old ? 0 : 1) };
    }
    case "note.moved": {
      const { vaultId, from, to } = e.data;
      const f = s.files.get(from);
      if (vaultId !== s.vaultId || !f) return s;
      const files = new Map(s.files);
      files.delete(from);
      files.set(to, { ...f, path: to, kind: kindOf(to) });
      return { ...s, files, folders: withParents(s.folders, to), pathsVersion: s.pathsVersion + 1 };
    }
    case "note.removed": {
      const { vaultId, path } = e.data;
      if (vaultId !== s.vaultId || !s.files.has(path)) return s;
      const files = new Map(s.files);
      files.delete(path);
      return { ...s, files, pathsVersion: s.pathsVersion + 1 };
    }
    default:
      return s;
  }
}

const now = () => new Date().toISOString();

/** noteEvent applies a note event to the store, or queues it while the listing loads. */
export function noteEvent(e: Event): void {
  useFiles.setState((s) => (s.loaded ? applyFileEvent(s, e, now) : { ...s, queued: [...s.queued, e] }));
}

/**
 * loadFiles lists vault vaultId into the store; since is the last
 * sequence number notesd had sent when the listing was asked for, so
 * events after it are applied over the listing, in order.
 */
export async function loadFiles(c: Client, vaultId: number, since: number): Promise<void> {
  const prev = useFiles.getState();
  const generation = prev.generation + 1;
  useFiles.setState({ ...emptyFiles(vaultId, since, generation), pathsVersion: prev.pathsVersion + 1 });
  let listing: VaultFiles;
  try {
    listing = await c.note.files({ vaultId });
  } catch (err) {
    useFiles.setState((s) =>
      s.generation === generation ? { ...s, loaded: true, error: err instanceof Error ? err.message : String(err) } : s,
    );
    return;
  }
  useFiles.setState((s) => {
    if (s.generation !== generation) return s;
    let next = { ...fromListing(vaultId, listing, since, generation), pathsVersion: s.pathsVersion + 1 };
    for (const e of s.queued) next = applyFileEvent(next, e, now);
    return next;
  });
}
