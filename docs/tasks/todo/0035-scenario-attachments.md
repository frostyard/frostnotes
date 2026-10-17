---
id: "0035"
title: Test attachments, one of them over 10 MB
milestone: M2
size: S # S: at most 150 non-test lines in 3 files; M: 400 lines in 6 files; L: 1,000 lines, one package or feature (ADR-0010).
touch:
  - internal/synctest/scenario_attachments_test.go
acceptance: go test ./internal/synctest -run '^TestScenarioAttachment|^TestScenarioBig' -count=1 -timeout 300s
---
# T-0035: Test attachments, one of them over 10 MB

## Goal

Files that are not text cannot merge: when two devices change one, the
later push wins and the content it replaced is kept as a conflict
revision, and the file is flagged. Notes over 10 MB do the same, and
attachments over 10 MB sync whole, byte for byte. Write the scenarios
that check each, and a seeded run of attachments (plan 0004, Phase 6).

## Read first

- docs/tasks/EXECUTOR.md
- docs/adr/0018-move-blobs-over-http-named-by-their-hash.md
- docs/specs/sync-protocol.md, "A push, on the hub", rules 6 and 8 (a
  file that is not UTF-8, or is over 10 MB, takes the pushed content and
  is flagged).
- internal/synctest/script.go: `Token`, `WriteDisk`, `Offline`,
  `Online`, `SettleOnline`, `Settle`, `Heads`, `Files`, `HubFile`,
  `History`; internal/synctest/profile.go: `Profile`, `PlanWith`, the
  `BigAttach` op and `bigSize`.
- internal/synctest/sim.go (`Options`, `Op`, `Run`) and check.go (a
  token in a conflict revision counts as kept).

## Contract

One new file, `internal/synctest/scenario_attachments_test.go`,
`package synctest`, no build tag. Each test makes its own world
(`NewWorld(t, Options{Seed: 4})`, `defer w.Close()`), fails on any
scripted call's error, takes tokens from `w.Token()`, and ends its
script with `w.Check()` returning nil. `d0`, `d1`, `d2` are
`w.Devices[0]`, `[1]`, `[2]`. Attachments are written on disk with
`WriteDisk`, as another program would.

**`TestScenarioAttachmentConflict`:** d0 writes `files/pic.bin` as
`"\x00\xff" + a + "\xff first"`; `Settle`. d0 and d1 offline; d0 writes
it as `"\x00\xff" + b + "\xff from d0"`, d1 as `"\x00\xff" + c + "\xff
from d1"`. d0 online, `SettleOnline`; d1 online, `Settle`; `Check`.
Then the head of `files/pic.bin` is d1's content; `HubFile` says it is
conflicted; and `History` holds a `conflict` revision whose hash is the
SHA-256 (lowercase hex) of d0's content.

**`TestScenarioBigAttachment`:** d2 writes `files/big.bin`: `bigSize`
bytes, byte i being `byte(i * 7)`, with a token copied in at offset 100.
`Settle`, `Check`. Every device's `files/big.bin` (from `Files`) equals
those bytes exactly.

**`TestScenarioBigNote`:** d0 writes `long.md`, a text note of lines
`"a line of a long note\n"` repeated until it is over 10 MB (10 << 20
bytes); `Settle`. d0 and d1 offline; d0 writes it with a line `e + "\n"`
appended; d1 with a line `f + "\n"` put in front. d0 online,
`SettleOnline`; d1 online, `Settle`; `Check`. Then the head is d1's
content (it holds f and not e), and `HubFile` says it is conflicted: a
note that big is not merged.

**`TestScenarioAttachmentsProfile`:** a `Profile` with weights `Attach:
4, BigAttach: 1, Edit: 3, Delete: 1, Offline: 2, Online: 3`. Run it
through `Run` and `PlanWith(seed, 3, steps, weights)` for seed 1
stepwise and seed 2 concurrent, 100 steps each, as subtests. The world's
checks are the point; no `Check` of its own.

## Tests

The four above. They must pass with `-count=3`.

## Gotchas

- `WriteDisk` takes bytes; the oracle finds tokens in them, so put each
  token between non-text bytes as the examples do.
- Hash with `crypto/sha256` and `hex.EncodeToString`.
- Build the big contents once per test; 10 MB strings are fine, but do
  not copy them in loops.
- The later push wins: d1 comes online after d0 has settled.

## Out of scope

The harness and sync itself. If a test fails because of either, stop and
explain it in your final message instead of changing them or weakening
the test.

## Done when

`make accept T=0035` passes, `make check` passes, and only the file under
touch changed.
