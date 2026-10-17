---
id: "0040"
title: Build the Files pane's rows as Obsidian sorts them
milestone: M3
size: M
touch:
  - app/src/lib/fileTree.ts
given:
  - app/src/lib/fileTree.test.ts
acceptance: make ui-vitest F=src/lib/fileTree.test.ts
---
# T-0040: Build the Files pane's rows as Obsidian sorts them

## Goal

The Files pane draws a vault's folders and files as a tree
(`docs/specs/ui.md`, Files pane). Its rows come from a pure function:
the vault's files and folders nested, sorted as Obsidian 1.14.4's file
explorer sorts them, files Obsidian cannot open left out unless asked
for, and flattened by which folders are open, so the pane can draw a
plain list. Implement it, with the helpers that name a file's extension
and type.

## Read first

- `app/src/lib/fileTree.ts`: `FileType`, `TreeFile`, `SortOrder`,
  `TreeOptions`, `TreeRow` and the stubs.
- `docs/specs/ui.md`, "Files pane".
- The given test `app/src/lib/fileTree.test.ts`.
- `docs/tasks/EXECUTOR.md`, especially the TypeScript conventions.

## Contract

Keep every exported name and signature; replace the stubs and the file's
"Task T-0040 …" sentence with a description of the tree.

Paths are vault-relative and slash-separated, without a leading or
trailing `/`. A path's **name** is its last segment.

**`extensionOf(path)`**: in the name, the text after its last `.`, in
lower case; `""` when the name has no `.`, starts with its only `.`
(`.hidden`), or ends with one (`trailing.`). Dots in folders do not
count (`v1.2/notes` has none).

**`fileType(path)`** by `extensionOf`: `md` note; `canvas` canvas;
`base` base; `pdf` pdf; `bmp`, `png`, `jpg`, `jpeg`, `gif`, `svg`,
`webp`, `avif` image; `mp3`, `wav`, `m4a`, `3gp`, `flac`, `ogg`, `oga`,
`opus` audio; `mp4`, `webm`, `ogv`, `mov`, `mkv` video; anything else
other. Write the table as a constant.

**`ancestors(path)`**: the folders above path, outermost first:
`"a/b/c.md"` gives `["a", "a/b"]`, `"c.md"` gives `[]`.

**`treeRows(files, folders, options)`**:

- **Folders** are every entry of `folders` (empty ones included; `""`,
  the root, is not a row), every folder above them, and every folder
  above a file, each once. A folder shows even when all its files are
  hidden.
- **Files** of type other are left out unless `options.showUnsupported`.
- **A folder's rows** (the root's first): its subfolders, then its
  files. Each subfolder's row is followed, when its path is in
  `options.expanded`, by its own rows a level deeper. What lies in a
  folder that is not open never shows, whatever is open below it.
- **Subfolders** sort by name, always A to Z, whatever the sort order.
- **Files** sort by `options.sort`:
  - `alphabetical`: by the name without its extension, then, for equal
    ones (`Banana.md`, `banana.png`), by the whole name;
  - `alphabeticalReverse`: exactly the reverse of `alphabetical`;
  - `byModifiedTime`: newest `modifiedAt` first, equal times
    `alphabetical`;
  - `byModifiedTimeReverse`: oldest first, equal times `alphabetical`.
- **Names compare** with one constant
  `new Intl.Collator(undefined, { usage: "sort", sensitivity: "base", numeric: true })`
  (numbers by value, so "Note 2" before "Note 10"; case and accents
  ignored), and where it finds two names equal, by plain string order
  (`<`).
- **Rows:** a folder's row is `{ kind: "folder", path, name,
  extension: "", type: "folder", depth, expanded }`, `expanded` true
  when its path is in `options.expanded`. A file's row is `{ kind:
  "file", path, name, extension, type, depth, expanded: false }`, where
  `name` is the name without `.` and its extension when `extensionOf`
  found one (`archive.tar.gz` is "archive.tar"; `README` stays
  "README"). `depth` is 0 at the root and one more per folder.

It runs in time proportional to the files and folders, plus sorting: the
given test builds 10,101 rows.

## Tests (given, do not edit)

`app/src/lib/fileTree.test.ts`: the rows of a made-up vault closed and
open, hidden types, empty and dotted folders, an empty vault, a large
vault, each sort order, and the helpers.

## Gotchas

- Folders always come first and always sort A to Z: Obsidian reverses
  and times only files.
- The collator finds "Banana" and "banana" equal; the whole name with its
  extension, then plain string order, breaks such ties so every order is
  deterministic.
- Do not build lists by spreading them on every insert; group files and
  folders by parent once.

## Out of scope

Drawing the tree, keys, renaming and drag and drop (later cards and the
planner's containers), the vault's file listing, and every file except
`app/src/lib/fileTree.ts`.

## Done when

`make accept T=0040`, `make check` and `make ui-check` pass, and only
`app/src/lib/fileTree.ts` changed.
