import { describe, expect, it } from "vitest";

import { type FuzzyMatch, matchPath, prepareFuzzySearch, sortByScore } from "./fuzzy";

function expectMatch(m: FuzzyMatch | null, score: number, matches: [number, number][]) {
  expect(m).not.toBeNull();
  expect(m?.score).toBeCloseTo(score, 10);
  expect(m?.matches).toEqual(matches);
}

const match = (query: string, text: string) => prepareFuzzySearch(query)(text);

describe("prepareFuzzySearch", () => {
  it("matches every text with an empty query, scoring 0", () => {
    expect(match("", "Garden plan")).toEqual({ score: 0, matches: [] });
    expect(match("", "")).toEqual({ score: 0, matches: [] });
  });

  it("matches nothing with a query of spaces", () => {
    expect(match("  ", "Garden plan")).toBeNull();
  });

  it("matches a word where it starts, ignoring case", () => {
    expectMatch(match("plan", "Garden plan"), -0.0181, [[7, 11]]);
    expectMatch(match("GARDEN", "garden plan"), -0.0111, [[0, 6]]);
    expectMatch(match("plan", "Plans/Weekly"), -0.0112, [[0, 4]]);
  });

  it("costs a tenth for a token found inside a word", () => {
    expectMatch(match("plan", "Explanation"), -0.1131, [[2, 6]]);
    // An upper case letter after another is inside a word (an acronym's middle)...
    expectMatch(match("df", "PDFs"), -0.1114, [[1, 3]]);
  });

  it("matches the query's words in order, each its own range unless they touch", () => {
    expectMatch(match("garden plan", "Garden plan"), -1.0111, [
      [0, 6],
      [7, 11],
    ]);
    // Punctuation is a token of its own, and tokens that touch share a range.
    expectMatch(match("a/b", "a/b.md"), -0.0106, [[0, 3]]);
    // Each CJK character is a token of its own.
    expectMatch(match("日記", "2026 日記"), -0.0157, [[5, 7]]);
    // Separate words may touch in the text, and then cost as inside one.
    expectMatch(match("g p", "Gp"), -0.1002, [[0, 2]]);
  });

  it("falls back to single characters when a token is missing", () => {
    expectMatch(match("gp", "Garden plan"), -1.0711, [
      [0, 1],
      [7, 8],
    ]);
    // ...but an upper case letter after a lower case one starts a word.
    expectMatch(match("dp", "DailyPlan"), -1.0509, [
      [0, 1],
      [5, 6],
    ]);
    // A character right after the previous one costs nothing; one further
    // into the same word does.
    expectMatch(match("pln", "plan"), -1.1204, [
      [0, 2],
      [3, 4],
    ]);
  });

  it("does not match out of order or what is missing", () => {
    expect(match("nalp", "plan")).toBeNull();
    expect(match("xyz", "Garden plan")).toBeNull();
    expect(match("plan", "")).toBeNull();
  });

  it("ranks a match at a word's start and earlier and in a shorter text higher", () => {
    const search = prepareFuzzySearch("plan");
    const ranked = sortByScore(
      ["Explanation", "Garden plan", "Plans/Weekly"].map((text) => {
        const m = search(text);
        if (!m) throw new Error(`no match for ${text}`);
        return { text, match: m };
      }),
    ).map((x) => x.text);
    expect(ranked).toEqual(["Plans/Weekly", "Garden plan", "Explanation"]);
  });
});

describe("matchPath", () => {
  it("matches the last segment first, moving the matches into the path", () => {
    expectMatch(matchPath(prepareFuzzySearch("plan"), "Projects/Garden plan"), -0.0181, [[16, 20]]);
    expectMatch(matchPath(prepareFuzzySearch("plan"), "Garden plan"), -0.0181, [[7, 11]]);
  });

  it("falls back to the whole path, for one point less", () => {
    expectMatch(matchPath(prepareFuzzySearch("plan"), "Plan/Notes"), -1.011, [[0, 4]]);
    expect(matchPath(prepareFuzzySearch("zzz"), "Plan/Notes")).toBeNull();
  });

  it("ranks a match in the name above one in a folder", () => {
    const search = prepareFuzzySearch("week");
    const a = matchPath(search, "Journal/Weekly review");
    const b = matchPath(search, "Weekly/Notes");
    expectMatch(a, -0.0113, [[8, 12]]);
    expectMatch(b, -1.0112, [[0, 4]]);
  });
});

describe("sortByScore", () => {
  it("orders best first, keeps equal scores in order, and leaves its input alone", () => {
    const m = (score: number): FuzzyMatch => ({ score, matches: [] });
    const items = [
      { id: "a", match: m(-1) },
      { id: "b", match: m(-0.5) },
      { id: "c", match: m(-1) },
      { id: "d", match: m(0) },
    ];
    expect(sortByScore(items).map((x) => x.id)).toEqual(["d", "b", "a", "c"]);
    expect(items.map((x) => x.id)).toEqual(["a", "b", "c", "d"]);
  });
});
