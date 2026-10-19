import { describe, expect, it } from "vitest";

import { noteRef, noteRoute, noteTitle, parseNoteRoute } from "./routes";

describe("notes' routes", () => {
  it("encodes each segment and reads it back", () => {
    const path = "Projects/Q3 plan #2?/Ünïcode & 100%.md";
    const route = noteRoute(1, path);
    expect(route).toBe("/1/Projects/Q3%20plan%20%232%3F/%C3%9Cn%C3%AFcode%20%26%20100%25.md");
    expect(parseNoteRoute(route)).toEqual({ vaultId: 1, path });
    expect(noteRef(2, "a b.md")).toBe("notes/2/a%20b.md");
  });

  it("names a vault alone", () => {
    expect(noteRoute(7, "")).toBe("/7");
    expect(parseNoteRoute("/7")).toEqual({ vaultId: 7, path: "" });
    expect(parseNoteRoute("/7/")).toEqual({ vaultId: 7, path: "" });
  });

  it("refuses what names no vault or no clean path", () => {
    for (const bad of [
      "/",
      "",
      "/0/a.md",
      "/01/a.md",
      "/x/a.md",
      "/1/a//b.md",
      "/1/../a.md",
      "/1/./a.md",
      "/1/%E0%A4%A.md",
    ]) {
      expect(parseNoteRoute(bad)).toBeNull();
    }
  });

  it("titles a note by its name", () => {
    expect(noteTitle("Projects/Plan.md")).toBe("Plan");
    expect(noteTitle("Assets/photo.png")).toBe("photo.png");
  });
});
