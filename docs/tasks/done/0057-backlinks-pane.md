---
id: "0057"
title: Draw the Backlinks pane
milestone: M3
size: M
touch:
  - app/src/features/links/BacklinksPane.tsx
given:
  - app/src/features/links/BacklinksPane.test.tsx
acceptance: make ui-vitest F=src/features/links/BacklinksPane.test.tsx
---
# T-0057: Draw the Backlinks pane

## Goal

The right sidebar's Backlinks pane shows which notes link to the active
one, each link's line with the link marked, and the notes that mention
it without linking (`docs/specs/ui.md`, Right sidebar panes). Draw both
sections from groups the container gathers (`note.backlinks`' lines and
a search for the title).

## Read first

- `app/src/features/links/BacklinksPane.tsx`: `Mention`,
  `MentionGroup`, `BacklinksPaneProps` and the stub.
- `app/src/rpc/gen/api.ts`: `SnippetPart`.
- `app/src/features/panes/common.ts`: `PaneClick`, `paneClick`,
  `paneEmpty`.
- `docs/specs/ui.md`, "Right sidebar panes" (Backlinks).
- The given test `app/src/features/links/BacklinksPane.test.tsx`.
- `docs/tasks/EXECUTOR.md`, especially the TypeScript conventions.

## Contract

Keep every exported name and signature; replace the stub and the file's
"Task T-0057 …" sentence with a description of the pane.

- `linked === null`: a paragraph "No note is open." (`paneEmpty`) and
  nothing else.
- Otherwise two sections, each a `section` with `aria-label` its title:
  "Linked mentions", open at first, then "Unlinked mentions", closed at
  first. The pane keeps whether each is open.
- **A section's header** is its first `button`: `aria-expanded`, a
  14px `ChevronRight` (`rotate-90` while open, `text-secondary`), the
  title (600) taking the remaining width, then the number of groups in
  a `span` with `text-status text-secondary`; for Unlinked mentions no
  number while `unlinked` is null. Its text is title then number
  ("Linked mentions2"). `text-sidebar-row`, inset 8, 28 high, rounded
  6, hover `bg-selection-inactive`. A click opens or closes the
  section.
- **An open section** shows its groups (`linked`, or `unlinked` when it
  is not null), or, with none, a paragraph (`paneEmpty`) "No backlinks
  found." or "No unlinked mentions found.". Unlinked mentions with
  `unlinked` null show nothing under the header.
- **A group** (a `li` keyed by path in a `ul`): a 28-high row, 16px left
  padding, holding a `button` named "Fold <title>" or "Unfold <title>"
  (`aria-label`, `aria-expanded`, `tabIndex={-1}`, a 14px
  `ChevronRight`, `rotate-90` while open) that folds or unfolds the
  group (open at first; the pane keeps this by path), a `button` with
  the title (`title` the path, truncating, the remaining width), and the
  number of mentions in a `span` (`text-status text-secondary`). The
  row's text is title then number ("2026-10-172"). While open, a `ul`
  of the mentions follows, each a `button` (12/16, 36px left padding,
  rounded 6, hover `bg-selection-inactive`) holding the parts: a part
  that matches in a `mark` with `bg-highlight` (and `text-inherit`),
  the others in `span`s; its text is the parts joined.
- **Clicks:** a group's title calls `onOpen(path, null,
  paneClick(e))`; a mention calls `onOpen(path, mention.line,
  paneClick(e))`.

## Tests (given, do not edit)

`app/src/features/links/BacklinksPane.test.tsx`: the sections, their
counts and states; groups, counts and marked lines; opening at a line or
a note with modifiers; folding a group and a section; the unlinked
mentions; empty, loading and no-note states.

## Gotchas

- Keys for mentions and snippet parts are their positions; Biome's
  `noArrayIndexKey` then wants a `biome-ignore` comment saying they are
  known by their place, which is acceptable for those two lists only.
- A button's accessible name joins its parts' text as written.
- The pane is presentational: no stores, no client
  (`docs/tasks/EXECUTOR.md`).

## Out of scope

Gathering backlinks and unlinked mentions, opening notes at a line (the
planner's container), the Outgoing links pane (T-0054), and every file
except `app/src/features/links/BacklinksPane.tsx`.

## Done when

`make accept T=0057`, `make check` and `make ui-check` pass, and only
`app/src/features/links/BacklinksPane.tsx` changed.
