---
id: "0030"
title: Add notesctl hub commands
milestone: M2
size: M # S: at most 150 non-test lines in 3 files; M: 400 lines in 6 files; L: 1,000 lines, one package or feature (ADR-0010).
touch:
  - cmd/notesctl/hub.go
  - cmd/notesctl/root.go
given:
  - cmd/notesctl/hub_test.go # copied from 0030-notesctl-hub/_given/ when the task starts
acceptance: go test ./cmd/notesctl -run 'TestHub' -count=1
---
# T-0030: Add notesctl hub commands

## Goal

On a hub, the user serves vaults and approves and revokes devices with
`notesctl hub`, over the local socket only (ADR-0016); on a device, `notesctl
hub join` pairs with a hub and starts syncing a vault. Implement the `hub`
command and its subcommands in `cmd/notesctl/hub.go`, over the `hub` and
`sync` domains.

## Read first

- docs/tasks/EXECUTOR.md
- docs/specs/sync-protocol.md: "Pairing" and "Local domains".
- api/zz_generated.go: `HubClient`, `SyncClient` and their params and
  results (`HubStatus`, `HubVault`, `PendingPairing`, `HubDevice`,
  `Pairing`, `PairResult`, `SyncHub`, `SyncStatus`).
- cmd/notesctl/root.go, vault.go and notes.go: how a command dials notesd,
  picks the vault (`withVault`), prints, and honors `--json`
  (`clix.OutputJSON`).
- cmd/notesctl/write.go: `formatVersion`'s time format.
- cmd/notesctl/hub_test.go (given): its fakes are shared with T-0031.

## Contract

`notesctl hub` with these subcommands, each one `newHubXxxCmd(opts)` in
`hub.go`, and `newHubCmd(opts)` added to the root's commands. Times print
as `t.UTC().Format("2006-01-02T15:04:05Z")`. Commands marked (json) print
their result with `clix.OutputJSON` instead when `--json` is given.
Output lines end in `\n`; fields are separated by tabs.

| Command | Calls | Prints |
| --- | --- | --- |
| `hub status` (json) | `hub.status` | `hub <id>`, or `hub: none yet (serve a vault first)` when the ID is empty; then `listening on <addr>`, or `not listening: start notesd with --hub ADDR` when `listen` is absent; then `<v> vaults, <d> devices, <c> connected` |
| `hub serve` (json) | `hub.serve {vaultId}` for the vault `--vault` picks | `serving <name> as <sync>` |
| `hub vaults` (json) | `hub.vaults` | `<name>\t<sync>\t<seq>` each; `no vaults served` when none |
| `hub pending` (json) | `hub.pending` | `<code>\t<name>\t<addr>\t<expiresAt>`, with `\tapproved` added when approved; `no pending pairings` when none |
| `hub approve CODE` | `hub.approve {code}` with CODE as typed | `approved <name> (<code>)` from the result |
| `hub devices` (json) | `hub.devices` | `<id>\t<name>\t<pairedAt>\t<seenAt or never>\t<state>`, state `revoked` when revoked, else `connected` when connected, else `offline`; `no paired devices` when none |
| `hub revoke ID` | `hub.revoke {device}` | `revoked device <id>`; an ID that is not a number is an error naming it, before dialing |
| `hub join URL NAME [--device NAME]` | see below | see below |

`hub join URL NAME`, for the vault `--vault` picks, with a trailing `/`
removed from URL:

1. `sync.hubs`. If a hub has exactly that URL, skip to step 3.
2. `sync.pair {url, name}` (`name` only when `--device` is given). Print
   `approve this device on the hub: notesctl hub approve <code>`. Then call
   `sync.paired {pairing}` again and again (each call waits up to 30 s on
   notesd's side; do not sleep): on `approved` print `paired with hub
   <hub>` and go on; on `expired` fail with an error containing `expired`
   and do not join; on `pending` ask again.
3. `sync.join {vaultId, url, name}` and print `syncing <hubVault> at <url>:
   <state>` from the result.

## Tests (given, do not edit)

`cmd/notesctl/hub_test.go`: `TestHubStatus`, `TestHubServeAndVaults`,
`TestHubPendingAndApprove`, `TestHubDevicesAndRevoke`,
`TestHubJoinPairsThenJoins`, `TestHubJoinWithAPairedHub`,
`TestHubJoinExpired`.

## Gotchas

- `hub serve` and `hub join` need a vault, so they go through
  `withVault`; the other commands do not pick one.
- The code a user types to `hub approve` goes to notesd as typed: notesd
  normalizes it.
- `--device` is this command's flag; `--vault` and `--json` are the root's.
- The tests' fakes record the params of every call in order, so make no
  calls beyond those in the table.

## Out of scope

The `sync` and `history` commands (T-0031), anything notesd does with
these calls (Phase 4), and every file other than the two under touch.

## Done when

`make accept T=0030` passes, `make check` passes, and only the files under
touch changed.
