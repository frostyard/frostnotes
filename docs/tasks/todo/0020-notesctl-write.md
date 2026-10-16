---
id: "0020"
title: Add notesctl write, mv, rm, versions and revert
milestone: M1
size: M # S: at most 150 non-test lines in 3 files; M: 400 lines in 6 files; L: 1,000 lines, one package or feature (ADR-0010).
touch:
  - cmd/notesctl/write.go
  - cmd/notesctl/root.go
given:
  - cmd/notesctl/write_test.go # copied from 0020-notesctl-write/_given/ when the task starts
acceptance: go test ./cmd/notesctl -run 'TestNote(Write|MoveAndRemove|VersionsAndRevert)' -count=1
---
# T-0020: Add notesctl write, mv, rm, versions and revert

## Goal

`notesctl` changes notes over the `note` domain. Every write names the
hash it started from (ADR-0006): `--base` is required and never defaults,
because a default would make the write blind (plan 0003, "notesctl
write"). Deleted and overwritten notes come back with `versions` and
`revert`. Runs after T-0019, whose vault choice and test helpers it
reuses.

## Read first

- docs/tasks/EXECUTOR.md
- docs/specs/rpc-api.md, section "note": `note.create`, `note.write`,
  `note.move`, `note.delete`, `note.versions`, `note.revert`,
  `WriteResult`, `MoveResult`, `Version`.
- cmd/notesctl/notes.go (T-0019): `withVault`, and the command pattern.
- cmd/notesctl/write_test.go (given), and notes_test.go (its helpers).

## Contract

**`root.go`** adds, after T-0019's commands, `newWriteCmd(opts)`,
`newMvCmd(opts)`, `newRmCmd(opts)`, `newVersionsCmd(opts)` and
`newRevertCmd(opts)`, all in **`write.go`**. Each command checks its
arguments and flags before dialing, prints with `clix.OutputJSON` first,
and returns notesd's errors as they come.

| Command | Calls | Prints |
| --- | --- | --- |
| `write PATH --base HASH [--file F]` | `note.write` with the content | `wrote PATH HASH`, or `merged PATH HASH` + CONFLICTS |
| `write PATH --create [--file F]` | `note.create` with the content | `created PATH HASH` |
| `mv PATH TO [--update-links]` | `note.move`; `UpdateLinks` set only with the flag | `moved PATH to NEWPATH`, then `rewrote links in X` for each rewritten note |
| `rm PATH --base HASH` | `note.delete` | `deleted PATH` |
| `versions PATH [--limit N]` | `note.versions`; `Limit` only when given | one line per version, below |
| `revert PATH VERSION [--base HASH]` | `note.revert`; `Base` is `""` without the flag | `reverted PATH to version N: HASH`, + `, merged` + CONFLICTS when merged |

- **Content** is everything read from `--file`, or else from the
  command's stdin (`cmd.InOrStdin()`), byte for byte. A file that cannot
  be read is an error (wrapped, so its name shows).
- `write` without `--base` and without `--create`: an error naming
  `--base`. Both: an error naming `--create`. `rm` without `--base`: an
  error naming `--base`. `Source` and `Author` are never sent: notesd
  defaults them.
- **CONFLICTS** is empty for none, `, 1 conflict`, or `, N conflicts`.
- **A version line** is tab-separated: the ID; `At` in UTC as
  `2006-01-02T15:04:05Z`; the source; the author or `-`; `deleted` for a
  deletion, else the first 12 characters of the hash, with ` conflict`
  after them for a conflict version; the version's path.
- `revert`'s VERSION must be an integer: otherwise an error that quotes it.
- Argument counts: `write`, `rm`, `versions` take one; `mv` and `revert`
  two.

## Tests (given, do not edit)

`cmd/notesctl/write_test.go`: `TestNoteWrite`, `TestNoteMoveAndRemove`,
`TestNoteVersionsAndRevert`.

## Gotchas

- Read the content before dialing, and refuse bad flags before reading:
  the test checks that refused commands call nothing.
- The test feeds stdin through `root.SetIn`; read `cmd.InOrStdin()`,
  never `os.Stdin`.
- Print the path notesd returns (`res.Path`) for writes and moves: a
  move's result is the new path.
- `Limit` and `UpdateLinks` are pointers in the params: leave them nil
  unless the flag was given.

## Out of scope

Opening an editor, reading hashes for the user, daily notes, and every
file not under touch.

## Done when

`make accept T=0020` passes, `make check` passes, and only the files under
touch changed.
