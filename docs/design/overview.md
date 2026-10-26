# Design: overview

Living document. Rationale: [ADR-0003](../adr/0003-plain-markdown-files-are-the-source-of-truth.md),
[ADR-0004](../adr/0004-a-notes-daemon-with-thin-clients.md),
[ADR-0005](../adr/0005-sync-through-a-hub-with-revisions-and-three-way-merges.md),
[ADR-0026](../adr/0026-notesd-runs-as-a-lodge-app.md)
(notesd as a lodge app).
Contracts: [specs/rpc-protocol.md](../specs/rpc-protocol.md), [specs/rpc-api.md](../specs/rpc-api.md),
[specs/lodge-app.md](../specs/lodge-app.md).

**Status (2026-10-25):**
- **M0 and M1 are done:** the headless vault, the index and MCP.
- **M2 and M3 are done** (2026-10-25, after their 7-day trials): sync
  through a hub, and the desktop app.
- **M4 is built in part:** notesd as lodge's notes app, `/mcp` through
  lodge, and the web app.

What M5 and later add is the planned system. This document is rewritten as
each milestone lands ([roadmap](../plans/0002-roadmap.md)).

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
 browser, phone (web app in lodge's frame)    agents (/mcp/notes)    devices (/device/notes/)
        └──────────── https://lodge.<tailnet>.ts.net: sign-in, tokens, the web build ──────┘
                                             │ HTTP/1.1 on /run/lodge/apps/notes.sock, Lodge-* headers
 notesd --lodge, hub role (user frostnotes) ─┬─ /api            browsers: JSON-RPC, caller session
                                             ├─ /mcp            agents: Streamable HTTP, caller agent (in process, scoped)
                                             ├─ /content/blobs  files by hash, through lodge-content
                                             ├─ /device/sync…   devices: push, pull, blobs ─ revision log ─ merge
                                             └─ /health
```

## Design

- **notesd** (`cmd/notesd`) is one pure-Go binary. In the device role it
  serves the socket and syncs each vault with its hub. In the hub role,
  from M4, it is lodge's notes app: it serves lodge's app socket and no
  network address ([below](#notesd-as-a-lodge-app), [hub.md](hub.md)).
  A hub has vault directories on disk like any device.
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
  [ADR-0011](../adr/0011-look-like-frostmails-sibling-with-obsidians-layout.md));
  it saves as the user types and takes merges as remote changes, and shows
  files by their hash ([app.md](app.md), [specs/ui.md](../specs/ui.md)).
- **MCP** tools are hand-written adapters over the engine, served over stdio
  by `notesctl mcp` and over HTTP through lodge's `/mcp/notes` with a
  scoped token
  ([ADR-0012](../adr/0012-agents-reach-notes-through-mcp.md),
  [ADR-0026](../adr/0026-notesd-runs-as-a-lodge-app.md)).
  `internal/mcpserver` is a client of notesd's API like the app: its read
  tools mark note text as data, and its writes name their base and are
  attributed to the MCP client ([specs/mcp.md](../specs/mcp.md)).

### notesd as a lodge app

From M4, on lodge's machine, notesd is lodge's `notes` app
([ADR-0026](../adr/0026-notesd-runs-as-a-lodge-app.md),
[lodge ADR-0003](https://github.com/frostyard/lodge/blob/main/docs/adr/0003-lodge-is-the-only-process-on-the-network.md)).

- **One process is on the network: lodge.** It is a tailnet node with
  TLS. It:
  - signs browsers in by their tailnet identity;
  - issues agents' tokens;
  - serves notes' web build from `/usr/share/lodge/apps/notes/web/` at
    `/app/notes/`;
  - serves untrusted bytes from its second node, `lodge-content`.

  notesd runs as the system user `frostnotes` and serves HTTP/1.1 on
  `/run/lodge/apps/notes.sock`, which only lodge's user may open
  ([specs/lodge-app.md](../specs/lodge-app.md)).
- **Callers come from lodge's headers.** lodge sets `Lodge-Caller` and
  the caller's identity, and notesd trusts them because nothing but
  lodge can connect:
  - `/api` is a session;
  - `/mcp` is an agent, over an in-process connection bound to its
    token's scope;
  - `/content/blobs/` is lodge-content;
  - `/device/` is a device, which still shows its device token.
- **Rights stay in notesd.** notesd decides which methods each caller
  kind may call, method by method, what an agent's scope allows, which
  the engine enforces, and who wrote what. lodge never parses notesd's
  JSON-RPC.
- **The private socket stays `local`:** `notesctl`, run as `frostnotes`
  on the hub, and the desktop app's bridge on a device. The desktop app
  is not a lodge client; it works against its own notesd.

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
| `schema/rpc/`, `api/` | the IDL; generated types, router and client, on lodge kit's JSON-RPC runtime (`jsonrpc`, [ADR-0026](../adr/0026-notesd-runs-as-a-lodge-app.md)), with notes' caller kinds, scope and identities |
| `internal/store`, `internal/events` | SQLite (modernc): migrations, the single-writer `Tx`, the changes log; event fan-out |
| `internal/rpcserver`, `internal/engine` | `rpc.hello`, `events.subscribe` and each connection's identity over the kit's server and private socket; the API domains over the store and the vaults |
| `internal/vault` | one actor per vault: scanning, watching, the path guard, atomic compare-and-merge writes, versions, settings, daily notes |
| `internal/markdown` | frontmatter, links, tags, headings, blocks and tasks from a note |
| `internal/merge` | diff3 by line and word, frontmatter by key (planner-owned) |
| `internal/blobs` | the content-addressed blob store |
| `internal/render` | goldmark rendering and the HTML sanitizer |
| `internal/syncer`, `internal/hub` | the device's sync loops; the hub's device routes, pairing, the token check, relocation, and its `log` and `hub` domains |
| `internal/lodgeapp` | the lodge app socket (M4): the peer check and `Lodge-*` headers through lodge's Go kit, `/health`, `/api`, `/content/blobs/`, `/mcp`, and `/device/` over `internal/hub` |
| `internal/blobxfer`, `internal/secrets` | blobs over HTTP by hash; device tokens in the Secret Service |
| `internal/blobhttp` | vault files by hash for browsers: the development gateway's `/blobs/` and the lodge socket's `/content/blobs/`, with their headers and limits |
| `packaging/` | what notesd installs for lodge: `lodge/app.json`, the systemd socket and service, and `sysusers.d` for `frostnotes`; `lodge/stage.sh` stages them with the binaries under `usr/` ([specs/lodge-app.md](../specs/lodge-app.md#what-notesd-installs), [core ADR-0058](https://github.com/frostyard/core/blob/main/docs/adr/0058-ship-suite-desktop-apps-as-flatpaks-and-services-as-sysexts.md)); `flatpak/` is the desktop app's Flatpak with its own notesd ([design/app.md](app.md#the-flatpak), [ADR-0031](../adr/0031-the-desktop-app-ships-as-a-flatpak-with-its-own-notesd.md)) |
| `internal/synctest` | sync's seeded simulation: a hub and devices on loopback, a token oracle, checks at each quiet point (`make sync-sim`) |
| `internal/mcpserver` | MCP tools over notesd's API (the only package importing the MCP SDK) |
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
| `FROSTNOTES_SOCKET` | `$XDG_RUNTIME_DIR/frostnotes/notesd.sock`; in the Flatpak, `$XDG_RUNTIME_DIR/app/org.frostyard.Frostnotes/frostnotes/notesd.sock` | notesd, notesctl, the app |
| `FROSTNOTES_SECRETS=file` | unset | notesd: keep device tokens in a private file, not the Secret Service (development) |

The socket directory is 0700 and the socket 0600, owned by the user. The
database must be on a local file system; vaults may be anywhere. A hub
behind lodge (`notesd --lodge`, run as `frostnotes`; `--hub` on loopback
for tests) is in [hub.md](hub.md#operational-notes).

## References

- Rationale: [ADR-0003](../adr/0003-plain-markdown-files-are-the-source-of-truth.md) through [ADR-0012](../adr/0012-agents-reach-notes-through-mcp.md); notesd as a lodge app, [ADR-0026](../adr/0026-notesd-runs-as-a-lodge-app.md) and lodge's [ADR-0003](https://github.com/frostyard/lodge/blob/main/docs/adr/0003-lodge-is-the-only-process-on-the-network.md)
- Details: [vault.md](vault.md), [sync.md](sync.md), [app.md](app.md), [hub.md](hub.md)
- Contracts: [specs/rpc-protocol.md](../specs/rpc-protocol.md), [specs/rpc-api.md](../specs/rpc-api.md), [specs/mcp.md](../specs/mcp.md), [specs/lodge-app.md](../specs/lodge-app.md)
- Built in: [roadmap](../plans/0002-roadmap.md), starting with [M0](../plans/0001-m0-foundations.md); notesd inside lodge in [M4](../plans/0006-m4-notes-inside-lodge.md)
