---
id: "0052"
title: Build the command palette over the fuzzy search
milestone: M3
size: M
touch:
  - app/src/lib/palette.ts
  - app/src/features/prompt/CommandPalette.tsx
given:
  - app/src/lib/palette.test.ts
  - app/src/features/prompt/CommandPalette.test.tsx
acceptance: make ui-vitest F="src/lib/palette.test.ts src/features/prompt/CommandPalette.test.tsx"
---
# T-0052: Build the command palette over the fuzzy search

## Goal

Ctrl+P runs any command that applies now by a few letters of its name,
the last ones run first, each with its keys shown as caps
(`docs/specs/ui.md`, Prompts). Build its results as a pure function
over T-0039's fuzzy search, and its component over T-0049's prompt
frame. The container gives it the commands and the recent ones, and
runs what it asks for.

## Read first

- `app/src/lib/palette.ts` and `app/src/features/prompt/CommandPalette.tsx`:
  the types and the stubs.
- `docs/specs/ui.md`, "Prompts: quick switcher and command palette".
- `app/src/lib/fuzzy.ts`: `prepareFuzzySearch` and `sortByScore`
  (stable).
- `app/src/features/prompt/Prompt.tsx`: `Prompt` and `highlight`, which
  T-0049 implements.
- `app/src/styles/app.css`: `bg-badge`, `text-badge-text` and the
  `text-keycap` type role.
- The given tests `app/src/lib/palette.test.ts` and
  `app/src/features/prompt/CommandPalette.test.tsx`.
- `docs/tasks/EXECUTOR.md`, especially the TypeScript conventions.

## Contract

Keep every exported name and signature; replace the stubs and each
file's "Task T-0052 …" sentence with a description.

**`paletteResults(commands, query, recent)`**, with `sorted` the
commands by name (`localeCompare`) and `q` the query trimmed:

- `q` empty: first the commands whose IDs `recent` lists, in its order,
  each once, skipping IDs not among `commands`, at most
  `PALETTE_RECENT`; then every other command in `sorted`'s order. Each
  as `{ command, match: null }`.
- Else, with one `prepareFuzzySearch(q)`, every command of `sorted`
  whose name matches, as `{ command, match }`, `sortByScore`d (so equal
  scores stay by name). No limit.

**`CommandPalette`** keeps the query in state (empty at first) and draws
`Prompt` with `label="Command palette"`, `placeholder="Select a
command…"`, the items `paletteResults(commands, query, recent)`
(memoized), `itemKey` the command's ID, and:

- **A row:** the name in `text-sidebar-row`, truncated, taking the
  remaining width, its matches drawn with `highlight`; then one `kbd`
  per entry of `keys`, in order, with the classes `text-keycap`,
  `bg-badge`, `text-badge-text`, rounded 4, padding 2px 6px, 4px apart
  (8px from the name). A row's text is the name then the keys
  ("Go to next tabCtrl + TabCtrl + Page Down").
- **Enter or a click** with an item: `onClose()`, then
  `onRun(command.id)`, so a command that opens another prompt opens it
  after this one closed. With no item, nothing.
- **`onClose`** is passed to the frame as is.
- **The footer:** `↑↓` "to navigate", `↵` "to use", `esc` "to dismiss".
- **Empty:** `emptyText` "No commands found.".

## Tests (given, do not edit)

`app/src/lib/palette.test.ts`: the blank query by name, recent commands
first and their limit, fuzzy ranking with spans, ties by name.
`app/src/features/prompt/CommandPalette.test.tsx`: the dialog and its
first rows, key caps, matching with highlights, closing before running,
clicks, no match, the footer.

## Gotchas

- `highlight` comes from `./Prompt`; do not write another.
- Sort a copy; never reorder the caller's array.
- The given component test renders T-0049's real `Prompt`: it runs only
  once T-0049 has merged.
- Presentational: no stores, no client, no registry imports
  (`docs/tasks/EXECUTOR.md`).

## Out of scope

Which commands apply, remembering the ones run, Ctrl+P (the
planner's), the quick switcher (T-0051), and every file except the two
under `touch`.

## Done when

`make accept T=0052`, `make check` and `make ui-check` pass, and only
the two files under `touch` changed.
