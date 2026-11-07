// The height map visits only outdated or forced children (docs/adr/0025-patch-what-costs-the-editor-its-frames.md).
// The test fails when the patch is dropped before CodeMirror walks only what changed.
import { readFileSync } from "node:fs";
import { resolve } from "node:path";

import { describe, expect, it } from "vitest";

const leftString = "else if (force || left.outdated)";
const rightString = "else if (force || right.outdated)";

describe("the CodeMirror height map patch", () => {
  for (const file of ["dist/index.js", "dist/index.cjs"]) {
    it(`is applied to ${file}`, () => {
      const source = readFileSync(resolve("node_modules/@codemirror/view", file), "utf8");
      expect(source).toContain(leftString);
      expect(source).toContain(rightString);
    });
  }
});
