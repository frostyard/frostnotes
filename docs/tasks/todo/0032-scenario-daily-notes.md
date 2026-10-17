---
id: "0032"
title: Test three devices making one daily note offline
milestone: M2
size: S # S: at most 150 non-test lines in 3 files; M: 400 lines in 6 files; L: 1,000 lines, one package or feature (ADR-0010).
touch:
  - internal/synctest/scenario_daily_test.go
acceptance: go test ./internal/synctest -run '^TestScenarioDaily' -count=1 -timeout 300s
---
# T-0032: Test three devices making one daily note offline

## Goal

Today's note is the file most likely to be made on several devices at
once, each from the same template, while they are apart. Write the
simulation scenario that checks they become one note holding every
device's lines, with no copy and no conflict, in scripted runs and in a
seeded run weighted towards daily notes (plan 0004, Phase 6).

## Read first

- docs/tasks/EXECUTOR.md
- docs/adr/0020-test-sync-with-a-seeded-simulation-of-a-hub-and-devices.md
- docs/design/sync.md, "Testing".
- docs/specs/sync-protocol.md, "A push, on the hub", rule 3 (a create at
  a live file's exact path merges as two creates).
- internal/synctest/script.go: the scripted API (`Token`, `Write`,
  `Offline`, `Online`, `Settle`, `SettleOnline`, `Heads`, `Files`,
  `HubFile`) and how its writes keep the oracle's books.
- internal/synctest/profile.go: `Profile` and `PlanWith`.
- internal/synctest/sim.go: `Options`, the `Op` constants, `Run`;
  internal/synctest/check.go: what `World.Check` checks.
- internal/syncer/pairs_test.go, `TestTwoDailyNotesFromOneTemplateMerge`:
  the two-device case.

## Contract

One new file, `internal/synctest/scenario_daily_test.go`, `package
synctest`, with no build tag (unit tests: `make test` runs them). Each
test makes its world with `NewWorld(t, Options{Seed: …})` and `defer
w.Close()`; every scripted call's error fails the test (`t.Fatal`).
Every token a test writes comes from `w.Token()`.

**`TestScenarioDailyNoteOneAtATime`:** all three devices offline; device
0 writes `daily/2026-10-17.md` as `"# 2026-10-17\n## Tasks\n- [ ] " + a
+ "\n## Notes\n"`, device 1 as `"# 2026-10-17\n## Tasks\n## Notes\n" + b
+ "\n"`, device 2 as `"# 2026-10-17\n" + c + "\n## Tasks\n## Notes\n"`
(a, b, c tokens). Each device then comes online in turn, 0, 1, 2, with
`SettleOnline` after each; then `Settle` and `Check`. Then:

- the hub's head at `daily/2026-10-17.md` is exactly `"# 2026-10-17\n" +
  c + "\n## Tasks\n- [ ] " + a + "\n## Notes\n" + b + "\n"`;
- it is the only file under `daily/` on the hub;
- `HubFile` says it is not conflicted;
- every device's files equal the hub's (`Check` does this; do not repeat
  it).

**`TestScenarioDailyNoteAllAtOnce`:** the same three writes in a world
of its own, then all three devices `Online` before one `Settle`: the same
head, still one file, not conflicted.

**`TestScenarioDailyNotesProfile`:** a `Profile` with weights `Daily: 10,
Offline: 4, Online: 5, Edit: 2, Append: 2` and a `Check` that fails
(returns an error naming the path) when the hub holds a numbered daily
note: a path matching `^daily/.* \d+\.md$`, which only a second copy of
one day's note could be. Run it, through `Run` and `PlanWith(seed, 3,
steps, weights)`, for seed 1 stepwise and seed 2 concurrent
(`Concurrent: true`), 200 steps each, as subtests.

## Tests

The three above, in the file under touch. They must pass with
`-count=3` too: concurrent runs differ from run to run.

## Gotchas

- `Write` creates the note where there is none; each device has no
  `daily/` folder until it writes.
- `SettleOnline` settles the devices whose links are up and leaves cut
  links cut; `Settle` heals every link.
- The merged head's line order is what the protocol's two-way create
  merge gives for these three edits, each in its own region of the
  template; do not sort lines or compare as sets.
- Do not read the vault from disk with `os` calls: use `Heads` and
  `Files`.

## Out of scope

The harness (`script.go`, `profile.go`, `sim.go`, `check.go`,
`world.go`, `steps.go`) and sync itself. If a test fails because of
either, stop and explain it in your final message instead of changing
them or weakening the test.

## Done when

`make accept T=0032` passes, `make check` passes, and only the file under
touch changed.
