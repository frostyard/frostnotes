---
id: "0061"
title: Draw the history's lists of versions and revisions
milestone: M3
size: M
touch:
  - app/src/lib/when.ts
  - app/src/features/history/HistoryList.tsx
given:
  - app/src/features/history/HistoryList.test.tsx
acceptance: make ui-vitest F=src/features/history/HistoryList.test.tsx
---
# T-0061: Draw the history's lists of versions and revisions

## Goal

A file's history tab lists its versions, newest first, each with when,
who and how big, and for a synced vault the hub's revisions of it
(`docs/specs/ui.md`, History). Write the time and size formats as pure
functions, and draw the lists; the container reads the versions and
shows the one selected.

## Read first

- `app/src/lib/when.ts` and `app/src/features/history/HistoryList.tsx`:
  the types and the stubs.
- `docs/specs/ui.md`, "History".
- `app/src/styles/app.css`: `bg-badge`, `text-badge-text`, `text-keycap`,
  `text-sidebar-section`.
- The given test `app/src/features/history/HistoryList.test.tsx`.
- `docs/tasks/EXECUTOR.md`, especially the TypeScript conventions.

## Contract

Keep every exported name and signature, except that `SOURCE_ICONS`
becomes `Readonly<Record<HistorySource, LucideIcon>>` (a whole record);
replace the stubs and each file's "Task T-0061 …" sentence with a
description.

**`whenText(at, now)`**, in local time with a 24-hour clock `H:mm` (the
hour without a leading zero, the minutes with one):

- `at` on the same calendar day as `now`: `Today at H:mm`;
- on the day before (across months and years): `Yesterday at H:mm`;
- else `Mon D, YYYY at H:mm` with the month's English three-letter name
  (`Oct 15, 2026 at 9:41`).

**`sizeText(bytes)`:** under 1024: `1 byte`, else `N bytes`; under
1024 KB: KB, under 1024 MB: MB, each with one decimal below 10 (a
trailing `.0` dropped) and rounded to a whole number from 10 up:
`1 KB`, `1.5 KB`, `12 KB`, `2.3 MB`, `40 MB`.

**`SOURCE_ICONS`:** `editor` Lucide `Pencil`, `agent` `Bot`, `outside`
`HardDrive`, `sync` `Cloud`.

**`HistoryList`** (`now` defaults to the current time):

- A `nav` with `aria-label="Versions"`, 8px vertical padding. Its
  versions, or, with none, a paragraph "No versions yet."
  (13/18 `text-secondary`, 16px side padding).
- When `revisions` is not null, after them a `section` with
  `aria-label="On the hub"`, 12px above: an `h3` "On the hub"
  (`text-sidebar-section text-tertiary uppercase`, 16px side padding),
  then its revisions or a paragraph "No revisions on the hub.".
- Each list is a `ul`; each entry a `li` keyed by id holding a
  `button` with `aria-current="true"` for the selected entry (absent
  otherwise), `data-entry` its id, 44 high, inset 8, rounded 6, 8px
  side padding, two lines: `bg-selection-sidebar` when selected, else
  hover `bg-selection-inactive`. A click calls `onSelect(id)`.
  - Line one (`text-sidebar-row`): `whenText(new Date(at), now)`, then,
    for a conflict, a badge "Conflict" (`text-keycap`, rounded 4, 4px
    side padding, `bg-badge text-badge-text`), 8px apart.
  - Line two (12/16, `text-secondary`): the source's 12px icon
    (`aria-hidden`, `data-source` the source), then "Deleted" for a
    deletion, else the author and `sizeText(size)` joined by " · ",
    leaving out an empty author or a null size; 6px apart.
  The button's text is the two lines' texts in order
  ("Yesterday at 21:05Conflictclaude-code · 300 bytes").

## Tests (given, do not edit)

`app/src/features/history/HistoryList.test.tsx`: the time format
around today, yesterday and older days and across a month's and year's
turn; sizes; rows, sources, badges and details; the icons' table;
selection; the hub's list; empty lists.

## Gotchas

- Compare calendar days with `getFullYear`, `getMonth` and `getDate`,
  not by subtracting 24 hours (daylight saving shifts).
- The tests build their dates with local-time constructors, so format in
  local time too (`getHours`, not `getUTCHours`).
- The list is presentational: no stores, no client
  (`docs/tasks/EXECUTOR.md`).

## Out of scope

Reading versions and revisions, showing the selected one, Restore and
Compare (the planner's container), and every file except the two under
`touch`.

## Done when

`make accept T=0061`, `make check` and `make ui-check` pass, and only
the two files under `touch` changed.
