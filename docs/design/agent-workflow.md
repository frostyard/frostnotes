# Design: agent workflow

Living document. Rationale: [ADR-0010](../adr/0010-claude-plans-and-codex-runs-task-cards.md).
Contracts: [tasks/TEMPLATE.md](../tasks/TEMPLATE.md), [tasks/EXECUTOR.md](../tasks/EXECUTOR.md).

## Overview

The planner (Claude) writes cards with exact contracts and given tests; the
executor (Codex) implements one card at a time on its own branch;
`tools/taskrun` holds the executor to the card. The loop is frostmail's,
copied at M0.

```
planner: card + _given tests ──▶ make task T=NNNN ──▶ executor ──▶ taskrun verify ──▶ human merges
              ▲                                         │  retries with the failure  │
              └──────────── planner reviews ◀───────────┴────────────────────────────┘
```

## Design

### Roles

- **Planner (Claude):** milestone plans, ADRs, designs, specs, the RPC and
  SQL schemas and migrations, interfaces, security boundaries (the hub's
  authentication, pairing, the sanitizer, MCP scopes), the merge library,
  task cards with contract tests, and a review of every card before it
  merges.
- **Executor (Codex with the user's model; opencode with a local model on
  request):** one task card at a time.
- **Human:** approves plans, merges task branches, runs anything with real
  credentials or real notes.

### A card's life

1. The planner writes `docs/tasks/todo/NNNN-slug.md` from
   [TEMPLATE.md](../tasks/TEMPLATE.md), with given files under
   `NNNN-slug/_given/<repo path>` (the underscore keeps `go ./...` out), and
   checks the given tests against a reference solution that is then deleted.
   Given files are formatted as the gates format them (`gofmt`; `make ui-fmt`
   for app tests). Stubs the card replaces may live on the branch if it
   stays green.
2. `make task T=NNNN` (`tools/taskrun run`) requires a clean tree, creates
   branch `task/NNNN` from the current branch, copies the given files into
   place, moves the card to `doing/` and commits that as
   `chore(tasks): start T-NNNN …`.
3. It runs the executor with the card (`EXECUTOR`, `codex` by default):
   `codex exec` in the `workspace-write` sandbox, with Go's build cache and
   golangci-lint's and mise's caches writable and the network on (tests start
   servers on loopback). Codex loads `AGENTS.md` itself and runs the gates
   through `mise exec -- make …`; it cannot reach the nsl machine, so for a
   card that touches `app/` taskrun runs `make ui-fmt` between attempts.
   (`EXECUTOR=opencode` runs `opencode run --model $(EXECUTOR_MODEL)`, which
   loads `docs/tasks/EXECUTOR.md` through `opencode.json` and may only run
   `make`, `go` and read-only commands.)
4. After each attempt it runs the acceptance command, then `taskrun verify`;
   on either failure it resumes the session with the error and a digest of
   the output, up to `-attempts` (3).
5. `taskrun verify` (and `make task-verify T=NNNN`) runs acceptance,
   `make check` and, for a card that touches `app/`, `make ui-check`; then,
   because `make check` formats, it checks that every changed file is in
   `touch` and every given file still matches `_given`. Any failure fails
   the card.
6. `make task-finish T=NNNN` re-verifies, moves the card to `done/` and
   commits the work. The human reviews and merges the branch; the planner
   reviews all of a milestone's merged cards.

### Batching cards

Cards run one at a time in an executor worktree,
`git worktree add .worktrees/exec -b <milestone>/cards`, so the main
checkout may stay dirty while cards run (taskrun requires a clean tree).
Cards that share no file and no Go package may run at once in separate
executor worktrees (`.worktrees/exec-<name>`), each on its own branch, as
M2's store and notesctl cards did.
Each card branches from the worktree's current branch, and the planner
merges each reviewed card back before the next starts.
 Cards that touch the same file, or the same Go package, run one
after another with the first merged before the second starts: two cards
branched from the same commit cannot see each other's code, and same-package
helpers collide only at merge. A card whose given tests call another card's
code waits for that card's merge.

### Sizing

- **S:** at most 150 non-test lines in at most 3 files.
- **M:** at most 400 lines in at most 6 files.
- **L:** at most 1,000 lines: one package or one feature.

A card of any size keeps one concern, a fixed file list and given tests at
its boundary; schemas, migrations, the merge library and security boundaries
stay with the planner regardless of size.

## Operational notes

- `make tasks` lists cards by state; `make accept T=NNNN` runs one card's
  acceptance command.
- A run that fails three times leaves its branch for review; the planner
  either sharpens the card or takes the work.
- Codex uses the model in `~/.codex/config.toml` unless `EXECUTOR_MODEL`
  names one.

## References

- Rationale: [ADR-0010](../adr/0010-claude-plans-and-codex-runs-task-cards.md)
- Contracts: [tasks/TEMPLATE.md](../tasks/TEMPLATE.md), [tasks/EXECUTOR.md](../tasks/EXECUTOR.md)
- Built in: [M0 plan, Phase 6](../plans/0001-m0-foundations.md)
