---
id: "0076"
title: Read and edit a note's properties in its frontmatter's text
milestone: M3
size: M
touch:
  - app/src/editor/properties.ts
given:
  - app/src/editor/properties.test.ts
acceptance: make ui-vitest F=src/editor/properties.test.ts
---
# T-0076: Read and edit a note's properties in its frontmatter's text

## Goal

The properties block edits a note's frontmatter as text, as MCP's
`edit_note` does in Go: it rewrites only the lines of the keys it
changes, so every other byte of the note stays (`docs/design/app.md`,
Properties; `docs/specs/mcp.md`, edit_note's Properties). Write the
reading, setting, removing and renaming, and a property's type.

## Read first

- `app/src/editor/properties.ts`: the types and stubs.
- `internal/mcpserver/edit.go`: `setProperties`, `editProperties`,
  `propertyUnits`, `propertyUnitEnd` and `renderProperty`, the rules to
  port; `internal/mcpserver/edit_test.go`, `TestSetProperties`, the
  cases the given test takes.
- The `yaml` package (2.9.1, in `app/package.json`): `parseDocument`
  (with `uniqueKeys`), `isMap`, a pair's `key.range`, `doc.errors`,
  `doc.toJS()`, and `stringify`.
- The given test.

## Contract

Keep every exported name and type; replace the file's "Task T-0076 …"
sentence with a description.

**The frontmatter** is at the start of the note, after a byte-order mark
(U+FEFF) if any: a line `---`, then its text, then a line `---`
(`/^---(\r?\n)(?:([\s\S]*?\r?\n))?---(?:\r?\n|$)/`). Its lines end with
`\r\n` when its first line does, else `\n`. Its text parses with
`parseDocument(text, { uniqueKeys: true })`; it cannot be properties
when the document has an error, when its contents are not a map (an
empty document is an empty map), or when the map is a flow map
(`{a: 1}`).

**`readFrontmatter(text)`:** `{ kind: "none" }` without frontmatter;
`{ kind: "error", message }` when it cannot be properties;
`{ kind: "properties", properties }` with each key and its value
(`toJS()`), in order.

**`setProperties(text, props)`** returns the new text or an `Error`:

- A key's lines: the line its key starts on (a key on a line that starts
  with a space, a tab, `#` or `-` is not one), and the lines after it
  that start with a space, a tab or `-`, and blank lines between such
  lines (not blank lines before a line that does not).
- For each key of `props`, in order of their names: its value rendered
  as `stringify({ [key]: value }, { indent: 2, lineWidth: 0 })`, line
  endings made the frontmatter's, or nothing for `null`. A key that
  exists has its first line replaced by that and the rest of its lines
  emptied; a key that does not goes after the last line of the text.
- With frontmatter, the result is the note with its frontmatter's text
  replaced. Without, nothing to add leaves the note as it is; else
  `---\n` + the added lines + `---\n` go before the note (after a
  byte-order mark).
- An error when the frontmatter cannot be properties, or when the
  result's cannot.

**`renameProperty(text, from, to)`:** the same text when `from === to`;
an `Error` when the frontmatter cannot be properties, has no `from`, or
has a `to` already; else `from`'s lines replaced by `to` with `from`'s
value, rendered as above, in place.

**`propertyType(key, value, vaultType = "")`:** `vaultType` when it is
one of the eight `PropertyType`s; else `tags` or `aliases` for those
keys in any case; else `multitext` for an array, `checkbox` for a
boolean, `number` for a number, `date` for a string
`YYYY-MM-DD`, `datetime` for `YYYY-MM-DDTHH:mm` with optional seconds,
and `text` for anything else.

## Tests (given, do not edit)

`app/src/editor/properties.test.ts`: MCP's cases (in place, removal, a
quoted key, new keys, new and empty frontmatter, CRLF, nested values, a
byte-order mark, refusals), reading, renaming and types.

## Gotchas

- `yaml` quotes a string that needs it in double quotes and writes a
  date-like string plain; the given tests expect exactly that, which is
  how Obsidian writes them (Go's yaml.v3 differs).
- `key.range[0]` is an offset in the frontmatter's text: find its line
  from the offsets where lines start.
- Write a byte-order mark in source as `﻿`, never as the character.

## Out of scope

The properties block and its container, and every file except
`app/src/editor/properties.ts`.

## Done when

`make accept T=0076`, `make check` and `make ui-check` pass, and only
`app/src/editor/properties.ts` changed.
