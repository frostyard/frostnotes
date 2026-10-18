---
id: "0059"
title: Draw the Files pane's toolbar and tree
milestone: M3
size: L
touch:
  - app/src/features/explorer/FilesPane.tsx
given:
  - app/src/features/explorer/FilesPane.test.tsx
acceptance: make ui-vitest F=src/features/explorer/FilesPane.test.tsx
---
# T-0059: Draw the Files pane's toolbar and tree

## Goal

The left sidebar's Files pane is Obsidian's file explorer: a toolbar and
the vault's folders and files as a tree, with keys, renaming in place
and drag and drop (`docs/specs/ui.md`, Files pane). Draw it over
T-0040's rows; every action is a command its container carries out.

## Read first

- `app/src/features/explorer/FilesPane.tsx`: `DRAG_TYPE`,
  `FilesCommand`, `FilesPaneProps` and the stub.
- `app/src/lib/fileTree.ts`: `TreeRow` (T-0040).
- `app/src/features/panes/common.ts`: `PaneClick`, `paneClick`,
  `paneToolbar`.
- `app/src/features/chrome/IconButton.tsx`.
- `docs/specs/ui.md`, "Files pane".
- The given test `app/src/features/explorer/FilesPane.test.tsx`.
- `docs/tasks/EXECUTOR.md`, especially the TypeScript conventions.

## Contract

Keep every exported name, value and signature; replace the stub and the
file's "Task T-0059 …" sentence with a description of the pane.

