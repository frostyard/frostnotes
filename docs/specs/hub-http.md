# Spec: the hub's HTTP surface

Every route a hub serves, who may call it, with what credential, and
what it answers: the contract `internal/hub`, `internal/tailnet` and
`internal/mcpserver` implement, the web app and agents rely on, and M4's
security review checks. The device routes' own protocol is
[sync-protocol.md](sync-protocol.md), the RPC's wire rules are
[rpc-protocol.md](rpc-protocol.md), and the MCP tools are
[mcp.md](mcp.md). Rationale:
[ADR-0026](../adr/0026-the-hub-stays-on-the-tailnet-and-joins-it-itself.md),
[ADR-0027](../adr/0027-browsers-sign-in-with-their-tailnet-identity.md),
[ADR-0028](../adr/0028-remote-mcp-serves-local-agents-with-scoped-tokens.md),
[ADR-0029](../adr/0029-the-hub-serves-the-web-apps-build-from-a-directory.md);
where it fits: [design/hub.md](../design/hub.md).

**Status: M4's contract,** written in
[plan 0006](../plans/0006-m4-web-app.md)'s Phase 1. It binds each route
as its phase lands (Phases 2, 4 and 5). Until then the hub serves only
[sync-protocol.md](sync-protocol.md)'s routes on its plain address.

## Interface

### Listeners

| Listener | Flag | Serves |
| --- | --- | --- |
| the hub's tailnet node, port 443, TLS with the node's certificate | `--hub-tailnet NAME` | every route below |
| plain HTTP on a loopback address | `--hub ADDR` | every route below; browser routes refuse every request, since no peer on it has a tailnet identity (but in an `e2e` build, below) |

- `NAME` is the node's host name. The hub's name, written `HUB` below,
  is the node's full DNS name, the first of `tsnet`'s `CertDomains`
  (`NAME.<tailnet>.ts.net`).
- `--hub ADDR` refuses an address that is not loopback (`127.0.0.0/8`,
  `::1`) at start.
- The node listens on no port but 443, and never through Funnel.
- A test binary built with the `e2e` tag may give the loopback
  listener's peers one fixed identity. Release builds have no such
  code: `internal/repocheck` checks that no file without the tag
  defines it.

### Routes

