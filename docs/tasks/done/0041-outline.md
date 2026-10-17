---
id: "0041"
title: Nest a note's headings into the outline
milestone: M3
size: S
touch:
  - app/src/lib/outline.ts
given:
  - app/src/lib/outline.test.ts
acceptance: make ui-vitest F=src/lib/outline.test.ts
---
# T-0041: Nest a note's headings into the outline

## Goal

The Outline pane shows the active note's headings as a tree, live as
the user types (`docs/specs/ui.md`, Right sidebar panes). The editor
reads the headings from its syntax tree; this card nests them as
Obsidian 1.14.4 does, flattens the tree into the pane's rows with some
headings folded, and finds the heading a cursor position is under.

## Read first

- `app/src/lib/outline.ts`: `Heading`, `OutlineNode`, `OutlineRow` and
  the stubs.
- `docs/specs/ui.md`, "Right sidebar panes" (Outline).
- The given test `app/src/lib/outline.test.ts`.
- `docs/tasks/EXECUTOR.md`, especially the TypeScript conventions.

## Contract

Keep every exported name and signature; replace the stubs and the file's
"Task T-0041 …" sentence with a description of the outline.

**`buildOutline(headings)`**, headings in document order: each heading
is nested under the nearest heading before it whose level is lower;
with none, it is at the top. Keep a stack of open nodes: for each
heading, pop every node whose level is at least the heading's, add the
heading to the top node's children (or to the roots when the stack is
empty), then push it. So an H4 after an H1 nests under the H1, an H2
after an H3 at the top stays at the top, and headings of one level are
siblings. Each node holds the given `Heading` object itself, not a copy.

**`outlineRows(nodes, folded)`**: the nodes depth first, each as a row
`{ heading, depth, hasChildren, expanded }`: `depth` 0 at the top and one
more per level of nesting; `hasChildren` when it has children;
`expanded` when it has children and its heading's `from` is not in
`folded`. A row's children follow it only when it is `expanded`. A fold
of a heading without children changes nothing.

**`headingAt(headings, pos)`**: the index of the last heading whose
`from` is at most `pos`, or -1 when there is none (`pos` before the first
heading, or no headings). Headings are in document order, so search them
by bisection: a note may have thousands.

## Tests (given, do not edit)

`app/src/lib/outline.test.ts`: nesting, skipped levels, headings before
any lower one, siblings, folding, rows, and `headingAt` at the edges and
over 5,000 headings.

## Gotchas

- Pop while the top's level is greater than **or equal to** the
  heading's: two H2s are siblings, not parent and child.
- `folded` holds `from` offsets, not indexes.

## Out of scope

Reading headings from the editor, drawing the pane, scrolling to a
heading, and every file except `app/src/lib/outline.ts`.

## Done when

`make accept T=0041`, `make check` and `make ui-check` pass, and only
`app/src/lib/outline.ts` changed.
