// Link text by the vault's New link format, for completion.
import { describe, expect, it } from "vitest";

import { linkTextFor } from "./catalog";

const PATHS = ["Plan.md", "Orchard/Plan.md", "Orchard/North Block/Soil.md", "Recipes/Apple Butter.md", "media/map.png"];

describe("link text", () => {
  it("is the file's name when no other file has it, else its path, by the shortest format", () => {
    expect(linkTextFor("Orchard/North Block/Soil.md", "Notes.md", PATHS, "shortest")).toBe("Soil");
    expect(linkTextFor("Orchard/Plan.md", "Notes.md", PATHS, "shortest")).toBe("Orchard/Plan");
    expect(linkTextFor("Plan.md", "Notes.md", PATHS, "shortest")).toBe("Plan");
    expect(linkTextFor("media/map.png", "Notes.md", PATHS, "shortest")).toBe("map.png");
  });

  it("is the path by the absolute format", () => {
    expect(linkTextFor("Orchard/North Block/Soil.md", "Notes.md", PATHS, "absolute")).toBe("Orchard/North Block/Soil");
  });

  it("is the path from the note's folder by the relative format", () => {
    expect(linkTextFor("Orchard/North Block/Soil.md", "Orchard/Plan.md", PATHS, "relative")).toBe("North Block/Soil");
    expect(linkTextFor("media/map.png", "Orchard/North Block/Soil.md", PATHS, "relative")).toBe("../../media/map.png");
    expect(linkTextFor("Plan.md", "Plan.md", PATHS, "relative")).toBe("Plan");
  });
});
