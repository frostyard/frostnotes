import { describe, expect, it } from "vitest";

import { createPrefs, DEFAULT_PREFS, PREFS_KEY, parsePrefs } from "./prefs";
import { memoryStore } from "./storage";

describe("preferences", () => {
  it("default to Obsidian's", () => {
    expect(parsePrefs(null)).toEqual(DEFAULT_PREFS);
    expect(parsePrefs("not json")).toEqual(DEFAULT_PREFS);
    expect(parsePrefs("[1,2]")).toEqual(DEFAULT_PREFS);
  });

  it("keep what they can take and drop the rest", () => {
    const p = parsePrefs(
      JSON.stringify({ fontSize: 20, theme: "dark", tabSize: 99, spellcheck: "yes", lastVault: "/v", unknown: 1 }),
    );
    expect(p).toEqual({ ...DEFAULT_PREFS, fontSize: 20, theme: "dark", lastVault: "/v" });
  });

  it("are kept in storage as they change", () => {
    const storage = memoryStore();
    const prefs = createPrefs(storage);
    prefs.getState().setPrefs({ readableLineLength: false, fontSize: 31 });
    expect(prefs.getState().readableLineLength).toBe(false);
    expect(prefs.getState().fontSize).toBe(16);
    expect(parsePrefs(storage.get(PREFS_KEY))).toEqual({ ...DEFAULT_PREFS, readableLineLength: false });
    expect(createPrefs(storage).getState().readableLineLength).toBe(false);
  });
});
