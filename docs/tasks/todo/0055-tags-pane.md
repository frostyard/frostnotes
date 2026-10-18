---
id: "0055"
title: Draw the Tags pane over nested tag counts
milestone: M3
size: M
touch:
  - app/src/lib/tagTree.ts
  - app/src/features/tags/TagsPane.tsx
given:
  - app/src/features/tags/TagsPane.test.tsx
acceptance: make ui-vitest F=src/features/tags/TagsPane.test.tsx
---
# T-0055: Draw the Tags pane over nested tag counts

## Goal

The right sidebar's Tags pane lists every tag in the vault with its
count, nested at "/", sorted by name or by count, and searches a tag
when it is clicked (`docs/specs/ui.md`, Right sidebar panes). Nest
`note.tags`' counts into a tree as a pure function, and draw it.

## Read first

- `app/src/lib/tagTree.ts` and `app/src/features/tags/TagsPane.tsx`: the
  types and the stubs.
- `app/src/rpc/gen/api.ts`: `TagCount` (nested tags come whole,
  `a/b`; parents only when a note has the parent itself).
- `app/src/features/panes/common.ts`: `CountSort`, `paneToolbar`,
  `paneEmpty`.
- `app/src/features/chrome/IconButton.tsx`: the toolbar's button.
- `docs/specs/ui.md`, "Right sidebar panes" (Tags).
- The given test `app/src/features/tags/TagsPane.test.tsx`.
- `docs/tasks/EXECUTOR.md`, especially the TypeScript conventions.

## Contract

Keep every exported name and signature; replace the stubs and each
file's "Task T-0055 …" sentence with a description.

**`tagTree(counts, sort)`:**

- Split each tag at "/" (ignoring empty segments). Its segments make
  nodes from the root down; segments match existing nodes ignoring
  case, so `Garden` and `garden/fruit` share the node `Garden`. A
  node's `name` is its segment and its `tag` its path from the root,
  each segment as first met (`Garden/fruit` above). A node made only as
  a parent has no notes of its own.
- A node's `count` is the counts of the tags it stands for (its own,
  summed if several spellings) plus its children's counts.
- Siblings sort at every level: `"name"` by name ignoring case
  (A to Z); `"count"` by count, most first, then by name.
- An empty list gives `[]`.

**`TagsPane`:**

- A toolbar (`paneToolbar`) with one `IconButton` of size 24, "Change
  sort order", Lucide `ArrowUpNarrowWide` 16px; a click calls
  `onSortMenu(left, bottom)` with the button's
  `getBoundingClientRect()`.
- `tags === null`: nothing under the toolbar. Empty: a paragraph "No
  tags found." (`paneEmpty`).
- Otherwise a `div` with `role="tree"`, `aria-label="Tags"`: the top
  nodes in order, each followed by its children when it is open (all
  are closed at first; the pane keeps which are open, by tag ignoring
  case).
- **A row:** a `div` with `role="treeitem"`, `tabIndex={-1}`,
  `aria-level` depth + 1, `aria-expanded` only for a node with
  children; `text-sidebar-row`, inset 8, 28 high, rounded 6, hover
  `bg-selection-inactive`, `style.paddingLeft` `4 + 16 × depth`. In it:
  a 16-wide column with, for a node with children, a `button`
  (`tabIndex={-1}`) named "Expand #<tag>" or "Collapse #<tag>" with a
  14px `ChevronRight` (`rotate-90` while open), which opens or closes
  the node; then a `button` with `title` "#<tag>", taking the remaining
  width, holding the name in a truncating `span` and the count in a
  `span` with `text-status text-secondary`; a click calls
  `onChoose(tag)`. The row's text is name then count ("Garden7").

## Tests (given, do not edit)

`app/src/features/tags/TagsPane.test.tsx`: `tagTree`'s nesting, counts,
spellings and both sorts; the pane's rows, opening and closing, levels
and padding; choosing a tag; the sort button; empty and loading.

## Gotchas

- Join a child's `tag` from its parent's `tag`, not from the spelling
  in the count you are reading: `garden/fruit` under `Garden` is
  `Garden/fruit`.
- A button's accessible name joins its spans with a space ("Garden 7").
- The pane is presentational: no stores, no client
  (`docs/tasks/EXECUTOR.md`).

## Out of scope

Calling `note.tags`, the sort order's menu and searching (the
planner's), the All properties pane (T-0056), and every file except the
two under `touch`.

## Done when

`make accept T=0055`, `make check` and `make ui-check` pass, and only
the two files under `touch` changed.
