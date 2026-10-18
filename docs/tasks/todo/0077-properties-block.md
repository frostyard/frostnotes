---
id: "0077"
title: Draw the properties block
milestone: M3
size: L
touch:
  - app/src/features/properties/PropertiesBlock.tsx
given:
  - app/src/features/properties/PropertiesBlock.test.tsx
acceptance: make ui-vitest F=src/features/properties/PropertiesBlock.test.tsx
---
# T-0077: Draw the properties block

## Goal

A note's frontmatter shows above its text as the properties block
(`docs/specs/ui.md`, Editor and reading view): a row per key with its
type's icon, the key and an editor for its value, foldable, and a row to
add a property. Draw it; the container reads the rows from the
frontmatter and writes each change into it.

## Read first

- `app/src/features/properties/PropertiesBlock.tsx`: the types and stub.
- `docs/specs/ui.md`, "Editor and reading view", the Properties block.
- `app/src/features/properties/PropertiesPane.tsx`: `TYPE_ICONS`, the
  types' icons.
- `app/src/editor/properties.ts`: `PropertyType`, `PropertyValue`.
- The given test.
- `docs/tasks/EXECUTOR.md`, the TypeScript conventions.

## Contract

Keep every exported name and signature; replace the file's "Task
T-0077 …" sentence with a description.

- **The block** is a `section` with `aria-label="Properties"`. First a
  `button` "Properties" with a chevron (`ChevronRight`, turned 90° while
  open), `aria-expanded` the opposite of `folded`, 13/16
  `text-secondary`; a click calls `onFold(!folded)`. While folded,
  nothing else shows.
- **With an error** (not null): a paragraph saying the frontmatter is not
  properties, holding the error's text, and no rows or Add property.
- **A row** per entry of `rows`, in order: a `div` with `role="group"`,
  `aria-label` the key, `data-property` the key; 30 high, rounded 6,
  hover `bg-code`. In a 9em column, the type's icon from `TYPE_ICONS`
  (14px, `aria-hidden`, `data-type` the type) and a text field
  `aria-label="Property name"` holding the key; then the value's editor;
  then a button `aria-label="Remove <key>"` (shown on hover and focus)
  that calls `onRemove(key)`.
- **Text fields** (the name, text and number values) commit when Enter
  is pressed (which leaves the field) or the field loses focus, and only
  when the text changed; Escape restores the text and leaves without
  committing. A name commits trimmed and not blank as
  `onRename(key, name)`.
- **Values by type**, each editor's `aria-label` the key:
  - `text` (and any other): a text field, committing
    `onChange(key, text)`; a value that is an object shows as its JSON.
  - `number`: a field of type number, committing `onChange(key, n)` for
    a finite number, nothing for a blank one.
  - `checkbox`: a checkbox, `onChange(key, checked)` on change.
  - `date`, `datetime`: an input of type `date` or `datetime-local`,
    `onChange(key, value)` on change.
  - `multitext`, `aliases`, `tags`: the value's items (export
    `items(value)`: an array's items as text, `[]` for null or `""`, any
    other value as one item) as pills, each with a button
    `aria-label="Remove <item>"` that calls `onChange(key, items without
    it)`; then a text field `aria-label="Add to <key>"` whose Enter adds
    its trimmed text (for tags without a leading `#`), when not blank,
    as `onChange(key, [...items, text])`, and empties it. Tag pills are
    `bg-tag text-accent`, others `bg-code`.
- **Add property** (`Plus` icon), last while not folded: a click shows a
  field `aria-label="New property name"` with focus in place of the
  button. Enter with a trimmed name no row has calls
  `onChange(name, "")` and closes it; a blank or taken name does
  nothing; Escape, or leaving it blank, closes it.
- **`addRequest`:** each time it changes after the first render, the
  block calls `onFold(false)` and opens the new row.

## Tests (given, do not edit)

`app/src/features/properties/PropertiesBlock.test.tsx`: rows and icons,
folding, each editor, lists, JSON, renaming, removing, adding,
`addRequest`, and an error.

## Gotchas

- Biome forbids an array index as a key: a list may repeat an item, so
  key each pill by its item and how many times it came before.
- A group of fields is a `div role="group"`: Biome asks for a fieldset;
  say why not in a `biome-ignore` comment.
- Escape then blur: make sure the blur that follows Escape does not
  commit.
- The block is presentational: no stores, no editor imports beyond the
  types (`docs/tasks/EXECUTOR.md`).

## Out of scope

Reading and writing the frontmatter (T-0076 and the planner's
container), and every file except
`app/src/features/properties/PropertiesBlock.tsx`.

## Done when

`make accept T=0077`, `make check` and `make ui-check` pass, and only
`app/src/features/properties/PropertiesBlock.tsx` changed.
