# 0043 — The typing gate is looser on CI

- **Status:** Proposed
- **Date:** 2026-11-12

## Context

[Plan 0009](../plans/0009-mygo-desktop.md) holds the native editor to a
typing gate: a keystroke in a 10,000-line note, with a frame drawn on the
CPU, costs under 16 ms, one frame at 60 Hz, in source mode (Phase 2) and in
live preview (Phase 5). `BenchmarkTypingInALongNote`
(`internal/desktop/editor/typing_bench_test.go`, `make desktop-bench`)
measures it. The review of
[#145](https://github.com/frostyard/frostnotes/pull/145) asked for a named
reference machine, and the plan named the CI runner: the `desktop` job of
`ci.yml` runs `make desktop-bench` on every run. Until now the benchmark
only printed its figure; nothing failed over 16 ms.

GitHub's `ubuntu-24.04` runner is not one machine. In 19 runs of `main` and
its branches from 2026-11-11 to 2026-11-12
([#193](https://github.com/frostyard/frostnotes/issues/193)), a keystroke in
live preview measured:

| Runner CPU | Runs | Live preview | Source mode |
|---|---|---|---|
| AMD EPYC 9V45 | 6 | 10.4 to 13.3 ms | 2.4 to 3.1 ms |
| AMD EPYC 9V74 | 3 | 15.1 to 15.6 ms | 3.6 to 3.7 ms |
| AMD EPYC 7763 | 7 | 17.1 to 21.1 ms | 4.4 to 5.6 ms |
| Intel Xeon Platinum 8370C, 8573C | 3 | 18.8 to 20.8 ms | 4.9 to 5.2 ms |

The slowest CPU is about twice as slow as the fastest. A 16 ms gate on CI
would pass or fail by the runner a job draws, not by the change. On the
developer machine (an AMD Ryzen AI MAX+ 395) the same benchmark measures
about 10 ms in live preview and 2.4 ms in source mode.

## Decision

- **16 ms on a developer machine.** `make desktop-bench` fails when a
  keystroke, in source mode or in live preview, costs more than
  `localTypingBudget`, 16 ms. Plan 0009's done-when lines use this gate.
- **25 ms on CI.** When the environment variable `CI` is true, as
  `strconv.ParseBool` reads it (GitHub Actions sets `CI=true`), the benchmark fails over `ciTypingBudget`, 25
  ms, instead. 25 ms is the highest figure seen, 21.1 ms, plus a margin of
  about a fifth for the spread of runs on one CPU (up to 8%) and for a CPU
  not seen yet.
- **Two named values.** The two limits are separate constants beside the
  benchmark. `TestTypingBudgets` fails if the local gate is not 16 ms, if
  the CI limit is not above it, or if a `CI` value that `strconv.ParseBool`
  does not read as true (empty, `false`, `0`, `yes`) selects the CI limit.

## Consequences

- CI now fails a change that makes typing clearly slower: on the slowest
  CPU seen, a keystroke that grows by about a fifth fails. Before, CI only
  printed the figure.
- CI catches less on a fast runner: on an EPYC 9V45, a keystroke can nearly
  double before it passes 25 ms. The 16 ms gate on a developer machine stays
  the strict one, so a change to the editor or live preview runs
  `make desktop-bench` before review.
- A new, slower runner CPU can fail the job with no change in the code. The
  figure and the CPU are in the job's log (`go test` prints `cpu:`); the fix
  is a faster editor or a new ADR for the limit, not a quiet change of the
  constant.
- A developer machine slower than the reference machine can fail the local
  gate. The plan's figures name their CPU.

## Alternatives considered

- **16 ms on CI too:** rejected. It fails 10 of the 19 runs above, by the
  runner's CPU.
- **Measure against `main` in the same job:** rejected for now. A relative
  limit (live preview no more than 10% over `main`) is fair on every CPU,
  but it builds and runs the benchmark twice in each job and still needs an
  absolute gate somewhere.
- **Pin a runner class:** rejected. A larger or self-hosted runner costs
  money or a machine to keep, for one benchmark.
- **Only print the figure on CI:** rejected. A regression would then reach
  `main` with nothing to stop it.

## References

- Shapes: [plan 0009, Phases 2 and 5](../plans/0009-mygo-desktop.md#phase-5--live-preview-about-3-weeks),
  `internal/desktop/editor/typing_bench_test.go`,
  `internal/desktop/editor/typing_budget_test.go`, `.github/workflows/ci.yml`
  (the `desktop` job), `Makefile` (`desktop-bench`)
- Builds on: [ADR-0038](0038-draw-the-desktop-app-natively-with-mygo-and-frostui.md),
  [ADR-0039](0039-live-preview-is-replacements-over-the-source.md)
