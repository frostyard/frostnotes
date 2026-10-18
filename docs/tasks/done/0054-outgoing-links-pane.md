---
id: "0054"
title: Draw the Outgoing links pane
milestone: M3
size: S
touch:
  - app/src/lib/outgoing.ts
  - app/src/features/links/OutgoingPane.tsx
given:
  - app/src/features/links/OutgoingPane.test.tsx
acceptance: make ui-vitest F=src/features/links/OutgoingPane.test.tsx
---
# T-0054: Draw the Outgoing links pane

## Goal

The right sidebar's Outgoing links pane lists where the active note
links to, each target once, unresolved ones dimmed, and opens or
creates one on a click (`docs/specs/ui.md`, Right sidebar panes). Build
the list from `note.links`' answer as a pure function, and draw it.

## Read first

- `app/src/lib/outgoing.ts` and `app/src/features/links/OutgoingPane.tsx`:
  the types and the stubs.
- `app/src/rpc/gen/api.ts`: `Link` (`target`, `resolved`).
- `app/src/features/panes/common.ts`: `PaneClick`, `paneClick`,
  `paneRow`, `paneEmpty`.
- `docs/specs/ui.md`, "Right sidebar panes" (Outgoing links).
- The given test `app/src/features/links/OutgoingPane.test.tsx`.
- `docs/tasks/EXECUTOR.md`, especially the TypeScript conventions.

## Contract

Keep every exported name and signature; replace the stubs and each
file's "Task T-0054 …" sentence with a description.

**`outgoingLinks(links, self)`**, in `links`' order:

- Skip a link whose `target` is empty (it names the note itself) or
  whose `resolved` is `self`.
- A link counts once per target: a resolved one by its `resolved` path,
  an unresolved one by its `target` ignoring case. Later links to a
  target already listed are skipped.
- A resolved link's row is `{ label, path: resolved, target }`, its
  `label` the last segment of `resolved` with a final `.md` (any case)
  dropped; an unresolved one's is `{ label: target, path: null, target
  }`, as first written.
- Return the resolved rows, then the unresolved rows, each in the order
  first met.

**`OutgoingPane`**:

- `links === null`: a paragraph "No note is open." (`paneEmpty`);
  empty: "No links found." (`paneEmpty`).
- Otherwise a `ul` with `aria-label="Outgoing links"`, 4px vertical
  padding, one `li` per row (keyed by its path, or the target for an
  unresolved one) holding a `button` with the classes `paneRow`, 8px
  side padding, the width less its insets, left-aligned, `title` the
  path (or the target for an unresolved one), and the label in a
  truncating `span`; an unresolved row's button also has
  `text-tertiary`.
- A click calls `onOpen(link, paneClick(e))`; a middle click
  (`onAuxClick` with `button` 1) calls it too, with `middle` true.

## Tests (given, do not edit)

`app/src/features/links/OutgoingPane.test.tsx`: `outgoingLinks`' order,
de-duplication, labels and self-links; the pane's rows, titles and
dimming; clicks with modifiers and the middle button; the empty and
no-note texts.

## Gotchas

- Compare unresolved targets ignoring case, but keep the first spelling
  in the row.
- The pane is presentational: no stores, no client
  (`docs/tasks/EXECUTOR.md`).

## Out of scope

Calling `note.links`, opening or creating notes (the planner's
container), the Backlinks pane (T-0057), and every file except the two
under `touch`.

## Done when

`make accept T=0054`, `make check` and `make ui-check` pass, and only
the two files under `touch` changed.
