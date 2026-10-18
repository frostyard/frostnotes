---
id: "0062"
title: Draw the confirmation dialog
milestone: M3
size: S
touch:
  - app/src/features/dialog/ConfirmDialog.tsx
given:
  - app/src/features/dialog/ConfirmDialog.test.tsx
acceptance: make ui-vitest F=src/features/dialog/ConfirmDialog.test.tsx
---
# T-0062: Draw the confirmation dialog

## Goal

Deleting a file asks first ("Delete "Garden plan"?" … Delete), and so
does moving one while links are not updated without asking
(`docs/specs/ui.md`, Files pane; Settings). Draw the window's one
confirmation dialog in place of the browser's `confirm`; the container
asks and awaits the answer.

## Read first

- `app/src/features/dialog/ConfirmDialog.tsx`: `ConfirmDialogProps` and
  the stub.
- `docs/specs/ui.md`, "Files pane" (Delete) and "Prompts" (the frame's
  look).
- `app/src/features/prompt/Prompt.tsx`: a dialog in a portal over
  `bg-backdrop` that gives focus back.
- The given test `app/src/features/dialog/ConfirmDialog.test.tsx`.
- `docs/tasks/EXECUTOR.md`, especially the TypeScript conventions.

## Contract

Keep every exported name and signature; replace the stub and the file's
"Task T-0062 …" sentence with a description.

- Renders through `createPortal` into `document.body`: a backdrop `div`
  (`fixed inset-0`, `z-50`, `bg-backdrop`, centering) whose own
  `pointerdown` (its target is the backdrop) calls `onCancel()`.
- In it a `div` with `role="alertdialog"`, `aria-modal="true"`,
  `aria-labelledby` the title's `id` and `aria-describedby` the text's
  (`useId`), 400 wide at most 90%, `bg-window`, 1px `border-separator`,
  rounded 12, `shadow-popover`, 20px padding: an `h2` with the title
  (15/20, 600), a `p` with the text (13/18, `text-secondary`, 8px
  below), and, 20px below at the right 8px apart, two `button`s:
  the cancelling one labeled `cancelLabel` or "Cancel" (1px
  `border-separator`, hover `bg-selection-inactive`) → `onCancel()`;
  then the confirming one labeled `confirmLabel`
  (`text-accent-contrast`, 500, `bg-flag-1` when `danger`, else
  `bg-accent`) → `onConfirm()`. Both 28 high, rounded 6, 12px side
  padding.
- On mount the confirming button takes focus (so Enter confirms); when
  the dialog goes, focus returns to the element that had it, if it is
  still in the document.
- The dialog's `onKeyDown` stops every key's propagation; Escape
  (prevented) calls `onCancel()`.

## Tests (given, do not edit)

`app/src/features/dialog/ConfirmDialog.test.tsx`: the portal, roles,
name and description; focus and colors; both labels; confirming by
click and Enter; cancelling by button, Escape and the backdrop; keys
kept from the window; focus given back.

## Gotchas

- Biome reports nothing for the backdrop's `onPointerDown`; do not add a
  suppression it does not need (`suppressions/unused`).
- The dialog is presentational: no stores, no promise of its own
  (`docs/tasks/EXECUTOR.md`).

## Out of scope

Asking and awaiting (the planner's `data/` store), which actions ask,
and every file except `app/src/features/dialog/ConfirmDialog.tsx`.

## Done when

`make accept T=0062`, `make check` and `make ui-check` pass, and only
`app/src/features/dialog/ConfirmDialog.tsx` changed.
