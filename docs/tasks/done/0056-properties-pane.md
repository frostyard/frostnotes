---
id: "0056"
title: Draw the All properties pane
milestone: M3
size: S
touch:
  - app/src/features/properties/PropertiesPane.tsx
given:
  - app/src/features/properties/PropertiesPane.test.tsx
acceptance: make ui-vitest F=src/features/properties/PropertiesPane.test.tsx
---
# T-0056: Draw the All properties pane

## Goal

The right sidebar's All properties pane lists every frontmatter key in
the vault with its type's icon and count, sorted as the Tags pane sorts,
and lists the notes with a key when it is clicked (`docs/specs/ui.md`,
Right sidebar panes).

## Read first

- `app/src/features/properties/PropertiesPane.tsx`: `PropertiesPaneProps`,
  `TYPE_ICONS`, `sortProperties` and the stubs.
- `app/src/rpc/gen/api.ts`: `PropertyCount` and its types.
- `app/src/features/panes/common.ts`: `CountSort`, `paneRow`,
  `paneToolbar`, `paneEmpty`.
- `app/src/features/chrome/IconButton.tsx`.
- `docs/specs/ui.md`, "Right sidebar panes" (All properties).
- The given test `app/src/features/properties/PropertiesPane.test.tsx`.
- `docs/tasks/EXECUTOR.md`, especially the TypeScript conventions.

## Contract

Keep every exported name and signature; replace the stubs and the
file's "Task T-0056 …" sentence with a description.

- **`TYPE_ICONS`** maps each property type to a Lucide icon component:
  `text` `Text`, `multitext` `List`, `number` `Binary`, `checkbox`
  `SquareCheck`, `date` `Calendar`, `datetime` `Clock`, `aliases`
  `Forward`, `tags` `Tags`. A key with no type, or one not listed, draws
  `Text`.
- **`sortProperties(properties, sort)`** returns a sorted copy (never
  sort the input): `"name"` by key ignoring case, A to Z; `"count"` by
  count, most first, then by key.
- **`PropertiesPane`:** a toolbar (`paneToolbar`) with one `IconButton`
  of size 24, "Change sort order", Lucide `ArrowUpNarrowWide` 16px; a
  click calls `onSortMenu(left, bottom)` with the button's
  `getBoundingClientRect()`. Under it: nothing while `properties` is
  null; "No properties found." (`paneEmpty`) when it is empty; else a
  `ul` with `aria-label="All properties"`, one `li` per key in
  `sortProperties` order, holding a `button` with `paneRow`, 8px side
  padding, the width less its insets, `data-type` the key's type (or
  `text` when it has none): the type's icon (14px, `text-secondary`,
  `aria-hidden`), the key in a truncating `span` taking the remaining
  width, and the count in a `span` with `text-status text-secondary`. A
  click calls `onChoose(key)`. A row's text is the key then the count
  ("status4").

## Tests (given, do not edit)

`app/src/features/properties/PropertiesPane.test.tsx`: both sorts and a
copy; rows, types, icons and counts; the icons' table; choosing a key
and the sort button; empty and loading.

## Gotchas

- `TYPE_ICONS` values are components (`LucideIcon`), drawn as
  `<Icon size={14} … />`.
- The pane is presentational: no stores, no client
  (`docs/tasks/EXECUTOR.md`).

## Out of scope

Calling `note.properties`, the sort order's menu, listing the notes
(the planner's), the Tags pane (T-0055), and every file except
`app/src/features/properties/PropertiesPane.tsx`.

## Done when

`make accept T=0056`, `make check` and `make ui-check` pass, and only
`app/src/features/properties/PropertiesPane.tsx` changed.
