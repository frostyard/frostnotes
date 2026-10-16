# Design: overview

Living document. Rationale: [ADR-0003](../adr/0003-plain-markdown-files-are-the-source-of-truth.md),
[ADR-0004](../adr/0004-a-notes-daemon-with-thin-clients.md),
[ADR-0005](../adr/0005-sync-through-a-hub-with-revisions-and-three-way-merges.md).
Contracts: [specs/rpc-protocol.md](../specs/rpc-protocol.md), [specs/rpc-api.md](../specs/rpc-api.md).

**Status: M0 built** ([plan 0001](../plans/0001-m0-foundations.md)): notesd
with the store, the socket and caller kinds; `notesctl hello` and `vault`;
the app shell on the desktop and in a browser; `internal/merge` and
`internal/markdown`'s first parsers. Everything else here is the planned
system, rewritten as each milestone lands ([roadmap](../plans/0002-roadmap.md)).

## Overview

Frostnotes is a notes app in the spirit of Obsidian: plain markdown files
with YAML frontmatter, a desktop app, a web app, built-in sync, and an MCP
server for agents. A Go daemon, notesd, owns every vault; everything else is
a thin client.

```
 desktop app (Tauri v2, WebKitGTK)      notesctl           agent (stdio MCP)
 React UI ── Client ── TauriTransport   Client             notesctl mcp
        │ lines over Tauri IPC            │                    │
 Rust bridge (moves lines) ─────┐         │                    │
                                ▼         ▼                    ▼
              $XDG_RUNTIME_DIR/frostnotes/notesd.sock   JSON-RPC 2.0, one message per line
                                │
 notesd, device role ── rpcserver ── api.Router ── engine ─┬─ vault: scan, watch, parse, write
                                                           ├─ index (SQLite, FTS5) and local versions
                                                           ├─ render (goldmark, sanitizer)
                                                           └─ sync client ──┐ WebSocket, device token
                                                                            ▼
 notesd, hub role ── HTTPS ─┬─ /sync  devices: push, pull, blobs ─ revision log ─ merge
                            ├─ /api   browsers: JSON-RPC over WebSocket, session
                            ├─ /mcp   agents: Streamable HTTP, scoped token
                            └─ /      the web app (the same React UI, WebSocketTransport)
```

## Design

- **notesd** (`cmd/notesd`) is one pure-Go binary. In the device role it
  serves the socket and syncs each vault with its hub. In the hub role it
  also serves HTTPS. A hub has vault directories on disk like any device.
- **The vault engine** scans and watches each vault, parses notes, keeps
  the index and the local versions, and performs every write as a
  compare-and-merge ([vault.md](vault.md),
  [ADR-0006](../adr/0006-every-write-names-the-version-it-changes.md)).
- **Sync** is a star: each device pushes changed files with their base
  revision, and the hub appends or merges them into its revision log
  ([sync.md](sync.md)).
- **The contract** is generated from `schema/rpc/*.yaml` by `tools/rpcgen`
  into Go and TypeScript, and each domain declares which callers may use it
  ([ADR-0008](../adr/0008-generate-the-rpc-contract-from-a-yaml-idl.md)).
- **The UI** is one React app behind a `Transport`: the Tauri bridge on the
  desktop, a WebSocket in the browser, and a mock in tests. The editor is
  CodeMirror 6 with live preview
  ([ADR-0007](../adr/0007-edit-in-codemirror-6-with-live-preview.md),
  [ADR-0011](../adr/0011-look-like-frostmails-sibling-with-obsidians-layout.md)).
- **MCP** tools are hand-written adapters over the engine, served over stdio
  by `notesctl mcp` and over HTTP by the hub
  ([ADR-0012](../adr/0012-agents-reach-notes-through-mcp.md)).

### Application icon

Frostnotes uses a snowflake-marked notebook with a faceted blue spine, ruled
cover and ribbon bookmark. Its chamfered frame, palette and snow crystal
match Frostmail's icon ([ADR-0013](../adr/0013-use-a-sibling-frostyard-notebook-icon.md)),
extending the sibling identity in [ADR-0011](../adr/0011-look-like-frostmails-sibling-with-obsidians-layout.md).
It belongs to the [M0 app shell](../plans/0001-m0-foundations.md#phase-4--app-shell).

The editable master is `app/src-tauri/icons/icon.svg`. Its comments record
the exact color tokens from core's `frostyard-design/tokens/colors.css`.
Run `python3 scripts/render-icons.py` with PyGObject, GdkPixbuf and its
librsvg SVG loader to regenerate the 32, 128 and 512 pixel RGBA PNGs used
by Tauri. Commit the master and all three exports together; ordinary builds
need only the PNGs. Check the two sibling icons at native size on light and
dark backgrounds after editing. This artwork does not change the UI tokens.

### Planned layout

| Path | Contents |
| --- | --- |
| `cmd/notesd`, `cmd/notesctl` | the daemon; the CLI (clix/cobra, one `newXxxCmd(opts)` per file) |
| `schema/rpc/`, `api/` | the IDL; generated types, router and client plus the hand-written runtime |
| `internal/store`, `internal/events` | SQLite (modernc): migrations, the single-writer `Tx`, the changes log; event fan-out |
| `internal/rpcserver`, `internal/engine` | the socket server; the API domains over the store |
| `internal/vault` | scanning, watching, ignore rules, atomic compare-and-swap writes |
| `internal/markdown` | frontmatter, links, tags, headings, blocks and tasks from a note |
| `internal/merge` | diff3 by line and word, frontmatter by key (planner-owned) |
| `internal/blobs` | the content-addressed blob store |
| `internal/render` | goldmark rendering and the HTML sanitizer |
| `internal/sync`, `internal/hub` | the device's sync client; the hub's log, pairing, HTTP and sessions |
| `internal/mcpserver` | MCP tools over the engine |
| `app/` | Tauri v2 + React + TypeScript; `app/src/rpc/gen/api.ts` is generated |
| `tools/rpcgen`, `tools/taskrun`, `tools/vaultgen` | the contract generator, the task-card runner, generated test vaults |

## Key patterns

- **Files are the truth.** The index is derived and can be rebuilt; notesd's
  state never lives in the vault.
- **Every write names its base.** Writers that are behind get a merge, never
  an overwrite.
- **Write, emit, commit.** Every database change is a `store.Tx`, and its
  durable event is written in the same transaction and delivered after
  commit, as in frostmail.
- **Contract first.** Change `schema/rpc/*.yaml`, run `make gen`, then
  implement.
- **One merger.** Between devices, only the hub merges.

## Operational notes

Planned configuration:

| Variable | Default | Used by |
| --- | --- | --- |
| `FROSTNOTES_DATA_DIR` | `$XDG_DATA_HOME/frostnotes` | notesd: `notesd.db`, `blobs/` |
| `FROSTNOTES_SOCKET` | `$XDG_RUNTIME_DIR/frostnotes/notesd.sock` | notesd, notesctl, the app |
| `FROSTNOTES_SECRETS=file` | unset | notesd: keep device tokens in a private file, not the Secret Service (development) |

The socket directory is 0700 and the socket 0600, owned by the user. The
database must be on a local file system; vaults may be anywhere.

## References

- Rationale: [ADR-0003](../adr/0003-plain-markdown-files-are-the-source-of-truth.md) through [ADR-0012](../adr/0012-agents-reach-notes-through-mcp.md)
- Details: [vault.md](vault.md), [sync.md](sync.md)
- Built in: [roadmap](../plans/0002-roadmap.md), starting with [M0](../plans/0001-m0-foundations.md)