**The pane** is a column filling its space: the toolbar, then the tree
(or the empty vault's text).

**The toolbar** (`paneToolbar`), `IconButton`s of size 24 with 16px
Lucide icons, in order: "New note" (`SquarePen`, keys "Ctrl + N") →
`{ kind: "newNote" }`; "New folder" (`FolderPlus`) → `newFolder`;
"Change sort order" (`ArrowUpNarrowWide`) → `{ kind: "sortMenu", x, y }`
with the button's `getBoundingClientRect()` `left` and `bottom`; then
"Collapse all" (`ChevronsDownUp`) → `collapseAll` while `anyExpanded`,
else "Expand all" (`ChevronsUpDown`) → `expandAll`.

**An empty vault** (`loaded` and no rows): instead of the tree, centered
in a column 8px apart with 32px vertical padding, "No notes yet"
(`text-empty text-secondary`) and a `button` "New note" (`text-empty
text-accent`) → `newNote`. Before `loaded`, an empty list draws an empty
tree.

**The tree:** a `div` with `role="tree"`, `aria-label="Files"`,
`tabIndex={-1}`, scrolling, 8px bottom padding, one row per entry of
`rows` keyed by path.

**A row:** a `div` with `role="treeitem"`, `aria-level` depth + 1,
`aria-expanded` for a folder only, `aria-selected` (true for the row
whose path is `active`), `aria-label` the row's `name`, `data-path` its
path, `title` its path, `draggable` unless it is being renamed.
`text-sidebar-row`, relative, inset 8, 28 high, rounded 6;
`bg-selection-sidebar` for the active row, else hover
`bg-selection-inactive`; `style.paddingLeft` `4 + 16 × depth`. In it:

1. with `indentGuides`, one guide per level above the row (`depth` of
   them): a `span` with `data-guide`, `aria-hidden`, absolutely placed
   over the row's height, 1px wide, `bg-separator`, at `left` `4 + 16 ×
   level + 8` pixels;
2. a 16-wide column (`text-secondary`) holding, for a folder, a 14px
   `ChevronRight`, `rotate-90` while expanded;
3. the name in a truncating `span`; for a file whose `type` is not
   `"note"` and that has an extension, a badge after it (6px left
   margin): a `span` with `text-keycap`, `rounded` (4), 4px side
   padding, `bg-badge text-badge-text uppercase`, holding the extension
   as `TreeRow` gives it (lower case; the class shows it upper case).
   The row's text is name then extension ("Seedspng").

**Roving focus:** one row has `tabIndex` 0, the others -1: the row last
focused or clicked if it is still listed, else the active row, else the
first. A row's `onFocus` makes it that row.

**Pointer:**

- a click on a folder → `{ kind: "toggle", path }`; on a file →
  `{ kind: "open", path, click: paneClick(e) }`. Either also makes the
  row the focused one;
- a middle click (`onAuxClick`, `button` 1) on a file → `open` with
  `paneClick(e)` (its `middle` is true); on a folder nothing;
- a right click on a row prevents the default, stops propagation and
  sends `{ kind: "menu", path, folder, x: clientX, y: clientY }`; one
  on the tree's own empty space (the event's target is the tree) sends
  `{ kind: "menu", path: null, folder: true, x, y }`.

**Keys** (`onKeyDown` on the tree), acting on the row the event comes
from (the closest `[data-path]` of its target), ignoring events from an
`input`. Each key below calls `preventDefault()` and
`stopPropagation()`:

- ArrowDown, ArrowUp: focus the next or previous row (not past either
  end); Home, End: the first or last;
- ArrowRight: on a closed folder → `toggle`; on an open folder whose
  next row is deeper → focus that row; on a file nothing;
- ArrowLeft: on an open folder → `toggle`; else focus the nearest
  earlier row one level up (its parent), if any;
- Enter: on a folder → `toggle`; on a file → `open` with `{ ctrl:
  e.ctrlKey, alt: e.altKey, middle: false }`;
- F2 → `{ kind: "rename", path }`.

Focusing a row moves the browser's focus to its element once the tree
has drawn it.

**Renaming** (`renaming` is the row's path): in place of the name, an
`input` named "Name" (`aria-label`), holding the row's `name`, focused
with all its text selected on mount, `spellCheck={false}`, 24 high,
rounded 4, 1px `border-accent` (`border-flag-1` while invalid),
`bg-window`, taking the remaining width. Its key and click events stop
propagating. A name holding "/" is invalid: `aria-invalid="true"`.
Enter (prevented): when valid, `{ kind: "renamed", path, name }` with
the name trimmed; when invalid, nothing. Escape (prevented) →
`{ kind: "cancelRename" }`. Leaving the field (blur) commits as Enter
does when the name is valid and not blank, else cancels. The field sends
one command at most, however often it blurs.

**Drag and drop:**

- `onDragStart` on a row sets `DRAG_TYPE` to `JSON.stringify([path])`
  and `text/plain` to the path, and `effectAllowed` to `"all"`.
- A drag carrying `DRAG_TYPE` over a folder row targets that folder;
  over a file row, the file's folder (its path before the last "/", ""
  at the root); over the tree's empty space, "" (the root). For such a
  drag, `onDragOver` prevents the default, stops propagation, sets
  `dropEffect` to `"move"`, and marks the target: a folder row that is
  the target gets the classes `ring-1 ring-accent ring-inset`. A drag
  without `DRAG_TYPE` is left alone (no `preventDefault`).
- `onDrop` reads `DRAG_TYPE`, clears the mark and, when it holds a JSON
  array of strings, sends `{ kind: "drop", paths, folder }` for the
  target. Anything else sends nothing.
- Leaving the tree (`onDragLeave` to outside it) clears the mark.

**Scrolling:** when `active` changes, scroll its row into view
(`scrollIntoView({ block: "nearest" })`, called only if the element has
it).

## Tests (given, do not edit)

`app/src/features/explorer/FilesPane.test.tsx`: rows, levels, padding,
chevrons and badges; the active row and roving focus; guides; clicks;
the arrows, Home and End; → and ←; Enter and F2; renaming, its refusal
and its blur; context menus; dragging and dropping; foreign drags; the
toolbar; the empty vault.

## Gotchas

- Act on the row whose element sent the key event: tests move focus
  with `element.focus()`, which a React state of your own does not see.
- Biome: a `treeitem` with `onClick` and no key handler of its own
  trips `useKeyWithClickEvents`; the tree's handler serves it, and a
  `biome-ignore` saying so is acceptable on the row only. Guides keyed
  by an array index trip `noArrayIndexKey`: map over
  `[...Array(depth).keys()]` instead.
- happy-dom's `DataTransfer` is the test's own object: use `types`,
  `setData`, `getData`, `effectAllowed` and `dropEffect` only.
- The pane is presentational: no stores, no client, no workspace
  imports (`docs/tasks/EXECUTOR.md`).

## Out of scope

What the commands do (creating, moving, renaming, menus, sorting), the
rows themselves (T-0040), dropping a file on an editor, and every file
except `app/src/features/explorer/FilesPane.tsx`.

## Done when

`make accept T=0059`, `make check` and `make ui-check` pass, and only
`app/src/features/explorer/FilesPane.tsx` changed.
