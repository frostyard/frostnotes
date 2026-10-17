---
id: "0033"
title: Test move chains, swaps and case-only renames
milestone: M2
size: S # S: at most 150 non-test lines in 3 files; M: 400 lines in 6 files; L: 1,000 lines, one package or feature (ADR-0010).
touch:
  - internal/synctest/scenario_moves_test.go
acceptance: go test ./internal/synctest -run '^TestScenarioMove|^TestScenarioSwap|^TestScenarioCase' -count=1 -timeout 300s
---
# T-0033: Test move chains, swaps and case-only renames

## Goal

A file follows its identity through renames, whatever another device did
to it meanwhile: a chain of moves, two notes swapping names, a rename
that changes only case, and a new note whose name differs from another's
only by case. Write the simulation scenarios that check each, scripted,
and a seeded run weighted towards moves (plan 0004, Phase 6).

## Read first

- docs/tasks/EXECUTOR.md
- docs/adr/0020-test-sync-with-a-seeded-simulation-of-a-hub-and-devices.md
- docs/design/sync.md, "Device loop" and "Testing".
- docs/specs/sync-protocol.md: "A push, on the hub" (the order of a
  push's moves, rules 3 and 6) and "Pulling and applying" (rule 5,
  parking).
- internal/synctest/script.go: `Token`, `Write`, `Move`, `Offline`,
  `Online`, `Settle`, `Heads`; internal/synctest/profile.go: `Profile`,
  `PlanWith`, and the `Swap` and `CaseRename` ops.
- internal/synctest/sim.go (`Options`, `Op`, `Run`) and check.go.
- internal/syncer/pairs_test.go, `TestASwapKeepsBothNames` and
  `TestACaseOnlyCollisionGetsANumber`.

## Contract

One new file, `internal/synctest/scenario_moves_test.go`, `package
synctest`, no build tag. Each test makes its own world (`NewWorld(t,
Options{Seed: …})`, `defer w.Close()`), fails on any scripted call's
error, takes its tokens from `w.Token()`, and ends its script with
`w.Check()` returning nil. `d0`, `d1` are `w.Devices[0]`, `[1]`.

Setup for the scripted tests: d0 writes the notes the test names, then
`Settle`, so every device has them.

**`TestScenarioMoveChain`:** `n.md` holds `"# n\n"`. d1 offline; d0
moves `n.md` to `a/n.md`, then `a/n.md` to `b/n.md`; d1 writes `n.md` as
`"# n\n" + t + "\n"`. `Settle`. The hub's heads have `b/n.md` holding
`"# n\n" + t + "\n"`, and neither `n.md` nor `a/n.md`.

**`TestScenarioSwapWhileAnotherEdits`:** `x.md` holds `"X\n"`, `y.md`
`"Y\n"`. d1 offline; d0 moves `x.md` to `t.md`, `y.md` to `x.md`, `t.md`
to `y.md` (d0 online, so each move goes alone); d1 writes `x.md` as `"X\n"
+ tx + "\n"` and `y.md` as `"Y\n" + ty + "\n"`. `Settle`. Heads: `x.md`
is `"Y\n" + ty + "\n"`, `y.md` is `"X\n" + tx + "\n"`; no `t.md`; no path
that ends in ` 1.md`.

**`TestScenarioSwapInOnePush`:** the same, but d0 is offline too during
the swap, so one push carries both moves; then d0 and d1 come online
(`Online` both, `Settle`). The same heads.

**`TestScenarioCaseOnlyRename`:** `plan.md` holds `"plan\n"`. d1
offline; d0 moves `plan.md` to `Plan.md`; d1 writes `plan.md` as
`"plan\n" + t + "\n"`. `Settle`. Heads: `Plan.md` is `"plan\n" + t +
"\n"`; no `plan.md`.

**`TestScenarioCaseOnlyCollision`:** `notes.md` holds `"notes\n"`. d1
offline writes a new `Notes.md`, `"Notes\n" + t + "\n"`. `Settle`. Heads:
`notes.md` is `"notes\n"`, `Notes 1.md` is `"Notes\n" + t + "\n"`, no
`Notes.md`.

**`TestScenarioMovesProfile`:** a `Profile` with weights `Edit: 5, Move:
5, Swap: 5, CaseRename: 5, DiskMove: 2, Create: 2, Offline: 3, Online:
4`, and a `Check` that fails when the hub or any device (`w.Hub`,
`w.Devices`, read with `Files`) holds a path containing `.sync-`: sync
parks a file under such a name only for a moment, and none may last to a
quiet point. Run it through `Run` and `PlanWith(seed, 3, steps,
weights)` for seed 1 stepwise and seed 2 concurrent, 200 steps each, as
subtests.

## Tests

The six above. They must pass with `-count=3` too.

## Gotchas

- `Move` goes through notesd and keeps the file's identity; do not
  rename on disk in the scripted tests.
- The vault is on a file system that tells case apart, so `plan.md` and
  `Plan.md` are two paths; the hub keeps paths unique ignoring case.
- In `TestScenarioSwapWhileAnotherEdits` each note keeps d1's edit and
  takes the other's name: compare whole contents, not substrings.
- Do not read the vault from disk with `os` calls: use `Heads` and
  `Files`.

## Out of scope

The harness and sync itself. If a test fails because of either, stop and
explain it in your final message instead of changing them or weakening
the test.

## Done when

`make accept T=0033` passes, `make check` passes, and only the file under
touch changed.
