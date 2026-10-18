import { describe, expect, it } from "vitest";

import { headingsOfText } from "./outline";

describe("headingsOf", () => {
  it("lists ATX and setext headings with their text and line starts, not code or frontmatter", () => {
    const text = [
      "---",
      "title: # not a heading",
      "---",
      "# Orchard plan #",
      "",
      "Goals",
      "-----",
      "",
      "```",
      "# not either",
      "```",
      "   ### Later   ",
      "#nope is a tag",
    ].join("\n");
    const lines = text.split("\n");
    const start = (i: number) => lines.slice(0, i).join("\n").length + (i > 0 ? 1 : 0);
    expect(headingsOfText(text)).toEqual([
      { level: 1, text: "Orchard plan", from: start(3) },
      { level: 2, text: "Goals", from: start(5) },
      { level: 3, text: "Later", from: start(11) },
    ]);
  });

  it("finds none in a note without headings", () => {
    expect(headingsOfText("Just text.\n")).toEqual([]);
  });
});
