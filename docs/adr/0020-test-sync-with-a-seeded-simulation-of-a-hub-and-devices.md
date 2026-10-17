# 0020 — Test sync with a seeded simulation of a hub and devices

- **Status:** Proposed
- **Date:** 2026-10-16

## Context

Lost edits in sync are the roadmap's first risk.
[ADR-0005](0005-sync-through-a-hub-with-revisions-and-three-way-merges.md)
expects sync to be testable in one process, and M2's exit asks for 10,000
seeded steps over a hub and three devices that converge with no lost
token and no conflict file. Sync fails in orderings: an edit while a pull
is applied, an answer lost after the hub committed, a crash between a
rename and its `Tx`, a device offline while two others move the same
file. notesd uses real files, fsnotify, goroutines and HTTP; M1's tests
drive it through `rpctest`, and `make vault-it` kills a child notesd.

## Decision

- **`internal/synctest` runs a hub and N devices in one test process,**
  each a whole notesd (store, blobs, vault actors, and the hub or the sync
  client) on its own temporary data directory and vault, talking over
  loopback HTTP through a transport the test controls: it partitions a
  device, delays, and drops an answer after the hub has acted on it.
- **A seeded scheduler** (`math/rand/v2`'s PCG) picks each step: an
  editor's write with a current or stale base, an agent's append, an
  outside edit made on disk as vim or another program makes it, a create
  (among them, two devices creating today's daily note from one
  template), a delete, a move through notesd or on disk, an attachment, a
  device going offline or coming back, a crash at a chosen point (after a
  rename and before its `Tx`, between an upload and its push, after the
  hub's commit and before its answer) and the restart, a hub restart, and
  a reset.
- **Every write inserts a unique token,** and the oracle records the
  tokens each writer saw, so a token a writer removed on purpose (a
  delete, or an edit of a note that held it) is told apart from one that
  sync lost.
- **At each quiet point** (every device online, no change pending, every
  loop idle) it checks that:
  1. every device's files equal the hub's heads, paths and bytes;
  2. every token not removed on purpose is in a head or a conflict
     revision;
  3. no file exists that no step created or moved there, beyond the
     protocol's ` 1` names: there are no conflict copies;
  4. the log's sequence numbers have no gaps, every base names an earlier
     revision, and no two live files share a path, ignoring case;
  5. every device's agreed state is the hub's heads, and its index equals
     a rebuild.
- **Two modes.** Stepwise runs each step to quiet before the next, so a
  seed replays exactly. Concurrent runs let steps overlap sync, to find
  races; a seed replays their plan of steps, not their interleaving.
- **A failing seed is reduced** by dropping steps while it still fails,
  and committed with its mode and step count as a regression test.
- **Cost:** `make test` runs a few hundred steps on fixed seeds; `make
  sync-sim`, an integration test, runs 10,000 steps over a hub and three
  devices and is M2's exit evidence. A fake clock drives quiet periods
  and retention; timeouts use real time.
- **The harness and the oracle are the planner's;** scenario profiles over
  them can be task cards
  ([ADR-0010](0010-claude-plans-and-codex-runs-task-cards.md)).

## Consequences

- Each sync bug found becomes a seed that fails until it is fixed.
- The simulation is only as good as its oracle and its steps, so M2 checks
  it by mutation: a hub that overwrites instead of merging, a device that
  applies a pull without comparing, and a hub that drops conflict
  revisions must each fail it.
- A concurrent failure may not replay exactly; its seed and step log still
  narrow it down.
- 10,000 steps over four notesd instances take minutes, so the long run
  stays out of `make test`.

## Alternatives considered

- **Deterministic simulation of all I/O (as FoundationDB does):** exact
  replays, but every file, socket, timer and goroutine of notesd would run
  through a simulated scheduler.
- **Separate processes on a real network (as Jepsen does):** closer to
  production, slower, and harder to check token by token; `make vault-it`
  and M2's 100 MB run cover real processes.
- **A model checker over a model of the protocol (TLA+):** it proves the
  model, not the code; later, if the simulation finds races it cannot
  replay.
- **Example tests alone:** they test the orderings someone thought of.

## References

- Shapes: [design/sync.md](../design/sync.md#testing)
- Builds on: [ADR-0005](0005-sync-through-a-hub-with-revisions-and-three-way-merges.md), [ADR-0010](0010-claude-plans-and-codex-runs-task-cards.md)
- Built in: [plan 0004](../plans/0004-m2-sync.md)
