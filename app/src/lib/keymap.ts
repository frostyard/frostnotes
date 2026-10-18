// Obsidian's default hotkeys (docs/specs/ui.md, Keyboard map): the window's
// key listener asks commandFor which command a key press runs, and menus
// and the command palette show hotkeysFor through formatHotkey. The map
// preserves Obsidian's Linux binding order and requires exact modifiers.

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

const KEYBOARD_MAP: readonly { command: HotkeyCommand; keys: Hotkey[] }[] = [
  { command: "switcher:open", keys: [{ modifiers: ["Ctrl"], key: "O" }] },
  { command: "command-palette:open", keys: [{ modifiers: ["Ctrl"], key: "P" }] },
  { command: "file-explorer:new-file", keys: [{ modifiers: ["Ctrl"], key: "N" }] },
  { command: "file-explorer:new-file-in-new-pane", keys: [{ modifiers: ["Ctrl", "Shift"], key: "N" }] },
  { command: "workspace:new-tab", keys: [{ modifiers: ["Ctrl"], key: "T" }] },
  { command: "workspace:close", keys: [{ modifiers: ["Ctrl"], key: "W" }] },
  { command: "workspace:close-window", keys: [{ modifiers: ["Ctrl", "Shift"], key: "W" }] },
  { command: "workspace:undo-close-pane", keys: [{ modifiers: ["Ctrl", "Shift"], key: "T" }] },
  {
    command: "workspace:next-tab",
    keys: [
      { modifiers: ["Ctrl"], key: "Tab" },
      { modifiers: ["Ctrl"], key: "PageDown" },
    ],
  },
  {
    command: "workspace:previous-tab",
    keys: [
      { modifiers: ["Ctrl", "Shift"], key: "Tab" },
      { modifiers: ["Ctrl"], key: "PageUp" },
    ],
  },
  { command: "workspace:goto-tab-1", keys: [{ modifiers: ["Ctrl"], key: "1" }] },
  { command: "workspace:goto-tab-2", keys: [{ modifiers: ["Ctrl"], key: "2" }] },
  { command: "workspace:goto-tab-3", keys: [{ modifiers: ["Ctrl"], key: "3" }] },
  { command: "workspace:goto-tab-4", keys: [{ modifiers: ["Ctrl"], key: "4" }] },
  { command: "workspace:goto-tab-5", keys: [{ modifiers: ["Ctrl"], key: "5" }] },
  { command: "workspace:goto-tab-6", keys: [{ modifiers: ["Ctrl"], key: "6" }] },
  { command: "workspace:goto-tab-7", keys: [{ modifiers: ["Ctrl"], key: "7" }] },
  { command: "workspace:goto-tab-8", keys: [{ modifiers: ["Ctrl"], key: "8" }] },
  { command: "workspace:goto-last-tab", keys: [{ modifiers: ["Ctrl"], key: "9" }] },
  { command: "app:go-back", keys: [{ modifiers: ["Ctrl", "Alt"], key: "ArrowLeft" }] },
  { command: "app:go-forward", keys: [{ modifiers: ["Ctrl", "Alt"], key: "ArrowRight" }] },
  { command: "app:open-settings", keys: [{ modifiers: ["Ctrl"], key: "," }] },
  { command: "app:open-help", keys: [{ modifiers: [], key: "F1" }] },
  { command: "workspace:edit-file-title", keys: [{ modifiers: [], key: "F2" }] },
  { command: "markdown:toggle-preview", keys: [{ modifiers: ["Ctrl"], key: "E" }] },
  { command: "markdown:add-metadata-property", keys: [{ modifiers: ["Ctrl"], key: ";" }] },
  { command: "global-search:open", keys: [{ modifiers: ["Ctrl", "Shift"], key: "F" }] },
  { command: "editor:open-search", keys: [{ modifiers: ["Ctrl"], key: "F" }] },
  { command: "editor:open-search-replace", keys: [{ modifiers: ["Ctrl"], key: "H" }] },
  { command: "editor:save-file", keys: [{ modifiers: ["Ctrl"], key: "S" }] },
  { command: "editor:toggle-bold", keys: [{ modifiers: ["Ctrl"], key: "B" }] },
  { command: "editor:toggle-italics", keys: [{ modifiers: ["Ctrl"], key: "I" }] },
  { command: "editor:insert-link", keys: [{ modifiers: ["Ctrl"], key: "K" }] },
  { command: "editor:toggle-checklist-status", keys: [{ modifiers: ["Ctrl"], key: "L" }] },
  { command: "editor:toggle-comments", keys: [{ modifiers: ["Ctrl"], key: "/" }] },
  { command: "editor:delete-paragraph", keys: [{ modifiers: ["Ctrl"], key: "D" }] },
  { command: "editor:follow-link", keys: [{ modifiers: ["Alt"], key: "Enter" }] },
  { command: "editor:open-link-in-new-leaf", keys: [{ modifiers: ["Ctrl"], key: "Enter" }] },
  { command: "editor:open-link-in-new-split", keys: [{ modifiers: ["Ctrl", "Alt"], key: "Enter" }] },
  { command: "editor:open-link-in-new-window", keys: [{ modifiers: ["Ctrl", "Alt", "Shift"], key: "Enter" }] },
  { command: "graph:open", keys: [{ modifiers: ["Ctrl"], key: "G" }] },
  { command: "window:zoom-in", keys: [{ modifiers: ["Ctrl"], key: "=" }] },
  { command: "window:zoom-out", keys: [{ modifiers: ["Ctrl"], key: "-" }] },
  { command: "window:reset-zoom", keys: [{ modifiers: ["Ctrl"], key: "0" }] },
];

