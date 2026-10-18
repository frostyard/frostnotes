// The window's one key listener (docs/design/app.md#commands-and-keys): in
// the bubble phase, so CodeMirror and fields handle their own keys first.
// It skips presses already handled, maps the rest with the keymap
// (Obsidian's default hotkeys), and runs the command when it applies. In a
// text field outside the editor, only presses with Ctrl or Alt, and F1 and
// F2, reach it.
import { commandFor } from "../lib/keymap";
import type { CommandRegistry } from "./registry";

/** inTextField reports whether an element takes typed text. */
export function inTextField(el: Element | null): boolean {
  if (!el) return false;
  if (el instanceof HTMLTextAreaElement || el instanceof HTMLSelectElement) return true;
  if (el instanceof HTMLInputElement) {
    return !["button", "checkbox", "radio", "range", "color", "file", "submit", "reset", "image"].includes(el.type);
  }
  return el.closest("[contenteditable]:not([contenteditable='false'])") !== null;
}

/** inEditor reports whether an element is in a CodeMirror editor. */
export function inEditor(el: Element | null): boolean {
  return el?.closest(".cm-editor") != null;
}

/**
 * keyOf is a press's key as the keymap names it. WebKitGTK names Shift+Tab,
 * which X sends as ISO_Left_Tab, "Unidentified"; its code still says Tab
 * (found by M3's exit evidence 2, docs/plans/0005-m3-desktop-app.md).
 */
export function keyOf(e: Pick<KeyboardEvent, "key" | "code">): string {
  return e.key === "Unidentified" && e.code === "Tab" ? "Tab" : e.key;
}

/** keyHandler returns the listener that runs the registry's commands for key presses. */
export function keyHandler(registry: CommandRegistry): (e: KeyboardEvent) => void {
  return (e) => {
    if (e.defaultPrevented || e.isComposing) return;
    const target = e.target instanceof Element ? e.target : null;
    if (inTextField(target) && !inEditor(target) && !e.ctrlKey && !e.altKey && e.key !== "F1" && e.key !== "F2") {
      return;
    }
    const id = commandFor({ key: keyOf(e), ctrl: e.ctrlKey, alt: e.altKey, shift: e.shiftKey, meta: e.metaKey });
    if (id !== null && registry.run(id)) e.preventDefault();
  };
}

/** installKeyListener listens on the window; it returns the removal. */
export function installKeyListener(registry: CommandRegistry, target: Window = window): () => void {
  const handler = keyHandler(registry);
  target.addEventListener("keydown", handler);
  return () => target.removeEventListener("keydown", handler);
}
