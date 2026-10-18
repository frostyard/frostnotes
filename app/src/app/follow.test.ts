// Following a link no note answers (review finding 13, plan 0005 Phase 7):
// a name makes the note, as Obsidian does, but a protocol-relative URL
// (//host) names no note, as the reading view's sanitizer has it.
import { afterEach, describe, expect, it, vi } from "vitest";

import { useNotes } from "../data/stores";
import type { Client } from "../rpc/gen/api";
import { useWorkspace } from "../workspace/store";
import { followLink } from "./commands";

afterEach(() => {
  useNotes.setState({ vaultId: null });
});

describe("followLink", () => {
  it("makes a note for a name, and nothing for //host", async () => {
    useNotes.setState({ vaultId: 1 });
    const create = vi.fn(async () => ({}));
    const c = { note: { create } } as unknown as Client;
    const tab = useWorkspace.getState().ws.root;
    const tabId = tab.kind === "tabs" ? (tab.tabs[0]?.id ?? "") : "";
    await followLink(c, { kind: "follow", target: "//evil.test/x", path: null, where: "here" }, tabId);
    await followLink(c, { kind: "follow", target: " //evil.test", path: null, where: "here" }, tabId);
    expect(create).not.toHaveBeenCalled();
    await followLink(c, { kind: "follow", target: "Ideas", path: null, where: "here" }, tabId);
    expect(create).toHaveBeenCalledWith(expect.objectContaining({ path: "Ideas.md" }));
  });
});
