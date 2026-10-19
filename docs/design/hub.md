# Design: the hub on the tailnet

Living document. Rationale:
[ADR-0026](../adr/0026-the-hub-stays-on-the-tailnet-and-joins-it-itself.md)
(the tailnet node),
[ADR-0027](../adr/0027-browsers-sign-in-with-their-tailnet-identity.md)
(browser sessions),
[ADR-0028](../adr/0028-remote-mcp-serves-local-agents-with-scoped-tokens.md)
(agents' tokens),
[ADR-0029](../adr/0029-the-hub-serves-the-web-apps-build-from-a-directory.md)
(the web app's files),
[ADR-0030](../adr/0030-the-web-app-installs-on-a-home-screen-and-needs-the-hub.md)
(the home-screen page).
Contracts: [specs/hub-http.md](../specs/hub-http.md) (every route, its
checks and headers), [specs/rpc-protocol.md](../specs/rpc-protocol.md)
(caller kinds), [specs/mcp.md](../specs/mcp.md#the-hubs-endpoint),
[specs/sync-protocol.md](../specs/sync-protocol.md) (devices).

**Status: planned** for M4 ([plan 0006](../plans/0006-m4-web-app.md)).
Today's hub serves only devices, on a plain HTTP address behind a
`tailscale serve` on another machine ([sync.md](sync.md)); this
document is rewritten as each phase lands.

## Overview

A hub is a notesd started with `--hub-tailnet NAME`. It joins the
tailnet as a node of its own and serves three kinds of caller there, on
port 443 with the node's certificate. Each kind has its own credential:

- devices sync with a device token;
- browsers run the web app with the identity of their tailnet node;
- agents use MCP with a scoped token.

Nothing of the hub is reachable off the tailnet.

```
 phone, second machine (browser)       laptop (Claude Code)          desktops, VMs (notesd, device role)
        │ https://NAME.<tailnet>.ts.net      │ Bearer fnm_…                    │ Bearer fnd_…
        └──────────────── WireGuard (the tailnet) ──────────────────────────────┘
                                         │
 notesd --hub-tailnet NAME ── internal/tailnet (tsnet: the node, TLS on 443, WhoIs) ── data dir: tailnet/
        │
        ├─ /            /assets/…  the web app's build (--hub-web DIR)      session: WhoIs → allowed login
        ├─ /api         JSON-RPC over WebSocket, caller session            session: + Origin, Host
        ├─ /blobs/…     files by hash                                      session
        ├─ /mcp         MCP Streamable HTTP, caller agent (in process)     MCP token: vault, read or write
        └─ /sync, /sync/blobs/…, /sync/pair  (sync-protocol)              device token
                │
        api.Router ── engine ── vaults, index, versions, the revision log
```

The hub's own machine keeps the local socket. Over it, `notesctl hub`
does what only the owner may: serve a vault, approve a pairing, issue an
MCP token, and allow a login to sign in.

## Design

### The tailnet node

- `internal/tailnet` is the only package that imports `tailscale.com`.
  It starts a `tsnet.Server`:
  - its hostname is NAME and its state is in `$FROSTNOTES_DATA_DIR/tailnet/`
    (0700);
  - its log upload is off;
  - its auth key comes from `TS_AUTHKEY` on the first start.

  It gives the hub a TLS listener on `:443` (`ListenTLS`), the node's
  name (the first of `CertDomains`), and `WhoIs(ctx, remoteAddr)`, which
  returns a `Peer`: the node's stable ID, its short name, whether it is
  tagged, and its owner's login and display name.
- The hub serves its whole handler on that listener and on nothing
  else, except a plain loopback listener (`--hub 127.0.0.1:PORT`) for
  tests, the simulation and a device on the same machine.
- The node's certificate renews inside `tsnet`. Tailscale's policy for
  the tailnet decides which nodes may connect at all. The hub's docs
  suggest a tag for the hub and a grant from the owner's devices to
  that tag's port 443, and no other.

### Routes

Every request passes the router of
[specs/hub-http.md](../specs/hub-http.md). It sorts each path into
device, browser or agent routes, applies that kind's checks, and answers
404 for any other path. The device routes keep their `Origin` rule (only
`notesd://device`, or none). The browser routes want the hub's own
origin, and `/mcp` wants none. One rule never leaks into another.

### Browsers

- **Sign-in is the connection.** On each browser request, the hub asks
  WhoIs about the remote address. It admits the request when:
  - the peer's node is not tagged;
  - its owner's login is on the hub's list (`hub_users`, managed with
    `notesctl hub allow` and `disallow`).

  The answer is cached per connection for at most a minute.
- **A session** is the `/api` WebSocket, upgraded after the `Host`,
  `Origin` and `Sec-Fetch-Site` checks. The hub's session registry
  (`internal/hub/sessions.go`) holds each live one:
  - its random ID, the peer, the client name from `rpc.hello`;
  - when it started, and its last request.

  A ticker asks WhoIs again each minute and closes any session whose
  peer no longer passes. `disallow` closes the login's sessions, and
  `hub.endSession` closes one. Per node and per hub, a new session
  beyond the limit is refused, not an old one closed.
- **The connection serves caller `session`.** The router admits only
  the methods whose callers list it, method by method. A session's
  writes carry source `editor` and the author `<node name> (web)`. The
  connection sets them in the context, and the engine prefers them to
  the request's parameters.
- **Files by hash** (`/blobs/<sha256>.<ext>`) pass the same identity
  check per request, with no token in the URL. They answer as the dev
  gateway's do, with `Cross-Origin-Resource-Policy: same-origin`, so
  another site cannot embed them.
- **Nothing else writes:** every browser route but the WebSocket
  answers `GET` and `HEAD` only.

### Agents

- `/mcp` is `internal/mcpserver`'s Streamable HTTP handler, so the MCP
  SDK stays in that package. The hub gives it a token check and a way
  to open an in-process connection to the engine.
- **The token check:** the request's bearer token is hashed, and
  `mcp_tokens` gives its row (name, vault, access). A revoked or
  unknown token is 401, and a request with an `Origin` is 403. The
  token's last use is recorded at most once a minute.
- **Each MCP session** gets a tool server built for its token:
  - `Options{VaultID: token's vault, ReadOnly: access == read, Author:
    token's name}`;
  - an `api.Client` over a `net.Pipe` whose other end
    `rpcserver.ServeConn` serves as caller `agent`.

  The session is bound to the token's ID, and revoking the token closes
  it. The agent's `clientInfo.name` and node go to the log.

### The web app's files

`--hub-web DIR` names the build `make web-build` makes
([ADR-0029](../adr/0029-the-hub-serves-the-web-apps-build-from-a-directory.md)).
At start the hub reads `DIR/frostnotes-web.json`, and refuses to serve
a build of another protocol major. Per request it opens the file by its
cleaned path under `DIR`, refusing what is not a regular file inside
`DIR`. Headers come from one function shared by every browser route:
the content security policy, `nosniff`, the referrer, opener and
resource policies, framing and caching.

### Hub settings in the app

When the app's notesd is a hub (`hub.status` answers), the settings
dialog has a Hub section, on the desktop of the hub's machine and in
the web app alike:

- devices (revoke), and pending pairings (shown with the command that
  approves them on the hub);
- MCP tokens (revoke), and the command that issues one;
- allowed logins, read-only;
- sessions (end).

What a session may not call is not drawn.

### Moving devices to the node

A device's hub URL names the old `tailscale serve` address until it
relocates. `notesctl hub relocate URL NEW-URL` (`sync.relocate`) first
asks `NEW-URL/sync/hub`, which answers the hub's ID to anyone on the
tailnet. Only if that ID is the paired hub's does the device keep
`NEW-URL`. Its token, replicas, agreed states and cursors stay, and the
next connection goes to the new address.

### Testing

- `internal/tailnet`: real `tsnet` nodes against Tailscale's in-process
  test control server. WhoIs names a user's node and a tagged node, the
  TLS listener answers on the node's name, and no listener exists off
  the tailnet.
- `internal/hub`: the routes with a fake WhoIs and `httptest`:
  - every check of [specs/hub-http.md](../specs/hub-http.md) has a test
    that fails it, and one that passes it;
  - forged identity headers change nothing;
  - `disallow`, `endSession` and token revocation close live
    connections;
  - path tricks on the static route find nothing.
- `internal/mcpserver`: the Streamable HTTP handler under the MCP SDK's
  client, read and write tokens, attribution, and a revoked token
  mid-session.
- The web app: Vitest over the hub transport's reconnect with a fake
  WebSocket. Exit evidence runs it in Firefox
  ([plan 0006](../plans/0006-m4-web-app.md#phase-8--the-users-hub-and-exit-evidence)),
  against a hub on loopback with the `e2e` build's fixed identity.

## Operational notes

- **First start:** create a tagged, single-use auth key in the
  tailnet's admin console, and start the hub once with it in
  `TS_AUTHKEY`. Later starts read `tailnet/` and need no key. Then
  `notesctl hub allow <your login>` and open `https://NAME.<tailnet>.ts.net/`.
- **Flags:**
  - `--hub-tailnet NAME`, the node;
  - `--hub ADDR`, a loopback address for tests;
  - `--hub-web DIR`, the web build;
  - `--hub-max-blob`, as before.
- **Losing a phone:** remove it from the tailnet. Its sessions close at
  the next minute's check. No credential on it outlives that.
- **A token in the wrong hands:** `notesctl hub token revoke NAME`. Its
  writes are in each note's history under its name, and each can be
  reverted.
- **Back up** `tailnet/` with the rest of the data directory. Restoring
  it keeps the node's name; losing it means a new auth key, and the
  same name if the old node is first removed from the tailnet.
- **Nothing private in the name:** the node's name is in public
  certificate logs.
- If the hub cannot reach Tailscale's coordination service at start, it
  logs that and keeps trying. Devices offline from the hub keep
  working, and the web app shows the hub as out of reach.

## References

- Rationale: [ADR-0026](../adr/0026-the-hub-stays-on-the-tailnet-and-joins-it-itself.md), [ADR-0027](../adr/0027-browsers-sign-in-with-their-tailnet-identity.md), [ADR-0028](../adr/0028-remote-mcp-serves-local-agents-with-scoped-tokens.md), [ADR-0029](../adr/0029-the-hub-serves-the-web-apps-build-from-a-directory.md), [ADR-0030](../adr/0030-the-web-app-installs-on-a-home-screen-and-needs-the-hub.md); [ADR-0016](../adr/0016-pair-devices-with-a-one-time-code-and-a-revocable-token.md) (devices), [ADR-0012](../adr/0012-agents-reach-notes-through-mcp.md) (MCP)
- Contracts: [specs/hub-http.md](../specs/hub-http.md), [specs/rpc-protocol.md](../specs/rpc-protocol.md), [specs/mcp.md](../specs/mcp.md), [specs/sync-protocol.md](../specs/sync-protocol.md)
- Related: [overview.md](overview.md), [sync.md](sync.md), [app.md](app.md#in-a-browser)
- Built in: [plan 0006 — M4](../plans/0006-m4-web-app.md) ([roadmap — M4](../plans/0002-roadmap.md))
