---
id: "0060"
title: Draw the settings dialog and its controls
milestone: M3
size: M
touch:
  - app/src/features/settings/SettingsDialog.tsx
given:
  - app/src/features/settings/SettingsDialog.test.tsx
acceptance: make ui-vitest F=src/features/settings/SettingsDialog.test.tsx
---
# T-0060: Draw the settings dialog and its controls

## Goal

Ctrl+, opens Obsidian's settings: sections at the left, the current
section's settings at the right, each a row with its name, description
and a switch, a select, a field or read-only keys (`docs/specs/ui.md`,
Settings). Draw the dialog from sections the container builds; every
change is sent at once and the container applies it.

## Read first

- `app/src/features/settings/SettingsDialog.tsx`: `SettingControl`,
  `SettingRow`, `SettingsSection`, `SettingsDialogProps` and the stub.
- `docs/specs/ui.md`, "Settings" and "Prompts" (the frame's look).
- `app/src/features/prompt/Prompt.tsx`: how a dialog goes in a portal
  over `bg-backdrop` and gives focus back when it goes.
- `app/src/styles/app.css`: the colors and type roles.
- The given test `app/src/features/settings/SettingsDialog.test.tsx`.
- `docs/tasks/EXECUTOR.md`, especially the TypeScript conventions.

## Contract

Keep every exported name and signature; replace the stub and the file's
"Task T-0060 …" sentence with a description of the dialog.

**The frame** renders through `createPortal` into `document.body`: a
backdrop `div` (`fixed inset-0`, `z-50`, `bg-backdrop`, centering its
child) whose own `pointerdown` (target is the backdrop) calls
`onClose()`; in it a `div` with `role="dialog"`, `aria-modal="true"`,
`aria-label="Settings"`, `tabIndex={-1}`, 900 × 640 at most 90% × 85% of
the window, `bg-window`, 1px `border-separator`, rounded 12,
`shadow-popover`, a row. On mount the dialog takes focus; when it goes,
focus returns to the element that had it (if still in the document).
Its `onKeyDown` stops every key's propagation (the window's keymap never
sees a key pressed in it); Escape (prevented) calls `onClose()`. A
`button` named "Close" (`aria-label`, a 16px Lucide `X`) at its top right
calls `onClose()`.

**The sections:** a `nav` with `aria-label="Settings sections"`, 200
wide, `bg-sidebar`, 8px padding: one `button` per section with its name,
28 high, rounded 6; the current one has `aria-current="page"` and
`bg-selection-sidebar`, the others hover `bg-selection-inactive`. A click
calls `onSection(id)`. The current section is the one whose `id` is
`section`, else the first.

**The current section:** a `section` with `aria-label` its name, 24px
padding, scrolling: an `h2` with the name (15/20, 600), then one row per
entry of `rows`, in order.

**A row:** a `div` with `data-setting` its id, a 1px `border-separator`
bottom, 12px vertical padding, a row 24px apart: at the left its name
(13/18) and, when given, its description (12/16, `text-secondary`); at
the right its control:

- `toggle`: a `button` with `role="switch"`, `aria-checked` the value,
  `aria-label` the row's name, `disabled` when the row is, 36 × 20,
  rounded full, `bg-accent` when on and `bg-badge` when off, with a 16px
  round knob (`bg-window`) at the right when on and the left when off; a
  click calls `onChange(id, !value)`.
- `select`: a `select` with `aria-label` the name, `value` the value,
  one `option` per entry, `disabled` when the row is, 200 wide; a change
  calls `onChange(id, newValue)`.
- `text`: an `input type="text"` with `aria-label` the name, the
  placeholder, `spellCheck={false}`, 200 wide, holding a draft that
  starts as the value (and follows a new value). It calls
  `onChange(id, draft)` on blur or Enter, and only when the draft differs
  from the value; typing alone sends nothing.
- `number`: an `input type="number"` with `aria-label`, `min`, `max`,
  80 wide, holding a draft. On blur or Enter: a blank or non-numeric
  draft goes back to the value and sends nothing; else the number is
  rounded and clamped to `[min, max]`, the field shows it, and
  `onChange(id, number)` is called when it differs from the value.
- `keys`: one `kbd` per key, in order (`text-keycap`, rounded 4,
  `bg-badge text-badge-text`, 2px 6px padding, `font-sans`).
- `info`: a `span` with the text (`text-secondary`, selectable).

Text and number fields: 28 high, rounded 6, 1px `border-separator`
(`border-accent` while focused), `bg-window`, 8px side padding.

## Tests (given, do not edit)

`app/src/features/settings/SettingsDialog.test.tsx`: the portal,
backdrop and focus; sections, the current one and its rows; choosing a
section and the fallback; toggles and disabled ones; selects; number
and text fields; keys and info; closing and keys kept from the window;
focus given back.

## Gotchas

- A `select` has the role `combobox` and a number input `spinbutton` in
  the tests' queries.
- Keep text and number drafts in state, so typing does not send a change
  per keystroke; reset the draft when `value` changes.
- Biome reports nothing for the backdrop's `onPointerDown`; do not add a
  suppression it does not need (`suppressions/unused`).
- The dialog is presentational: no stores, no client
  (`docs/tasks/EXECUTOR.md`).

## Out of scope

Which settings exist and what a change does (the planner's container),
Ctrl+, and F1, and every file except
`app/src/features/settings/SettingsDialog.tsx`.

## Done when

`make accept T=0060`, `make check` and `make ui-check` pass, and only
`app/src/features/settings/SettingsDialog.tsx` changed.
