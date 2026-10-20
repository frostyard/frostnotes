import { readFileSync } from "node:fs";
import { resolve } from "node:path";
import { describe, expect, it } from "vitest";

import { lodgeAddress, noteRef, noteRoute, noteTitle, parseNoteRoute } from "./routes";

// The cases notesd's MCP tools share: their url is /notes plus each route
// (internal/mcpserver/url_test.go reads the same file).
const shared = JSON.parse(readFileSync(resolve("src/lodge/note-routes.json"), "utf8")) as {
  vaultId: number;
  path: string;
  route: string;
}[];

describe("notes' routes", () => {
  it("encodes each segment and reads it back", () => {
    const path = "Projects/Q3 plan #2?/Ünïcode & 100%.md";
    const route = noteRoute(1, path);
    expect(route).toBe("/1/Projects/Q3%20plan%20%232%3F/%C3%9Cn%C3%AFcode%20%26%20100%25.md");
    expect(parseNoteRoute(route)).toEqual({ vaultId: 1, path });
    expect(noteRef(2, "a b.md")).toBe("notes/2/a%20b.md");
  });

  it("routes the cases notesd's MCP tools share", () => {
    expect(shared.length).toBeGreaterThan(10);
    for (const c of shared) {
      expect(noteRoute(c.vaultId, c.path)).toBe(c.route);
      expect(parseNoteRoute(c.route)).toEqual({ vaultId: c.vaultId, path: c.path });
    }
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

  it("takes a rooted link as lodge's address, each segment encoded", () => {
    expect(lodgeAddress("/mail/message/abc@example.com")).toBe("/mail/message/abc%40example.com");
    expect(lodgeAddress("/notes/2/Daily/2026-10-20.md")).toBe("/notes/2/Daily/2026-10-20.md");
    expect(lodgeAddress("/notes/2/Q3 plan #2?.md")).toBe("/notes/2/Q3%20plan%20%232%3F.md");
    expect(lodgeAddress("/mail")).toBe("/mail");
  });

  it("refuses an address no lodge app can have", () => {
    for (const bad of [
      "/",
      "mail/message/1",
      "/Projects/Plan",
      "/my notes/Plan",
      "/app/notes/1/Plan.md",
      "/lodge",
      "/mcp/notes",
      "/device/notes",
      `/mail/${"x".repeat(2048)}`,
    ]) {
      expect(lodgeAddress(bad)).toBeNull();
    }
  });
});
