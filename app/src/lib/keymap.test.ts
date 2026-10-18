import { describe, expect, it } from "vitest";

import {
  boundCommands,
  commandFor,
  formatHotkey,
  HOTKEY_NAMES,
  type Hotkey,
  type HotkeyCommand,
  hotkeysFor,
  type KeyInput,
} from "./keymap";

const press = (
  key: string,
  mods: { ctrl?: boolean; alt?: boolean; shift?: boolean; meta?: boolean } = {},
): KeyInput => ({
  key,
  ctrl: mods.ctrl ?? false,
  alt: mods.alt ?? false,
  shift: mods.shift ?? false,
  meta: mods.meta ?? false,
});

const hk = (modifiers: Hotkey["modifiers"], key: string): Hotkey => ({ modifiers, key });

// The keyboard map of docs/specs/ui.md: Obsidian 1.14.4's defaults on Linux.
const MAP: [HotkeyCommand, Hotkey[]][] = [
  ["switcher:open", [hk(["Ctrl"], "O")]],
  ["command-palette:open", [hk(["Ctrl"], "P")]],
  ["file-explorer:new-file", [hk(["Ctrl"], "N")]],
  ["file-explorer:new-file-in-new-pane", [hk(["Ctrl", "Shift"], "N")]],
  ["workspace:new-tab", [hk(["Ctrl"], "T")]],
  ["workspace:close", [hk(["Ctrl"], "W")]],
  ["workspace:close-window", [hk(["Ctrl", "Shift"], "W")]],
  ["workspace:undo-close-pane", [hk(["Ctrl", "Shift"], "T")]],
  ["workspace:next-tab", [hk(["Ctrl"], "Tab"), hk(["Ctrl"], "PageDown")]],
  ["workspace:previous-tab", [hk(["Ctrl", "Shift"], "Tab"), hk(["Ctrl"], "PageUp")]],
  ["workspace:goto-tab-1", [hk(["Ctrl"], "1")]],
  ["workspace:goto-tab-2", [hk(["Ctrl"], "2")]],
  ["workspace:goto-tab-3", [hk(["Ctrl"], "3")]],
  ["workspace:goto-tab-4", [hk(["Ctrl"], "4")]],
  ["workspace:goto-tab-5", [hk(["Ctrl"], "5")]],
  ["workspace:goto-tab-6", [hk(["Ctrl"], "6")]],
  ["workspace:goto-tab-7", [hk(["Ctrl"], "7")]],
  ["workspace:goto-tab-8", [hk(["Ctrl"], "8")]],
  ["workspace:goto-last-tab", [hk(["Ctrl"], "9")]],
  ["app:go-back", [hk(["Ctrl", "Alt"], "ArrowLeft")]],
  ["app:go-forward", [hk(["Ctrl", "Alt"], "ArrowRight")]],
  ["app:open-settings", [hk(["Ctrl"], ",")]],
  ["app:open-help", [hk([], "F1")]],
  ["workspace:edit-file-title", [hk([], "F2")]],
  ["markdown:toggle-preview", [hk(["Ctrl"], "E")]],
  ["markdown:add-metadata-property", [hk(["Ctrl"], ";")]],
  ["global-search:open", [hk(["Ctrl", "Shift"], "F")]],
  ["editor:open-search", [hk(["Ctrl"], "F")]],
  ["editor:open-search-replace", [hk(["Ctrl"], "H")]],
  ["editor:save-file", [hk(["Ctrl"], "S")]],
  ["editor:toggle-bold", [hk(["Ctrl"], "B")]],
  ["editor:toggle-italics", [hk(["Ctrl"], "I")]],
  ["editor:insert-link", [hk(["Ctrl"], "K")]],
  ["editor:toggle-checklist-status", [hk(["Ctrl"], "L")]],
  ["editor:toggle-comments", [hk(["Ctrl"], "/")]],
  ["editor:delete-paragraph", [hk(["Ctrl"], "D")]],
  ["editor:follow-link", [hk(["Alt"], "Enter")]],
  ["editor:open-link-in-new-leaf", [hk(["Ctrl"], "Enter")]],
  ["editor:open-link-in-new-split", [hk(["Ctrl", "Alt"], "Enter")]],
  ["editor:open-link-in-new-window", [hk(["Ctrl", "Alt", "Shift"], "Enter")]],
  ["graph:open", [hk(["Ctrl"], "G")]],
  ["window:zoom-in", [hk(["Ctrl"], "=")]],
  ["window:zoom-out", [hk(["Ctrl"], "-")]],
  ["window:reset-zoom", [hk(["Ctrl"], "0")]],
];

describe("hotkeysFor", () => {
  it("gives every command its keys from the keyboard map, in order", () => {
    for (const [command, keys] of MAP) {
      expect(hotkeysFor(command), command).toEqual(keys);
    }
  });
});

