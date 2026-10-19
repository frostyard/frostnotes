// The app's copy of Tailwind's preflight (preflight.css): Tailwind's own
// but for its reset's pseudo-elements, which leave out the editor's lines
// and gutter (docs/adr/0025-patch-what-costs-the-editor-its-frames.md).
import { readFileSync } from "node:fs";
import { resolve } from "node:path";

import { describe, expect, it } from "vitest";

const SCOPE = ":where(:not(.cm-line, .cm-line *, .cm-gutterElement, .cm-gutterElement *))";

// plain is CSS without comments, its whitespace collapsed and its quotes
// double, as the formatter writes them.
const plain = (css: string) =>
  css
    .replace(/\/\*[\s\S]*?\*\//g, "")
    .replace(/\s+/g, " ")
    .replaceAll("'", '"')
    .trim();

describe("the preflight", () => {
  const ours = readFileSync(resolve("src/styles/preflight.css"), "utf8");
  const tailwind = readFileSync(resolve("node_modules/tailwindcss/preflight.css"), "utf8");

  it("is Tailwind's but for the reset's pseudo-elements", () => {
    const back = plain(ours).replaceAll(`${SCOPE}::after`, "::after").replaceAll(`${SCOPE}::before`, "::before");
    expect(back).toBe(plain(tailwind));
  });

  it("leaves the editor's lines out of the reset's pseudo-elements", () => {
    expect(plain(ours)).toContain(`*, ${SCOPE}::after, ${SCOPE}::before, ::backdrop, ::file-selector-button {`);
  });
});
