---
id: "0038"
title: Map key presses to Obsidian's default hotkeys
milestone: M3
size: S
touch:
  - app/src/lib/keymap.ts
given:
  - app/src/lib/keymap.test.ts
acceptance: make ui-vitest F=src/lib/keymap.test.ts
---
# T-0038: Map key presses to Obsidian's default hotkeys

## Goal

The window has one key listener that turns key presses into commands
(`docs/design/app.md`, Commands and keys), and the command palette and
menus show each command's keys. Implement the keyboard map of
`docs/specs/ui.md`, Obsidian 1.14.4's default hotkeys on Linux: which
command a press runs, which hotkeys a command has, and how a hotkey is
written.

## Read first

- `app/src/lib/keymap.ts`: `HotkeyCommand`, `Modifier`, `Hotkey`,
  `KeyInput` and the three stubs.
- `docs/specs/ui.md`, "Keyboard map": the table and how keys are shown.
- The given test `app/src/lib/keymap.test.ts`.
- `docs/tasks/EXECUTOR.md`, especially the TypeScript conventions.

## Contract

Keep every exported name and signature; replace the stubs and the file's
"Task T-0038 …" sentence with a description of the map.

**The table.** Write the keyboard map as one constant array of
`{ command, keys }` entries, in the spec's order, each `keys` a list of
`Hotkey`s in the spec's order. Letters are written in upper case ("O",
"L"); other keys as `KeyboardEvent.key` names them: digits, `","`,
`";"`, `"/"`, `"="`, `"-"`, `"Enter"`, `"Tab"`, `"PageUp"`,
`"PageDown"`, `"ArrowLeft"`, `"ArrowRight"`, `"F1"`, `"F2"`. Modifiers are
listed in the order Ctrl, Alt, Shift. Every `HotkeyCommand` has an entry.

**`commandFor(input)`** returns the command of the first hotkey that
matches, or null:

- a press with `meta` set matches nothing;
- Ctrl, Alt and Shift must each be held exactly when the hotkey lists
  them;
- a single letter (a–z or A–Z) matches the hotkey's letter in either
  case; any other key must equal the hotkey's key exactly (so `"f2"` is
  not F2, nor `"tab"` Tab, and `"!"` or `"Dead"` match nothing).

**`hotkeysFor(command)`** returns copies of the command's hotkeys, in the
table's order (an empty list for a command without one; every command
has one today). Changing the returned objects must not change the table.

**`formatHotkey(hotkey)`** writes the modifiers the hotkey holds in the
order Ctrl, Alt, Shift (whatever their order in `modifiers`), then the
key, joined with `" + "`. The key is written as Obsidian writes it:
`ArrowLeft` "←", `ArrowRight` "→", `ArrowUp` "↑", `ArrowDown` "↓", `" "`
"Space"; any other key gets a space before each upper case letter after
the first, and its first character in upper case: `PageDown` "Page Down",
`"l"` "L", `F2` "F2", `Enter` "Enter", `","` ",". Examples: "Ctrl + O",
"Ctrl + Alt + ←", "Ctrl + Page Down", "F2", "Ctrl + Alt + Shift + Enter".

## Tests (given, do not edit)

`app/src/lib/keymap.test.ts`: `hotkeysFor`, `commandFor` and
`formatHotkey` over the whole map, letter case, exact modifiers, Meta,
digits and named keys, and the map's own uniqueness.

## Gotchas

- With Shift held, a letter arrives in upper case (`"F"`); with Caps
  Lock, in lower case even with Shift. Both must match.
- `Ctrl + 0` is Reset zoom and `Ctrl + 9` Go to last tab; `Ctrl + 1` to
  `Ctrl + 8` are the tabs.
- Only single letters ignore case. Named keys (`"Tab"`, `"Enter"`,
  `"F2"`) compare exactly.

## Out of scope

The window's listener and what runs a command (the planner's
`commands/`), where keys apply (text fields, the editor), changing
hotkeys, and every file except `app/src/lib/keymap.ts`.

## Done when

`make accept T=0038`, `make check` and `make ui-check` pass, and only
`app/src/lib/keymap.ts` changed.
