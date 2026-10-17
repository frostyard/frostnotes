# 0004 — A notes daemon with thin clients

- **Status:** Accepted
- **Date:** 2026-10-16

## Context

Frostnotes needs a desktop app, a web app, a CLI and an MCP server over the
same notes. Sync must keep running while no window is open. The web app
should be the same UI as the desktop app, not a second one. Frostmail
proved the shape in nine days: a headless Go daemon, thin clients over a
generated JSON-RPC contract, and a React UI behind a swappable transport
(frostmail ADR-0002, ADR-0009). Codex writes Go and TypeScript reliably.

## Decision

- **notesd**, a per-user Go daemon (pure Go, `CGO_ENABLED=0`), owns vaults:
  the watcher, the parser, the SQLite index, every write, sync, rendering
  and the MCP tools. It serves JSON-RPC 2.0
  ([ADR-0008](0008-generate-the-rpc-contract-from-a-yaml-idl.md)).
- **One binary, two roles.** On a device, notesd serves a 0600 Unix socket
  to local clients and syncs its vaults with a hub. In the hub role, the
  same notesd also listens on HTTP for devices (sync), browsers (the web
  app and its WebSocket API) and agents (MCP). A hub's vaults are
  directories of plain files like any other
  ([ADR-0003](0003-plain-markdown-files-are-the-source-of-truth.md)).
- **Clients:**
  - the desktop app: Tauri v2 with a React UI in the system WebKitGTK,
    whose Rust layer only moves lines between the webview and the socket;
  - the web app: the same React bundle, served by the hub, over a
    WebSocket;
  - `notesctl`: a CLI over the socket;
  - agents, through MCP ([ADR-0012](0012-agents-reach-notes-through-mcp.md)).
- **The UI reaches notesd only through the generated client over a
  `Transport`.** Tauri, WebSocket and an in-memory mock are its three
  implementations, and nothing above the transport knows which one runs.

## Consequences

- One UI codebase serves desktop and web.
- The engine (vault, index, writes, sync) is built and tested headless
  before any UI exists. Sync tests run a hub and several devices in one
  test process.
- The desktop app works offline against its local notesd; the web app
  needs the hub.
- Two processes are versioned together: `rpc.hello` negotiates the
  protocol, and changes within a major version are additive.
- The hub is a network-facing service. Its authentication, sessions and
  TLS get their own ADR before M4.

## Alternatives considered

- **An Obsidian-style monolith** with the vault in the app process: sync,
  the CLI and MCP would stop when the window closes, and the web app would
  need a separate server anyway.
- **A separate hub binary:** duplicates the engine. A hub is a notesd with
  more listeners.
- **A Rust engine linked into Tauri:** Codex writes correct Go more
  reliably, and the daemon boundary is wanted regardless, as frostmail
  found.
- **Electron:** ships its own Chromium; heavier than the system WebKitGTK
  for no gain.

## References

- Shapes: [design/overview.md](../design/overview.md), [design/app.md](../design/app.md) (the app as a thin client)
- Builds on: frostmail ADR-0002 and ADR-0009
- Related: [ADR-0005](0005-sync-through-a-hub-with-revisions-and-three-way-merges.md), [ADR-0008](0008-generate-the-rpc-contract-from-a-yaml-idl.md), [ADR-0011](0011-look-like-frostmails-sibling-with-obsidians-layout.md)
