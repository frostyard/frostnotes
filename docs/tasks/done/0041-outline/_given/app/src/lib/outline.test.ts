import { describe, expect, it } from "vitest";

import { buildOutline, type Heading, headingAt, type OutlineNode, outlineRows } from "./outline";

let at = 0;
const h = (level: number, text: string): Heading => {
  at += 10;
  return { level, text, from: at };
};

/** shape writes an outline as nested [text, children] pairs. */
type Shape = [string, Shape[]];
const shape = (nodes: readonly OutlineNode[]): Shape[] => nodes.map((n) => [n.heading.text, shape(n.children)]);

describe("buildOutline", () => {
  it("nests each heading under the nearest heading before it of a lower level", () => {
    const headings = [h(1, "Plan"), h(2, "Beds"), h(3, "North"), h(3, "South"), h(2, "Seeds"), h(1, "Log")];
    expect(shape(buildOutline(headings))).toEqual([
      [
        "Plan",
        [
          [
            "Beds",
            [
              ["North", []],
              ["South", []],
            ],
          ],
          ["Seeds", []],
        ],
      ],
      ["Log", []],
    ]);
  });

  it("nests across skipped levels", () => {
    expect(shape(buildOutline([h(1, "A"), h(4, "B"), h(2, "C"), h(3, "D")]))).toEqual([
      [
        "A",
        [
          ["B", []],
          ["C", [["D", []]]],
        ],
      ],
    ]);
  });

  it("keeps headings at the top until one of a lower level comes", () => {
    expect(shape(buildOutline([h(3, "A"), h(2, "B"), h(3, "C"), h(1, "D"), h(2, "E")]))).toEqual([
      ["A", []],
      ["B", [["C", []]]],
      ["D", [["E", []]]],
    ]);
  });

  it("makes headings of one level siblings, and keeps the headings themselves", () => {
    const headings = [h(2, "A"), h(2, "B")];
    const outline = buildOutline(headings);
    expect(shape(outline)).toEqual([
      ["A", []],
      ["B", []],
    ]);
    expect(outline[1]?.heading).toBe(headings[1]);
    expect(buildOutline([])).toEqual([]);
  });
});

describe("outlineRows", () => {
  const headings = [h(1, "Plan"), h(2, "Beds"), h(3, "North"), h(2, "Seeds"), h(1, "Log")];
  const [plan, beds, north, seeds, log] = headings;
  const outline = buildOutline(headings);
  const brief = (folded: number[]) =>
    outlineRows(outline, new Set(folded)).map(
      (r) => `${r.depth} ${r.heading.text}${r.hasChildren ? (r.expanded ? " -" : " +") : ""}`,
    );

  it("lists every heading with its depth when nothing is folded", () => {
    expect(brief([])).toEqual(["0 Plan -", "1 Beds -", "2 North", "1 Seeds", "0 Log"]);
  });

  it("leaves out what a folded heading holds", () => {
    expect(brief([beds?.from ?? 0])).toEqual(["0 Plan -", "1 Beds +", "1 Seeds", "0 Log"]);
    expect(brief([plan?.from ?? 0])).toEqual(["0 Plan +", "0 Log"]);
  });

  it("ignores folds of headings without children", () => {
    expect(brief([north?.from ?? 0, seeds?.from ?? 0, log?.from ?? 0])).toEqual(brief([]));
  });

  it("describes each row", () => {
    expect(outlineRows(outline, new Set())[1]).toEqual({ heading: beds, depth: 1, hasChildren: true, expanded: true });
    expect(outlineRows(outline, new Set())[2]).toEqual({
      heading: north,
      depth: 2,
      hasChildren: false,
      expanded: false,
    });
  });
});

describe("headingAt", () => {
  const headings: Heading[] = [
    { level: 1, text: "A", from: 0 },
    { level: 2, text: "B", from: 40 },
    { level: 2, text: "C", from: 90 },
  ];

  it("finds the last heading at or before a position", () => {
    expect(headingAt(headings, 0)).toBe(0);
    expect(headingAt(headings, 39)).toBe(0);
    expect(headingAt(headings, 40)).toBe(1);
    expect(headingAt(headings, 89)).toBe(1);
    expect(headingAt(headings, 90)).toBe(2);
    expect(headingAt(headings, 10_000)).toBe(2);
  });

  it("is -1 before the first heading and without headings", () => {
    expect(headingAt([{ level: 1, text: "A", from: 12 }], 11)).toBe(-1);
    expect(headingAt([], 0)).toBe(-1);
  });

  it("finds its way through many headings", () => {
    const many: Heading[] = Array.from({ length: 5000 }, (_, i) => ({ level: 2, text: `H${i}`, from: i * 100 }));
    expect(headingAt(many, 123_456)).toBe(1234);
    expect(headingAt(many, 499_900)).toBe(4999);
  });
});
