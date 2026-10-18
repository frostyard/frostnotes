// The app's preferences (ADR-0021; docs/specs/ui.md#settings-batch-2): one
// versioned record in local storage per device, whose defaults are
// Obsidian 1.14.4's. A stored value that is not one a preference takes is
// dropped for its default, so an older or newer record never breaks the
// window.
import { create } from "zustand";

import { type KeyValue, localStore } from "./storage";

/** Prefs are the app's preferences. */
export interface Prefs {
  defaultView: "edit" | "read";
  editingMode: "live" | "source";
  inlineTitle: boolean;
  viewHeader: boolean;
  readableLineLength: boolean;
  spellcheck: boolean;
  properties: "visible" | "hidden" | "source";
  lineNumbers: boolean;
  indentGuides: boolean;
  foldHeadings: boolean;
  foldIndent: boolean;
  autoPairBrackets: boolean;
  autoPairMarkdown: boolean;
  smartLists: boolean;
  indentWithTabs: boolean;
  tabSize: number;
  confirmDelete: boolean;
  loadRemoteImages: boolean;
  showUnsupported: boolean;
  theme: "system" | "light" | "dark";
  fontSize: number;
  showRibbon: boolean;
  /** The path of the vault the window showed last; null before one. */
  lastVault: string | null;
}

/** DEFAULT_PREFS are Obsidian's defaults. */
export const DEFAULT_PREFS: Prefs = {
  defaultView: "edit",
  editingMode: "live",
  inlineTitle: true,
  viewHeader: true,
  readableLineLength: true,
  spellcheck: true,
  properties: "visible",
  lineNumbers: false,
  indentGuides: true,
  foldHeadings: true,
  foldIndent: true,
  autoPairBrackets: true,
  autoPairMarkdown: true,
  smartLists: true,
  indentWithTabs: true,
  tabSize: 4,
  confirmDelete: true,
  loadRemoteImages: true,
  showUnsupported: false,
  theme: "system",
  fontSize: 16,
  showRibbon: true,
  lastVault: null,
};

/** PREFS_KEY is the local-storage key of the record. */
export const PREFS_KEY = "frostnotes.prefs.v1";

type Check = (v: unknown) => boolean;
const oneOf =
  (...values: string[]): Check =>
  (v) =>
    typeof v === "string" && values.includes(v);
const isBool: Check = (v) => typeof v === "boolean";
const intIn =
  (lo: number, hi: number): Check =>
  (v) =>
    Number.isInteger(v) && (v as number) >= lo && (v as number) <= hi;

const CHECKS: { [K in keyof Prefs]: Check } = {
  defaultView: oneOf("edit", "read"),
  editingMode: oneOf("live", "source"),
  inlineTitle: isBool,
  viewHeader: isBool,
  readableLineLength: isBool,
  spellcheck: isBool,
  properties: oneOf("visible", "hidden", "source"),
  lineNumbers: isBool,
  indentGuides: isBool,
  foldHeadings: isBool,
  foldIndent: isBool,
  autoPairBrackets: isBool,
  autoPairMarkdown: isBool,
  smartLists: isBool,
  indentWithTabs: isBool,
  tabSize: intIn(1, 8),
  confirmDelete: isBool,
  loadRemoteImages: isBool,
  showUnsupported: isBool,
  theme: oneOf("system", "light", "dark"),
  fontSize: intIn(10, 30),
  showRibbon: isBool,
  lastVault: (v) => v === null || typeof v === "string",
};

/** parsePrefs reads a stored record, each value it lacks or cannot take at its default. */
export function parsePrefs(json: string | null): Prefs {
  let stored: Record<string, unknown> = {};
  try {
    const v: unknown = json === null ? {} : JSON.parse(json);
    if (v && typeof v === "object" && !Array.isArray(v)) stored = v as Record<string, unknown>;
  } catch {
    // Not JSON: every default.
  }
  const out = { ...DEFAULT_PREFS } as Record<string, unknown>;
  for (const key of Object.keys(CHECKS) as (keyof Prefs)[]) {
    if (key in stored && CHECKS[key](stored[key])) out[key] = stored[key];
  }
  return out as unknown as Prefs;
}

/** PrefsState is the preferences and the setter that keeps them. */
export interface PrefsState extends Prefs {
  /** setPrefs changes preferences, ignoring values they cannot take, and keeps them. */
  setPrefs(change: Partial<Prefs>): void;
}

/** createPrefs makes a preferences store over storage. */
export function createPrefs(storage: KeyValue) {
  return create<PrefsState>((set, get) => ({
    ...parsePrefs(storage.get(PREFS_KEY)),
    setPrefs: (change) => {
      const valid = Object.fromEntries(
        Object.entries(change).filter(([k, v]) => k in CHECKS && CHECKS[k as keyof Prefs](v)),
      ) as Partial<Prefs>;
      set(valid);
      const { setPrefs: _, ...prefs } = get();
      storage.set(PREFS_KEY, JSON.stringify(prefs));
    },
  }));
}

/** usePrefs is the window's preferences. */
export const usePrefs = createPrefs(localStore());
