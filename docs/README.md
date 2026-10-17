# Documentation

Docs are split by the question they answer (frostyard/core ADR-0025):

| Directory | Question | Contents |
| --- | --- | --- |
| [adr/](adr/) | **Why** did we choose this? | Architecture Decision Records — immutable once accepted; superseded, never edited |
| [design/](design/) | **How** does it fit together? | Living documents describing the current architecture |
| [specs/](specs/) | **What exactly** is the contract? | Precise, testable interface definitions |
| [plans/](plans/) | **When/in what order** do we build? | Roadmaps and phase plans; updated as work lands |

Task cards for the executor live in [tasks/](tasks/), with their
[template](tasks/TEMPLATE.md) and [executor rules](tasks/EXECUTOR.md)
([agent workflow](design/agent-workflow.md)).

Org-wide decisions that bind this repository: [org-adrs.md](org-adrs.md).

## Index

### Decisions (ADRs)

- [0001 — Record architecture decisions](adr/0001-record-architecture-decisions.md)
- [0002 — Agent-portable instruction surface](adr/0002-agent-portable-instruction-surface.md)
- [0003 — Plain markdown files are the source of truth](adr/0003-plain-markdown-files-are-the-source-of-truth.md)
- [0004 — A notes daemon with thin clients](adr/0004-a-notes-daemon-with-thin-clients.md)
- [0005 — Sync through a hub with revisions and three-way merges](adr/0005-sync-through-a-hub-with-revisions-and-three-way-merges.md)
- [0006 — Every write names the version it changes](adr/0006-every-write-names-the-version-it-changes.md)
- [0007 — Edit in CodeMirror 6 with live preview](adr/0007-edit-in-codemirror-6-with-live-preview.md)
- [0008 — Generate the RPC contract from a YAML IDL](adr/0008-generate-the-rpc-contract-from-a-yaml-idl.md)
- [0009 — Development environment and gates](adr/0009-development-environment-and-gates.md)
- [0010 — Claude plans and verifies; Codex runs task cards](adr/0010-claude-plans-and-codex-runs-task-cards.md)
- [0011 — Look like frostmail's sibling, with Obsidian's layout](adr/0011-look-like-frostmails-sibling-with-obsidians-layout.md)
- [0012 — Agents reach notes through MCP](adr/0012-agents-reach-notes-through-mcp.md)
- [0013 — Use a sibling Frostyard notebook icon](adr/0013-use-a-sibling-frostyard-notebook-icon.md)
- [0014 — The API carries text, and keeps the bytes it cannot show](adr/0014-the-api-carries-text-and-keeps-bytes-it-cannot-show.md)
- [0015 — Index what comments hide, as Obsidian does](adr/0015-index-what-comments-hide-as-obsidian-does.md)
- [0016 — Pair devices with a one-time code and a revocable token](adr/0016-pair-devices-with-a-one-time-code-and-a-revocable-token.md)
- [0017 — Revisions number the hub's log, and hashes name content](adr/0017-revisions-number-the-hubs-log-and-hashes-name-content.md)
- [0018 — Move blobs over HTTP, named by their hash](adr/0018-move-blobs-over-http-named-by-their-hash.md)
- [0019 — Keep every revision's record, and thin the content by age](adr/0019-keep-every-revisions-record-and-thin-the-content-by-age.md)
- [0020 — Test sync with a seeded simulation of a hub and devices](adr/0020-test-sync-with-a-seeded-simulation-of-a-hub-and-devices.md)

### Design

- [Overview](design/overview.md) — notesd, its two roles, the clients, the planned layout
- [Vault](design/vault.md) — scan, watch, parse, index, link resolution, writes, versions
- [Sync](design/sync.md) — the hub's revision log, pairing, push and pull, blobs, merges, conflicts, retention, the simulation
- [Agent workflow](design/agent-workflow.md) — planner, executor and taskrun; a card's life; sizing


### Specs

- [RPC protocol](specs/rpc-protocol.md) — transports, caller kinds, messages, connection setup, events
- [RPC API](specs/rpc-api.md) — every method, type and event (generated from `schema/rpc`)
- [Markdown](specs/markdown.md) — the Obsidian dialect, link resolution and the parse fixtures
- [MCP tools](specs/mcp.md) — what `notesctl mcp` offers an agent: tools, inputs, outputs, edits


### Plans

- [0001 — M0 foundations](plans/0001-m0-foundations.md)
- [0002 — Roadmap to replacing Obsidian](plans/0002-roadmap.md)
- [0003 — M1 headless vault](plans/0003-m1-headless-vault.md)
- [0004 — M2 sync](plans/0004-m2-sync.md)

## Conventions

- **New docs start from their category's `TEMPLATE.md`** (in each directory).
- New decision → new ADR with the next number; if it reverses an old one, mark
  the old one `Superseded by NNNN` rather than editing it.
- Design docs are updated in place to always reflect reality.
- Specs change only alongside the code that implements them.
- Cross-links between categories are mandatory in both directions — see the
  documentation rules in [AGENTS.md](../AGENTS.md) (CLAUDE.md/GEMINI.md are
  symlinks to it, ADR-0002).
- Adding a doc means adding it to the index above.
