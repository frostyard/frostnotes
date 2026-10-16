import { describe, expect, it } from "vitest";

import { percentile, stats } from "./measure";

describe("stats", () => {
  it("uses nearest-rank percentiles", () => {
    const values = Array.from({ length: 200 }, (_, i) => i + 1);
    expect(percentile(values, 50)).toBe(100);
    expect(percentile(values, 95)).toBe(190);
    expect(percentile([7], 95)).toBe(7);
    expect(percentile([], 50)).toBeNaN();
  });

  it("summarizes unsorted samples", () => {
    expect(stats([3, 1, 2, 10])).toEqual({ n: 4, median: 2, p95: 10, max: 10, mean: 4 });
  });
});
