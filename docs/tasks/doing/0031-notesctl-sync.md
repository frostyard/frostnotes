---
id: "0031"
title: Add notesctl sync and history commands
milestone: M2
size: M # S: at most 150 non-test lines in 3 files; M: 400 lines in 6 files; L: 1,000 lines, one package or feature (ADR-0010).
touch:
  - cmd/notesctl/sync.go
  - cmd/notesctl/root.go
given:
  - cmd/notesctl/sync_test.go # copied from 0031-notesctl-sync/_given/ when the task starts
acceptance: go test ./cmd/notesctl -run 'TestSync|TestHistory' -count=1
---
# T-0031: Add notesctl sync and history commands

## Goal

On a device, the user sees how each vault's sync stands, lists and
resolves conflicts, resets or leaves a hub, checks a vault against its hub
(M2's exit evidence), and reads a file's history on the hub. Implement
`notesctl sync` and `notesctl history` in `cmd/notesctl/sync.go`, over the
`sync` domain. Runs after T-0030.

## Read first

- docs/tasks/EXECUTOR.md
- docs/specs/sync-protocol.md: "Local domains", "`log` methods"
  (`Revision`).
- api/zz_generated.go: `SyncClient` and its params and results
  (`SyncStatus`, `SyncRefusal`, `SyncConflict`, `VerifyReport`,
  `Revision`).
- cmd/notesctl/hub.go (T-0030) and notes.go: how a command dials, picks
  the vault and prints; reuse the helpers hub.go defines (time formatting,
  dialing without a vault) instead of defining a second one with the same
  name.
- cmd/notesctl/write.go: `formatVersion`, whose layout `history` follows.
- cmd/notesctl/hub_test.go (T-0030's fakes) and sync_test.go (given).

## Contract

`notesctl sync` with the subcommands below, `newSyncCmd(opts)` and
`newHistoryCmd(opts)` added to the root's commands. Times print as
`t.UTC().Format("2006-01-02T15:04:05Z")`; an empty author prints as `-`.
Commands marked (json) print their result with `clix.OutputJSON` instead
when `--json` is given. Output lines end in `\n`; fields are separated by
tabs. Every command but `sync status` works on the vault `--vault` picks
(`withVault`).

| Command | Calls | Prints |
| --- | --- | --- |
| `sync status` (json) | `sync.status`, with `vaultId` only when `--vault` is given (resolved as `withVault` would) | per vault `<vaultId>\t<hubVault>\t<url>\t<state>\t<cursor>/<hubSeq>\t<pending> pending\t<conflicts> conflicted`, then `\trefused <path>: <reason>` for each refusal, `\tskipped <path>` for each skipped path, and `\terror: <error>` when there is one; `no synced vaults` when none |
| `sync conflicts` (json) | `sync.conflicts {vaultId}` | per file its path, then `\t<seq>\t<at>\t<deviceName>\t<author>` per revision; `no conflicts` when none |
| `sync resolve PATH` | `sync.resolve {vaultId, path}` | `resolved <path>` |
| `sync reset` | `sync.reset {vaultId}` | `reset vault <vaultId>: <state>` |
| `sync leave` | `sync.leave {vaultId}` | `vault <vaultId> no longer syncs; its files stay` |
| `sync verify` (json) | `sync.verify {vaultId}` | `differs\t<path>` per differing path, `unrecorded\tpush <id>` per unrecorded push, `conflicted\t<path>` per conflict, in that order; then, when `ok`, `ok: <files> files match the hub`; when not, fail with an error containing `differs from its hub` |
| `history PATH [--limit N]` (json) | `sync.history {vaultId, path, limit}` (`limit` only when given) | per revision `<seq>\t<at>\t<op>\t<deviceName>\t<author>\t<state>\t<path>`: state is the hash's first 12 characters, `deleted` for a delete, and ` pruned` appended when pruned |

## Tests (given, do not edit)

`cmd/notesctl/sync_test.go`: `TestSyncStatus`,
`TestSyncConflictsAndResolve`, `TestSyncResetAndLeave`, `TestSyncVerify`,
`TestHistory`.

## Gotchas

- `sync status` without `--vault` must not pick a vault: with several
  registered, it shows them all.
- `sync verify` prints what differs before failing, so the user sees why.
- The fakes record every call in order: make no calls beyond those in the
  table (vault resolution uses `vault.list`, which they do not record).

## Out of scope

The `hub` commands (T-0030), anything notesd does with these calls (Phase
4), and every file other than the two under touch.

## Done when

`make accept T=0031` passes, `make check` passes, and only the files under
touch changed.
