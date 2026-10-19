# Design: the hub behind lodge

Living document. Rationale:
[ADR-0026](../adr/0026-notesd-runs-as-a-lodge-app.md) (notesd as a
lodge app),
[ADR-0016](../adr/0016-pair-devices-with-a-one-time-code-and-a-revocable-token.md)
(devices),
[ADR-0005](../adr/0005-sync-through-a-hub-with-revisions-and-three-way-merges.md)
(the hub merges).
Contracts: [specs/lodge-app.md](../specs/lodge-app.md#device) (the device
routes on the lodge socket, relocation),
[specs/sync-protocol.md](../specs/sync-protocol.md) (the device
protocol).

**Status: built** in M4's Phases 2 and 3
([plan 0006](../plans/0006-m4-notes-inside-lodge.md); merged
2026-10-19). The user's hub moves behind lodge in Phase 7, from
2026-10-26; until then it serves devices on a plain HTTP address behind
a `tailscale serve` on another machine ([sync.md](sync.md)). This document covers what changes for the
hub and its devices when notesd becomes lodge's notes app; browsers and
agents are in [overview.md](overview.md#notesd-as-a-lodge-app) and
[app.md](app.md#in-a-browser).

## Overview

A hub is a notesd that serves vaults to devices and merges their pushes
([sync.md](sync.md)). From M4 it is also lodge's `notes` app. It runs on
lodge's machine as the system user `frostnotes`, and listens on two Unix
sockets only:

- **the lodge app socket.** lodge forwards devices' requests from its
  `/device/notes/…` to it, as well as browsers' `/api`, lodge-content's
  blobs and agents' `/mcp`
  ([specs/lodge-app.md](../specs/lodge-app.md));
- **the private socket,** for `notesctl` run as `frostnotes`, caller
  `local`. It is the only place to approve a pairing or record the hub's
  URL.

```
 desktop notesd (device role) ── https://lodge.<tailnet>.ts.net/device/notes/sync…  Bearer fnd_…
                                       │
 lodge (tailnet node, TLS, no identity check on /device/)
                                       │ /device/sync… on /run/lodge/apps/notes.sock, Lodge-Caller: device
 notesd --lodge  (user frostnotes) ── internal/hub: pairing, the token check, /sync, blobs, log domain
        │                                     └─ the revision log, merges, the vault directory
        └─ /run/frostnotes/notesd.sock ── notesctl hub approve | url | devices | revoke (as frostnotes)
```

## Design

### Device routes through lodge

- `internal/hub`'s handler is mounted twice:
  - at `/device/` on the lodge socket, for requests whose
    `Lodge-Caller` is `device`;
  - at `/` on a loopback `--hub ADDR` for tests, the simulation and the
    exit evidence.

  The handler is the same, so M2's simulation and regressions run
  unchanged.
- lodge checks nothing on `/device/`; the device token is the
  credential, as in M2. The `Origin` rule stays: only `notesd://device`,
  or none.
- lodge may name the device's tailnet node (`Lodge-Node-Name`,
  `Lodge-Login`). The hub shows it beside a pending pairing's own name,
  to tell two pairings apart, and never admits anything by it.
- TLS and the hub's public name are lodge's. A device's hub URL is
  `https://lodge.<tailnet>.ts.net/device/notes`, and the device appends
  sync-protocol's paths to it, as it does to any hub URL.

### Moving devices to lodge

Devices paired during M2 know the hub by its old `tailscale serve` URL.

1. On the hub, `notesctl hub url https://lodge.<tailnet>.ts.net/device/notes`
   records the URL devices should use (`hub.setURL`).
2. At each connection a device asks `log.hub`. Over its authenticated
   sync connection, still at the old URL, it learns the new one, keeps
   it, and reconnects there with the same token, replicas, agreed
   states and cursors.
3. Once every device has moved (`notesctl hub devices` shows each one
   seen at the new address), the old `tailscale serve` goes.

A device that missed the move, after the old URL is gone, runs
`notesctl hub relocate OLD-URL NEW-URL`:

- it posts a fresh nonce to `NEW-URL/sync/hub`;
- it keeps the URL only when the answer names its hub, carries the hub's
  recorded URL equal to `NEW-URL`, and proves the hub holds its token's
  hash: an HMAC over the hub's ID, that URL and the nonce, keyed with
  the hash;
- the token itself never crosses in this exchange.

A host that relays the nonce to the real hub gets a proof naming the
real URL, which the device refuses, since it asked another host
([specs/lodge-app.md](../specs/lodge-app.md#device)).

### Hub settings in the app

When the app's notesd is a hub (`hub.status` answers), the settings
dialog has a Hub section. It lists the devices, each with Revoke, and
pending pairings, with the node lodge named for each and the command
that approves one on the hub's machine. It shows in the web app and on
the hub's desktop alike; a session may list and revoke, never approve.
lodge's sign-in allow-lists, sessions and MCP tokens are managed in
lodge.

### Testing

- `internal/hub` keeps its tests on loopback. New tests mount the
  handler under `/device/` on a socket with `Lodge-Caller: device`, and
  check:
  - requests with other callers, or without one, are refused;
  - display headers change no decision;
  - `log.hub` moves a device;
  - the relocation proof refuses a relayed answer, a wrong hub, a URL
    that differs and a forged HMAC.
- `internal/synctest` runs as before.
- M2's 7-day trial's devices move to lodge in
  [plan 0006](../plans/0006-m4-notes-inside-lodge.md)'s last phase. The
  devices' sync is watched for a week after.

## Operational notes

- **Run as `frostnotes`:** the hub is a system service of the user
  `frostnotes`. Its private socket is `/run/frostnotes/notesd.sock`,
  the service's runtime directory, 0700. Run `notesctl` as `frostnotes`
  (`sudo -u frostnotes notesctl hub pending`).
- **Data directory:** `/var/lib/frostnotes`, the service's state
  directory, passed as `FROSTNOTES_DATA_DIR`. Like every notesd's, it is
  on a local disk.
- **Vault directory:** `frostnotes` must be able to write it. On a CIFS
  mount, the mount's `uid=` and `gid=` name `frostnotes`: the files'
  local owner comes from the mount options, and the file server sees
  every write as the mount's credential.
- **Moving the M2 hub:** `sudo scripts/move-hub-behind-lodge.sh` on the
  hub's machine checks lodge, the `frostnotes` user and units, the M2
  unit and its data, and the vault's mount, and prints the move. With
  `--apply --url https://lodge.<tailnet>.ts.net/device/notes` it stops
  and disables the M2 unit, copies its data directory whole to
  `/var/lib/frostnotes`, sets the mount's `uid=` and `gid=` to
  `frostnotes`, checks that `frostnotes` writes to the share, starts
  the lodge socket and service, records the URL and reloads lodge. It
  never touches the vault's files, keeps the old data directory and
  fstab, and prints `--rollback BACKUP --apply`, which puts the M2 hub
  back. The rollback is for before devices sync through lodge: the M2
  notesd cannot open the moved database, whose schema is newer, and its
  own copy would be behind its devices, so the rollback refuses once
  the hub's log has moved, unless `--force` ([plan 0006, Phase 7](../plans/0006-m4-notes-inside-lodge.md#phase-7--the-users-hub-in-lodge-and-exit-evidence)).
- **Pairing a new device:**
  - on the device, `notesctl hub join https://lodge.<tailnet>.ts.net/device/notes NAME`;
  - on the hub, `sudo -u frostnotes notesctl hub approve CODE`.
- **If lodge is down,** devices keep working offline and push when it
  returns, as with any hub outage.
- **Backups:** the data directory and the vault directory, as in
  [sync.md](sync.md#operational-notes). lodge's own state (tokens,
  allow-lists) is lodge's.

## References

- Rationale: [ADR-0026](../adr/0026-notesd-runs-as-a-lodge-app.md), [ADR-0016](../adr/0016-pair-devices-with-a-one-time-code-and-a-revocable-token.md), [ADR-0005](../adr/0005-sync-through-a-hub-with-revisions-and-three-way-merges.md); lodge's [ADR-0003](https://github.com/frostyard/lodge/blob/main/docs/adr/0003-lodge-is-the-only-process-on-the-network.md)
- Contracts: [specs/lodge-app.md](../specs/lodge-app.md), [specs/sync-protocol.md](../specs/sync-protocol.md)
- Related: [overview.md](overview.md), [sync.md](sync.md), [app.md](app.md#in-a-browser)
- Built in: [plan 0006 — M4](../plans/0006-m4-notes-inside-lodge.md) ([roadmap — M4](../plans/0002-roadmap.md))
