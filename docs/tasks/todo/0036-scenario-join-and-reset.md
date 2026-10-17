---
id: "0036"
title: Test joining with an older copy, and a reset
milestone: M2
size: S # S: at most 150 non-test lines in 3 files; M: 400 lines in 6 files; L: 1,000 lines, one package or feature (ADR-0010).
touch:
  - internal/synctest/scenario_join_test.go
acceptance: go test ./internal/synctest -run '^TestScenarioJoin|^TestScenarioReset|^TestScenarioRejoin' -count=1 -timeout 300s
---
# T-0036: Test joining with an older copy, and a reset

## Goal

A machine restored from a backup, or a copy of the vault made weeks ago,
joins the hub: each file that is an older copy of the hub's takes the
hub's head, a file the hub deleted goes, and anything the copy holds of
its own is kept. A reset drops a device's sync state and joins again
without losing a change it had not pushed. Write the scenarios, and a
seeded run with rejoins and resets (plan 0004, Phase 6).

## Read first

- docs/tasks/EXECUTOR.md
- docs/adr/0017-revisions-number-the-hubs-log-and-hashes-name-content.md
- docs/specs/sync-protocol.md, "Joining" (matching by `log.match`).
- internal/synctest/script.go: `Token`, `Write`, `Delete`, `Move`,
  `Offline`, `Online`, `Settle`, `Heads`, `Files`, `HubFile`,
  `Snapshot`, `AddDevice`, `Reset`; internal/synctest/profile.go:
  `Profile`, `PlanWith`, the `Snapshot` and `Rejoin` ops.
- internal/synctest/sim.go (`Options`, `Op`, `Run`) and check.go.

## Contract

One new file, `internal/synctest/scenario_join_test.go`, `package
synctest`, no build tag. Each test makes its own world (`NewWorld(t,
Options{Seed: 5})`, `defer w.Close()`), fails on any scripted call's
error, takes tokens from `w.Token()`, and ends its script with
`w.Check()` returning nil. `d0`, `d1` are `w.Devices[0]`, `[1]`. The
world starts with the seed files `Inbox.md` (`"# Inbox\n\n- start\n"`),
`Projects/Plan.md`, `Journal/day.md` and `attachment.bin`.

**`TestScenarioJoinWithAnOlderCopy`:** `old := w.Snapshot(d0)`. Then d0
writes `Inbox.md` as `"# Inbox\n\n- start\n" + t + "\n"`, deletes
`Journal/day.md`, moves `Projects/Plan.md` to `Plan.md`; `Settle`. Then
`AddDevice("d3", old)` and `Settle`, `Check`. The new device's files
(`Files`) equal the hub's heads: `Inbox.md` holds t, there is no
`Journal/day.md`, `Plan.md` is there and `Projects/Plan.md` is not, and
the heads hold 3 files.

**`TestScenarioJoinWithAnEditOfItsOwn`:** `old := w.Snapshot(d1)`, and
`old["Inbox.md"]` gains a line `t + "\n"` at its end: an edit the copy
made after it was copied, which the hub never saw. `AddDevice("d3",
old)`, `Settle`, `Check`. The head of `Inbox.md` holds t, it is not
conflicted, and the heads hold the same four paths as before.

**`TestScenarioResetKeepsAChange`:** d1 offline writes `Inbox.md` with a
line `t + "\n"` appended; then d1 `Online` and at once `Reset`; then
`Settle`, `Check`. The head of `Inbox.md` holds t.

**`TestScenarioRejoinProfile`:** a `Profile` with weights `Edit: 6,
Create: 3, Delete: 2, Move: 2, StaleEdit: 2, Snapshot: 2, Rejoin: 1,
Reset: 1, Offline: 2, Online: 3`. Run it through `Run` and
`PlanWith(seed, 3, steps, weights)` for seed 1 stepwise and seed 2
concurrent, 200 steps each, as subtests. No `Check` of its own.

## Tests

The four above. They must pass with `-count=3`.

## Gotchas

- `Snapshot` copies a device's files as they are at that moment; change
  the copy, not the device, for the edit of its own.
- `AddDevice` pairs and joins the new device and adds it to
  `w.Devices`, so `Check` compares it too.
- `Reset` needs the hub reachable: bring the device online first. The
  loop may push the change before the reset or the join may find it:
  either way it must survive, which is what the test checks.
- Do not read the vault from disk with `os` calls: use `Heads` and
  `Files`.

## Out of scope

The harness and sync itself. If a test fails because of either, stop and
explain it in your final message instead of changing them or weakening
the test.

## Done when

`make accept T=0036` passes, `make check` passes, and only the file under
touch changed.
