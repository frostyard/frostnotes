# 0008 — Generate the RPC contract from a YAML IDL

- **Status:** Accepted
- **Date:** 2026-10-16

## Context

notesd has four kinds of caller: local clients on the socket, browsers
through the hub, devices syncing with the hub, and agents through MCP
([ADR-0004](0004-a-notes-daemon-with-thin-clients.md)). Go and TypeScript
must agree on every type. Frostmail's `tools/rpcgen` generates the Go
server, the Go client, the TypeScript client and the API reference from
YAML (frostmail ADR-0007), and has carried frostmail through 160 task
cards.

## Decision

- **The contract is YAML** in `schema/rpc/*.yaml`, one file per domain, with
  enums, types, methods (params, result, errors) and events (durable or
  transient).
- **`tools/rpcgen` generates** `api/zz_generated.go` (types, one service
  interface per domain, the router, a typed client, event decoding),
  `app/src/rpc/gen/api.ts` and `docs/specs/rpc-api.md`. `make gen-check`
  fails when they are stale.
- **The generator is copied from frostmail at M0 and owned here.** Shared
  code waits until a third app needs it.
- **The wire is JSON-RPC 2.0.** On the socket: one message per line on a
  0600 Unix socket, with a peer-UID check. On the hub: one message per
  WebSocket text frame over HTTPS.
- **Each domain declares who may call it:** `local` (the socket), `session`
  (a signed-in browser) or `device` (a paired device's sync). The router
  enforces it, so the hub cannot serve a local-only method to a browser.
- **MCP is not JSON-RPC from this IDL.** Its tools are hand-written
  adapters over the same engine
  ([ADR-0012](0012-agents-reach-notes-through-mcp.md)).

## Consequences

- A domain with no implementation fails `api.NewRouter`, a wrong signature
  fails to compile, and a wrong TypeScript call fails the typecheck.
- The Rust bridge and the WebSocket transport only move messages, so
  neither changes when the API does.
- Changes within protocol 1 are additive only.
- The generator (about 900 lines in frostmail) grows a caller-kind check
  and stays planner-owned.

## Alternatives considered

- **gRPC or Connect:** a protobuf toolchain, and HTTP/2 does not fit the
  Tauri line bridge.
- **REST with OpenAPI:** no push, which live views and sync need.
- **A shared Go module with frostmail now:** couples two pre-release apps
  that change weekly.

## References

- Shapes: [design/overview.md](../design/overview.md), [specs/rpc-protocol.md](../specs/rpc-protocol.md)
- Builds on: frostmail ADR-0007
- Built in: [M0 plan](../plans/0001-m0-foundations.md)
