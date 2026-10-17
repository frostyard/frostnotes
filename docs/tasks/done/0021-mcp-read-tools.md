---
id: "0021"
title: Build the MCP read tools
milestone: M1
size: M # S: at most 150 non-test lines in 3 files; M: 400 lines in 6 files; L: 1,000 lines, one package or feature (ADR-0010).
touch:
  - internal/mcpserver/read.go
given:
  - internal/mcpserver/read_test.go # copied from 0021-mcp-read-tools/_given/ when the task starts
acceptance: go test ./internal/mcpserver -run 'Test(ReadNote|SearchNotes|ListNotes|GetBacklinks)' -count=1
---
# T-0021: Build the MCP read tools

## Goal

An agent searches, reads and lists the user's notes, and finds their
backlinks, through `notesctl mcp` (ADR-0012). The server, its tool
definitions and the test harness exist; this card replaces the four read
handlers in `internal/mcpserver/read.go`, which answer "not built" now,
with ones that call notesd's `note` domain.

## Read first

- docs/tasks/EXECUTOR.md
- docs/specs/mcp.md: "Results" and "Read tools".
- internal/mcpserver/mcpserver.go: `tools` (its `o.Client` and
  `o.VaultID`), `Notice`, and how the handlers are registered.
- internal/mcpserver/types.go: `searchIn`, `searchOut`, `readIn`,
  `noteOut`, `listIn`, `listOut`, `backlinksIn`, `backlinksOut`.
- api/zz_generated.go: `NoteClient.Search`, `Read`, `List`, `Backlinks`,
  their params (`NoteSearchParams`, `NoteReadParams`, `NoteListParams`,
  `NoteBacklinksParams`), `SnippetPart`, `Note`, `NoteEntry`, `Backlink`.
- internal/mcpserver/harness_test.go and read_test.go (given).

## Contract

Each handler keeps its signature, calls `t.o.Client.Note()` once with
`VaultID: t.o.VaultID`, and returns `nil` for the `*sdk.CallToolResult`,
the output, and a nil error; a failure is the returned error (notesd's
`*api.Error` returned as it is), which the SDK turns into an error result.

- **`searchNotes`**: `Limit` 0 means 20; outside 1–100 is an error whose
  text contains `limit`, before calling notesd. Calls `note.search` with
  the query and the limit. Each hit is `{path, title, snippet}`, the
  snippet being the `SnippetPart`s' text joined, a part with `Match`
  between `**` and `**`. `Hits` is never nil (`[]` for none); `Notice` is
  `Notice`.
- **`readNote`**: calls `note.read`. `Properties` is `Note.Props` decoded
  into a `map[string]any` (`{}` when empty); `Tags` and `Aliases` are
  never nil; `Modified` is `ModifiedAt` in UTC formatted with
  `time.RFC3339`. Path, title, hash and content are `note.read`'s.
- **`listNotes`**: `Limit` 0 means 200; outside 1–1000 is an error whose
  text contains `limit`. Calls `note.list` with each of folder, tag,
  property and value that is not empty (as a pointer; empty ones stay
  nil). `Notes` holds the first `Limit` entries as
  `{path, title, modified}` (modified as for `readNote`), never nil;
  `Total` is how many `note.list` returned.
- **`getBacklinks`**: calls `note.backlinks`. Each is `{source, line,
  target, alias, embed}` from `Backlink.Source` and its `Link`; never nil.
- Every output's `Notice` is the constant `Notice`.

## Tests (given, do not edit)

`internal/mcpserver/read_test.go`: `TestReadNote`, `TestSearchNotes`,
`TestListNotes`, `TestGetBacklinks`. It also defines the helpers `call`,
`ok`, `fails`, `resultText` and `notesRead` that later cards' tests use.

## Gotchas

- A nil slice or map encodes as `null`, which the tests refuse: initialize
  `[]searchHit{}` and the like.
- `api.Note.Props` is `jsontext.Value`; decode it with
  `encoding/json/v2`'s `json.Unmarshal`.
- Pass notesd's errors through unwrapped or wrapped with `%w`: the tests
  look for `notFound` and `invalidParams` in the error text.
- The SDK validates input before the handler runs, but not ranges: check
  `Limit` yourself.

## Out of scope

The write tools (T-0023), `mcpserver.go`, `types.go`, the harness, and
`notesctl mcp`.

## Done when

`make accept T=0021` passes, `make check` passes, and only
`internal/mcpserver/read.go` changed.