/** commandFor returns the command the keyboard map binds to a key press, or null. */
export function commandFor(input: KeyInput): HotkeyCommand | null {
  if (input.meta) return null;
  const key = /^[a-zA-Z]$/.test(input.key) ? input.key.toUpperCase() : input.key;
  return (
    KEYBOARD_MAP.find(({ keys }) =>
      keys.some(
        (hotkey) =>
          hotkey.key === key &&
          input.ctrl === hotkey.modifiers.includes("Ctrl") &&
          input.alt === hotkey.modifiers.includes("Alt") &&
          input.shift === hotkey.modifiers.includes("Shift"),
      ),
    )?.command ?? null
  );
}

/** hotkeysFor returns a command's hotkeys, in the keyboard map's order. */
export function hotkeysFor(command: HotkeyCommand): Hotkey[] {
  const keys = KEYBOARD_MAP.find((entry) => entry.command === command)?.keys ?? [];
  return keys.map(({ modifiers, key }) => ({ modifiers: [...modifiers], key }));
}

/** formatHotkey writes a hotkey as Obsidian shows it, such as "Ctrl + Shift + F". */
export function formatHotkey(hotkey: Hotkey): string {
  const keyNames: Record<string, string> = {
    ArrowLeft: "←",
    ArrowRight: "→",
    ArrowUp: "↑",
    ArrowDown: "↓",
    " ": "Space",
  };
  const key = Object.hasOwn(keyNames, hotkey.key)
    ? keyNames[hotkey.key]
    : hotkey.key.charAt(0).toUpperCase() + hotkey.key.slice(1).replace(/[A-Z]/g, " $&");
  const order: Modifier[] = ["Ctrl", "Alt", "Shift"];
  return [...order.filter((modifier) => hotkey.modifiers.includes(modifier)), key].join(" + ");
}

/** HOTKEY_NAMES are the bound commands' names, as the keyboard map's table gives them (the Hotkeys settings). */
export const HOTKEY_NAMES: Readonly<Record<HotkeyCommand, string>> = {
  "switcher:open": "Open quick switcher",
  "command-palette:open": "Open command palette",
  "file-explorer:new-file": "Create new note",
  "file-explorer:new-file-in-new-pane": "Create new note to the right",
  "workspace:new-tab": "New tab",
  "workspace:close": "Close current tab",
  "workspace:close-window": "Close window",
  "workspace:undo-close-pane": "Undo close tab",
  "workspace:next-tab": "Go to next tab",
  "workspace:previous-tab": "Go to previous tab",
  "workspace:goto-tab-1": "Go to tab 1",
  "workspace:goto-tab-2": "Go to tab 2",
  "workspace:goto-tab-3": "Go to tab 3",
  "workspace:goto-tab-4": "Go to tab 4",
  "workspace:goto-tab-5": "Go to tab 5",
  "workspace:goto-tab-6": "Go to tab 6",
  "workspace:goto-tab-7": "Go to tab 7",
  "workspace:goto-tab-8": "Go to tab 8",
  "workspace:goto-last-tab": "Go to last tab",
  "app:go-back": "Navigate back",
  "app:go-forward": "Navigate forward",
  "app:open-settings": "Open settings",
  "app:open-help": "Open help",
  "workspace:edit-file-title": "Rename file",
  "markdown:toggle-preview": "Toggle reading view",
  "markdown:add-metadata-property": "Add file property",
  "global-search:open": "Search in all files",
  "editor:open-search": "Search current file",
  "editor:open-search-replace": "Search & replace in current file",
  "editor:save-file": "Save current file",
  "editor:toggle-bold": "Toggle bold",
  "editor:toggle-italics": "Toggle italics",
  "editor:insert-link": "Insert Markdown link",
  "editor:toggle-checklist-status": "Toggle checkbox status",
  "editor:toggle-comments": "Toggle comment",
  "editor:delete-paragraph": "Delete paragraph",
  "editor:follow-link": "Follow link under cursor",
  "editor:open-link-in-new-leaf": "Open link under cursor in new tab",
  "editor:open-link-in-new-split": "Open link under cursor to the right",
  "editor:open-link-in-new-window": "Open link under cursor in new window",
  "graph:open": "Open graph view",
  "window:zoom-in": "Zoom in",
  "window:zoom-out": "Zoom out",
  "window:reset-zoom": "Reset zoom",
};

/** boundCommands lists the keyboard map's commands in its order. */
export function boundCommands(): HotkeyCommand[] {
  return KEYBOARD_MAP.map((entry) => entry.command);
}
