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
[plan 0006](../plans/0006-m4-notes-inside-lodge.md)'s Phase 1 and built
in Phases 2 to 4 (merged 2026-10-19). The user's hub serves only
[sync-protocol.md](sync-protocol.md)'s routes on its plain address until
Phase 7 moves it behind lodge.

## Interface

### What notesd installs

| What | Where (lodge's contract) | Contents |
| --- | --- | --- |
| manifest | `/usr/share/lodge/apps/notes/app.json` | the JSON below; `version` is notesd's |
| web build | `/usr/share/lodge/apps/notes/web/` | `make web-build`'s output, built for the base `/app/notes/`, with `icon.svg` |
| daemon and CLI | `/usr/bin/notesd`, `/usr/bin/notesctl` | `make build`'s binaries, mode 0755 |
| units | `/usr/lib/systemd/system/frostnotes.service`, `frostnotes-lodge.socket` | `packaging/systemd/`; the service's `ExecStart=` is `/usr/bin/notesd -lodge` |
| system user | `/usr/lib/sysusers.d/frostnotes.conf` | `u frostnotes`, in group `lodge-apps` and never in group `lodge`; `g lodge -` too, because group `lodge` owns the app socket (`SocketGroup=lodge`) and the socket must start without lodge's own sysext |

`packaging/lodge/stage.sh DESTDIR VERSION [BUILD]` stages all of this
under `DESTDIR` in that layout, and `make lodge-package` runs it into
`LODGE_DESTDIR` (default `build/lodge-root`) after `make build` and
`make web-build`. It writes only under `usr/`: no `/etc`, no `/var`, no
`/opt`, no `/usr/local`, as a sysext's payload is `/usr` only
([core ADR-0058](https://github.com/frostyard/core/blob/main/docs/adr/0058-ship-suite-desktop-apps-as-flatpaks-and-services-as-sysexts.md)).
`internal/packagecheck` tests the staged tree. `make lodge-app` still lays
out the manifest and the web build alone in `build/lodge/apps/notes`.

### Release

`make bump` is the only way to tag a release: on a clean `main` equal to
`origin/main`, it runs `make check`, then tags the version that `svu`
computes from the conventional commits and pushes the tag. The tag's
[release workflow](https://github.com/frostyard/frostnotes/blob/main/.github/workflows/release.yml)
builds `frostyard-notesd_X.Y.Z_amd64.deb` (`make deb`, which stages the
`/usr` tree with `packaging/lodge/stage.sh`, packs it with nFPM from
`packaging/nfpm.yaml` and runs `packaging/deb-check.py`), attests it and
attaches it to the GitHub release.

`deb-check.py` (`make deb-check`) reads the archive's exact member names. It
fails unless the control fields are right (`Depends: systemd`, no maintainer
scripts), and the package holds only root-owned, non-writable directories,
files and links under `/usr`, with the binaries (mode 0755), the units, the
sysusers.d entry, the web build's `index.html`, and an `app.json` whose
`version` is the package's. `internal/packagecheck` feeds it bad packages,
and each must fail. Verify a download with:

```sh
gh attestation verify frostyard-notesd_X.Y.Z_amd64.deb --repo frostyard/frostnotes
```

A pull request's CI builds a snapshot, `frostyard-notesd_0.0.0-<commit>-snapshot_amd64.deb`,
with the same target. Snosi picks the release's .deb up later
([core plan 0010](https://github.com/frostyard/core/blob/main/docs/plans/0010-suite-packaging.md), Phase 3).

A hand-installed host copies from the staged root. On lodge's host,
notesd and notesctl are in `/opt/lodge/bin`, which the unit's
`ExecStart=` names:

1. Copy the staged `usr/bin/notesd` and `usr/bin/notesctl` to
   `/opt/lodge/bin`. Replace each file (install to a new name, then
   `mv` it over); never write into the running binary. Run that
   `notesctl` by its absolute path, `/opt/lodge/bin/notesctl`, as
   [plan 0006](../plans/0006-m4-notes-inside-lodge.md)'s run sheet does.
2. Replace `/usr/share/lodge/apps/notes/` with the staged one, owned by
   root (directories 0755, files 0644).
3. Keep the host's units, drop-ins and sysusers.d entry in `/etc`. The
   staged sysusers.d file adds only `g lodge -`, which the host's group
   `lodge` already satisfies.
4. Run `sudo systemctl restart frostnotes.service` and
   `sudo /opt/lodge/bin/lodgectl reload`, then check that
   `sudo /opt/lodge/bin/lodgectl apps` shows notes up at the new version.
   On that host `lodgectl` is in `/opt/lodge/bin`, not on root's `PATH`.

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
| `GET`, `HEAD` `/content/blobs/<mac>/<hash>.<ext>` | `content` | files by hash, named by a MAC |
| `POST`, `GET`, `DELETE` `/mcp` | `agent` | MCP Streamable HTTP |
| `POST /device/sync/pair` | `device` | start a pairing |
| `GET /device/sync/pair/{pairing}` | `device` | collect a device token |
| `GET /device/sync`, WebSocket | `device` | JSON-RPC as caller `device` |
| `HEAD`, `GET`, `PUT` `/device/sync/blobs/{hash}` | `device` | contents |
| `POST /device/sync/hub` | `device` | the hub's ID, URL and relocation proof |

- A path outside this table is 403, since it admits no caller, as is a
  caller kind its row does not admit
  ([lodge's app-contract.md](https://github.com/frostyard/lodge/blob/main/docs/specs/app-contract.md#what-an-app-serves-on-its-socket)).
  `/x/…`, which notes does not serve, is 404 to a session, and a
  `/device/` path the table does not list is 404 to a device.
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

`GET` or `HEAD /content/blobs/<mac>/<hash>.<ext>`:

- `hash` is 64 lowercase hex digits, and `ext` 1 to 8 lowercase letters
  or digits.
- `mac` is 43 base64url characters: HMAC-SHA256 of
  `frostnotes content\n` + `hash`, keyed with notesd's content key, in
  unpadded base64url.
  - The content key is 32 random bytes that notesd keeps in its data
    directory (`content.key`, mode 0600). It stays the same across
    restarts, so URLs stay cacheable.
  - `rpc.hello` hands it to a session as `contentKey`, and the web app
    computes each file's `mac`.
- A path without the right `mac` is 404. Under `ts.net` (a public
  suffix) every page on every node of the tailnet is same-site to
  lodge-content. Without the key, such a page cannot make a file's URL,
  so it cannot learn whether the vault holds a file it knows.

The public URL is
`https://lodge-content.<tailnet>.ts.net/notes/blobs/<mac>/<hash>.<ext>`. Answers:

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

- **The scope** is `Lodge-Scope`, which lodge passes unread
  ([ADR-0028](../adr/0028-an-agents-scope-can-limit-writes-to-folders.md)):

  ```
  vault=<id>[;write[=<prefix>[,<prefix>…]]][;except=<prefix>[,<prefix>…]][;daily]
  ```

  - `<id>` is the vault's ID on this notesd (`notesctl vault list`).
    Every scope reads the whole vault.
  - `;write` also writes the whole vault. `;write=` writes only under
    its prefixes.
  - `;except=` takes its prefixes out of every write, `daily`'s
    included, and always wins. It needs `write`, `write=` or `daily`.
  - `;daily` writes today's daily note, in notesd's time zone, and
    nothing else.
  - A prefix is a folder: a vault-relative path ending in `/`, whose
    names pass the path guard and are in NFC. It is percent-encoded as
    a URL path segment is, so `,`, `;`, `%`, spaces and every byte
    outside ASCII are encoded, and it is written only that way:
    `Porter/`, `My%20Notes/`, `R%C3%A9sum%C3%A9s/`.
  - The parts appear at most once each, in that order, so a scope reads
    one way.
  - Anything else is 403 at `initialize`. So is any scope that a notesd
    from before this grammar cannot parse: it fails closed.
  - Porter's scope is `vault=<id>;write=Porter/;except=Porter/Routines/;daily`.
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
  - Under a scope with no write right, a method marked `writes` is
    `methodNotFound`. `daily` admits `note.daily` and `note.appendDaily`.
  - **A write outside the scope** is `invalidParams`, and its message
    says what the token may write.
  - **How a path is judged:** after the path guard, segment by segment
    against each prefix, after NFC normalization and case folding, so
    no spelling of an `except` folder escapes it.
  - **Aliases:** a path that reaches an existing file or folder by a
    name its folder does not list is `invalidParams` too. Examples are
    an 8.3 short name, or a trailing dot that a share trims.
  - **What each write needs:**
    - `note.create`, `write`, `upload`, `delete` and `revert`: the path.
    - `note.move`: both ends.
    - `note.mkdir`: the folder, which may be a `write=` folder itself.
    - `note.moveFolder` and `note.deleteFolder`: each folder writable
      whole, with no `except` at, above or under it.
    - A move with `updateLinks` whose rewrites would reach a note
      outside the scope is refused whole.
    - `note.daily` and `note.appendDaily`: the day's note writable, or
      under `daily` today's, and not under `except`. It is judged under
      the vault's lock, at the path the daily-note settings give.
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
--scope 'vault=<id>;write'`, or a folder scope such as
`'vault=<id>;write=Porter/;except=Porter/Routines/;daily'`.

### Limits

| What | Value |
| --- | --- |
| a JSON-RPC message, requests in flight | 16 MiB, 32 per connection |
| MCP sessions | 8 per token name, the least recently used evicted; an hour idle |
| a blob answer | a 206 of 8 MiB; 64 MiB whole |
| a relocation request | 1 KiB; a 32-byte nonce |
| pending pairings | 8 in all; 2 from one node behind lodge |
| requests without a device token (`POST /sync/pair`, `POST /sync/hub`) | 30 at once per peer, then one a second; past it 429 with `Retry-After`. The peer is lodge's node ID, a rate key and never an admission, or the remote address without lodge |

## Rules

- notesd listens on no network address. `--hub` refuses an address that
  is not loopback.
- notesd reads `Lodge-*` headers only on the app socket, and only after
  the peer check.
- Each path on the app socket admits one caller kind.
- An agent's scope is enforced by the engine. Tests call:
  - every `note` method on another vault;
  - every method marked `writes` under a read scope;
  - every method marked `writes` outside a folder scope;
  - each kind of write at the folder boundaries, with case and Unicode
    spellings of an `except` folder.
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

- Rationale: [ADR-0026](../adr/0026-notesd-runs-as-a-lodge-app.md), [ADR-0016](../adr/0016-pair-devices-with-a-one-time-code-and-a-revocable-token.md), [ADR-0012](../adr/0012-agents-reach-notes-through-mcp.md), [ADR-0023](../adr/0023-the-app-shows-files-by-their-hash-and-uploads-them-through-notesd.md) (files by hash), [ADR-0028](../adr/0028-an-agents-scope-can-limit-writes-to-folders.md) (an agent's folder scope)
- Context: [design/hub.md](../design/hub.md), [design/overview.md](../design/overview.md), [design/app.md](../design/app.md#in-a-browser)
- lodge: [app contract](https://github.com/frostyard/lodge/blob/main/docs/specs/app-contract.md), [lodge-http](https://github.com/frostyard/lodge/blob/main/docs/specs/lodge-http.md), [ADR-0003](https://github.com/frostyard/lodge/blob/main/docs/adr/0003-lodge-is-the-only-process-on-the-network.md), [ADR-0006](https://github.com/frostyard/lodge/blob/main/docs/adr/0006-agents-reach-each-app-through-its-own-mcp-endpoint.md), [ADR-0010](https://github.com/frostyard/lodge/blob/main/docs/adr/0010-untrusted-bytes-come-from-a-second-node.md)
- Related: [sync-protocol.md](sync-protocol.md), [rpc-protocol.md](rpc-protocol.md), [mcp.md](mcp.md)
- Built in: [plan 0006](../plans/0006-m4-notes-inside-lodge.md); folder scopes in [plan 0007](../plans/0007-folder-scopes-for-agents.md)
