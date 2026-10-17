# 0016 — Pair devices with a one-time code and a revocable token

- **Status:** Accepted
- **Date:** 2026-10-16

## Context

From M2 the hub listens on a network: devices on other machines push and
pull every note through it
([ADR-0005](0005-sync-through-a-hub-with-revisions-and-three-way-merges.md)),
so whoever can call it can read and change the vault. ADR-0005 decided
that a device joins with a one-time code approved on the hub and keeps a
device token, revocable per device, in the Secret Service, without saying
how. One person owns the hub and every device. Browser sign-in, sessions
and the hub's TLS wait for M4's authentication ADR
([ADR-0004](0004-a-notes-daemon-with-thin-clients.md)); the roadmap
expects TLS from a reverse proxy or Tailscale until then. Frostmail keeps
secrets in the Secret Service over D-Bus, with a 0600 file where no
Secret Service runs (frostmail ADR-0011). notesd fixes a WebSocket
connection's caller kind when it upgrades, and the `device` kind already
exists ([rpc-protocol](../specs/rpc-protocol.md#caller-kinds)).

## Decision

- **Pairing starts on the device.** `notesctl hub join <url> <vault>` has
  the device's notesd post its name to the hub's `/sync/pair`, the only
  sync endpoint without a token. The hub keeps a pending pairing for 10
  minutes, at most 8 at once, and answers with a pairing ID, a pairing
  secret only that device sees, and a code of 8 Crockford base32
  characters (40 bits, shown as `XXXX-XXXX`), which the device shows.
- **Approval happens only on the hub's machine.** `notesctl hub pending`
  lists pending pairings with each device's name, address and time, and
  `notesctl hub approve <code>` approves the one with that code, over the
  hub's local socket; M4's web app adds a devices page behind its
  sign-in. No network caller can approve a pairing, so the code only has
  to tell the user's device from another pending one, not resist
  guessing.
- **The device collects its token once.** It waits on `/sync/pair/<id>`
  with the pairing secret and receives a device token, 32 bytes from
  `crypto/rand` in base64url with the prefix `fnd_`, and the hub's ID. The
  hub stores the token's SHA-256 with the device's ID, name and times,
  never the token, and forgets the pairing.
- **The Secret Service keeps the token** on the device, through a port of
  frostmail's `internal/secrets`, keyed by the hub's ID;
  `FROSTNOTES_SECRETS=file` keeps it in a 0600 file for CI and
  development.
- **Every sync request carries it** as `Authorization: Bearer`: the
  WebSocket upgrade of `/sync`, which then serves JSON-RPC as caller
  `device` for that device, and each blob request
  ([ADR-0018](0018-move-blobs-over-http-named-by-their-hash.md)). The hub
  finds the device by the token's hash. A token admits every vault the
  hub serves, since one person owns them all.
- **Revocation is per device and immediate.** `notesctl hub revoke
  <device>` deletes the token's hash and closes the device's connections.
  The device reports that it was revoked, stops syncing and keeps its
  vault. Tokens do not expire.
- **Tokens cross a network only inside TLS.** The hub listens on the plain
  HTTP address that `notesd --hub` names; a device refuses a hub URL that
  is not `https://` unless its host is loopback (tests, and a hub on the
  same machine). TLS comes from a reverse proxy or `tailscale serve` until
  M4 decides whether the hub terminates it itself.
- **Requests are bounded.** A pairing request is at most 1 KiB, a JSON-RPC
  message at most 16 MiB (`api.MaxMessageSize`), and a blob at most the
  hub's limit (ADR-0018).

## Consequences

- A hub that devices on other machines reach needs a TLS front.
- A device token is a bearer secret: whoever copies it syncs until it is
  revoked. It lives in the Secret Service or a 0600 file, and never in a
  log, the database or the vault.
- The hub's machine is the place of trust: approving and revoking need
  its local socket.
- The token check and pairing are in `internal/hub`, which the planner
  owns (ADR-0010); a review of the device endpoints is part of M2's exit.
- M4 adds browser sessions and scoped MCP tokens beside device tokens.

## Alternatives considered

- **TLS client certificates:** strong, but a reverse proxy or `tailscale
  serve` terminates TLS, so the hub would never see them, and
  certificates need issuing and renewal.
- **A password or shared secret per hub:** one leak exposes every device,
  and there is nothing per device to revoke.
- **A PAKE or a QR code for the code exchange:** approval on the hub's
  machine already shows the user is present, and TLS protects the token on
  its way.
- **Tokens per vault, or expiring tokens with refresh:** one person owns
  every vault and device, and revocation covers a lost device without
  scopes or refreshes. Scopes belong to M4's MCP tokens.
- **An OAuth device flow:** the right shape for other people's clients in
  M4, and heavier than a code and a token for one person's own devices.

## References

- Shapes: [design/sync.md](../design/sync.md#pairing-and-sessions), [specs/sync-protocol.md](../specs/sync-protocol.md), [specs/rpc-protocol.md](../specs/rpc-protocol.md)
- Builds on: [ADR-0004](0004-a-notes-daemon-with-thin-clients.md), [ADR-0005](0005-sync-through-a-hub-with-revisions-and-three-way-merges.md), frostmail ADR-0011
- Related: [ADR-0018](0018-move-blobs-over-http-named-by-their-hash.md)
- Built in: [plan 0004](../plans/0004-m2-sync.md)
