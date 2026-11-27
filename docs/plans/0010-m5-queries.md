# Plan: M5, queries

This plan builds the query half of
[roadmap M5, Graph and queries](0002-roadmap.md):
[ADR-0037](../adr/0037-queries-are-obsidian-bases-compiled-to-sql.md)'s
subset of Obsidian Bases, which notesd compiles to SQL over its index. Its
contract is [specs/bases.md](../specs/bases.md); where it fits is
[design/vault.md — Queries](../design/vault.md#queries). M5's done-when for
queries is that the user's Dataview dashboards are rebuilt as `.base`
files and render in Lodge's React app. [ADR-0050](../adr/0050-frostnotes-has-no-desktop-window.md)
removes the Frostnotes desktop window; Obsidian is a comparison tool,
not another Frostnotes UI target. The graph half of M5 (the graph API
and the renderer choice) is not in this plan.

Work is split per
[ADR-0010](../adr/0010-claude-plans-and-codex-runs-task-cards.md): the
planner owns the compiler and its contract. No cards.

Status (2026-11-26):
- ADR-0037 was accepted on 2026-11-16.
- Phase 1 is done: frostnotes#245 merged on 2026-11-16.
- Phase 2 is built: [#247](https://github.com/frostyard/frostnotes/pull/247)
  merged on 2026-11-17 with MCP handler and Lodge socket tests. A run
  through Lodge's `/mcp/notes` route is not recorded here.
- Phases 3 and 4 are open; no React rendering or dashboard acceptance
  evidence is recorded.

## Phase 1 — `note.query` and `notesctl query` (1 PR)

- The method `note.query` in `schema/rpc/note.yaml`, additive in protocol
  1 ([specs/rpc-api.md](../specs/rpc-api.md), generated), and its
  contract in [specs/bases.md](../specs/bases.md).
- `internal/query`: the YAML parser, the subset check, the statement
  parser and the compiler to SQL; `store.DB` runs it; the engine serves
  it ([design/vault.md — Queries](../design/vault.md#queries)).
- `notesctl query PATH [--view NAME] [--json]`.
- Tests: a fixture vault and its expected rows for each statement form
  (check 2), a fuzz test that no base text reaches SQL text (check 4), a
  refusal test for each feature outside the subset, with the ignored
  keys, unshown columns and view notices (check 5), a benchmark (check
  6), and each operator, `!` and `not` on a missing value, a value that
  does not convert and an attachment row (check 10).
- No UI, no MCP tool and no graph.
- **Done when:** `notesctl query` prints the rows of a `.base` file from
  the fixture vault, and `make ci` passes with the tests above.

## Phase 2 — `query_notes` for agents

- `query_notes` is a read tool over `note.query`. Use the
  [MCP query contract](../specs/mcp.md#queries) and the
  [Lodge MCP route](../specs/mcp.md#through-lodge).
- **Done when:** record an MCP run through a scratch Lodge's `/mcp/notes`
  using a read-only token pinned to a generated vault. Both `path` and
  `filters` calls return the expected rows, properties and count. A
  matching fixture in a second vault does not appear in the results.
  The kanban and `groupOrder` views each return a tool error naming the
  view and the notice, with no `files` payload (check 7). The run must
  use Lodge's route, not call notesd's socket directly.

## Phase 3 — Base views in Lodge's React app

- Use `note.query` from the [Bases spec](../specs/bases.md#notequery) in
  React at `/app/notes/`. Follow the [reading view](../design/app.md#reading-view)
  and [CodeMirror live preview](../design/app.md#live-preview) designs.
- **Done when:** `make ui-check` passes and a browser run through a
  scratch Lodge records the following against a generated fixture:
  1. Opening a `.base` file shows its default view. `![[File.base]]`,
     `![[File.base#View]]` and a `base` fenced block show the expected
     view in reading view and live preview. Exercise both table and
     cards, and compare the displayed rows and ordering with the
     fixture's expected results.
  2. An edit to a matching note changes a result row after a `note.*`
     event, without reloading the page. An unsupported view shows its
     notice rather than an empty result.
  3. HTML-like values display as text, with no script execution
     (check 8). The `.base` file's bytes are unchanged after opening,
     rendering and refreshing its views (check 9).

## Phase 4 — The user's dashboards in Lodge

- Inventory the user's Dataview dashboards and map each to a `.base`
  file in the supported subset or name its gap (check 1). Use made-up
  names and data in every committed fixture; keep the real vault and
  its inventory out of the repository.
- Open the generated fixtures in Obsidian and record its rows for
  comparison with Frostnotes (check 3). If Obsidian differs on a
  `choice` in the [Bases spec](../specs/bases.md), update the spec, code
  and fixtures together.
- **Done when:** the private inventory has an outcome for every dashboard.
  For each rebuilt dashboard, record the `.base` view opened in Lodge's
  React app, its expected and observed rows, and any discrepancy. Record
  the Obsidian version and the fixture comparison, including the outcome
  of each `choice`. Every dashboard either renders its expected results
  in Lodge or has a named gap with explicit user acceptance. Keep only
  generated evidence and a redacted outcome summary in the repository.

## ADR-0037's checks

| Check | Phase | State |
| --- | --- | --- |
| 1. Inventory the dashboards | 4 | open |
| 2. Golden fixtures | 1 | built |
| 3. Obsidian's rows for the fixture vault | 4 | open: every `choice` in the spec waits on it |
| 4. Fuzz: no base text in SQL text | 1 | built |
| 5. Refusals, ignored keys, unshown columns, notices | 1 | built |
| 6. Benchmark | 1 | built; budget below |
| 7. `query_notes` under a read scope, notices as tool errors | 2 | built: handler and Lodge socket tests in `internal/mcpserver`; Lodge `/mcp/notes` run not recorded |
| 8. Rendering through the sanitizer | 3 | open |
| 9. No rewrite of a `.base` file | 3 | open (Phase 1 only reads) |
| 10. Operators on missing, unconvertible and attachment values | 1 | built |

Benchmark budget (check 6): `BenchmarkQuery` in `internal/query` runs a
filter with a sort, a group and a text search over a generated vault of
1,000 notes. Budget: 50 ms for each query on a developer machine.
Measured on 2026-11-16 on the team's VM (20 runs each): filter and sort
5.0 ms, group 11.1 ms, text search 13.9 ms. The benchmark is not a CI
gate; run it after a change to the compiler or the store's query.

## Open questions

- **Choices against Obsidian:** the spec's `choice` rules (empty `and`,
  `or` and `not`; text order; lists against scalars; `file.tags` without
  `#`; the root folder's text; the type order in a sort) wait for
  Obsidian's answers in Phase 4.

## References

- Implements: [specs/bases.md](../specs/bases.md),
  [design/vault.md — Queries](../design/vault.md#queries),
  [specs/rpc-api.md](../specs/rpc-api.md),
  [specs/mcp.md](../specs/mcp.md#queries),
  [design/app.md — In a browser](../design/app.md#in-a-browser)
- Decisions: [ADR-0050](../adr/0050-frostnotes-has-no-desktop-window.md),
  [ADR-0037](../adr/0037-queries-are-obsidian-bases-compiled-to-sql.md),
  [ADR-0012](../adr/0012-agents-reach-notes-through-mcp.md),
  [ADR-0007](../adr/0007-edit-in-codemirror-6-with-live-preview.md)
- Roadmap: [plan 0002 — M5](0002-roadmap.md)
