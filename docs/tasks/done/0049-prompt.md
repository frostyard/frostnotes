---
id: "0049"
title: Draw the prompt frame the switcher and palette share
milestone: M3
size: M
touch:
  - app/src/features/prompt/Prompt.tsx
given:
  - app/src/features/prompt/Prompt.test.tsx
acceptance: make ui-vitest F=src/features/prompt/Prompt.test.tsx
---
# T-0049: Draw the prompt frame the switcher and palette share

## Goal

The quick switcher (Ctrl+O) and the command palette (Ctrl+P) are one
frame with different rows (`docs/specs/ui.md`, Prompts): a dialog over
the dimmed window with a query field, a list whose selected row the
arrow keys move while the field keeps focus, and a footer of
instructions. Draw that frame, generic over its items, and the
`highlight` helper both use to mark fuzzy matches. The caller holds the
query and decides what Enter does.

## Read first

- `app/src/features/prompt/Prompt.tsx`: `PromptMods`,
  `PromptInstruction`, `PromptProps`, the stubs of `highlight` and
  `Prompt`.
- `docs/specs/ui.md`, "Prompts: quick switcher and command palette" and
  the `--backdrop` token.
- `app/src/lib/fuzzy.ts`: `Range`, the spans `highlight` draws.
- `app/src/styles/app.css`: the colors (`bg-backdrop`,
  `bg-selection-inactive`), `shadow-popover`.
- The given test `app/src/features/prompt/Prompt.test.tsx`.
- `docs/tasks/EXECUTOR.md`, especially the TypeScript conventions.

## Contract

Keep every exported name and signature; replace the stubs and the
file's "Task T-0049 …" sentence with a description of the frame.

**`highlight(text, ranges, from = 0)`** returns `text` as React nodes,
with each range's characters wrapped in a `span` with the classes
`font-semibold text-accent`. A range `[s, e)` is in the coordinates of
some longer text in which `text` starts at `from`: draw
`text.slice(s - from, e - from)`, clipped to `[0, text.length)`; skip a
range that falls outside `text` or that starts before the end of the
last range drawn. Unmatched text stays plain strings; the nodes'
text, joined, is always `text`.

**The frame** renders through `createPortal` into `document.body`:

- **The backdrop:** a `div`, `fixed inset-0`, `z-50`, `bg-backdrop`,
  centering the dialog horizontally. A `pointerdown` whose target is the
  backdrop itself calls `onClose()`; one inside the dialog does not.
- **The dialog:** a `div` with `role="dialog"`, `aria-modal="true"`,
  `aria-label={label}`; 60px from the top, 700 wide but at most 80% of
  the window, at most 70% of its height, a column; `bg-window`, 1px
  `border-separator`, rounded 12 (`rounded-xl`), `shadow-popover`,
  clipping its overflow.
- **The field:** an `input` with `role="combobox"`, `aria-label={label}`,
  `aria-expanded="true"`, `aria-controls` the list's `id`,
  `aria-autocomplete="list"`, `aria-activedescendant` the selected
  option's `id` (absent while the list is empty), `placeholder`, `value=
  {query}`, `spellCheck={false}`; `onChange` calls `onQuery` with the
  new value. 44 high, 15/20, 16px side padding, 1px `border-separator`
  at its bottom, no background or outline. It takes focus on mount.
- **The list:** a `div` with a unique `id` (`useId`), `role="listbox"`,
  `aria-label="Results"`, scrolling, 8px padding. One `div` per item,
  keyed by `itemKey(item)`, with `role="option"`, a unique `id`,
  `tabIndex={-1}`, `aria-selected` (`true` for the selected row only),
  at least 32 high (`min-h-8`), rounded 6, 8px side padding, holding
  `renderItem(item)`; the selected row is `bg-selection-inactive`.
  While `items` is empty and `emptyText` is given, a paragraph with
  `emptyText` (13/18, `text-secondary`) instead.
- **The footer:** 28 high, 11/14 `text-tertiary`, 16px side padding,
  1px `border-separator` at its top, items 16px apart. Each instruction
  is one `span` holding a `span` with `keys` (`font-semibold`), a space,
  and `text`, so that instruction's own text reads `"↑↓ to navigate"`.

**Selection:** row 0 is selected at first and whenever `items` changes
(a new array). ArrowDown and ArrowUp in the field move it by one,
wrapping at both ends (nothing with an empty list), and scroll the row
into view (`scrollIntoView({ block: "nearest" })`, called only if the
element has it). The pointer moving over a row (`onMouseMove`) selects
it.

**Keys** in the field (`onKeyDown`): ArrowDown, ArrowUp, Enter and
Escape call `preventDefault()` and `stopPropagation()` (the window's
keymap never sees them); Tab calls `preventDefault()` only. Enter calls
`onEnter(items[selected], mods)`, which is `undefined` for an empty
list; Escape calls `onClose()`. `mods` is `{ ctrl, alt, shift }` from
the event's `ctrlKey`, `altKey` and `shiftKey`.

**Clicks:** a click on a row calls `onEnter(item, mods)` with the
click's modifiers.

## Tests (given, do not edit)

`app/src/features/prompt/Prompt.test.tsx`: the portal, dialog, backdrop
and focus; options, selection and the footer; `aria-activedescendant`;
the arrows and wrapping; typing and a new list; Enter with modifiers;
the empty list; the pointer and clicks; closing; and `highlight`.

## Gotchas

- An `option` must be focusable for Biome (`useFocusableInteractive`):
  `tabIndex={-1}` does it; focus stays in the field.
- A row's `onClick` without a key handler trips Biome's
  `useKeyWithClickEvents`: the field's keys choose rows, and a
  `biome-ignore` comment naming that rule with that reason is
  acceptable on the row only. The backdrop's `onPointerDown` may need
  one for `noStaticElementInteractions` (Escape does the same).
- Selection resets on a new `items` array, so a `useEffect` on `items`
  is right; Biome's `useExhaustiveDependencies` may then want a
  `biome-ignore` with the reason "items is the trigger".
- `T` is generic: write `export function Prompt<T>(props: PromptProps<T>)`.
- The frame is presentational: no stores, no client
  (`docs/tasks/EXECUTOR.md`).

## Out of scope

The quick switcher's and the palette's rows and results (T-0051,
T-0052), opening the prompts (the planner's), and every file except
`app/src/features/prompt/Prompt.tsx`.

## Done when

`make accept T=0049`, `make check` and `make ui-check` pass, and only
`app/src/features/prompt/Prompt.tsx` changed.
