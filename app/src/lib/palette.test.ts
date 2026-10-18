import { describe, expect, it } from "vitest";

import { PALETTE_RECENT, type PaletteCommand, paletteResults } from "./palette";

const C = (id: string, name: string, keys: string[] = []): PaletteCommand => ({ id, name, keys });
const COMMANDS = [
  C("workspace:new-tab", "New tab", ["Ctrl + T"]),
  C("switcher:open", "Open quick switcher", ["Ctrl + O"]),
  C("app:open-settings", "Open settings", ["Ctrl + ,"]),
  C("workspace:close", "Close current tab", ["Ctrl + W"]),
  C("editor:toggle-bold", "Toggle bold", ["Ctrl + B"]),
];
const names = (q: string, recent: string[] = []) => paletteResults(COMMANDS, q, recent).map((r) => r.command.name);

describe("paletteResults", () => {
  it("lists every command by name for an empty or blank query", () => {
    for (const q of ["", "  "]) {
      expect(names(q)).toEqual(["Close current tab", "New tab", "Open quick switcher", "Open settings", "Toggle bold"]);
    }
    expect(paletteResults(COMMANDS, "", []).every((r) => r.match === null)).toBe(true);
  });

  it("leads with the commands run last, newest first, once each, skipping ones that do not apply", () => {
    expect(names("", ["editor:toggle-bold", "gone:command", "app:open-settings", "editor:toggle-bold"])).toEqual([
      "Toggle bold",
      "Open settings",
      "Close current tab",
      "New tab",
      "Open quick switcher",
    ]);
  });

  it(`leads with at most ${PALETTE_RECENT} recent commands`, () => {
    const many = Array.from({ length: 15 }, (_, i) => C(`c${i}`, `Command ${String(i).padStart(2, "0")}`));
    const recent = many.map((c) => c.id).reverse(); // c14 newest
    const r = paletteResults(many, "", recent).map((x) => x.command.id);
    expect(PALETTE_RECENT).toBe(10);
    expect(r.slice(0, 10)).toEqual(["c14", "c13", "c12", "c11", "c10", "c9", "c8", "c7", "c6", "c5"]);
    expect(r.slice(10)).toEqual(["c0", "c1", "c2", "c3", "c4"]);
  });

  it("matches names fuzzily, best first, with the matched spans", () => {
    const r = paletteResults(COMMANDS, "open", []);
    expect(r.map((x) => x.command.name)).toEqual(["Open settings", "Open quick switcher"]); // the shorter first
    expect(r[0]?.match?.matches).toEqual([[0, 4]]);
  });

  it("breaks equal scores by name", () => {
    const cmds = [C("b", "Tab b"), C("a", "Tab a")];
    expect(paletteResults(cmds, "tab", []).map((x) => x.command.id)).toEqual(["a", "b"]);
  });

  it("ranks a word's start before its inside", () => {
    expect(names("tab")).toEqual(["New tab", "Close current tab"]);
  });

  it("ignores the recent list once there is a query, and finds nothing for a stranger", () => {
    expect(names("bold", ["workspace:new-tab"])).toEqual(["Toggle bold"]);
    expect(names("zebra")).toEqual([]);
  });
});
