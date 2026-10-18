// T-0076's given tests: a note's properties read, set, removed and renamed
// in its frontmatter's text, every other byte kept. The cases are MCP's
// (internal/mcpserver/edit_test.go), with the app writing a string that
// needs quotes in double quotes, and a date plain, as Obsidian does.
import { describe, expect, it } from "vitest";

import { propertyType, readFrontmatter, renameProperty, setProperties } from "./properties";

const NOTE =
  "---\n" +
  "title: Plan # the name\n" +
  "status: draft\n" +
  "tags:\n" +
  "  - a\n" +
  "  - b\n" +
  "\n" +
  "# owners follow\n" +
  '"due date": 2026-10-01\n' +
  "other: 1\n" +
  "---\n" +
  "body\n";

describe("setProperties", () => {
  it("replaces a key's lines in place", () => {
    expect(setProperties(NOTE, { status: "done", tags: ["x"] })).toBe(
      NOTE.replace("status: draft\n", "status: done\n").replace("tags:\n  - a\n  - b\n", "tags:\n  - x\n"),
    );
  });

  it("removes a key set to null, and nothing when it is missing", () => {
    expect(setProperties(NOTE, { other: null, tags: null })).toBe(
      NOTE.replace("other: 1\n", "").replace("tags:\n  - a\n  - b\n", ""),
    );
    expect(setProperties(NOTE, { missing: null })).toBe(NOTE);
  });

  it("finds a quoted key by its name, and writes a date plain", () => {
    expect(setProperties(NOTE, { "due date": "2026-11-01" })).toBe(
      NOTE.replace('"due date": 2026-10-01\n', "due date: 2026-11-01\n"),
    );
  });

  it("adds new keys at the end, in order of their names, quoting a link", () => {
    expect(setProperties(NOTE, { up: "[[Plan]]", count: 3 })).toBe(
      NOTE.replace("other: 1\n---\n", 'other: 1\ncount: 3\nup: "[[Plan]]"\n---\n'),
    );
  });

  it("makes frontmatter where there is none, or fills an empty one", () => {
    expect(setProperties("# Plan\n", { status: "done" })).toBe("---\nstatus: done\n---\n# Plan\n");
    expect(setProperties("# Plan\n", { status: null })).toBe("# Plan\n");
    expect(setProperties("---\n---\nbody", { a: true })).toBe("---\na: true\n---\nbody");
  });

  it("keeps CRLF line endings", () => {
    expect(setProperties("---\r\ntitle: t\r\n---\r\nbody\r\n", { title: "u", new: "v" })).toBe(
      "---\r\ntitle: u\r\nnew: v\r\n---\r\nbody\r\n",
    );
  });

  it("writes mappings and lists with two-space indentation", () => {
    expect(setProperties("---\na: 1\n---\n", { meta: { by: "wren" }, list: ["x", "y"] })).toBe(
      "---\na: 1\nlist:\n  - x\n  - y\nmeta:\n  by: wren\n---\n",
    );
  });

  it("puts new frontmatter after a byte-order mark", () => {
    expect(setProperties("\ufeff# Plan\n", { a: "b" })).toBe("\ufeff---\na: b\n---\n# Plan\n");
    expect(setProperties("\ufeff---\na: 1\n---\n", { a: 2 })).toBe("\ufeff---\na: 2\n---\n");
  });

  it("refuses frontmatter that is not a block mapping", () => {
    for (const note of [
      "---\n- a\n- b\n---\nbody",
      "---\na: [\n---\nbody",
      "---\na: 1\na: 2\n---\nbody",
      "---\n{a: 1, b: 2}\n---\nbody",
    ]) {
      expect(setProperties(note, { k: "v" })).toBeInstanceOf(Error);
    }
  });
});

describe("readFrontmatter", () => {
  it("reads the properties in order, with their values", () => {
    expect(readFrontmatter(NOTE)).toEqual({
      kind: "properties",
      properties: [
        { key: "title", value: "Plan" },
        { key: "status", value: "draft" },
        { key: "tags", value: ["a", "b"] },
        { key: "due date", value: "2026-10-01" },
        { key: "other", value: 1 },
      ],
    });
  });

  it("says when there is none, when it is empty and when it is not a mapping", () => {
    expect(readFrontmatter("# Plan\n")).toEqual({ kind: "none" });
    expect(readFrontmatter("---\n---\nbody")).toEqual({ kind: "properties", properties: [] });
    expect(readFrontmatter("---\n- a\n---\n").kind).toBe("error");
    expect(readFrontmatter("---\na: [\n---\n").kind).toBe("error");
  });
});

describe("renameProperty", () => {
  it("renames a key in its place, keeping its value", () => {
    expect(renameProperty(NOTE, "status", "state")).toBe(NOTE.replace("status: draft\n", "state: draft\n"));
    expect(renameProperty(NOTE, "tags", "labels")).toBe(NOTE.replace("tags:\n", "labels:\n"));
    expect(renameProperty("---\r\na: 1\r\nb: 2\r\n---\r\n", "a", "c")).toBe("---\r\nc: 1\r\nb: 2\r\n---\r\n");
  });

  it("refuses a missing key, a name taken, and leaves a same name alone", () => {
    expect(renameProperty(NOTE, "missing", "x")).toBeInstanceOf(Error);
    expect(renameProperty(NOTE, "status", "other")).toBeInstanceOf(Error);
    expect(renameProperty(NOTE, "status", "status")).toBe(NOTE);
  });
});

describe("propertyType", () => {
  it("takes the vault's type when it names one", () => {
    expect(propertyType("due", "2026-10-01", "text")).toBe("text");
    expect(propertyType("x", "1", "number")).toBe("number");
    expect(propertyType("x", "a", "unknown")).toBe("text");
  });

  it("knows tags and aliases by their keys, in any case", () => {
    expect(propertyType("tags", ["a"])).toBe("tags");
    expect(propertyType("Aliases", "a")).toBe("aliases");
  });

  it("reads other types from the value", () => {
    expect(propertyType("l", ["a", "b"])).toBe("multitext");
    expect(propertyType("c", true)).toBe("checkbox");
    expect(propertyType("n", 3.5)).toBe("number");
    expect(propertyType("d", "2026-10-18")).toBe("date");
    expect(propertyType("t", "2026-10-18T09:41")).toBe("datetime");
    expect(propertyType("s", "hello")).toBe("text");
    expect(propertyType("e", null)).toBe("text");
  });
});
