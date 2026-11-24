# 0048 — The race gate runs in 3 parts

- **Status:** Proposed
- **Date:** 2026-11-24

## Context

CI's "Race detector" job is a required check, and it ran
`go test -race -timeout 25m ./...` on one runner, as `make ci` did. It was
the slowest required check by far. Three runs of 2026-11-24, each on an
`ubuntu-24.04` runner:

| Run | Job | `go test` step | `internal/synctest` | The other packages, summed |
|---|---|---|---|---|
| [PR #275](https://github.com/frostyard/frostnotes/actions/runs/45427862610) | 17m46s | 17m24s | 817 s | 649 s |
| [`main`, #274's merge](https://github.com/frostyard/frostnotes/actions/runs/45324538062) | 13m37s | 12m24s | 604 s | 422 s |
| [PR #274](https://github.com/frostyard/frostnotes/actions/runs/45322892023) | 13m44s | 12m40s | 617 s | 435 s |

`internal/synctest` is sync's seeded simulation
([ADR-0020](0020-test-sync-with-a-seeded-simulation-of-a-hub-and-devices.md)).
In each run, every package that `go test` lists before it had reported
3.5 to 5.6 minutes into the step, and the step then printed nothing more
until the simulation ended. By its end time and its duration, the
simulation began 2.3 to 3.8 minutes into the step, behind the build and
the packages before it. The steps before `go test` took 20 to 72 seconds.
In PR #275's run, "Verify (make verify)" ran the same package with no
race detector in 47 s.

Measured on a developer machine (an AMD Ryzen AI MAX+ 395, the test pinned
to 4 cores, while the machine ran other work, so each figure is a sample
and not a limit), `go test` reported for `internal/synctest`:

| Run | Sample 1 | Sample 2 |
|---|---|---|
| `-race`, as the job ran it | 380 s | 387 s |
| `-race`, with no checkptr in `modernc.org`'s packages | 175 s | 189 s |
| no race detector, checkptr in every package | not kept (3m46s with its build) | 232 s |
| no race detector, no checkptr | 25 s | not run |

`-race` makes the compiler add checkptr's checks too: at each conversion
of an `unsafe.Pointer`, the program checks the pointer's alignment and
that pointer arithmetic stays inside one allocation. The store is
`modernc.org/sqlite`, SQLite's C translated to Go, which converts unsafe
pointers all the time. A CPU profile of 2 of the scenario tests under
`-race` had 43% of its samples in the runtime's checkptr functions.
checkptr is not part of the race detector: `-d=checkptr=0` takes out
those checks and leaves the race detector's own instrumentation of the
package as it was.

In one process, the simulation's tests gained little from running in
parallel: with `t.Parallel` in every top-level test, one sample on the
same 4 cores took 309 s, not 380 s, and used more CPU time. A cold build
of the package's race test took 30 s there, so a cache of race builds
could save about that much of a shard.

## Decision

- **The race gate is `make race`, in 3 parts.** `make ci` runs it.
  - `make race-rest`: `go test -race` on every package but
    `internal/synctest`, with the flags of the old single run.
  - `make race-sim`: `go test -race` on `internal/synctest`, with
    `-gcflags=modernc.org/...=-d=checkptr=0`: the race detector in every
    package, checkptr in every package but `modernc.org`'s.
  - `make race-sim-checkptr`: `go test -gcflags=all=-d=checkptr` on
    `internal/synctest`, with no race detector: checkptr in every package,
    `modernc.org`'s too.
- **CI runs each part on a runner of its own.** The `race-shards` job of
  `ci.yml` is a matrix of the 3 parts; each shard runs
  `make race-<shard>`.
- **The required check keeps its name.** The job named "Race detector"
  needs the shards, runs always, and passes only when every shard passed.
  A failed or cancelled shard fails it. No ruleset changes.
- **A test holds the two to each other.** `internal/workflowcheck` fails
  when `make race`'s parts and the matrix differ, when a part's `go test`
  loses the race detector, a package or checkptr, or gains `-short` or a
  test-name filter ([ADR-0035](0035-unit-runs-filter-no-test-names.md)),
  and when the required check could pass without every shard.

## Consequences

- Every package and every test still runs under the race detector on
  every run of the gate. No test, seed or step count changed.
- Every checkptr check of the old run still runs, but those in
  `modernc.org`'s packages during the simulation now run in a process
  with no race detector. A checkptr failure there that only the race
  detector's timing or memory layout brings out is no longer caught.
- The job waits for its slowest shard, not for the sum. In the samples
  above, `race-sim` took about half of the old simulation's time and
  `race-sim-checkptr` about three fifths, each before its own build. If
  CI keeps those ratios, the slowest shard runs its tests in 6 to 8
  minutes where the old step took 12 to 17. This is an estimate: the
  time on CI is not measured until a pull request runs the job.
- CI uses 3 runners and builds 3 times, so the job spends more runner
  minutes than before: the simulation's 2 parts together take about as
  long as the old simulation, and the other 2 builds come on top.
- `make ci` on a developer machine runs 3 `go test` commands one after
  another, and the simulation twice. Its time was not measured.
- A new slow package can be given a part of its own: a new `race-`
  target, its line in `internal/workflowcheck`, its shard.

## Alternatives considered

- **Only split the job, with every flag as before:** rejected as the
  whole answer. The simulation would still take its 10 to 14 minutes on
  its runner; the job would lose only the wait before it, 2 to 4 minutes.
- **Leave checkptr out of `modernc.org`'s packages and run it nowhere:**
  rejected for now. It saves one shard, and it drops a check that the
  gate ran until today. A later ADR can decide that the check of a
  dependency's translated C is not worth a runner.
- **Leave checkptr out of `modernc.org`'s packages in every package's
  race run:** not needed. The other packages are not what the job waits
  for.
- **`t.Parallel` in the simulation's tests:** rejected. One sample showed
  a fifth less time (309 s, not 380 s), and the worlds would share the
  CPUs their timeouts count on.
- **Shards of the simulation's tests by name:** rejected. A race run
  filters no test names ([ADR-0035](0035-unit-runs-filter-no-test-names.md)).
- **`-short`, fewer seeds or fewer steps under the race detector:**
  rejected. Each one runs less under the race detector.
- **A cache of the race build:** rejected. The Go jobs share one
  setup-go cache for each `go.sum`, and a run that finds it saves none; a
  cache of the race build needs a key of its own, from an action the
  workflows do not use yet. The cold build it would save took 30 s on the
  developer machine; on CI it is not measured.
- **Skip the job for a change with no Go code:** rejected for now. With
  the job at about the time of the other required checks, a skip saves
  little, and a required check that sometimes does not run needs care.

## References

- Shapes: [design/sync.md](../design/sync.md#testing), `Makefile` (`race`,
  `race-rest`, `race-sim`, `race-sim-checkptr`, `ci`),
  `.github/workflows/ci.yml` (the `race-shards` and `race` jobs),
  `internal/workflowcheck/racegate_test.go`, `AGENTS.md` (Gates),
  [development.md](../development.md#gates-and-dev-loops)
- Builds on: [ADR-0009](0009-development-environment-and-gates.md),
  [ADR-0020](0020-test-sync-with-a-seeded-simulation-of-a-hub-and-devices.md),
  [ADR-0035](0035-unit-runs-filter-no-test-names.md)
