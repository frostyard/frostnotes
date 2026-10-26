# Plan 0002: roadmap to replacing Obsidian

Frostnotes replaces the user's Obsidian, which they open over an SMB share
with no other sync: their vault (666 notes, 577 with frontmatter, daily
notes from a template) on two Linux machines, a hub, a browser and a phone, with agents reading and writing
through MCP. Each milestone gets its own plan with task cards before it
starts. Planner work stays with Claude; executor work goes on cards
([ADR-0010](../adr/0010-claude-plans-and-codex-runs-task-cards.md)).

Sync comes before the UI because it shapes storage and is the riskiest part.
The engine is headless and testable before any window exists, as in
frostmail.

| Milestone | Planner | Executor | Done when |
| --- | --- | --- | --- |
| **M0 Foundations** ([plan 0001](0001-m0-foundations.md); done 2026-10-16) | repository, gates, nsl machine, rpcgen and taskrun from frostmail, store and socket runtime, app shell, spikes | calibration cards | see plan 0001 |
| **M1 Headless vault** ([plan 0003](0003-m1-headless-vault.md); done 2026-10-16; [design/vault.md](../design/vault.md), [specs/markdown.md](../specs/markdown.md)) | the markdown spec and fixtures, the index schema, scan and watch, link resolution, the write path, `internal/merge` for local merges, local versions, the MCP server | parsers against the fixtures, index queries, full-text search, `notesctl ls/cat/search/backlinks/write/mv/versions/revert`, MCP tool handlers, `tools/vaultgen` | the user's vault indexes cold in under 5 s; every link in Obsidian's own dump of it (179 from 43 notes) is found and resolves as Obsidian resolved it; a vim edit is searchable within 1 s; `kill -9` during writes and indexing never damages a note or the index; Claude Code searches, reads and appends to today's daily note through `notesctl mcp` |
| **M2 Sync** ([plan 0004](0004-m2-sync.md); done 2026-10-25: its trial ended on 2026-10-24, its record is in the plan, and the user accepted it on 2026-10-25; [design/sync.md](../design/sync.md), ADRs [0016](../adr/0016-pair-devices-with-a-one-time-code-and-a-revocable-token.md) to [0020](../adr/0020-test-sync-with-a-seeded-simulation-of-a-hub-and-devices.md)) | the sync protocol and merge specs, the revision log, the hub role, pairing, push and pull, merges and conflicts, moves and deletes, retention, `internal/synctest` | blob transfer, `notesctl hub/sync/history` commands, simulation scenarios, golden merge cases | 10,000 seeded simulation steps over a hub and three devices converge with no lost token and no conflict file; the user's vault syncs between two machines and a hub for 7 days with zero lost edits; a 100 MB attachment syncs |
| **M3 Desktop app** ([plan 0005](0005-m3-desktop-app.md); done 2026-10-25: its trial ended on 2026-10-25, its record is in the plan, and the user accepted it on 2026-10-25; [design/app.md](../design/app.md), [specs/ui.md](../specs/ui.md), ADRs [0021](../adr/0021-keep-the-workspace-and-app-preferences-on-each-device.md) to [0023](../adr/0023-the-app-shows-files-by-their-hash-and-uploads-them-through-notesd.md)) | the Tauri bridge and transport, the workspace model (tabs, splits), the editor core (CM6, saves with a base, applying merges), the live-preview framework, the reading view, the merge view, `specs/ui.md` | file tree, quick switcher, command palette, search pane, backlinks, outgoing links, outline and properties panes, live-preview widgets, version history view, settings, daily notes and templates, keyboard map | the user uses Frostnotes instead of Obsidian on their desktop for 7 days, synced through M2's hub; typing in a 10,000-line note stays under 16 ms a keystroke; Obsidian's default shortcuts work |
| **M4 Notes inside lodge** ([plan 0006](0006-m4-notes-inside-lodge.md); in progress, Phases 2 to 5 merged 2026-10-19, the hub moved behind lodge on 2026-10-26, and Phase 7's evidence is recorded with the write token's test of Claude Code open and the phone's latency as 1 sample; [specs/lodge-app.md](../specs/lodge-app.md), [design/hub.md](../design/hub.md), ADRs [0026](../adr/0026-notesd-runs-as-a-lodge-app.md), [0027](../adr/0027-a-narrow-window-takes-obsidians-phone-layout.md) and [0029](../adr/0029-inside-lodge-a-rooted-link-no-file-answers-opens-through-lodge.md); after [lodge's roadmap](https://github.com/frostyard/lodge/blob/main/docs/plans/0001-roadmap.md) phases 1 and 2) | notesd as lodge's notes app (the app socket, callers from `Lodge-*` headers, `/api`, `/content/`, `/device/`), method-level callers and the `writes` mark, an agent's scope enforced by the engine, attribution by connection, `/mcp` behind lodge's `/mcp/notes`, devices moving behind lodge, the lodge transport and lodge-client wiring, the narrow layout's frame, a security review | `notesctl hub url` and `relocate`, the phone layout's components (drawers, bottom bar, tab switcher, editing toolbar, sheets), the Hub settings' device lists, the editing commands the phone toolbar lacks | the same UI works in Firefox on a second machine and on the user's phone through lodge, on the notes the desktop app writes (M3's second machine moved here on 2026-10-18: the user has one Linux desktop); an edit on the phone shows on the desktop within 2 s; Claude Code uses lodge's `/mcp/notes` with a scoped token (claude.ai and the Claude apps moved to later work on 2026-10-18); an independent review finds no open XSS, CSRF or session issue |
| **Folder scopes for agents** ([plan 0007](0007-folder-scopes-for-agents.md); in progress, Phases 2 and 3 built 2026-10-19; [ADR-0028](../adr/0028-an-agents-scope-can-limit-writes-to-folders.md)) | the scope's `write=`, `except=` and `daily`, enforced by the engine on every writing method, `note.appendDaily`, the MCP tools under a limited scope | — | an agent with Porter's token (`vault=<id>;write=Porter/;except=Porter/Routines/;daily`) writes only under `Porter/` outside its routines and appends to today's daily note, and every other write is refused, in any spelling of a path's case |
| **M5 Graph and queries** | the query ADR (Obsidian Bases compatibility), the query compiler to SQL, the graph API, the graph renderer choice | `.base` table and card views, the global and local graph with filters, `query_notes` for MCP | the user's dataview dashboards are rebuilt as `.base` files and render; the 666-note graph pans at 60 fps |
| **M6 Packaging** | Flatpak with portals, the hub's container image and systemd unit, frostyard apt packages | AppStream metadata, icons, install docs, CI | clean installs on snow and Debian; a hub deployed from the docs in under 10 minutes |

## Top risks

1. **Lost edits in sync.** One merger, revisions with bases, versions kept
   everywhere, the token simulation, and planner-owned merge code.
2. **Live preview's long tail** (tables, nested lists, embeds). Source mode
   is always correct, and widgets land one at a time.
3. **The hub on a network.** Only lodge is on the network, a
   tailnet-only node with no public endpoint
   ([lodge ADR-0003](https://github.com/frostyard/lodge/blob/main/docs/adr/0003-lodge-is-the-only-process-on-the-network.md),
   [ADR-0004](https://github.com/frostyard/lodge/blob/main/docs/adr/0004-lodge-stays-on-the-tailnet-and-joins-it-itself.md)).
   It signs browsers in by their tailnet identity with no credential to
   steal ([lodge ADR-0005](https://github.com/frostyard/lodge/blob/main/docs/adr/0005-browsers-sign-in-with-their-tailnet-identity.md)).
   notesd serves a Unix socket only lodge may open. Its agents' scopes
   are enforced by its engine
   ([ADR-0026](../adr/0026-notesd-runs-as-a-lodge-app.md)). Independent
   security reviews of lodge and of notes close M4.
4. **CodeMirror and the graph in WebKitGTK.** The M0 spike measures typing
   latency, and M5 picks the graph renderer by measurement.
5. **Two markdown parsers disagreeing.** Shared fixtures run in Go and
   Vitest.
6. **Instructions planted in notes** reaching agents. Scoped tokens, writes
   in the history, and note content marked as data.

## Later

Windows and macOS desktop builds, Canvas (JSON Canvas), Obsidian Tasks
syntax and queries, publishing, live co-editing with shared cursors,
end-to-end encryption, a mobile app (Tauri mobile), a web clipper, and
claude.ai and the Claude apps as MCP clients, which need a public
endpoint and OAuth 2.1 (moved from M4 on 2026-10-18; lodge's
[ADR-0006](https://github.com/frostyard/lodge/blob/main/docs/adr/0006-agents-reach-each-app-through-its-own-mcp-endpoint.md)).

## Open questions

- *Answered 2026-10-18:* **hub sign-in and TLS.** lodge, frostyard's
  web shell, stays on the tailnet with its own node's certificate and
  signs browsers in by their tailnet identity. notesd runs behind it as
  an app ([ADR-0026](../adr/0026-notesd-runs-as-a-lodge-app.md); lodge's
  [ADR-0004](https://github.com/frostyard/lodge/blob/main/docs/adr/0004-lodge-stays-on-the-tailnet-and-joins-it-itself.md)
  and [ADR-0005](https://github.com/frostyard/lodge/blob/main/docs/adr/0005-browsers-sign-in-with-their-tailnet-identity.md)).
- *Answered 2026-10-18:* **OAuth for remote MCP.** Not in M4: lodge's
  `/mcp/notes` serves local agents with scoped tokens, and claude.ai's
  connectors wait in later work (lodge's
  [ADR-0006](https://github.com/frostyard/lodge/blob/main/docs/adr/0006-agents-reach-each-app-through-its-own-mcp-endpoint.md)).
- **Query format:** Obsidian Bases (`.base` YAML and `base` code blocks) or
  a Frostnotes condition language. Leaning Bases. By M5, as an ADR.

## References

- Implements: [design/overview.md](../design/overview.md), [design/vault.md](../design/vault.md), [design/sync.md](../design/sync.md), [design/app.md](../design/app.md), [design/hub.md](../design/hub.md), [specs/markdown.md](../specs/markdown.md), [specs/ui.md](../specs/ui.md), [specs/lodge-app.md](../specs/lodge-app.md)
- Milestone plans: [M0](0001-m0-foundations.md), [M1](0003-m1-headless-vault.md), [M2](0004-m2-sync.md), [M3](0005-m3-desktop-app.md), [M4](0006-m4-notes-inside-lodge.md), and [plan 0007](0007-folder-scopes-for-agents.md) (folder scopes for agents)
- Decisions: [ADR-0003](../adr/0003-plain-markdown-files-are-the-source-of-truth.md) through [ADR-0012](../adr/0012-agents-reach-notes-through-mcp.md); for M2, [ADR-0016](../adr/0016-pair-devices-with-a-one-time-code-and-a-revocable-token.md) through [ADR-0020](../adr/0020-test-sync-with-a-seeded-simulation-of-a-hub-and-devices.md); for M3, [ADR-0021](../adr/0021-keep-the-workspace-and-app-preferences-on-each-device.md) through [ADR-0025](../adr/0025-patch-what-costs-the-editor-its-frames.md); for M4, [ADR-0026](../adr/0026-notesd-runs-as-a-lodge-app.md), [ADR-0027](../adr/0027-a-narrow-window-takes-obsidians-phone-layout.md) and [ADR-0029](../adr/0029-inside-lodge-a-rooted-link-no-file-answers-opens-through-lodge.md); for agents' folder scopes, [ADR-0028](../adr/0028-an-agents-scope-can-limit-writes-to-folders.md), with lodge's [ADRs](https://github.com/frostyard/lodge/blob/main/docs/adr/0003-lodge-is-the-only-process-on-the-network.md)
