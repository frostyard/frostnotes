# 0010 — Claude plans and verifies; Codex runs task cards

- **Status:** Accepted
- **Date:** 2026-10-16

## Context

Frostmail went from an empty repository on 2026-10-07 to a mail, calendar
and contacts client in its daily-driver trial nine days later, with a
split: Claude wrote plans, ADRs, contracts and given tests, built the hard
parts and reviewed; an executor ran task cards (frostmail ADR-0008), first
a local model, then Codex (frostmail ADR-0021). Task cards with a fixed
file list and protected given tests kept the executor's output honest
across 160 cards.

## Decision

- **Claude (planner)** writes milestone plans, ADRs, designs, specs, the
  RPC and SQL schemas and migrations, interfaces, security boundaries (the
  hub's authentication, device pairing, the HTML sanitizer, MCP scopes),
  the merge library, and each task card with its given tests. It reviews
  every card before merge and all of a milestone's cards at its end.
- **Codex (executor)** runs one card at a time through `tools/taskrun`
  (`make task T=NNNN`), copied from frostmail at M0: its own branch, the
  given files copied in, retries with the failure output, and rejection if
  a file outside the card's `touch` list changed, a given file changed, or
  the acceptance command or the gates fail. opencode with a local model
  remains selectable.
- **The human** approves plans and ADRs, merges task branches and runs
  anything with real credentials or real notes.
- **Cards are sized S, M or L** as in frostmail, keep one concern, and never
  touch schemas, generated code, dependencies, migrations or the merge
  library.
- **Real notes never enter the repository.** Tests use generated vaults
  and hand-written fixtures.

## Consequences

- Most code volume goes to Codex; contracts and given tests decide
  correctness.
- Card writing costs planner time up front, especially for sync, where
  given tests must describe merges exactly.
- `docs/tasks/` joins the four doc categories, with `TEMPLATE.md` and
  `EXECUTOR.md`.

## Alternatives considered

- **Claude writes everything:** slower per milestone and spends planner
  attention on code a card can specify.
- **The executor plans too:** frostmail's results came from the split.

## References

- Shapes: [design/agent-workflow.md](../design/agent-workflow.md), [tasks/EXECUTOR.md](../tasks/EXECUTOR.md)
- Builds on: frostmail ADR-0008 and ADR-0021
- Built in: [M0 plan](../plans/0001-m0-foundations.md)
