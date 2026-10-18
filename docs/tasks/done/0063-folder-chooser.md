---
id: "0063"
title: Build the folder chooser for Move file to
milestone: M3
size: S
touch:
  - app/src/lib/folders.ts
  - app/src/features/prompt/FolderChooser.tsx
given:
  - app/src/features/prompt/FolderChooser.test.tsx
acceptance: make ui-vitest F=src/features/prompt/FolderChooser.test.tsx
---
# T-0063: Build the folder chooser for Move file to

## Goal

More options' and the Files pane's "Move file to…" and "Move folder
to…" ask for a folder as Obsidian does: a prompt over the vault's
folders, matched by the fuzzy search (`docs/specs/ui.md`, Files pane;
Prompts). Build its results as a pure function and its component over
T-0049's prompt frame.

## Read first

- `app/src/lib/folders.ts` and `app/src/features/prompt/FolderChooser.tsx`:
  the types and the stubs.
- `app/src/lib/fuzzy.ts`: `prepareFuzzySearch`, `matchPath`,
  `sortByScore`.
- `app/src/features/prompt/Prompt.tsx` (`Prompt`, `highlight`) and
  `app/src/features/prompt/QuickSwitcher.tsx`, a prompt built the same
  way.
- `docs/specs/ui.md`, "Prompts".
- The given test `app/src/features/prompt/FolderChooser.test.tsx`.
- `docs/tasks/EXECUTOR.md`, especially the TypeScript conventions.

## Contract

Keep every exported name and signature; replace the stubs and each
file's "Task T-0063 …" sentence with a description.

- **`folderText(path)`:** `"/"` for `""` (the root), else the path.
- **`folderResults(folders, query)`:** the folders without duplicates
  or `""`, sorted with `localeCompare`. With the query blank (trimmed
  empty): the root (`{ path: "", match: null }`), then every folder as
  `{ path, match: null }`. Else, with one `prepareFuzzySearch` of the
  trimmed query, the root when `search("/")` matches, then each folder
  that `matchPath(search, path)` matches, as `{ path, match }`,
  `sortByScore`d (equal scores keep this order).
- **`FolderChooser`** keeps the query in state and draws `Prompt` with
  `label="Move to folder"`, `placeholder` from its props, the items
  `folderResults(folders, query)` (memoized), `itemKey` `folder:<path>`,
  each row a truncating `span` (`text-sidebar-row`) holding
  `highlight(folderText(path), match?.matches ?? [])`. Enter or a click
  with an item: `onClose()`, then `onChoose(path)`; with none, nothing.
  `onClose` passes to the frame. The footer: `↑↓` "to navigate", `↵`
  "to move", `esc` "to dismiss". `emptyText` "No folders found.".

## Tests (given, do not edit)

`app/src/features/prompt/FolderChooser.test.tsx`: the blank list,
matching and spans, the root for "/", `folderText`; the dialog, rows,
highlighting, moving after closing, the root's row, no match, the
footer.

## Gotchas

- The root's spans index "/" as shown; a folder's index its path, as
  `matchPath` gives them.
- The given component test renders T-0049's real `Prompt`.
- Presentational: no stores, no client (`docs/tasks/EXECUTOR.md`).

## Out of scope

Moving anything, opening the chooser (the planner's), and every file
except the two under `touch`.

## Done when

`make accept T=0063`, `make check` and `make ui-check` pass, and only
the two files under `touch` changed.
