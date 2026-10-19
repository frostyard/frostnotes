# 0026 — notesd runs as a lodge app

- **Status:** Proposed
- **Date:** 2026-10-18

## Context

M4 brings the notes to a browser on another machine, to the user's
phone, and to agents on the tailnet ([roadmap](../plans/0002-roadmap.md)).
Today a hub is `notesd --hub ADDR`, which serves devices' sync in plain
HTTP behind a `tailscale serve` on another machine
([ADR-0016](0016-pair-devices-with-a-one-time-code-and-a-revocable-token.md)).
Agents use `notesctl mcp` over stdio on a device
([ADR-0012](0012-agents-reach-notes-through-mcp.md)).

M4's first drafts had notesd join the tailnet itself, sign browsers in by
their tailnet identity, issue MCP tokens and serve its web build. On
2026-10-18 the user decided to build **lodge** instead
([frostyard/lodge](https://github.com/frostyard/lodge);
[core ADR-0057](https://github.com/frostyard/core/blob/main/docs/adr/0057-run-first-party-apps-behind-lodge.md)). It is one tailnet-only web shell for frostnotes, frostmail and
a later assistant:

- lodge is the only process on the network. Each app is a daemon on a
  Unix socket that only lodge's user may open, running as its own system
  user
  ([lodge ADR-0003](https://github.com/frostyard/lodge/blob/main/docs/adr/0003-lodge-is-the-only-process-on-the-network.md)).
- lodge is a tailnet node of its own, `lodge`
  ([lodge ADR-0004](https://github.com/frostyard/lodge/blob/main/docs/adr/0004-lodge-stays-on-the-tailnet-and-joins-it-itself.md)).
- It signs browsers in by their tailnet identity, with allow-lists of
  devices and logins
  ([lodge ADR-0005](https://github.com/frostyard/lodge/blob/main/docs/adr/0005-browsers-sign-in-with-their-tailnet-identity.md)).
- It issues each app's MCP tokens and serves `/mcp/<id>`
  ([lodge ADR-0006](https://github.com/frostyard/lodge/blob/main/docs/adr/0006-agents-reach-each-app-through-its-own-mcp-endpoint.md)).
- It serves every app's web build from disk
  ([lodge ADR-0007](https://github.com/frostyard/lodge/blob/main/docs/adr/0007-lodge-serves-every-apps-web-build-from-disk.md)).
- It installs on a home screen once, for all apps
  ([lodge ADR-0008](https://github.com/frostyard/lodge/blob/main/docs/adr/0008-lodge-installs-as-one-home-screen-app.md)).
- It shows each app in a frame that stays loaded
  ([lodge ADR-0009](https://github.com/frostyard/lodge/blob/main/docs/adr/0009-apps-switch-in-frames-that-stay-loaded.md)).
- It serves untrusted bytes from a second node, `lodge-content`
  ([lodge ADR-0010](https://github.com/frostyard/lodge/blob/main/docs/adr/0010-untrusted-bytes-come-from-a-second-node.md)).

Those drafts became lodge's ADRs 0004 to 0008.

The [app contract](https://github.com/frostyard/lodge/blob/main/docs/specs/app-contract.md)
says what an app installs and serves:

- a manifest and a web build under `/usr/share/lodge/apps/<id>/`;
- on its socket, `GET /health`, a JSON-RPC WebSocket at `GET /api`, and,
  when the manifest says so, `/content/…`, `/mcp` and `/device/…`.

Before proxying a request, lodge deletes any `Lodge-*` header the client
sent, then sets the caller's: `Lodge-Caller`, plus the login, the node's
ID and name and the session's ID for a session, or `Lodge-Token` and
`Lodge-Scope` for an agent. lodge never parses an app's JSON-RPC, and
does not interpret a token's scope.

The review of the drafts left the notes side two fixes:

- an agent's scope must not rest on the tools' code alone;
- moving a device to a new hub URL must not hand its token to whoever
  repeats the hub's ID.

## Decision

- **On a server, notesd is lodge's `notes` app.**
  - `notesd --lodge` serves HTTP/1.1 on the app socket the contract
    names for `notes`.
  - It runs as the system user `frostnotes`.
  - It ships its manifest and web build for lodge to install.

  Its manifest:

  ```json
  { "contract": 1, "id": "notes", "name": "Notes", "icon": "icon.svg",
    "version": "…", "user": "frostnotes", "key": "n",
    "content": true, "device": true, "mcp": true, "remoteImages": true }
  ```

- **notesd listens on no network address.**
  - The hub's routes for devices move under `/device/` on the app
    socket, which lodge serves at `/device/notes/`.
  - `--hub ADDR` stays for loopback only: tests, the simulation and
    the exit evidence.
- **The socket is what notesd trusts.** The headers count only on the
  app socket:
  - notesd accepts a connection there only from lodge's user (the
    socket peer's uid, through lodge's Go kit);
  - it refuses a request without `Lodge-Caller`;
  - each route takes one caller kind, `/api` session, `/mcp` agent,
    `/content/` content and `/device/` device, and any other is 403.
- **What stays notes' own:**
  - **The caller kinds.**
    - `session` and `agent` come from the `Lodge-*` headers on the app
      socket.
    - `device` is a device token on `/device/`.
    - `local` is notesd's private socket, which notesctl and the
      desktop app's bridge keep using.
  - **The callers each method admits.** A method may narrow its
    domain's callers, and methods that change notes are marked
    `writes` in `schema/rpc`.
  - **Attribution by connection.**
    - A session's writes are source `editor`, with the node's name
      lodge sends (`Lodge-Node-Name`) and ` (web)` as author.
    - An agent's writes are source `agent`, with the token's name as
      author.
    - The request's own `source` and `author` are ignored.
  - **The JSON-RPC behind `/api`,** whose frames lodge only moves.
  - **Files by hash** at `/content/blobs/<hash>.<ext>`. lodge-content
    serves them, and they keep notesd's blob headers.
  - **The MCP tools behind `/mcp`.** Notes defines the scope grammar,
    `vault=<id>` with an optional `;write`, and binds each MCP session
    to its token's name.
  - **The device protocol and pairing**
    ([ADR-0016](0016-pair-devices-with-a-one-time-code-and-a-revocable-token.md)),
    unchanged on the wire except for the path prefix.
- **The engine enforces an agent's scope,** not only the tools' code.
  Each MCP session reaches the engine over an in-process connection that
  carries its scope:
  - a call naming another vault is `notFound`, as for a vault that does
    not exist;
  - a method marked `writes`, under a read scope, is `methodNotFound`.
- **Approving a device stays on notesd's private socket**
  (`notesctl hub approve`), as ADR-0016 has it. lodge's allow-lists,
  sessions and MCP tokens are lodge's to manage. The web app lists the
  hub's devices and can revoke one.
- **Devices follow the hub to lodge without trusting a stranger.**
  - **The automatic move.** The hub records the public URL it is served
    at, with `notesctl hub url`. Devices learn that URL over their
    authenticated sync connection and move to it.
  - **The manual move,** for a device whose old URL is gone:
    `notesctl hub relocate URL NEW-URL`. The device sends its token only
    after `NEW-URL` returns a proof keyed with the token's stored hash:
    an HMAC over the hub's ID, the hub's recorded URL and the device's
    nonce. A relay that forwards the nonce to the real hub gets back a
    proof naming the real URL, not its own.
- **The desktop app stays local-first.** It talks to its own notesd
  over the private socket, and is not a lodge client.

## Consequences

- notesd carries no tailnet node and no TLS. It has no sign-in, MCP
  token store or static file server.
  - It depends on lodge's Go kit (`lodge/kit`), which keeps
    `tailscale.com` out.
  - Its JSON-RPC runtime is the kit's `jsonrpc`, shared with frostmail:
    the framing, the gate on `rpc.hello`, dispatch, the router's caller
    kinds, the client and the private socket. The contract and its
    generator stay notes' own
    ([ADR-0008](0008-generate-the-rpc-contract-from-a-yaml-idl.md);
    lodge's [ADR-0012](https://github.com/frostyard/lodge/blob/main/docs/adr/0012-the-kit-shares-the-rpc-runtime-not-the-generator.md)),
    and so do the caller kinds, the agent's scope and identities.
  - frostmail and the assistant get sign-in, tokens and serving from
    lodge too, not from copies.
- Using notes in a browser waits on lodge. M4 depends on lodge's
  roadmap phases 1 (the kit) and 2 (lodge v1 with notes)
  ([lodge's roadmap](https://github.com/frostyard/lodge/blob/main/docs/plans/0001-roadmap.md)).
- **On the server:**
  - The hub's machine runs lodge.
  - The vault's directory must be writable by `frostnotes`. On a
    network share, that is the mount's options.
  - `notesctl` runs as `frostnotes` there (`sudo -u frostnotes
    notesctl …`), since the private socket admits its own user only.
- **lodge is the trust root for callers.** A process running as lodge's
  user could claim any caller to notesd, as lodge itself can.
- **Agents.**
  - Agents on the tailnet reach notes only through lodge's
    `/mcp/notes`, with lodge's tokens.
  - `notesctl mcp` over stdio stays for an agent on a device with its
    own copy.
- The IDL gains method-level callers and a `writes` mark. These are
  additive changes to the generator.
- The M4 drafts' ADRs 0026 to 0030 left this repository before anyone
  accepted them, and live on as lodge's ADRs 0004 to 0008.

## Alternatives considered

- **notesd on the tailnet itself,** as M4's drafts had it: every app
  would carry its own node, sign-in, token store and file server.
  frostmail and the assistant would repeat them, each a copy to review.
- **lodge as a library linked into each app:** one network process per
  app again, each holding tailnet state and credentials.
- **lodge enforcing what each caller may call:** lodge would have to
  parse every app's JSON-RPC and know its methods. Rights stay with the
  app that defines them.
- **A proof of possession alone for relocation,** as the review
  suggested: it can be relayed through the unauthenticated endpoint
  unless it names the URL. Binding it to the hub's recorded URL closes
  that.

## References

- Shapes: [specs/lodge-app.md](../specs/lodge-app.md), [design/hub.md](../design/hub.md), [design/overview.md](../design/overview.md), [design/app.md](../design/app.md#in-a-browser), [specs/rpc-protocol.md](../specs/rpc-protocol.md#caller-kinds), [specs/mcp.md](../specs/mcp.md#through-lodge), [plan 0006](../plans/0006-m4-notes-inside-lodge.md)
- Builds on: [ADR-0004](0004-a-notes-daemon-with-thin-clients.md), [ADR-0005](0005-sync-through-a-hub-with-revisions-and-three-way-merges.md), [ADR-0006](0006-every-write-names-the-version-it-changes.md), [ADR-0008](0008-generate-the-rpc-contract-from-a-yaml-idl.md), [ADR-0012](0012-agents-reach-notes-through-mcp.md), [ADR-0016](0016-pair-devices-with-a-one-time-code-and-a-revocable-token.md); frostyard/core [ADR-0057](https://github.com/frostyard/core/blob/main/docs/adr/0057-run-first-party-apps-behind-lodge.md)
- lodge: [ADR-0003](https://github.com/frostyard/lodge/blob/main/docs/adr/0003-lodge-is-the-only-process-on-the-network.md) to [ADR-0010](https://github.com/frostyard/lodge/blob/main/docs/adr/0010-untrusted-bytes-come-from-a-second-node.md), [the app contract](https://github.com/frostyard/lodge/blob/main/docs/specs/app-contract.md), [lodge-http](https://github.com/frostyard/lodge/blob/main/docs/specs/lodge-http.md), [lodge-client](https://github.com/frostyard/lodge/blob/main/docs/specs/lodge-client.md)
- Related: [ADR-0027](0027-a-narrow-window-takes-obsidians-phone-layout.md) (the phone layout inside lodge's frame)
