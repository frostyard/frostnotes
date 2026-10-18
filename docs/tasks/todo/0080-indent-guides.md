---
id: "0080"
title: Draw indentation guides in live preview
milestone: M3
size: S
touch:
  - app/src/editor/preview/guides.ts
given:
  - app/src/editor/preview/guides.test.ts
acceptance: make ui-vitest F=src/editor/preview/guides.test.ts
---
# T-0080: Draw indentation guides in live preview

## Goal

Nested lists show indentation guides (`docs/specs/ui.md`, Editor and
reading view): a thin line down each nested level, under its parent's
bullet. Draw them as marks on the indentation, so the text stays as it
is.

## Read first

- `app/src/editor/preview/guides.ts`: the stub, already in `PROVIDERS`
  (`preview/index.ts`).
- `app/src/editor/preview/framework.ts` and `lists.ts` (bullets drawn by
  a provider the same way).
- @lezer/markdown's `ListItem`, `ListMark`, `BulletList`, `OrderedList`.
- The given test.

## Contract

Keep the export `GUIDES: readonly Provider[]`; replace the file's "Task
T-0080 …" sentence with a description.

One provider, `id: "guide"`, `kind: "inline"`, `nodes: ["ListItem"]`:
for a `ListItem` holding a `BulletList` or `OrderedList` (a nested list),
with its `ListMark` at column `c` of the item's first line, every later
line of the item whose leading spaces and tabs are longer than `c` gets
`Decoration.mark({ class: "cm-pv-guide" })` over the one character at
column `c`. Wherever the selection is; the theme draws the line, and the
Show indentation guides preference hides it.

## Tests (given, do not edit)

`app/src/editor/preview/guides.test.ts`: guides at each parent's column,
the text kept, ordered lists, tabs and continued lines, and where no
guide goes.

## Out of scope

The guide's style and its preference (the planner's), and every file
except `app/src/editor/preview/guides.ts`.

## Done when

`make accept T=0080`, `make check` and `make ui-check` pass, and only
`app/src/editor/preview/guides.ts` changed.
