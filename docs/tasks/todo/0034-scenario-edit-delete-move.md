---
id: "0034"
title: Test an edit, a delete and a move of one file at once
milestone: M2
size: S # S: at most 150 non-test lines in 3 files; M: 400 lines in 6 files; L: 1,000 lines, one package or feature (ADR-0010).
touch:
  - internal/synctest/scenario_contention_test.go
acceptance: go test ./internal/synctest -run '^TestScenarioEditDeleteMove|^TestScenarioContention' -count=1 -timeout 300s
---
# T-0034: Test an edit, a delete and a move of one file at once

## Goal

Three devices, apart, each do something different to one note: one
edits it, one deletes it, one moves it. Whatever order they come back
in, the edit survives (an edit beats a delete), in one file, and the
move applies unless the delete came after it. Write the scenario that
checks every order, and a seeded run of a few files under constant
contention (plan 0004, Phase 6).

## Read first

- docs/tasks/EXECUTOR.md
- docs/adr/0020-test-sync-with-a-seeded-simulation-of-a-hub-and-devices.md
- docs/specs/sync-protocol.md, "A push, on the hub": rules 4 (delete), 5
  (put on a deleted file) and 6 (put on a live file).
- internal/synctest/script.go: `Token`, `Write`, `Delete`, `Move`,
  `Offline`, `Online`, `SettleOnline`, `Settle`, `Heads`;
  internal/synctest/profile.go: `Profile`, `PlanWith`.
- internal/synctest/sim.go (`Options`, `Op`, `Run`) and check.go.

## Contract

One new file, `internal/synctest/scenario_contention_test.go`, `package
synctest`, no build tag. Every world is its own (`NewWorld(t,
Options{Seed: 3})`, closed when its subtest ends); scripted calls fail
the test on error; tokens come from `w.Token()`; each script ends with
`w.Check()` returning nil.

**`TestScenarioEditDeleteMove`:** one subtest per order in which the
three devices come back, all six of them. In each: device 0 writes `f.md`
as `"one\ntwo\n"`, then `Settle`; all three devices offline; device 0
writes `f.md` as `"one\ntwo\n" + t + "\n"`, device 1 deletes `f.md`,
device 2 moves `f.md` to `g.md`. Then each device in the order comes
online with `SettleOnline` after it; then `Settle` and `Check`. Then the
hub's heads hold the note, `"one\ntwo\n" + t + "\n"`, at exactly one of
the two paths, and the other path is absent:

| Order back (0 edits, 1 deletes, 2 moves) | The note is at |
| --- | --- |
| 0, 1, 2 | `g.md` |
| 0, 2, 1 | `g.md` |
| 1, 0, 2 | `g.md` |
| 1, 2, 0 | `g.md` |
| 2, 0, 1 | `g.md` |
| 2, 1, 0 | `f.md`: the delete took the moved note, and the edit brought it back at its own path |

and the heads hold no other file than the world's four seed files
(`Inbox.md`, `Projects/Plan.md`, `Journal/day.md`, `attachment.bin`)
and this one.

**`TestScenarioContentionProfile`:** a `Profile` with weights `Edit: 6,
Delete: 3, Move: 3, StaleEdit: 3, Offline: 4, Online: 5, Create: 1` (few
files, so most steps touch a file another device is touching). Run it
through `Run` and `PlanWith(seed, 3, steps, weights)` for seed 1
stepwise and seed 2 concurrent, 200 steps each, as subtests. The world's
own checks are the point; the profile needs no `Check`.

## Tests

The two above (seven subtests and two). They must pass with `-count=3`.

## Gotchas

- Table-drive the six orders; name each subtest after its order.
- `Delete` reads the note first and deletes with that base; on device 1
  the note is still at `f.md`.
- Compare the whole content, not a substring.
- Do not read the vault from disk with `os` calls: use `Heads`.

## Out of scope

The harness and sync itself. If a test fails because of either, stop and
explain it in your final message instead of changing them or weakening
the test.

## Done when

`make accept T=0034` passes, `make check` passes, and only the file under
touch changed.
