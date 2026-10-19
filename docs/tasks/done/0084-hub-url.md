---
id: "0084"
title: Add notesctl hub url and notesctl hub relocate
milestone: M4
size: S
touch:
  - cmd/notesctl/hub.go
given:
  - cmd/notesctl/hub_url_test.go
acceptance: go test ./cmd/notesctl -run 'TestHubURL|TestHubStatusShowsTheURL|TestHubRelocate' -count=1
---
# T-0084: Add notesctl hub url and notesctl hub relocate

## Goal

A hub moving behind lodge records the URL its devices should use
(`hub.setURL`), and a device that missed the move checks a new URL with
the hub's relocation proof (`sync.relocate`). Add the two commands that
call them, and show the recorded URL in `notesctl hub status`
(`docs/specs/lodge-app.md`, Commands and Device; plan 0006, Phase 2).

## Read first

- `cmd/notesctl/hub.go`: the hub commands. `newHubApproveCmd` and
  `newHubStatusCmd` are the patterns: dial, call, `clix.OutputJSON`,
  then a line of text.
- `api/zz_generated.go`: `HubClient.SetURL` (`HubSetURLParams{URL}`,
  answers `*HubStatus`, whose `URL` is a `*string`) and
  `SyncClient.Relocate` (`SyncRelocateParams{Hub, URL}`, answers
  `*SyncHub`).
- `docs/specs/lodge-app.md`, sections "Commands" and "`/device/`".
- The given test `cmd/notesctl/hub_url_test.go` and the fakes it uses
  in `cmd/notesctl/hub_test.go` (`fakeHub`, `fakeSync`, `syncServer`).
- `docs/tasks/EXECUTOR.md`.

## Contract

In `cmd/notesctl/hub.go`, register both commands in `newHubCmd`, each
from its own `newHubURLCmd(opts)` and `newHubRelocateCmd(opts)`, with a
`Short` line each.

- **`notesctl hub url URL`** (exactly one argument; `""` is allowed and
  forgets the URL): calls `hub.setURL {url: URL}` with the argument as
  given. With `--json`, prints the answer (`clix.OutputJSON`).
  Otherwise prints `devices use <url>\n` with the answer's `URL`, or
  `devices use the URL they paired with\n` when the answer has none.
- **`notesctl hub relocate URL NEW-URL`** (exactly two arguments):
  calls `sync.relocate {hub: URL, url: NEW-URL}` with the arguments as
  given. With `--json`, prints the answer. Otherwise prints
  `hub <hub ID> is at <url> now\n` from the answer's `Hub` and `URL`.
- **`notesctl hub status`**: when the status has a `URL`, a line
  `devices use <url>` comes right after the listening line; without
  one the output is unchanged.
- notesd's errors reach the user unchanged, as in the other commands.

## Tests (given, do not edit)

`cmd/notesctl/hub_url_test.go`: `TestHubURL` (setting, `--json`,
forgetting, a refused URL, a missing argument),
`TestHubStatusShowsTheURL`, and `TestHubRelocate` (the line, `--json`, a
refused relocation, wrong argument counts).

## Gotchas

- Pass the arguments through as given: notesd checks and normalizes
  URLs (`hub.CheckURL`); the command does not trim or validate them.
- `TestHubStatus` in `hub_test.go` pins the status output without a
  URL: keep it passing.

## Out of scope

The hub's and the device's sides (`internal/hub`, `internal/syncer`),
the schema, and every file but `cmd/notesctl/hub.go`.

## Done when

`make accept T=0084` passes, `make check` passes, and only
`cmd/notesctl/hub.go` changed.
