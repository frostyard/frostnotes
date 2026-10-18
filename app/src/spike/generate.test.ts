import { describe, expect, it } from "vitest";

import { generateNote } from "./generate";

describe("generateNote", () => {
  it("writes exactly the lines asked for, the same each time", () => {
    const note = generateNote(10_000);
    expect(note.split("\n")).toHaveLength(10_000);
    expect(generateNote(10_000)).toBe(note);
    expect(generateNote(10_000, 2)).not.toBe(note);
    expect(generateNote(50).split("\n")).toHaveLength(50);
  });

  it("covers what live preview draws", () => {
    const note = generateNote(10_000);
    for (const pattern of [
      /^# Section 1: /m,
      /^#{2,4} \w/m,
      /^- \[ \] /m,
      /^- \[x\] /m,
      /^ {2}- /m,
      /^1\. /m,
      /^> /m,
      /^```ts$/m,
      /\*\*\w[^*]*\*\*/,
      /(^|[^*])\*\w[^*]*\*/,
      /~~\w/,
      /\[\[[^\]|#]+\]\]/,
      /\[\[[^\]|]+\|[^\]]+\]\]/,
      /\[\[[^\]]+#\w+\]\]/,
      /\]\(https:\/\/example\.test\/\w+\)/,
      /==\w[^=]*==/,
      /(^| )#[a-z]+\b/m,
      /\$[^$\n]+\$/,
      /%%\w[^%]*%%/,
      /\w\[\^\d\]/,
      /^\[\^\d\]: /m,
      /^> \[!(note|tip|warning)\] /m,
      /^\| .+ \|$\n^\|( --- \|)+$/m,
      /^\$\$$/m,
      /^!\[\[[^\]]+\]\]$/m,
      /^---$/m,
    ]) {
      expect(note).toMatch(pattern);
    }
    // Code fences are balanced, so no fence swallows the rest of the note.
    expect((note.match(/^```/gm) ?? []).length % 2).toBe(0);
  });
});
