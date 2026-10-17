---
id: "0023"
title: Build the MCP write tools
milestone: M1
size: S # S: at most 150 non-test lines in 3 files; M: 400 lines in 6 files; L: 1,000 lines, one package or feature (ADR-0010).
touch:
  - internal/mcpserver/write.go
given:
  - internal/mcpserver/write_test.go # copied from 0023-mcp-write-tools/_given/ when the task starts
acceptance: go test ./internal/mcpserver -run 'Test(CreateNote|EditNote|AppendToDailyNote)' -count=1
---
# T-0023: Build the MCP write tools

## Goal

An agent creates notes, edits them and appends to the daily note through
`notesctl mcp` (ADR-0012). Every write is an ordinary write with a base,
attributed to the MCP client, so the user sees it in the history and can
revert it (ADR-0006). This card replaces the three write handlers in
`internal/mcpserver/write.go`, which answer "not built" now. Runs after
T-0021 (its test helpers) and T-0022 (the edits).

## Read first

- docs/tasks/EXECUTOR.md
- docs/specs/mcp.md: "Write tools" and "Edits".
- internal/mcpserver/mcpserver.go: `tools`, `agentWrite`.
- internal/mcpserver/types.go: `createIn`, `createOut`, `editIn`,
  `editOut`, `dailyIn`, `dailyOut`.
- internal/mcpserver/edit.go (T-0022): `applyEdit`, `appendText`.
- api/zz_generated.go: `NoteClient.Create`, `Read`, `Write`, `Daily`,
  their params, `WriteResult`, `DailyNote`.
- internal/mcpserver/write_test.go (given), and harness_test.go and
  read_test.go for the helpers it uses.

## Contract

Each handler keeps its signature and returns `nil` for the
`*sdk.CallToolResult`; a failure is the returned error, notesd's
`*api.Error` as it is. Every call that writes passes `Source` and `Author`
from `agentWrite(req)`, and `VaultID: t.o.VaultID`.

- **`createNote`**: a path that does not end in `.md` is an error whose
  text contains `.md`, before calling notesd. Calls `note.create` with the
  path and content; returns `{path, hash}` from the `WriteResult`.
- **`editNote`**: calls `note.read`, then `applyEdit(note.Content, in)`; an
  error from either is returned, and nothing is written. Then calls
  `note.write` with the note's path, `Base` the hash `note.read` returned,
  and the edited content. Returns `{path, hash, merged, conflicts}` from
  the `WriteResult`.
- **`appendToDailyNote`**: an empty `Text` is an error. Calls `note.daily`
  with `Date` set only when `in.Date` is not empty, and with `Source` and
  `Author` (so a daily note the call creates is the agent's version too).
  Then calls `note.write` on the daily note's path with `Base` its hash and
  `appendText(content, in.Text)` as the content. Returns
  `{path, hash, created, merged}`: `created` from the `DailyNote`, the rest
  from the `WriteResult`.

## Tests (given, do not edit)

`internal/mcpserver/write_test.go`: `TestCreateNote`, `TestEditNote`,
`TestAppendToDailyNote`.

## Gotchas

- Never write without the base `note.read` (or `note.daily`) returned, and
  never compute an edit on anything but the content it returned.
- Use `agentWrite(req)`; the tests check each version's source and author.
- `note.daily` gives the note in `DailyNote.Note`.

## Out of scope

The read tools, the edits, `mcpserver.go`, `types.go` and every other
file.

## Done when

`make accept T=0023` passes, `make check` passes, and only
`internal/mcpserver/write.go` changed.
