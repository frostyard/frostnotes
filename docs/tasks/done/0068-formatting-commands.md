---
id: "0068"
title: Write Obsidian's formatting commands for the editor
milestone: M3
size: M
touch:
  - app/src/editor/format.ts
given:
  - app/src/editor/format.test.ts
acceptance: make ui-vitest F=src/editor/format.test.ts
---
# T-0068: Write Obsidian's formatting commands for the editor

## Goal

Obsidian's editing hotkeys (Ctrl+B, Ctrl+I, Ctrl+K, Ctrl+L, Ctrl+/,
Ctrl+D) and its formatting commands, as CodeMirror `StateCommand`s
that the app's command registry runs on the active editor
(`docs/specs/ui.md`, Keyboard map).

## Read first

- `app/src/editor/format.ts`: the stubs.
- `docs/specs/ui.md`, "Keyboard map".
- CodeMirror's `StateCommand`, `EditorState.changeByRange`,
  `EditorState.wordAt`, `EditorState.changes` and
  `EditorSelection.map` (`@codemirror/state`).
- The given test: its texts show a cursor as `|` and a selection as
  `{…}`.

## Contract

Keep every exported name and signature; replace the file's
"Task T-0068 …" sentence with a description. Every command returns
false and does nothing when `state.readOnly`; else it dispatches one
transaction (`userEvent: "input.format"`, `scrollIntoView: true`) and
returns true.

**Marks.** `toggleBold` (`**`), `toggleItalics` (`*`),
`toggleStrikethrough` (`~~`), `toggleHighlight` (`==`) and `toggleCode`
(`` ` ``) toggle their mark for each selection range
(`changeByRange`):

- A non-empty range: when the mark is right outside it on both sides,
  take those away (the range keeps its text selected); else when the
  range's own text starts and ends with the mark and is longer than two
  marks, take those away; else wrap the range (it keeps its text
  selected).
- An empty range: when the mark is right before and right after the
  cursor, take both away; else when the cursor is in or at the edge of
  a word (`state.wordAt`), toggle the mark around the word as above,
  the cursor keeping its place in the word; else insert two marks, the
  cursor between them.
- Bold and italics read the run of `*`s on each side (up to three) as
  one: with n the shorter run, bold is on when n ≥ 2 and italics when n
  is odd. Taking either away takes its stars nearest the text; so
  `***x***` loses bold to `*x*` and italics to `**x**`, and toggling
  italics on `**x**` makes `***x***`.

**`toggleComment`** toggles `%%`: a non-empty range as the marks above;
an empty one comments its line from its first non-space character to
its end, or, when that text is `%%…%%`, takes them away, the cursor
keeping its place in the text.

**`insertLink`** for each range: empty, inserts `[]()` with the cursor
in the brackets; a selection that is a URL
(`^[a-z][a-z0-9+.-]*://\S+$`, any case) becomes `[](URL)` with the
cursor in the brackets; any other selection becomes `[text]()` with
the cursor in the parentheses.

**Lines.** These act on every line any range touches, once each:

- `toggleChecklist`: a task (`- [?] `, any list marker `-`, `*`, `+`,
  `1.` or `1)`, after any indentation) gets `x` for a space and a space
  for anything else; a list item without a checkbox gets `[ ] ` after
  its marker (and a space after a bare marker); any other line gets
  `- [ ] ` after its indentation.
- `deleteParagraph`: deletes the line with its line break (the one
  after it, or for the last line the one before); a lone line is
  emptied.
- `setHeading(level)`: replaces the line's heading marks (`#` to
  `######` then spaces or the line's end, after up to three spaces) or
  inserts at its start (after its indentation) `level` `#`s and a
  space; `level` 0 takes the marks away. A line already at that level
  is left alone.

`toggleChecklist` and `setHeading` map the selection so that a cursor
or a range's end where text is inserted moves past it
(`selection.map(changes, 1)`).

## Tests (given, do not edit)

`app/src/editor/format.test.ts`: each command on selections, cursors in
and out of words, empty pairs, several ranges, stars of bold and
italics, URL and text links, tasks and list items, deleting first,
middle and last lines, headings, and a read-only state.

## Gotchas

- `changeByRange` takes positions in the old document for `changes`
  and in the new document for `range`.
- Build changes of one transaction in order, none overlapping:
  collect a line command's changes per line, lines sorted.

## Out of scope

Registering the commands and their keys (the planner's), and every file
except `app/src/editor/format.ts`.

## Done when

`make accept T=0068`, `make check` and `make ui-check` pass, and only
`app/src/editor/format.ts` changed.
