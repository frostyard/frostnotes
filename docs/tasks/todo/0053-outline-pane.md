---
id: "0053"
title: Draw the Outline pane
milestone: M3
size: S
touch:
  - app/src/features/outline/OutlinePane.tsx
given:
  - app/src/features/outline/OutlinePane.test.tsx
acceptance: make ui-vitest F=src/features/outline/OutlinePane.test.tsx
---
# T-0053: Draw the Outline pane

## Goal

The right sidebar's Outline pane lists the active note's headings,
nested, and takes the editor to one when it is clicked
(`docs/specs/ui.md`, Right sidebar panes). Draw it from T-0041's rows;
the container reads the headings from the editor and folds them.

## Read first

- `app/src/features/outline/OutlinePane.tsx`: `OutlinePaneProps` and
  the stub.
- `app/src/lib/outline.ts`: `Heading` and `OutlineRow` (T-0041).
- `app/src/features/panes/common.ts`: `paneEmpty`.
- `docs/specs/ui.md`, "Right sidebar panes" (Outline).
- The given test `app/src/features/outline/OutlinePane.test.tsx`.
- `docs/tasks/EXECUTOR.md`, especially the TypeScript conventions.

## Contract

Keep every exported name and signature; replace the stub and the file's
"Task T-0053 …" sentence with a description of the pane.

- `rows === null`: a paragraph "No note is open." with the class
  `paneEmpty`. `rows` empty: a paragraph "No headings" with `paneEmpty`.
  Neither draws the tree.
- Otherwise a `div` with `role="tree"`, `aria-label="Outline"`, 4px
  vertical padding, holding one row per entry of `rows`, keyed by its
  heading's `from`.
- **A row:** a `div` with `role="treeitem"`, `tabIndex={-1}`,
  `aria-level` its depth + 1, `aria-expanded` (its `expanded`) only
  when `hasChildren`, `aria-selected` true for the row at index
  `current` and false for the others; `text-sidebar-row`, inset 8 at
  each side, 28 high, rounded 6, a flex row; `style.paddingLeft`
  `8 + 16 × depth` pixels; hover `bg-selection-inactive`, and
  `bg-selection-sidebar` for the current row. In it:
  1. a 16-wide column holding, when the heading has children, a
     `button` named "Fold <text>" while expanded and "Unfold <text>"
     while not (`aria-label`), `tabIndex={-1}`, with a 14px Lucide
     `ChevronRight` in `text-secondary`, turned 90° (`rotate-90`)
     while expanded; a click calls `onToggle(heading.from)`;
  2. a `button` with the heading's text, `title` the text, truncated,
     taking the remaining width; a click calls `onGo(heading)`.

The row's text content is the heading's text alone.

## Tests (given, do not edit)

`app/src/features/outline/OutlinePane.test.tsx`: the tree's rows,
levels, padding and expansion; the current row; the chevrons' names and
folding; folded rows; going to a heading; the empty and no-note texts.

## Gotchas

- Biome wants a `treeitem` focusable (`useFocusableInteractive`):
  `tabIndex={-1}` does it.
- The pane is presentational: no stores, no editor imports
  (`docs/tasks/EXECUTOR.md`).

## Out of scope

Reading headings from the editor, folding state, scrolling the editor
(the planner's container), and every file except
`app/src/features/outline/OutlinePane.tsx`.

## Done when

`make accept T=0053`, `make check` and `make ui-check` pass, and only
`app/src/features/outline/OutlinePane.tsx` changed.
