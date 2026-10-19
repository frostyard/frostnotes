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
| **M2 Sync** ([plan 0004](0004-m2-sync.md); [design/sync.md](../design/sync.md), ADRs [0016](../adr/0016-pair-devices-with-a-one-time-code-and-a-revocable-token.md) to [0020](../adr/0020-test-sync-with-a-seeded-simulation-of-a-hub-and-devices.md)) | the sync protocol and merge specs, the revision log, the hub role, pairing, push and pull, merges and conflicts, moves and deletes, retention, `internal/synctest` | blob transfer, `notesctl hub/sync/history` commands, simulation scenarios, golden merge cases | 10,000 seeded simulation steps over a hub and three devices converge with no lost token and no conflict file; the user's vault syncs between two machines and a hub for 7 days with zero lost edits; a 100 MB attachment syncs |
| **M3 Desktop app** ([plan 0005](0005-m3-desktop-app.md); [design/app.md](../design/app.md), [specs/ui.md](../specs/ui.md), ADRs [0021](../adr/0021-keep-the-workspace-and-app-preferences-on-each-device.md) to [0023](../adr/0023-the-app-shows-files-by-their-hash-and-uploads-them-through-notesd.md)) | the Tauri bridge and transport, the workspace model (tabs, splits), the editor core (CM6, saves with a base, applying merges), the live-preview framework, the reading view, the merge view, `specs/ui.md` | file tree, quick switcher, command palette, search pane, backlinks, outgoing links, outline and properties panes, live-preview widgets, version history view, settings, daily notes and templates, keyboard map | the user uses Frostnotes instead of Obsidian on their desktop for 7 days, synced through M2's hub; typing in a 10,000-line note stays under 16 ms a keystroke; Obsidian's default shortcuts work |
| **M4 Web app and remote MCP** ([plan 0006](0006-m4-web-app.md); [design/hub.md](../design/hub.md), [specs/hub-http.md](../specs/hub-http.md), ADRs [0026](../adr/0026-the-hub-stays-on-the-tailnet-and-joins-it-itself.md) to [0031](../adr/0031-a-narrow-window-takes-obsidians-phone-layout.md)) | the hub as its own tailnet node (`tsnet`), browser sign-in by tailnet identity (sessions, Origin and CSRF), method-level callers, `/api`, `/blobs` and the web build's routes, `/mcp` with scoped tokens and the `agent` caller, the hub transport, the narrow layout's frame, a security review | the store's logins and tokens, `notesctl hub` access commands, the phone layout's components (drawers, bottom bar, tab switcher, editing toolbar, sheets), the Hub settings' lists, the manifest and icons | the same UI works in Firefox on a second machine and on the user's phone against the hub over the tailnet, on the notes the desktop app writes (M3's second machine moved here on 2026-10-18: the user has one Linux desktop); an edit on the phone shows on the desktop within 2 s; Claude Code uses the hub's `/mcp` with a scoped token (claude.ai and the Claude apps moved to later work on 2026-10-18); an independent review finds no open XSS, CSRF or session issue |
| **M5 Graph and queries** | the query ADR (Obsidian Bases compatibility), the query compiler to SQL, the graph API, the graph renderer choice | `.base` table and card views, the global and local graph with filters, `query_notes` for MCP | the user's dataview dashboards are rebuilt as `.base` files and render; the 666-note graph pans at 60 fps |
| **M6 Packaging** | Flatpak with portals, the hub's container image and systemd unit, frostyard apt packages | AppStream metadata, icons, install docs, CI | clean installs on snow and Debian; a hub deployed from the docs in under 10 minutes |

## Top risks

1. **Lost edits in sync.** One merger, revisions with bases, versions kept
   everywhere, the token simulation, and planner-owned merge code.
2. **Live preview's long tail** (tables, nested lists, embeds). Source mode
   is always correct, and widgets land one at a time.
3. **The hub on a network.** It stays on the tailnet as a node of its
   own, with no public endpoint
   ([ADR-0026](../adr/0026-the-hub-stays-on-the-tailnet-and-joins-it-itself.md));
   browsers sign in by their tailnet identity with no credential to steal
   ([ADR-0027](../adr/0027-browsers-sign-in-with-their-tailnet-identity.md));
   agents carry tokens scoped to a vault and to read or write
   ([ADR-0028](../adr/0028-remote-mcp-serves-local-agents-with-scoped-tokens.md));
   and an independent security review closes M4.
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
endpoint and OAuth 2.1 (moved from M4 on 2026-10-18,
[ADR-0028](../adr/0028-remote-mcp-serves-local-agents-with-scoped-tokens.md)).

## Open questions

- *Answered 2026-10-18:* **hub sign-in and TLS.** The hub stays on the
  tailnet with its own node's certificate, and browsers sign in by their
  tailnet identity
  ([ADR-0026](../adr/0026-the-hub-stays-on-the-tailnet-and-joins-it-itself.md),
  [ADR-0027](../adr/0027-browsers-sign-in-with-their-tailnet-identity.md)).
- *Answered 2026-10-18:* **OAuth for remote MCP.** Not in M4: the hub's
  `/mcp` serves local agents with scoped tokens, and claude.ai's
  connectors wait in later work
  ([ADR-0028](../adr/0028-remote-mcp-serves-local-agents-with-scoped-tokens.md)).
- **Query format:** Obsidian Bases (`.base` YAML and `base` code blocks) or
  a Frostnotes condition language. Leaning Bases. By M5, as an ADR.

## References

- Implements: [design/overview.md](../design/overview.md), [design/vault.md](../design/vault.md), [design/sync.md](../design/sync.md), [design/app.md](../design/app.md), [design/hub.md](../design/hub.md), [specs/markdown.md](../specs/markdown.md), [specs/ui.md](../specs/ui.md), [specs/hub-http.md](../specs/hub-http.md)
- Milestone plans: [M0](0001-m0-foundations.md), [M1](0003-m1-headless-vault.md), [M2](0004-m2-sync.md), [M3](0005-m3-desktop-app.md), [M4](0006-m4-web-app.md)
- Decisions: [ADR-0003](../adr/0003-plain-markdown-files-are-the-source-of-truth.md) through [ADR-0012](../adr/0012-agents-reach-notes-through-mcp.md); for M2, [ADR-0016](../adr/0016-pair-devices-with-a-one-time-code-and-a-revocable-token.md) through [ADR-0020](../adr/0020-test-sync-with-a-seeded-simulation-of-a-hub-and-devices.md); for M3, [ADR-0021](../adr/0021-keep-the-workspace-and-app-preferences-on-each-device.md) through [ADR-0025](../adr/0025-patch-what-costs-the-editor-its-frames.md); for M4, [ADR-0026](../adr/0026-the-hub-stays-on-the-tailnet-and-joins-it-itself.md) through [ADR-0031](../adr/0031-a-narrow-window-takes-obsidians-phone-layout.md)