describe("commandFor", () => {
  it("runs each hotkey's command, as the press arrives", () => {
    for (const [command, keys] of MAP) {
      for (const { modifiers, key } of keys) {
        const input = press(key, {
          ctrl: modifiers.includes("Ctrl"),
          alt: modifiers.includes("Alt"),
          shift: modifiers.includes("Shift"),
        });
        expect(commandFor(input), `${modifiers.join("+")}+${key}`).toBe(command);
      }
    }
  });

  it("matches letters in either case", () => {
    expect(commandFor(press("o", { ctrl: true }))).toBe("switcher:open");
    expect(commandFor(press("O", { ctrl: true }))).toBe("switcher:open");
    expect(commandFor(press("l", { ctrl: true }))).toBe("editor:toggle-checklist-status");
    // With Shift, letters arrive in upper case, and with Caps Lock too in lower.
    expect(commandFor(press("F", { ctrl: true, shift: true }))).toBe("global-search:open");
    expect(commandFor(press("f", { ctrl: true, shift: true }))).toBe("global-search:open");
  });

  it("needs exactly the binding's modifiers", () => {
    expect(commandFor(press("o"))).toBeNull();
    expect(commandFor(press("O", { ctrl: true, shift: true }))).toBeNull();
    expect(commandFor(press("o", { ctrl: true, alt: true }))).toBeNull();
    expect(commandFor(press("F2", { shift: true }))).toBeNull();
    expect(commandFor(press("Enter"))).toBeNull();
    expect(commandFor(press("Enter", { shift: true }))).toBeNull();
    expect(commandFor(press("ArrowLeft", { ctrl: true }))).toBeNull();
    expect(commandFor(press("Tab"))).toBeNull();
  });

  it("matches nothing while Meta is held", () => {
    expect(commandFor(press("o", { ctrl: true, meta: true }))).toBeNull();
    expect(commandFor(press("F1", { meta: true }))).toBeNull();
  });

  it("tells digits and other keys apart", () => {
    expect(commandFor(press("0", { ctrl: true }))).toBe("window:reset-zoom");
    expect(commandFor(press("9", { ctrl: true }))).toBe("workspace:goto-last-tab");
    // Ctrl+Shift+1 arrives as "!" on a US layout: nothing is bound to it.
    expect(commandFor(press("!", { ctrl: true, shift: true }))).toBeNull();
    expect(commandFor(press("+", { ctrl: true, shift: true }))).toBeNull();
    expect(commandFor(press("Dead", { ctrl: true }))).toBeNull();
    expect(commandFor(press("", { ctrl: true }))).toBeNull();
    // Named keys are not letters: "f2" is not F2, nor "tab" Tab.
    expect(commandFor(press("f2"))).toBeNull();
    expect(commandFor(press("tab", { ctrl: true }))).toBeNull();
  });

  it("leaves no press bound twice in the keyboard map", () => {
    const seen = new Map<string, HotkeyCommand>();
    for (const [command, keys] of MAP) {
      for (const h of keys) {
        const id = `${h.modifiers.join("+")}+${h.key.toUpperCase()}`;
        expect(seen.get(id), id).toBeUndefined();
        seen.set(id, command);
      }
    }
  });
});

describe("formatHotkey", () => {
  it("writes hotkeys as Obsidian shows them", () => {
    expect(formatHotkey(hk(["Ctrl"], "O"))).toBe("Ctrl + O");
    expect(formatHotkey(hk(["Ctrl", "Shift"], "F"))).toBe("Ctrl + Shift + F");
    expect(formatHotkey(hk(["Ctrl", "Alt"], "ArrowLeft"))).toBe("Ctrl + Alt + ←");
    expect(formatHotkey(hk(["Ctrl", "Alt"], "ArrowRight"))).toBe("Ctrl + Alt + →");
    expect(formatHotkey(hk(["Alt"], "ArrowUp"))).toBe("Alt + ↑");
    expect(formatHotkey(hk(["Alt"], "ArrowDown"))).toBe("Alt + ↓");
    expect(formatHotkey(hk(["Ctrl"], "PageDown"))).toBe("Ctrl + Page Down");
    expect(formatHotkey(hk(["Ctrl", "Alt", "Shift"], "Enter"))).toBe("Ctrl + Alt + Shift + Enter");
    expect(formatHotkey(hk([], "F2"))).toBe("F2");
    expect(formatHotkey(hk(["Ctrl"], ","))).toBe("Ctrl + ,");
    expect(formatHotkey(hk(["Alt"], " "))).toBe("Alt + Space");
  });

  it("orders the modifiers Ctrl, Alt, Shift and capitalizes a letter", () => {
    expect(formatHotkey(hk(["Shift", "Ctrl"], "t"))).toBe("Ctrl + Shift + T");
    expect(formatHotkey(hk(["Shift", "Alt", "Ctrl"], "Enter"))).toBe("Ctrl + Alt + Shift + Enter");
    expect(formatHotkey(hk(["Ctrl"], "l"))).toBe("Ctrl + L");
  });

  it("writes every hotkey of the map", () => {
    expect(hotkeysFor("workspace:next-tab").map(formatHotkey)).toEqual(["Ctrl + Tab", "Ctrl + Page Down"]);
    expect(hotkeysFor("workspace:previous-tab").map(formatHotkey)).toEqual(["Ctrl + Shift + Tab", "Ctrl + Page Up"]);
    expect(hotkeysFor("window:zoom-out").map(formatHotkey)).toEqual(["Ctrl + -"]);
  });
});

describe("the bound commands' names", () => {
  it("names every command the map binds, in its order", () => {
    const bound = boundCommands();
    expect(bound[0]).toBe("switcher:open");
    expect(bound.at(-1)).toBe("window:reset-zoom");
    expect(new Set(bound).size).toBe(bound.length);
    expect(bound.every((c) => HOTKEY_NAMES[c] !== "")).toBe(true);
    expect(Object.keys(HOTKEY_NAMES).sort()).toEqual([...bound].sort());
    expect(HOTKEY_NAMES["editor:open-search-replace"]).toBe("Search & replace in current file");
  });
});
