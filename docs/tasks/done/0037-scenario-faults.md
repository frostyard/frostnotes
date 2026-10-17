---
id: "0037"
title: Test hub restarts and revocation during a run
milestone: M2
size: S # S: at most 150 non-test lines in 3 files; M: 400 lines in 6 files; L: 1,000 lines, one package or feature (ADR-0010).
touch:
  - internal/synctest/scenario_faults_test.go
acceptance: go test ./internal/synctest -run '^TestScenarioHub|^TestScenarioRevocation|^TestScenarioFaults' -count=1 -timeout 300s
---
# T-0037: Test hub restarts and revocation during a run

## Goal

The hub stops and starts again while devices work; a device is revoked
in the middle of a run, keeps what it writes, and once paired again
syncs it. Write the scenarios, and a seeded run of hub restarts and
crashes, lost answers, crashes on devices and revocations (plan 0004,
Phase 6).

## Read first

- docs/tasks/EXECUTOR.md
- docs/adr/0016-pair-devices-with-a-one-time-code-and-a-revocable-token.md
- docs/specs/sync-protocol.md, "URLs and tokens" (revocation, and a
  device that pairs again joins anew).
- internal/synctest/script.go: `Token`, `Write`, `Offline`, `Online`,
  `Settle`, `SettleOnline`, `Heads`, `Files`, `Status`, `Revoke`,
  `PairAgain`, `RestartHub`; internal/synctest/profile.go: `Profile`,
  `PlanWith`, the `RevokeAndPair` op.
- internal/synctest/sim.go (`Options`, `Op`, `Run`) and check.go.
- internal/syncer/pairs_test.go, `TestPairingAgainResumesSync`.

## Contract

One new file, `internal/synctest/scenario_faults_test.go`, `package
synctest`, no build tag. Each test makes its own world (`NewWorld(t,
Options{Seed: 6})`, `defer w.Close()`), fails on any scripted call's
error, takes tokens from `w.Token()`, and ends its script with
`w.Check()` returning nil. `d0`, `d1`, `d2` are `w.Devices[0]`, `[1]`,
`[2]`.

**`TestScenarioHubRestart`:** d0 writes `a.md` as `a + "\n"`;
`RestartHub`; d1 writes `b.md` as `b + "\n"`; `Settle`, `Check`. The
heads hold both notes as written, on every device.

**`TestScenarioRevocationDuringARun`:** `Revoke(d2)`, then `Status(d2)`
is `revoked`. d2 writes `c.md` as `c + "\n"`. d2 `Offline` (a revoked
device is never quiet); d0 writes `a.md` as `a + "\n"`; `SettleOnline`:
the heads hold `a.md` and no `c.md`. Then d2 `Online`, `PairAgain(d2)`,
`Settle`, `Check`: the heads hold `c.md` as d2 wrote it, and d2's
`a.md` (from `Files`) holds a.

**`TestScenarioFaultsProfile`:** a `Profile` with weights `Edit: 6,
Create: 3, HubRestart: 2, HubCrash: 1, DropAnswer: 2, RevokeAndPair: 2,
Crash: 2, Offline: 2, Online: 3`. Run it through `Run` and
`PlanWith(seed, 3, steps, weights)` for seed 1 stepwise and seed 2
concurrent, 200 steps each, as subtests. No `Check` of its own.

## Tests

The three above. They must pass with `-count=3`.

## Gotchas

- `Revoke` waits until the device's sync says `revoked`; `PairAgain`
  pairs it again over the hub's local socket and wakes its loop.
- `Settle` fails on a revoked device: take it offline before settling
  the others with `SettleOnline`.
- Do not read the vault from disk with `os` calls: use `Heads` and
  `Files`.

## Out of scope

The harness and sync itself. If a test fails because of either, stop and
explain it in your final message instead of changing them or weakening
the test.

## Done when

`make accept T=0037` passes, `make check` passes, and only the file under
touch changed.