| Path | Methods | Kind | Credential |
| --- | --- | --- | --- |
| `/sync/pair` | `POST` | device | none ([sync-protocol](sync-protocol.md#pairing)) |
| `/sync/pair/{pairing}` | `GET` | device | `Bearer` pairing secret |
| `/sync` | `GET` (WebSocket) | device | `Bearer` device token |
| `/sync/blobs/{hash}` | `HEAD`, `GET`, `PUT` | device | `Bearer` device token |
| `/sync/hub` | `GET` | device | none: answers `{"hub": "<hub ID>"}` |
| `/api` | `GET` (WebSocket) | browser | tailnet identity, `Origin` |
| `/blobs/{hash}.{ext}` | `GET`, `HEAD` | browser | tailnet identity |
| `/`, and the web build's files | `GET`, `HEAD` | browser | tailnet identity |
| `/mcp` | `POST`, `GET`, `DELETE` | agent | `Bearer` MCP token |

Any other path answers 404. A path's kind decides its checks, and no
check of one kind applies to another. Error responses are `text/plain;
charset=utf-8`, one line saying why, with no detail about other users,
nodes or tokens.

### Device routes

As [sync-protocol.md](sync-protocol.md#endpoints), unchanged: an
`Origin` other than `notesd://device` is 403, and a bad token is 401.
`/sync/hub` lets a device relocating to a new URL check the hub's ID
before it sends its token there.

### Browser checks

Every browser request passes these, in order. A failure is 403, and an
`/api` request fails before its upgrade.

1. **Host:** `Host` is `HUB` or `HUB:443`.
2. **Origin:** an `Origin` header, when present, is exactly
   `https://HUB`. On `/api` it must be present.
3. **Fetch metadata:** `Sec-Fetch-Site`, when present, is `same-origin`
   or `none`. On `/api` it must be `same-origin`. One exception: a
   top-level navigation to `/` (`GET`, `Sec-Fetch-Mode: navigate`,
   `Sec-Fetch-Dest: document`) passes from any site.
4. **Method:** `GET` or `HEAD`, else 405. `/api` takes `GET` with a
   WebSocket upgrade only.
5. **Identity:** WhoIs on the request's remote address names a peer
   whose node is not tagged and whose owner's login is on the hub's
   list of allowed logins, compared byte for byte. A peer WhoIs cannot
   name, a tagged node and a login not on the list each fail with their
   own line. The last of these names the login and the command that
   allows it (`notesctl hub allow LOGIN`).

Identity comes from WhoIs alone. The hub never reads `Tailscale-User-*`,
`Tailscale-App-Capabilities`, `X-Forwarded-*`, `Forwarded` or
`X-Real-IP`, nor a cookie or query parameter, for identity. A WhoIs
answer may be reused for requests on the same TCP connection for at
most 60 s.

### Headers on browser routes

Every browser response, errors included, carries:

| Header | Value |
| --- | --- |
| `Content-Security-Policy` | `default-src 'none'; script-src 'self'; style-src 'self' 'unsafe-inline'; img-src 'self' data: https:; media-src 'self'; font-src 'self' data:; connect-src 'self' wss://HUB; manifest-src 'self'; worker-src 'none'; frame-src 'none'; object-src 'none'; base-uri 'none'; form-action 'none'; frame-ancestors 'none'` |
| `X-Content-Type-Options` | `nosniff` |
| `X-Frame-Options` | `DENY` |
| `Referrer-Policy` | `no-referrer` |
| `Cross-Origin-Opener-Policy` | `same-origin` |
| `Cross-Origin-Resource-Policy` | `same-origin` |
| `Permissions-Policy` | `camera=(), microphone=(), geolocation=()` |

`/blobs/` responses replace the policy with the blob policy below. No
browser response carries `Access-Control-Allow-*` or `Set-Cookie`.

### `/api`

- The upgrade is a WebSocket (RFC 6455) whose text frames carry
  [rpc-protocol.md](rpc-protocol.md#transports)'s lines; the connection
  serves JSON-RPC as caller `session` for its lifetime.
- A `GET /api` without an upgrade answers as an upgrade would up to the
  upgrade itself: a failed check with its 403 line, else 426 Upgrade
  Required. A page whose WebSocket fails, which a browser does not tell
  it why, learns the reason this way.
- **Limits:** at most 8 live sessions per node and 32 per hub. One more
  is refused with 429 before the upgrade. Messages are at most 16 MiB,
  with at most 32 requests in flight.
- **The session** has an ID of 16 lowercase hex digits from
  `crypto/rand`, the peer (node ID, node name, login, display name),
  the client name given to `rpc.hello`, its start, and its last
  request. The hub keeps it in memory only.
- **Re-checks:** every 60 s the hub asks WhoIs again for each session's
  remote address. It closes the connection when the peer has changed,
  has become tagged, or its login is no longer allowed. `hub.disallow`
  and `hub.endSession` close at once.

### `/blobs/`

`GET` or `HEAD` `/blobs/<hash>.<ext>`: `hash` is 64 lowercase hex
digits, and `ext` 1 to 8 lowercase letters or digits. Answers:

- the content with that hash from the hub's blob store, else 404;
- the content type the extension names
  ([design/app.md](../design/app.md#attachments));
- `Content-Security-Policy: default-src 'none'; style-src 'unsafe-inline'; sandbox`;
- `Cache-Control: private, max-age=31536000, immutable`;
- one `Range` per request; a 206 carries at most 8 MiB, and a request
  without a range is answered whole up to 64 MiB, else 413.

### The web app's files

With `--hub-web DIR`:

- At start, `DIR/index.html` must exist, and `DIR/frostnotes-web.json`
  must be `{"version": "…", "protocol": N}` with N the hub's protocol
  major. Otherwise the hub logs why, and every web-file path answers
  503.
- `/` serves `DIR/index.html`. Any other path serves the file at that
  path under `DIR` when every one of these holds; otherwise 404:
  - the path matches `^(/[A-Za-z0-9_-][A-Za-z0-9._-]*)+$`, so no
    segment starts with `.` and no segment is empty;
  - no element of it is a symbolic link;
  - it names a regular file.
- `Cache-Control`: `private, max-age=31536000, immutable` under
  `/assets/`; `no-cache` for everything else.
- Content types by extension: `.html` `text/html; charset=utf-8`; `.js`
  `text/javascript; charset=utf-8`; `.css` `text/css; charset=utf-8`;
  `.json` `application/json`; `.webmanifest`
  `application/manifest+json`; `.svg` `image/svg+xml`; `.png`
  `image/png`; `.ico` `image/x-icon`; `.woff2` `font/woff2`; `.woff`
  `font/woff`; `.ttf` `font/ttf`; anything else
  `application/octet-stream`.
- Without `--hub-web`, these paths answer 404.

### `/mcp`

[mcp.md](mcp.md#the-hubs-endpoint) gives the endpoint's behavior; its
HTTP checks are, in order:

1. a request with an `Origin` header is 403;
2. `Authorization: Bearer <token>`, missing, unknown or revoked, is 401
   with `WWW-Authenticate: Bearer` and no detail;
3. `Mcp-Session-Id` names a session of the same token, else 404 (MCP's
   answer for an unknown session);
4. a ninth live session for one token is refused with 429.

The token's last use is recorded at most once a minute.

### Tokens and logins

| Credential | Form | Kept on the hub |
| --- | --- | --- |
| device token | `fnd_` + 43 base64url characters (32 random bytes) | SHA-256, hex ([sync-protocol](sync-protocol.md)) |
| MCP token | `fnm_` + 43 base64url characters (32 random bytes from `crypto/rand`) | SHA-256, hex, with its name, vault ID, access (`read` or `write`), creation, last use and revocation |
| allowed login | a tailnet login name, 1 to 256 bytes of UTF-8 without control characters | as given, with when it was added |

- A token's name is 1 to 64 characters from `[A-Za-z0-9@._-]`, unique
  on the hub among tokens not revoked.
- A token is shown once, when it is issued, and never again: not by
  `hub.tokens`, not in a log, not in the database.

### Methods on the hub

`schema/rpc/hub.yaml` (domain callers `local` and `session`), each
method's callers narrowing them
([rpc-protocol.md](rpc-protocol.md#caller-kinds)):

| Method | Callers | Does |
| --- | --- | --- |
| `hub.status` | local, session | `{hub, listen?, tailnet?, web, vaults, devices, connected, sessions}`; `tailnet` is `HUB`, `web` whether a build is served |
| `hub.vaults`, `hub.pending`, `hub.devices` | local, session | as [sync-protocol](sync-protocol.md#local-domains) |
| `hub.revoke {device}` | local, session | as before |
| `hub.serve {vaultId}`, `hub.approve {code}` | local | as before |
| `hub.users` | local, session | the allowed logins: `{login, addedAt}` |
| `hub.allow {login}` | local | adds a login; `conflict` if listed |
| `hub.disallow {login}` | local | removes it and closes its sessions; `notFound` if absent |
| `hub.tokens` | local, session | MCP tokens: `{id, name, vaultId, access, createdAt, usedAt?, revoked}` |
| `hub.issueToken {name, vaultId, write?}` | local | `{token, info}`: the secret, once, and the row; `conflict` for a name in use, `notFound` for a vault |
| `hub.revokeToken {name}` | local, session | revokes it and closes its MCP sessions; `notFound` if absent |
| `hub.sessions` | local, session | live browser sessions: `{id, login, name, node, client, startedAt, seenAt}` |
| `hub.endSession {id}` | local, session | closes one; `notFound` if absent |

On a device, `sync.relocate {hub, url}` (caller `local`) changes a
paired hub's URL after `GET url/sync/hub` answers that hub's ID, else
`conflict`; the URL follows sync-protocol's URL rules.

### Commands

| Command | Method |
| --- | --- |
| `notesctl hub users` | `hub.users` |
| `notesctl hub allow LOGIN`, `notesctl hub disallow LOGIN` | `hub.allow`, `hub.disallow` |
| `notesctl hub tokens` | `hub.tokens` |
| `notesctl hub token issue NAME --vault V [--write]` | `hub.issueToken`; prints the token on stdout, alone, and how to add it to Claude Code on stderr |
| `notesctl hub token revoke NAME` | `hub.revokeToken` |
| `notesctl hub sessions`, `notesctl hub end ID` | `hub.sessions`, `hub.endSession` |
| `notesctl hub relocate URL NEW-URL` | `sync.relocate` |

## Rules

- The hub serves nothing on a non-loopback address except through its
  tailnet node, and nothing through Funnel.
- No browser route writes, and no request to one reads identity from
  anything but WhoIs.
- Every browser check has a test that fails it, and the security
  review's findings each have a test.
- A session's and an agent's writes carry the connection's source and
  author, never the request's ([rpc-protocol.md](rpc-protocol.md#caller-kinds)).
- Approving a pairing, issuing an MCP token and allowing a login answer
  `methodNotFound` to every caller but `local`.
- No response, log line or database row holds a device token, an MCP
  token or a pairing secret.
- Every limit in this spec has a test at the limit and one past it.

## Derived artifacts

| Artifact | Derivation |
| --- | --- |
| `api/zz_generated.go`, `app/src/rpc/gen/api.ts`, [rpc-api.md](rpc-api.md) | `make gen` from `schema/rpc/hub.yaml`, `sync.yaml`, `note.yaml`, `vault.yaml` and `rpc.yaml` (Phase 3) |
| `internal/store/migrations/` | `hub_users` and `mcp_tokens` (Phase 3) |

## References

- Rationale: [ADR-0026](../adr/0026-the-hub-stays-on-the-tailnet-and-joins-it-itself.md), [ADR-0027](../adr/0027-browsers-sign-in-with-their-tailnet-identity.md), [ADR-0028](../adr/0028-remote-mcp-serves-local-agents-with-scoped-tokens.md), [ADR-0029](../adr/0029-the-hub-serves-the-web-apps-build-from-a-directory.md), [ADR-0030](../adr/0030-the-web-app-installs-on-a-home-screen-and-needs-the-hub.md); [ADR-0016](../adr/0016-pair-devices-with-a-one-time-code-and-a-revocable-token.md), [ADR-0023](../adr/0023-the-app-shows-files-by-their-hash-and-uploads-them-through-notesd.md) (files by hash)
- Context: [design/hub.md](../design/hub.md), [design/overview.md](../design/overview.md)
- Related: [sync-protocol.md](sync-protocol.md), [rpc-protocol.md](rpc-protocol.md), [mcp.md](mcp.md)
- Built in: [plan 0006](../plans/0006-m4-web-app.md)
