---
id: "0058"
title: Draw the Search pane
milestone: M3
size: M
touch:
  - app/src/features/search/SearchPane.tsx
given:
  - app/src/features/search/SearchPane.test.tsx
acceptance: make ui-vitest F=src/features/search/SearchPane.test.tsx
---
# T-0058: Draw the Search pane

## Goal

The left sidebar's Search pane searches the vault as the user types and
lists the notes that match, each with its snippet and the matches marked
(`docs/specs/ui.md`, Search pane). Draw it; the container holds the
query, runs `note.search` 250 ms after typing stops, and opens notes.

## Read first

- `app/src/features/search/SearchPane.tsx`: `SearchPaneProps` and the
  stub.
- `app/src/rpc/gen/api.ts`: `SearchHit` and `SnippetPart`.
- `app/src/features/panes/common.ts`: `PaneClick`, `paneClick`,
  `paneEmpty`.
- `docs/specs/ui.md`, "Search pane".
- The given test `app/src/features/search/SearchPane.test.tsx`.
- `docs/tasks/EXECUTOR.md`, especially the TypeScript conventions.

## Contract

Keep every exported name and signature; replace the stub and the file's
"Task T-0058 …" sentence with a description of the pane.

- **The field:** 8px margin, 28 high, `bg-window`, 1px
  `border-separator`, rounded 6, holding a 14px Lucide `Search` icon at
  the left (`text-secondary`, `aria-hidden`), an `input` with
  `type="search"`, `aria-label="Search"`, `placeholder="Search…"`,
  `spellCheck={false}`, `value={query}`, 28px padding at each side,
  no outline (and the browser's own cancel button hidden); `onChange`
  calls `onQuery(value)`. While the query is not empty, a `button`
  named "Clear search" with a 12px `X` sits at the right: a click calls
  `onQuery("")` and focuses the field.
- **Focus:** whenever `focusRequest` changes to a value other than 0,
  focus the field and select its text. Not on the first draw with 0.
- **Below the field**, nothing while the query is blank (spaces only).
  Else: `error` not null → a paragraph with the error (`paneEmpty`);
  `results` null → nothing; else a line "N results" ("1 result"),
  `text-status text-secondary`, 16px side padding, then a `ul` with
  `aria-label="Results"`, one `li` per hit keyed by path:
  - a 28-high row (`text-sidebar-row`, inset 8, rounded 6, hover
    `bg-selection-inactive`): a `button` (`tabIndex={-1}`) named "Fold
    <title>" or "Unfold <title>" with `aria-expanded` and a 14px
    `ChevronRight` (`rotate-90` while open) that folds the hit's
    snippet (open at first; the pane keeps this by path); then a
    `button` with `title` the path holding the title (600, truncating)
    and, for a note in a folder, the folder (the path before its last
    "/", 12/16 `text-secondary`) 8px after it; its accessible name is
    "<title> <folder>" or the title alone;
  - while open and when the hit has a snippet, a `button` (12/16, 24px
    left padding, rounded 6, hover `bg-selection-inactive`) holding the
    snippet's parts: matches in `mark` with `bg-highlight` (and
    `text-inherit`), the rest in `span`s.
- **Clicks:** the title's button and the snippet's both call
  `onOpen(path, paneClick(e))`.

## Tests (given, do not edit)

`app/src/features/search/SearchPane.test.tsx`: the field and count;
hits with folders and marked snippets; the singular; typing and
clearing; a blank query; loading and errors; opening with modifiers;
folding; focus requests.

## Gotchas

- The given test renders the pane in a harness that owns the query, so
  the field must be controlled: draw `query`, never keep its own text.
- happy-dom supports `select()` and `selectionStart`/`selectionEnd`.
- Keys for snippet parts are their positions; a `biome-ignore` for
  `noArrayIndexKey` saying they are known by their place is acceptable
  there only.
- The pane is presentational: no stores, no client, no timers
  (`docs/tasks/EXECUTOR.md`).

## Out of scope

Running the search, its delay, Ctrl+Shift+F and tag or property
searches (the planner's container), and every file except
`app/src/features/search/SearchPane.tsx`.

## Done when

`make accept T=0058`, `make check` and `make ui-check` pass, and only
`app/src/features/search/SearchPane.tsx` changed.
