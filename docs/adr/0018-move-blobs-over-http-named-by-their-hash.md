# 0018 — Move blobs over HTTP, named by their hash

- **Status:** Proposed
- **Date:** 2026-10-16

## Context

Every notesd keeps file contents in a content-addressed store, named by
SHA-256 ([vault.md](../design/vault.md#versions)). Devices and the hub
must exchange contents: notes of a few kilobytes, and attachments, which
M2's exit wants synced at 100 MB. notesd's JSON-RPC carries one message
per line, at most 16 MiB, and its API carries text, not bytes
([ADR-0014](0014-the-api-carries-text-and-keeps-bytes-it-cannot-show.md)).
A device can lose its link mid-transfer. The hub's store also holds
content of vaults it does not serve.

## Decision

- **Contents move outside JSON-RPC,** over `PUT`, `GET` and `HEAD` of
  `/sync/blobs/<sha256>` on the hub, with the device token
  ([ADR-0016](0016-pair-devices-with-a-one-time-code-and-a-revocable-token.md)).
  Pushes and pulls carry hashes only.
- **Both ends stream and check.** The hub writes an upload to a temporary
  file while hashing it, and publishes it only if the hash is the one the
  URL names; a mismatch stores nothing. A device does the same with a
  download, into its own store, and applies the file from there by
  streaming it into the vault's temporary file. Neither end holds a file
  in memory.
- **Upload, then push.** A device asks which of its hashes the hub lacks,
  uploads those, then pushes; the hub refuses a push whose content it
  lacks. An upload that no revision names after 24 hours is removed.
- **The hub hands out only logged content:** a `GET` of a hash that no
  revision of a served vault names is not found, so a token reads nothing
  else in the hub's store.
- **Limits:** the hub refuses a blob over its limit, 1 GiB by default,
  with 413; a device runs at most 4 transfers at once per hub.
- **Downloads resume; uploads restart.** `GET` honors `Range`; a broken
  upload starts again.
- **No compression or chunking** in M2.

## Consequences

- Identical contents travel and are stored once, across files, devices
  and vaults.
- Every push costs a round trip to ask for missing hashes; a first sync of
  666 notes uploads 666 small blobs over one kept-alive connection.
- A 100 MB attachment syncs in bounded memory, which M2's exit measures.
- A changed large attachment travels whole.

## Alternatives considered

- **Content inside JSON-RPC, as base64:** 16 MiB messages, a third
  larger, and bytes in an API that carries text.
- **Binary frames on the sync WebSocket:** the transport is a stream of
  lines, and a large transfer would stall the pushes and pulls behind it.
- **Content-defined chunks (as restic and rsync do):** they save
  bandwidth when a large file changes a little, which attachments rarely
  do; later, if needed.
- **Resumable uploads (tus):** later, if 100 MB uploads over slow links
  prove to fail.

## References

- Shapes: [design/sync.md](../design/sync.md#blob-transfer), [specs/sync-protocol.md](../specs/sync-protocol.md#blobs)
- Builds on: [ADR-0005](0005-sync-through-a-hub-with-revisions-and-three-way-merges.md), [ADR-0014](0014-the-api-carries-text-and-keeps-bytes-it-cannot-show.md), [ADR-0016](0016-pair-devices-with-a-one-time-code-and-a-revocable-token.md)
- Related: [ADR-0017](0017-revisions-number-the-hubs-log-and-hashes-name-content.md)
- Built in: [plan 0004](../plans/0004-m2-sync.md)
