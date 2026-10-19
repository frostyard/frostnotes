# Spec: notesd as a lodge app

What notesd installs for lodge and serves on its lodge app socket. Its
consumers:

- lodge, which proxies to it;
- the web app, through `/app/notes/`;
- agents, through `/mcp/notes`;
- devices, through `/device/notes/`.

This spec is the notes side of lodge's
[app contract](https://github.com/frostyard/lodge/blob/main/docs/specs/app-contract.md).
Sign-in, sessions, tokens, origins and the browser's headers are
lodge's ([lodge-http](https://github.com/frostyard/lodge/blob/main/docs/specs/lodge-http.md)).
The device protocol is [sync-protocol.md](sync-protocol.md), the RPC's
wire rules are [rpc-protocol.md](rpc-protocol.md), and the MCP tools
are [mcp.md](mcp.md).

Rationale: [ADR-0026](../adr/0026-notesd-runs-as-a-lodge-app.md),
[ADR-0016](../adr/0016-pair-devices-with-a-one-time-code-and-a-revocable-token.md),
[ADR-0012](../adr/0012-agents-reach-notes-through-mcp.md). Where it fits:
[design/hub.md](../design/hub.md), [design/overview.md](../design/overview.md).

**Status: M4's contract,** written in
[plan 0006](../plans/0006-m4-notes-inside-lodge.md)'s Phase 1. It binds
as Phases 2 to 4 land. Until then the hub serves only
[sync-protocol.md](sync-protocol.md)'s routes on its plain address.

## Interface

### What notesd installs

| What | Where (lodge's contract) | Contents |
| --- | --- | --- |
| manifest | `/usr/share/lodge/apps/notes/app.json` | the JSON below; `version` is notesd's |
| web build | `/usr/share/lodge/apps/notes/web/` | `make web-build`'s output, built for the base `/app/notes/`, with `icon.svg` |
| system user | `sysusers.d` | `frostnotes`, which notesd runs as |

```json
{ "contract": 1, "id": "notes", "name": "Notes", "icon": "icon.svg",
  "version": "0.5.0", "user": "frostnotes", "key": "n",
  "content": true, "device": true, "mcp": true, "remoteImages": true }
```

### Listeners

| Listener | Where | Who may connect | Caller |
| --- | --- | --- | --- |
| the app socket (`notesd --lodge`) | `/run/lodge/apps/notes.sock`, or under the contract's root override | lodge's user only, by the socket peer's uid | from `Lodge-Caller` |
| the private socket | `$FROSTNOTES_SOCKET`; as a system service, `/run/frostnotes/notesd.sock` | notesd's own user ([rpc-protocol](rpc-protocol.md#transports)) | `local` |
| `notesd --hub ADDR` | a loopback address only | tests, the simulation, the evidence | `device`, on [sync-protocol](sync-protocol.md)'s paths without `/device` |

- The app socket is created as the contract says (the app's user, group
  `lodge`, 0660). notesd closes a connection from any other uid before
  reading from it.
- notesd reads `Lodge-*` headers on the app socket only. Elsewhere they
  are ignored like any unknown header.

### Requests on the app socket

HTTP/1.1. Every request but `/health` must carry `Lodge-Caller`. Each
path admits one caller kind, and any other is 403:

| Request | `Lodge-Caller` | Purpose |
| --- | --- | --- |
| `GET /health` | any, or none | the app's state for lodge's rail |
| `GET /api`, WebSocket | `session` | JSON-RPC as caller `session` |
| `GET`, `HEAD` `/content/blobs/<hash>.<ext>` | `content` | files by hash |
| `POST`, `GET`, `DELETE` `/mcp` | `agent` | MCP Streamable HTTP |
| `POST /device/sync/pair` | `device` | start a pairing |
| `GET /device/sync/pair/{pairing}` | `device` | collect a device token |
| `GET /device/sync`, WebSocket | `device` | JSON-RPC as caller `device` |
| `HEAD`, `GET`, `PUT` `/device/sync/blobs/{hash}` | `device` | contents |
| `POST /device/sync/hub` | `device` | the hub's ID, URL and relocation proof |

- Any other path is 404.
- Errors are `text/plain; charset=utf-8`, one line saying why.
- **Required headers:**
  - `session`: `Lodge-Login`, `Lodge-Node-ID`, `Lodge-Node-Name` and
    `Lodge-Session`;
  - `agent`: `Lodge-Token` and `Lodge-Scope`;
  - `content`: `Lodge-Login`, `Lodge-Node-ID` and `Lodge-Node-Name`.

  A missing one is 403.
- **On `device` requests,** `Lodge-Node-ID`, `Lodge-Node-Name` and
  `Lodge-Login` may come too. They are for display only, never for
  authorization.
- **Syntax** is the app contract's. `Lodge-Login` and
  `Lodge-Display-Name` are percent-encoded UTF-8. notesd decodes what it
  shows, and logs the rest.

### `/health`

`200 {"ok": true, "version": "<notesd's version>", "protocol": 1}`.
`protocol` is notesd's RPC protocol major. notesd opens the app socket
only once its store is open, so a notesd that answers is serving.

### `/api`

- A WebSocket (RFC 6455) whose text frames carry
  [rpc-protocol.md](rpc-protocol.md#transports)'s lines. The connection
  is caller `session` for its lifetime.
- `rpc.hello` reports the caller kind, the login (`Lodge-Login`), the
  display name (`Lodge-Display-Name`, when sent) and the node's name
  (`Lodge-Node-Name`).
- A session's writes are source `editor`, with the author
  `<Lodge-Node-Name> (web)`.
- **Limits:** messages of at most 16 MiB, and 32 requests in flight
  ([rpc-protocol](rpc-protocol.md)).
- **What lodge decides:**
  - session counts and liveness;
  - lodge closes a session after 90 s without a frame, and the web app
    calls `rpc.ping` every 30 s while it is visible;
  - lodge ends a session by closing the proxied connection, and notesd
    then ends the connection and its event subscription.

### `/content/blobs/`

`GET` or `HEAD /content/blobs/<hash>.<ext>`: `hash` is 64 lowercase hex
digits, and `ext` 1 to 8 lowercase letters or digits. The public URL is
`https://lodge-content.<tailnet>.ts.net/notes/blobs/<hash>.<ext>`. Answers:

- the content with that hash from notesd's blob store, else 404;
- the content type the extension names
  ([design/app.md](../design/app.md#attachments));
- `X-Content-Type-Options: nosniff`;
- `Content-Security-Policy: default-src 'none'; style-src 'unsafe-inline'; sandbox`;
- `Cache-Control: private, max-age=31536000, immutable`;
- one `Range` per request; several, or one past the end, is 416. A 206
  carries at most 8 MiB: a longer or open range is cut to 8 MiB from
  its start. A request without a range is answered whole up to 64 MiB,
  else 413.
- With `?download=1`, also `Content-Disposition: attachment;
  filename="<hash>.<ext>"`. The web app uses it where a browser will not
  show a file in a sandboxed page.

lodge-content adds its own sandbox policy and cross-origin headers
([lodge ADR-0010](https://github.com/frostyard/lodge/blob/main/docs/adr/0010-untrusted-bytes-come-from-a-second-node.md)).

### `/mcp`

[mcp.md](mcp.md#through-lodge) gives the tools; on the socket:

- **The scope** is `Lodge-Scope`, which lodge passes unread:
  - `vault=<id>` for reading one vault, or `vault=<id>;write` to also
    write;
  - `<id>` is the vault's ID on this notesd (`notesctl vault list`);
  - anything else is 403.
- **Sessions:**
  - each MCP session is bound, at `initialize`, to the token's name
    (`Lodge-Token`) and its scope;
  - a request for a session whose name or scope differs is 404 (MCP's
    answer for an unknown session);
  - at most 8 live sessions per token name; a ninth evicts the least
    recently used;
  - a session idle for an hour ends.
- **Scope in the engine:** each session's tool server reaches notesd
  over an in-process connection of caller `agent`, which carries the
  scope.
  - Only `rpc` and `note` admit `agent`.
  - A call that names another vault is `notFound`.
  - A method marked `writes` is `methodNotFound` under a read scope.
- **Attribution:** an agent's writes are source `agent`, with
  `Lodge-Token` as author. The client's `clientInfo.name` goes to the
  log only.
- **Revocation** is lodge's. lodge refuses a revoked token and closes
  its proxied streams, so notesd keeps no list of tokens.

### `/device/`

[sync-protocol.md](sync-protocol.md#endpoints)'s endpoints, with
`/device` before each path. A device's hub URL is
`https://lodge.<tailnet>.ts.net/device/notes`. The device appends
`/sync…` to it, and lodge forwards the request to `/device/sync…`.

- **Sync-protocol's rules hold unchanged:**
  - an `Origin` other than `notesd://device` is 403;
  - a missing, unknown or revoked device token is 401;
  - so are pairing's limits and the blob rules.

  lodge adds no identity check here.
- **A pending pairing's address** behind lodge is the node lodge names
  (`Lodge-Node-Name`, and `Lodge-Login` when sent: `phone
  (ana@example.com)`), or empty when it names none. It is shown beside the device's own name and the time in
  `notesctl hub pending`, to tell pairings apart, never to admit one.
- **Pending pairings per node:** behind lodge, one node (`Lodge-Node-ID`)
  may have at most 2 pairings waiting, so no peer takes all 8 of the
  hub's slots; a third answers 429. The node ID is a rate key here,
  never an admission.
- **The hub's URL:**
  - `hub.setURL {url}` (caller `local`) records the URL devices should
    use, an `https://` URL by sync-protocol's rules (or `http://` on
    loopback, for tests); an empty `url` forgets it. It answers
    `hub.status`.
  - `hub.status` reports it.
  - Devices learn it with `log.hub` (caller `device`), which answers
    `{hub, url?}`. A device calls it at each connection. When `url` is
    set and differs from its own record, it first checks the host at
    `url` with the relocation proof, exactly as `sync.relocate` does
    (below), and only then keeps the new URL and reconnects there. Its
    token, replicas, agreed states and cursors stay. A host that does
    not prove itself is not followed, and the device stays and logs
    why: its next connection would send its token there, so a mistyped
    `notesctl hub url`, or a hub database someone changed, must not
    collect it.
- **`POST /device/sync/hub`** (no credential; body at most 1 KiB):
  - The request is `{"device": <device ID>, "nonce": "<43 base64url
    characters>"}`.
  - The answer is `{"hub": "<hub ID>", "url": "<recorded URL>",
    "proof": "<43 base64url characters>"}`.
  - `proof` is HMAC-SHA256. Its key is the 32 bytes of the SHA-256 the
    hub keeps of that device's token. Its message is `frostnotes
    relocate\n` + hub ID + `\n` + url + `\n` + nonce.
  - An unknown or revoked device gets the same answer as a paired one,
    with a proof under a key no token makes (an HMAC of its ID under a
    random key of the hub's), so the endpoint does not tell which
    devices exist. A hub with no recorded URL is 409. A peer that
    reaches `/device/` can learn the hub's ID and its recorded URL,
    neither of which is a secret. Requests are logged at debug only.
  - The proof's key is the hash of the token that the hub stores.
    Someone who can read the hub's database can make a valid proof, and
    so lead a device that runs `sync.relocate` to their URL into sending
    its token there. That is one more reason the hub's data directory is
    its own user's alone (0700). A body that is not that object, or a nonce that is not 32
    bytes of unpadded base64url, is 400; a body over 1 KiB is 413.
- **`sync.relocate {hub, url}`** (caller `local`, on a device; `hub` is
  the paired hub's URL now, as `sync.hubs` lists it):
  1. Post a fresh 32-byte nonce to `url` + `/sync/hub`.
  2. Keep `url` only when all of these hold:
     - the answer names that paired hub;
     - its `url` equals `url`, comparing scheme and host without case
       and the path exactly, with no trailing `/`;
     - its proof is the HMAC the device computes from the SHA-256 of
       its own token.
  3. Otherwise answer `conflict`, and the device's URL stays.

  The token is never sent in this exchange. A relay that forwards the
  nonce to the real hub gets back a proof naming the real hub's URL,
  which is not the relay's.

### Methods and callers

| Domain | Callers | Narrowed |
| --- | --- | --- |
| `rpc` (`hello`, `ping`) | local, session, device, agent | |
| `events` | local, session, device | |
| `note` | local, session, agent | the methods marked `writes`: `create`, `write`, `move`, `delete`, `revert`, `daily`, `upload`, `mkdir`, `moveFolder`, `deleteFolder` |
| `vault` | local, session | `add`, `remove`: local |
| `hub` | local, session | `serve`, `approve`, `setURL`: local |
| `log` | device | |
| `sync` | local | |

- `hub.status` gains `url`.
- `hub.pending`, `hub.devices` and `hub.revoke` admit sessions, for the
  web app's Hub settings.
- `rpc.ping` answers `null`.

### Commands

| Command | Method |
| --- | --- |
| `notesctl hub url URL` | `hub.setURL` |
| `notesctl hub relocate URL NEW-URL` | `sync.relocate` |
| `notesctl hub pending`, `approve CODE`, `devices`, `revoke ID` | as before ([sync-protocol](sync-protocol.md#local-domains)) |

On the server, run them as `frostnotes` (`sudo -u frostnotes notesctl
…`). Agents' tokens are lodge's: `lodgectl token issue NAME --app notes
--scope 'vault=<id>;write'`.

### Limits

| What | Value |
| --- | --- |
| a JSON-RPC message, requests in flight | 16 MiB, 32 per connection |
| MCP sessions | 8 per token name, the least recently used evicted; an hour idle |
| a blob answer | a 206 of 8 MiB; 64 MiB whole |
| a relocation request | 1 KiB; a 32-byte nonce |
| pending pairings | 8 in all; 2 from one node behind lodge |

## Rules

- notesd listens on no network address. `--hub` refuses an address that
  is not loopback.
- notesd reads `Lodge-*` headers only on the app socket, and only after
  the peer check.
- Each path on the app socket admits one caller kind.
- An agent's scope is enforced by the engine. A test calls every `note`
  method on another vault, and every method marked `writes` under a
  read scope.
- The writes of `session` and `agent` connections carry the
  connection's source and author, never the request's.
- Approving a device and recording the hub's URL answer
  `methodNotFound` to every caller but `local`.
- No response, log line or row holds a device token or a pairing
  secret. A relocation proof reveals only what an HMAC under the
  token's hash reveals.
- Every limit has a test at the limit and one past it.

## Derived artifacts

| Artifact | Derivation |
| --- | --- |
| `api/zz_generated.go`, `app/src/rpc/gen/api.ts`, [rpc-api.md](rpc-api.md) | `make gen` from `schema/rpc/` (method callers, `writes`, `hub.setURL`, `log.hub`, `sync.relocate`, `rpc.ping`, `rpc.hello`'s fields) |
| `packaging/lodge/app.json` | the manifest above, its `version` set at build |

## References

- Rationale: [ADR-0026](../adr/0026-notesd-runs-as-a-lodge-app.md), [ADR-0016](../adr/0016-pair-devices-with-a-one-time-code-and-a-revocable-token.md), [ADR-0012](../adr/0012-agents-reach-notes-through-mcp.md), [ADR-0023](../adr/0023-the-app-shows-files-by-their-hash-and-uploads-them-through-notesd.md) (files by hash)
- Context: [design/hub.md](../design/hub.md), [design/overview.md](../design/overview.md), [design/app.md](../design/app.md#in-a-browser)
- lodge: [app contract](https://github.com/frostyard/lodge/blob/main/docs/specs/app-contract.md), [lodge-http](https://github.com/frostyard/lodge/blob/main/docs/specs/lodge-http.md), [ADR-0003](https://github.com/frostyard/lodge/blob/main/docs/adr/0003-lodge-is-the-only-process-on-the-network.md), [ADR-0006](https://github.com/frostyard/lodge/blob/main/docs/adr/0006-agents-reach-each-app-through-its-own-mcp-endpoint.md), [ADR-0010](https://github.com/frostyard/lodge/blob/main/docs/adr/0010-untrusted-bytes-come-from-a-second-node.md)
- Related: [sync-protocol.md](sync-protocol.md), [rpc-protocol.md](rpc-protocol.md), [mcp.md](mcp.md)
- Built in: [plan 0006](../plans/0006-m4-notes-inside-lodge.md)
