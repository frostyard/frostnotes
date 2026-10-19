import { afterEach, describe, expect, it, vi } from "vitest";

import { openPreview, useOverlays } from "../data/overlays";
import { NARROW_BACKLINKS, openIdentityMenu, openNarrowMenu } from "./menus";

afterEach(() => useOverlays.setState({ menu: null, preview: null }));

const labels = () =>
  (useOverlays.getState().menu?.items ?? []).map((i) =>
    i.kind === "item" ? `${i.label}${i.disabled ? " (text)" : ""}` : "-",
  );

describe("a narrow window's Menu", () => {
  it("lists the ribbon's actions, Settings, then the status bar's items as text", () => {
    const run = vi.fn();
    openNarrowMenu(run, 0, 0, { hub: "Synced 9:41", backlinks: 1, counts: { words: 1200, characters: 6400 } });
    expect(labels()).toEqual([
      "Open quick switcher",
      "Open today's daily note",
      "Insert template",
      "Open command palette",
      "-",
      "Settings",
      "-",
      "Synced 9:41 (text)",
      "1 backlink",
      "1,200 words (text)",
      "6,400 characters (text)",
    ]);
    useOverlays.getState().menu?.onChoose(NARROW_BACKLINKS);
    expect(run).toHaveBeenCalledWith(NARROW_BACKLINKS);
  });

  it("leaves out what the window has none of", () => {
    openNarrowMenu(vi.fn(), 0, 0, { hub: null, backlinks: null, counts: null });
    expect(labels().at(-1)).toBe("Settings");
  });
});

describe("page previews", () => {
  it("do not show where the pointer is a finger", () => {
    vi.stubGlobal("matchMedia", (q: string) => ({ matches: q === "(pointer: coarse)", media: q }));
    try {
      openPreview({ anchor: { left: 0, top: 0, bottom: 0 }, path: "Plan.md", fill: () => {} });
      expect(useOverlays.getState().preview).toBeNull();
    } finally {
      vi.unstubAllGlobals();
    }
  });
});

describe("the browser's connection menu", () => {
  it("names who is signed in, and on which node", () => {
    openIdentityMenu({ login: "ana@example.com", displayName: "Ana", node: "phone" }, 100, 500);
    expect(labels()).toEqual(["Signed in as Ana (text)", "ana@example.com (text)", "on phone (text)"]);
  });

  it("names the login alone when lodge gives no display name or node", () => {
    openIdentityMenu({ login: "ana@example.com" }, 100, 500);
    expect(labels()).toEqual(["Signed in as ana@example.com (text)"]);
  });
});
