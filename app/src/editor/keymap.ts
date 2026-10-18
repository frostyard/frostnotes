// The editor's keys (docs/design/app.md#extensions): CodeMirror's default,
// history and search keymaps without the bindings Obsidian removes (Alt+↑
// and Alt+↓ with and without Shift, Alt+L, Alt+V, Ctrl+M) and without any
// key the app's keymap binds, which are commands (Ctrl+D, Ctrl+I,
// Ctrl+Enter, Ctrl+/, Ctrl+F … would otherwise run here first); then Tab
// and Shift+Tab indent.
import { defaultKeymap, historyKeymap, indentWithTab } from "@codemirror/commands";
import { searchKeymap } from "@codemirror/search";
import type { KeyBinding } from "@codemirror/view";

import { commandFor, type KeyInput } from "../lib/keymap";

/** OBSIDIAN_REMOVES are the default bindings Obsidian takes out, as CodeMirror names keys. */
const OBSIDIAN_REMOVES = new Set([
  "Alt-ArrowUp",
  "Alt-ArrowDown",
  "Shift-Alt-ArrowUp",
  "Shift-Alt-ArrowDown",
  "Alt-l",
  "Alt-v",
  "Ctrl-m",
]);

/** keyInput reads a CodeMirror key name ("Mod-Shift-z") as a press on Linux, where Mod is Ctrl. */
export function keyInput(name: string): KeyInput {
  const parts = name.split(/-(?!$)/);
  const key = parts.pop() ?? "";
  const has = (...m: string[]) => parts.some((p) => m.includes(p));
  return {
    key: key === "Space" ? " " : key,
    ctrl: has("Mod", "Ctrl", "Control", "c"),
    alt: has("Alt", "a"),
    shift: has("Shift", "s"),
    meta: has("Meta", "Cmd", "m"),
  };
}

// canonical writes a key name with its modifiers in one order, to compare names.
function canonical(name: string): string {
  const k = keyInput(name);
  return [k.ctrl && "Ctrl", k.alt && "Alt", k.shift && "Shift", k.meta && "Meta", k.key].filter(Boolean).join("-");
}

const REMOVED = new Set([...OBSIDIAN_REMOVES].map(canonical));

/** kept reports whether a binding stays in the editor's keymap. */
export function kept(binding: KeyBinding): boolean {
  const name = binding.linux ?? binding.key;
  if (!name) return true;
  if (REMOVED.has(canonical(name))) return false;
  const press = keyInput(name);
  if (commandFor(press) !== null) return false;
  // A binding's shift variant answers the same key with Shift.
  return !(binding.shift && commandFor({ ...press, shift: true }) !== null);
}

/** editorKeymap is the editor's keymap. */
export function editorKeymap(): KeyBinding[] {
  return [...defaultKeymap, ...historyKeymap, ...searchKeymap].filter(kept).concat(indentWithTab);
}
