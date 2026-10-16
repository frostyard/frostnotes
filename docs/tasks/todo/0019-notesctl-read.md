---
id: "0019"
title: Add notesctl ls, cat, search and backlinks
milestone: M1
size: M # S: at most 150 non-test lines in 3 files; M: 400 lines in 6 files; L: 1,000 lines, one package or feature (ADR-0010).
touch:
  - cmd/notesctl/notes.go
  - cmd/notesctl/root.go
given:
  - cmd/notesctl/notes_test.go # copied from 0019-notesctl-read/_given/ when the task starts
acceptance: go test ./cmd/notesctl -run 'TestNote(VaultChoice|List|Cat|Search|Backlinks)' -count=1
---
# T-0019: Add notesctl ls, cat, search and backlinks

## Goal

`notesctl` reads notes over the `note` domain: list them, print one,
search, and show what links to a file. These are M1's terminal tools
before the app exists, and the commands an agent's operator checks MCP's
work with. A `--vault` flag picks the vault. The tests run against a fake
note service through `rpctest.Options.Note`, so the card needs no engine.

## Read first

- docs/tasks/EXECUTOR.md
- docs/specs/rpc-api.md, section "note": `note.list`, `note.read`,
  `note.search`, `note.backlinks` and their types.
- cmd/notesctl/root.go, vault.go and hello_test.go: the command pattern
  (`newXxxCmd(opts)`, `opts.dial`, `clix.OutputJSON`, `runNotesctl`).
- api/zz_generated.go: `NoteClient`, the `Note…Params` types,
  `LinkKind…` constants.
- internal/rpctest/rpctest.go: `Options.Note`.
- cmd/notesctl/notes_test.go (given): `fakeNotes`, `notesServer`.

## Contract

**`root.go`**: `rootOptions` gains `vault string`, bound to a persistent
flag `--vault` ("The vault, by name or ID (default: the only one)"), and
the root adds, after the existing commands, `newLsCmd(opts)`,
`newCatCmd(opts)`, `newSearchCmd(opts)` and `newBacklinksCmd(opts)`.

**`notes.go`** holds those four and the vault choice, which T-0020's
commands reuse:

```go
// vaultID picks the vault --vault names, from vault.list.
func (o *rootOptions) vaultID(ctx context.Context, c *api.Client) (int64, error)
// withVault dials notesd, picks the vault and runs fn; it closes the client.
func (o *rootOptions) withVault(cmd *cobra.Command, fn func(c *api.Client, vault int64) error) error
```

- Without `--vault`: the only vault. None: an error containing `notesctl
  vault add`. Several: an error containing `--vault`.
- With `--vault V`: when V parses as an integer and a vault has that ID,
  that one; otherwise the vaults whose name equals V ignoring case
  (`strings.EqualFold`): exactly one is chosen, none or several is an
  error that quotes V. No note method is called when the choice fails.

Commands (each prints its result with `clix.OutputJSON` first, as `vault
list` does, and returns notesd's errors as they come):

| Command | Calls | Prints |
| --- | --- | --- |
| `ls [FOLDER] [--tag T] [--prop KEY[=VALUE]]` | `note.list`; `Folder`, `Tag`, `Property` and `Value` set only when given; `--prop` splits at its first `=` | each path on its own line; nothing for none |
| `cat PATH` | `note.read` | the content exactly, with nothing added |
| `search QUERY... [--limit N]` | `note.search`; `Query` = the arguments joined with one space; `Limit` only when `--limit` is given | `PATH<TAB>SNIPPET` per hit |
| `backlinks PATH` | `note.backlinks` | `SOURCE:LINE<TAB>LINK` per backlink |

- `cat`, `backlinks`: exactly one argument; `search`: at least one;
  `ls`: at most one.
- **SNIPPET**: the parts in order, a matching part wrapped in `**`, with
  every `\r\n`, `\n` or `\r` replaced by one space.
- **LINK**, written as markdown: a wiki link is `[[` + target + subpath +
  (`|` + alias, when there is one) + `]]`; a markdown link is
  `[` + alias + `](` + target + subpath + `)`; either gets a `!` before it
  when it is an embed; a property link is `KEY: ` + its wiki form.

## Tests (given, do not edit)

`cmd/notesctl/notes_test.go`: `TestNoteVaultChoice`, `TestNoteList`,
`TestNoteCat`, `TestNoteSearch`, `TestNoteBacklinks`. T-0020's tests reuse
its `fakeNotes`, `notesServer`, `notes`, `notesErr`, `ptr` and
`checkCall`.

## Gotchas

- `cat` must not add a final newline: 384 of the user's 666 notes have
  none, and output must be byte for byte.
- Use `cmd.Flags().Changed("limit")` to tell `--limit` given from not.
- A vault named `2026` and a vault with ID 2026 both exist in principle:
  the ID wins only when a vault has it.
- Keep each `RunE` short: `withVault` does the dialing and the choice.

## Out of scope

Writing commands (T-0020), `note.links`, formatting with colors, and every
file not under touch.

## Done when

`make accept T=0019` passes, `make check` passes, and only the files under
touch changed.
