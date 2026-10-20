// Following a link no note answers (review finding 13, plan 0005 Phase 7):
// a name makes the note, as Obsidian does, but a protocol-relative URL
// (//host) names no note, as the reading view's sanitizer has it. Inside
// lodge a rooted link no file answers is lodge's address (ADR-0029).
import { afterEach, describe, expect, it, vi } from "vitest";

import { useOverlays } from "../data/overlays";
import { useNotes } from "../data/stores";
import { type Lodge, useLodge } from "../lodge/lodge";
import type { Client } from "../rpc/gen/api";
import { emptyWorkspace } from "../workspace/model";
import { activeTab } from "../workspace/ops";
import { useWorkspace } from "../workspace/store";
import { followLink, openInLodge } from "./commands";

afterEach(() => {
  useNotes.setState({ vaultId: null });
  useLodge.setState({ lodge: null });
  useOverlays.setState({ notices: [] });
  useWorkspace.setState({ ws: emptyWorkspace() });
});

// start gives a client whose note.create is watched, and the first tab's id.
function start() {
  useNotes.setState({ vaultId: 1 });
  const create = vi.fn(async () => ({}));
  const c = { note: { create } } as unknown as Client;
  const tab = useWorkspace.getState().ws.root;
  const tabId = tab.kind === "tabs" ? (tab.tabs[0]?.id ?? "") : "";
  return { c, create, tabId };
}

// inLodge runs the window inside a shell whose open is watched.
function inLodge() {
  const open = vi.fn();
  useLodge.setState({ lodge: { open } as unknown as Lodge });
  return open;
}

const follow = (target: string, path: string | null = null) =>
  ({ kind: "follow", target, path, where: "here" }) as const;

describe("followLink", () => {
  it("makes a note for a name, and nothing for //host", async () => {
    const { c, create, tabId } = start();
    await followLink(c, follow("//evil.test/x"), tabId);
    await followLink(c, follow(" //evil.test"), tabId);
    expect(create).not.toHaveBeenCalled();
    await followLink(c, follow("Ideas"), tabId);
    expect(create).toHaveBeenCalledWith(expect.objectContaining({ path: "Ideas.md" }));
  });

  it("makes the note for a rooted link outside lodge, as Obsidian does", async () => {
    const { c, create, tabId } = start();
    await followLink(c, follow("/mail/message/1"), tabId);
    expect(create).toHaveBeenCalledWith(expect.objectContaining({ path: "mail/message/1.md" }));
  });

  it("opens a rooted link no file answers through lodge, and makes no note", async () => {
    const { c, create, tabId } = start();
    const open = inLodge();
    await followLink(c, follow("/mail/message/abc@example.com"), tabId);
    await followLink(c, follow(" /notes/2/Daily/2026-10-20.md"), tabId);
    expect(open.mock.calls).toEqual([["/mail/message/abc%40example.com"], ["/notes/2/Daily/2026-10-20.md"]]);
    expect(create).not.toHaveBeenCalled();
  });

  it("says nothing is at a rooted link no lodge app can have, and makes no note", async () => {
    const { c, create, tabId } = start();
    const open = inLodge();
    await followLink(c, follow("/Projects/Plan"), tabId);
    await followLink(c, follow("/app/notes/1/Plan.md"), tabId);
    expect(open).not.toHaveBeenCalled();
    expect(create).not.toHaveBeenCalled();
    expect(useOverlays.getState().notices.map((n) => n.text)).toEqual([
      "Nothing in this vault or in lodge is at /Projects/Plan.",
      "Nothing in this vault or in lodge is at /app/notes/1/Plan.md.",
    ]);
  });

  it("opens the file a rooted link resolves to, inside lodge too", async () => {
    const { c, create, tabId } = start();
    const open = inLodge();
    await followLink(c, follow("/mail/message/1", "mail/message/1.md"), tabId);
    const view = activeTab(useWorkspace.getState().ws).view;
    expect(view.type === "note" ? view.path : null).toBe("mail/message/1.md");
    expect(open).not.toHaveBeenCalled();
    expect(create).not.toHaveBeenCalled();
  });

  it("still makes the note for a name inside lodge, and hands web links to the shell", async () => {
    const { c, create, tabId } = start();
    const open = inLodge();
    await followLink(c, follow("Ideas"), tabId);
    expect(create).toHaveBeenCalledWith(expect.objectContaining({ path: "Ideas.md" }));
    await followLink(c, follow("https://lodge.example.ts.net/mail/message/abc%40example.com"), tabId);
    await followLink(c, follow("https://example.com/a"), tabId);
    expect(open.mock.calls).toEqual([
      ["https://lodge.example.ts.net/mail/message/abc%40example.com"],
      ["https://example.com/a"],
    ]);
  });
});

describe("openInLodge", () => {
  it("takes only rooted links, and only inside lodge", () => {
    expect(openInLodge("/mail/message/1")).toBe(false);
    const open = inLodge();
    expect(openInLodge("Ideas")).toBe(false);
    expect(openInLodge("//evil.test/x")).toBe(false);
    expect(openInLodge("/mail/message/1")).toBe(true);
    expect(open).toHaveBeenCalledWith("/mail/message/1");
  });
});
