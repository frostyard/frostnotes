# Plan: M5, queries

This plan builds the query half of
[roadmap M5, Graph and queries](0002-roadmap.md):
[ADR-0037](../adr/0037-queries-are-obsidian-bases-compiled-to-sql.md)'s
subset of Obsidian Bases, which notesd compiles to SQL over its index. Its
contract is [specs/bases.md](../specs/bases.md); where it fits is
[design/vault.md — Queries](../design/vault.md#queries). M5's done-when for
queries is that the user's Dataview dashboards are rebuilt as `.base`
files and render. The graph half of M5 (the graph API and the renderer
choice) is not in this plan.

Work is split per
[ADR-0010](../adr/0010-claude-plans-and-codex-runs-task-cards.md): the
planner owns the compiler and its contract. No cards.

Status (2026-11-16):
- ADR-0037 was accepted on 2026-11-16.
- Phase 1 is built on a pull request.

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

- The MCP read tool `query_notes` over `note.query`, as ADR-0037 shapes
  it, in [specs/mcp.md](../specs/mcp.md): the read tool list grows from 5
  to 6.
- A view with a notice is a tool error that names the view (check 7).
- **Done when:** an agent with a read scope gets a base's rows through
  lodge's `/mcp/notes`, pinned to its scope's vault, and the tool error
  for a kanban view and for `groupOrder` (check 7).

## Phase 3 — Base views in the apps

- `![[File.base]]`, `![[File.base#View]]` and `base` code blocks render as
  table and card views in reading view and live preview, in the desktop
  app and the web app; they run the query again on the vault's `note.*`
  events ([design/app.md](../design/app.md)).
- Values render as text through the existing sanitizer, never script
  (check 8). Frostnotes never rewrites a `.base` file (check 9).
- **Done when:** a `.base` file and an embedded view show the fixture
  vault's rows in both apps.

## Phase 4 — The user's dashboards

- Inventory the user's Dataview dashboards and map each to a base in the
  subset, or name its gap, with made-up names in every fixture (check 1).
- Open the fixture vault in Obsidian, record its rows, and compare them
  with Frostnotes' rows (check 3). Each `choice` in
  [specs/bases.md](../specs/bases.md) that Obsidian answers otherwise
  changes with its code and fixtures.
- **Done when:** M5's query done-when holds: the user's dashboards are
  rebuilt as `.base` files and render, or the user accepts the named
  gaps.

## ADR-0037's checks

| Check | Phase | State |
| --- | --- | --- |
| 1. Inventory the dashboards | 4 | open |
| 2. Golden fixtures | 1 | built |
| 3. Obsidian's rows for the fixture vault | 4 | open: every `choice` in the spec waits on it |
| 4. Fuzz: no base text in SQL text | 1 | built |
| 5. Refusals, ignored keys, unshown columns, notices | 1 | built |
| 6. Benchmark | 1 | built; budget below |
| 7. `query_notes` under a read scope, notices as tool errors | 2 | open |
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
  [specs/rpc-api.md](../specs/rpc-api.md); later
  [specs/mcp.md](../specs/mcp.md) and [design/app.md](../design/app.md)
- Decisions: [ADR-0037](../adr/0037-queries-are-obsidian-bases-compiled-to-sql.md),
  [ADR-0012](../adr/0012-agents-reach-notes-through-mcp.md),
  [ADR-0007](../adr/0007-edit-in-codemirror-6-with-live-preview.md)
- Roadmap: [plan 0002 — M5](0002-roadmap.md)
