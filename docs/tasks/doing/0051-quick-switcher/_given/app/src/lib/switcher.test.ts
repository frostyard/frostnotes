import { describe, expect, it } from "vitest";

import { SWITCHER_LIMIT, type SwitcherNote, type SwitcherResult, stripMd, switcherResults } from "./switcher";

const NOTES: SwitcherNote[] = [
  { path: "Inbox.md", aliases: [] },
  { path: "Projects/Garden plan.md", aliases: ["Allotment", "Veg plot"] },
  { path: "Plans/Weekly.md", aliases: [] },
  { path: "Explanation.md", aliases: [] },
];

const rows = (r: SwitcherResult[]) => r.map((x) => (x.kind === "alias" ? `${x.alias} → ${x.path}` : x.path));

describe("stripMd", () => {
  it("drops a note's extension in any case, and nothing else", () => {
    expect(stripMd("Projects/Garden plan.md")).toBe("Projects/Garden plan");
    expect(stripMd("Old.MD")).toBe("Old");
    expect(stripMd("Board.canvas")).toBe("Board.canvas");
  });
});

describe("switcherResults", () => {
  it("lists the recent notes that exist, newest first and once each, for an empty or blank query", () => {
    const recent = ["Plans/Weekly.md", "Gone.md", "Inbox.md", "Plans/Weekly.md"];
    for (const query of ["", "   "]) {
      const r = switcherResults(NOTES, query, recent);
      expect(r).toEqual([
        { kind: "note", path: "Plans/Weekly.md", match: null },
        { kind: "note", path: "Inbox.md", match: null },
      ]);
    }
  });

  it("ranks names before words inside them, and whole paths last", () => {
    expect(rows(switcherResults(NOTES, "plan", []))).toEqual([
      "Projects/Garden plan.md",
      "Explanation.md",
      "Plans/Weekly.md",
    ]);
  });

  it("gives each note's spans in its path without .md", () => {
    const [first, , last] = switcherResults(NOTES, "plan", []);
    expect(first?.match?.matches).toEqual([[16, 20]]);
    expect(last?.match?.matches).toEqual([[0, 4]]);
    expect(last?.match?.score).toBeLessThan(-1);
  });

  it("matches aliases as rows of their own, with spans in the alias", () => {
    const r = switcherResults(NOTES, "allot", []);
    expect(r).toHaveLength(1);
    expect(r[0]).toMatchObject({ kind: "alias", path: "Projects/Garden plan.md", alias: "Allotment" });
    expect(r[0]?.match?.matches).toEqual([[0, 5]]);
  });

  it("lists a note and its alias when both match, the better first", () => {
    expect(rows(switcherResults(NOTES, "plot", []))).toEqual(["Veg plot → Projects/Garden plan.md"]);
    const notes: SwitcherNote[] = [{ path: "Kitchen/Vegetables.md", aliases: ["Veg"] }];
    expect(rows(switcherResults(notes, "veg", []))).toEqual(["Veg → Kitchen/Vegetables.md", "Kitchen/Vegetables.md"]);
  });

  it("keeps the notes' order between equal scores", () => {
    const notes: SwitcherNote[] = [
      { path: "b/Note.md", aliases: [] },
      { path: "a/Note.md", aliases: [] },
    ];
    expect(rows(switcherResults(notes, "note", []))).toEqual(["b/Note.md", "a/Note.md"]);
  });

  it(`lists at most ${SWITCHER_LIMIT}`, () => {
    const many = Array.from({ length: 60 }, (_, i) => ({ path: `Note ${String(i).padStart(2, "0")}.md`, aliases: [] }));
    expect(SWITCHER_LIMIT).toBe(50);
    expect(switcherResults(many, "note", [])).toHaveLength(50);
    const recent = many.map((n) => n.path);
    expect(switcherResults(many, "", recent)).toHaveLength(50);
  });

  it("matches nothing for a query no note has", () => {
    expect(switcherResults(NOTES, "zebra", [])).toEqual([]);
  });
});
