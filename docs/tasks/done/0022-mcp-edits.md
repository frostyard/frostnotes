---
id: "0022"
title: Build edit_note's edits
milestone: M1
size: M # S: at most 150 non-test lines in 3 files; M: 400 lines in 6 files; L: 1,000 lines, one package or feature (ADR-0010).
touch:
  - internal/mcpserver/edit.go
given:
  - internal/mcpserver/edit_test.go # copied from 0022-mcp-edits/_given/ when the task starts
acceptance: go test ./internal/mcpserver -run 'Test(ReplaceOnce|AppendText|PrependText|SetProperties|ApplyEdit)' -count=1
---
# T-0022: Build edit_note's edits

## Goal

The MCP tool `edit_note` changes a note with one edit: replace a passage,
append, prepend, or set properties (docs/specs/mcp.md#edits). This card
writes those edits as pure functions on a note's content, so the write
tools (T-0023) only read, apply and write. Every byte an edit does not
change stays as it was (ADR-0003).

## Read first

- docs/tasks/EXECUTOR.md
- docs/specs/mcp.md, section "Edits".
- internal/mcpserver/types.go: `editIn`.
- internal/markdown/frontmatter.go: `SplitFrontmatter` and `Split`
  (`HasFront`, `Front`, `BodyStart`); internal/markdown/parse.go: `Parse`
  and `Doc.PropError`.
- internal/mcpserver/edit_test.go (given).

## Contract

Create `internal/mcpserver/edit.go` with these functions, on strings as
`note.read` returns content:

- **`applyEdit(content string, in editIn) (string, error)`**: exactly one
  of these is given: `OldText` (with `NewText`, which may be empty),
  `Append`, `Prepend`, `Properties` (non-nil). None, or more than one, is
  an error, and so is `NewText` without `OldText`, and an empty
  non-nil `Properties`. It then returns the one function's result below.
- **`replaceOnce(content, old, new string) (string, error)`**: `old`
  empty is an error containing `empty`; occurring zero times an error
  containing `does not occur`; more than once an error containing
  `N times` (`2 times`, …). Otherwise the occurrence becomes `new`.
- **`appendText(content, text string) string`**: when `content` is not
  empty and does not end with `\n`, a `\n` first; then `text`, then `\n`
  when `text` does not end with one.
- **`prependText(content, text string) string`**: `text`, followed by `\n`
  when it does not end with one, goes at `Split.BodyStart` of
  `SplitFrontmatter(content)`: just after the frontmatter's closing line,
  or at the start of a note without frontmatter, after a leading BOM
  (`"﻿"`) if there is one.
- **`setProperties(content string, props map[string]any) (string, error)`**:
  - A note whose frontmatter `Parse` reports a `PropError` for (not a
    mapping, invalid YAML, a repeated key) is an error.
  - A top-level key is a frontmatter line that does not start with a
    space, tab, `#` or `-`; its name is the key as YAML decodes it (the
    line `"due date": 2026-10-01` is the key `due date`). Its unit is that
    line and the lines after it that start with a space, tab or `-`, and
    blank lines followed by a line starting with a space or tab.
  - For each key of `props`, in byte order: a key whose unit exists has
    the unit replaced by the key rendered, or removed when its value is
    `nil`; a new key with a non-nil value is rendered at the end of the
    frontmatter, before the closing line; a new key with a nil value is
    nothing. Comments, blank lines and every other key's lines stay.
  - A key is rendered by `yaml.v3` (`go.yaml.in/yaml/v3`): an `Encoder`
    with `SetIndent(2)` encoding `map[string]any{key: value}`.
  - New and replaced lines end as the frontmatter's opening line does
    (`\r\n` after `---\r\n`, else `\n`).
  - A note without frontmatter gets `---\n`, the rendered keys and
    `---\n` at its start, after a leading BOM; when every value is `nil`
    it is returned unchanged.
  - A result whose frontmatter would not be a mapping (`PropError`, or
    none) is an error.

## Tests (given, do not edit)

`internal/mcpserver/edit_test.go`: `TestReplaceOnce`, `TestAppendText`,
`TestPrependText`, `TestSetProperties`, `TestApplyEdit`.

## Gotchas

- Never rebuild the frontmatter from a decoded map: that would reorder
  keys and drop comments. Edit lines.
- A string literal with a BOM is written `"﻿"`; never put the raw
  character in Go source (gofmt refuses it).
- `yaml.v3` quotes strings that would read as something else
  (`'[[Plan]]'`, `"2026-11-01"`, `"yes"`); keep its output as it is.
- JSON numbers arrive as `float64`: `float64(3)` renders `3`.
- The frontmatter's opening line follows a BOM when there is one.

## Out of scope

The tool handlers (T-0023) and every other file.

## Done when

`make accept T=0022` passes, `make check` passes, and only
`internal/mcpserver/edit.go` changed.
