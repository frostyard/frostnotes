// Obsidian's default hotkeys (docs/specs/ui.md, Keyboard map): the window's
// key listener asks commandFor which command a key press runs, and menus
// and the command palette show hotkeysFor through formatHotkey. Task T-0038
// implements the three functions; until then they bind nothing.

/** HotkeyCommand is a command the keyboard map binds, by Obsidian's command ID. */
export type HotkeyCommand =
  | "switcher:open"
  | "command-palette:open"
  | "file-explorer:new-file"
  | "file-explorer:new-file-in-new-pane"
  | "workspace:new-tab"
  | "workspace:close"
  | "workspace:close-window"
  | "workspace:undo-close-pane"
  | "workspace:next-tab"
  | "workspace:previous-tab"
  | "workspace:goto-tab-1"
  | "workspace:goto-tab-2"
  | "workspace:goto-tab-3"
  | "workspace:goto-tab-4"
  | "workspace:goto-tab-5"
  | "workspace:goto-tab-6"
  | "workspace:goto-tab-7"
  | "workspace:goto-tab-8"
  | "workspace:goto-last-tab"
  | "app:go-back"
  | "app:go-forward"
  | "app:open-settings"
  | "app:open-help"
  | "workspace:edit-file-title"
  | "markdown:toggle-preview"
  | "markdown:add-metadata-property"
  | "global-search:open"
  | "editor:open-search"
  | "editor:open-search-replace"
  | "editor:save-file"
  | "editor:toggle-bold"
  | "editor:toggle-italics"
  | "editor:insert-link"
  | "editor:toggle-checklist-status"
  | "editor:toggle-comments"
  | "editor:delete-paragraph"
  | "editor:follow-link"
  | "editor:open-link-in-new-leaf"
  | "editor:open-link-in-new-split"
  | "editor:open-link-in-new-window"
  | "graph:open"
  | "window:zoom-in"
  | "window:zoom-out"
  | "window:reset-zoom";

/** Modifier is a modifier key a hotkey holds; Ctrl is Obsidian's Mod on Linux. */
export type Modifier = "Ctrl" | "Alt" | "Shift";

/** Hotkey is a key and the modifiers held with it. */
export interface Hotkey {
  /** The modifiers held, in the order Ctrl, Alt, Shift. */
  modifiers: Modifier[];
  /**
   * KeyboardEvent.key of the press without its modifiers' effect: a letter
   * in upper case, a digit, a symbol, or a key's name ("Enter", "Tab",
   * "PageDown", "ArrowLeft", "F2").
   */
  key: string;
}

/** KeyInput is a key press: KeyboardEvent's key and modifier flags. */
export interface KeyInput {
  key: string;
  ctrl: boolean;
  alt: boolean;
  shift: boolean;
  meta: boolean;
}

/** commandFor returns the command the keyboard map binds to a key press, or null. */
export function commandFor(_input: KeyInput): HotkeyCommand | null {
  return null;
}

/** hotkeysFor returns a command's hotkeys, in the keyboard map's order. */
export function hotkeysFor(_command: HotkeyCommand): Hotkey[] {
  return [];
}

/** formatHotkey writes a hotkey as Obsidian shows it, such as "Ctrl + Shift + F". */
export function formatHotkey(_hotkey: Hotkey): string {
  return "";
}
