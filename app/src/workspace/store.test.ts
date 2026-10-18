import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import { memoryStore } from "../data/storage";
import type { Event } from "../rpc/gen/api";
import { emptyWorkspace, type ViewState } from "./model";
import { activeTab, open } from "./ops";
import { createWorkspaceStore, followEvent, readWorkspace, SAVE_DELAY, workspaceKey } from "./store";

const note = (path: string): ViewState => ({ type: "note", path, mode: "edit" });

beforeEach(() => vi.useFakeTimers());
afterEach(() => vi.useRealTimers());

describe("the workspace store", () => {
  it("keeps one workspace per vault, written SAVE_DELAY after the last change", () => {
    const storage = memoryStore();
    const store = createWorkspaceStore(storage);
    store.getState().load("/v/a");
    store.getState().update((ws) => open(ws, note("x.md")));
    expect(storage.get(workspaceKey("/v/a"))).toBeNull();
    vi.advanceTimersByTime(SAVE_DELAY);
    expect(activeTab(readWorkspace(storage, "/v/a")).view).toEqual(note("x.md"));

    // Switching vaults writes the pending change first, then shows the other's.
    store.getState().update((ws) => open(ws, note("y.md")));
    store.getState().load("/v/b");
    expect(activeTab(readWorkspace(storage, "/v/a")).view).toEqual(note("y.md"));
    expect(store.getState().ws).toEqual(emptyWorkspace());
    store.getState().load("/v/a");
    expect(activeTab(store.getState().ws).view).toEqual(note("y.md"));
  });

  it("starts empty from storage it cannot read, and resets", () => {
    const storage = memoryStore({ [workspaceKey("/v")]: "{broken" });
    const store = createWorkspaceStore(storage);
    store.getState().load("/v");
    expect(store.getState().ws).toEqual(emptyWorkspace());
    store.getState().update((ws) => open(ws, note("x.md")));
    store.getState().flush();
    expect(storage.get(workspaceKey("/v"))).not.toBeNull();
    store.getState().reset();
    expect(storage.get(workspaceKey("/v"))).toBeNull();
    expect(store.getState().ws).toEqual(emptyWorkspace());
  });

  it("follows moves in the shown vault and in the others' stored workspaces", () => {
    const storage = memoryStore();
    const store = createWorkspaceStore(storage);
    store.getState().load("/v/b");
    store.getState().update((ws) => open(ws, note("Old/x.md")));
    store.getState().load("/v/a");
    store.getState().update((ws) => open(ws, note("Old/y.md")));
    const paths: Record<number, string> = { 1: "/v/a", 2: "/v/b" };
    const moved = (vaultId: number): Event => ({
      seq: 1,
      event: "note.moved",
      data: { vaultId, from: "Old", to: "New" },
    });
    followEvent(store, storage, moved(1), (id) => paths[id]);
    followEvent(store, storage, moved(2), (id) => paths[id]);
    followEvent(store, storage, moved(3), (id) => paths[id]); // a vault this app never showed
    expect(activeTab(store.getState().ws).view).toEqual(note("New/y.md"));
    expect(activeTab(readWorkspace(storage, "/v/b")).view).toEqual(note("New/x.md"));
    expect(storage.data.size).toBe(1); // /v/a is not written yet; nothing for vault 3
  });
});
