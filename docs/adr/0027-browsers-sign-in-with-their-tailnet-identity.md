# 0027 — Browsers sign in with their tailnet identity

- **Status:** Proposed
- **Date:** 2026-10-18

## Context

M4's web app is the desktop app's React UI in a browser, talking to the
hub over a WebSocket as caller `session`
([ADR-0004](0004-a-notes-daemon-with-thin-clients.md),
[rpc-protocol](../specs/rpc-protocol.md#caller-kinds)). A session can
read and write every note, so its sign-in guards the whole vault. The
roadmap left sign-in open: passkeys, a password, or a proxy's identity.

On 2026-10-18 the user decided that browsers sign in by their Tailscale
identity, with no passwords or passkeys: notesd learns which tailnet
user is calling and turns that into a session. The hub is on the
tailnet only, as a node of its own whose WhoIs names the peer of each
connection
([ADR-0026](0026-the-hub-stays-on-the-tailnet-and-joins-it-itself.md)).

One person owns the hub, the tailnet and every device. The threats a
browser adds are not strangers on the network but:

- **other sites in the same browser.** A page on any site can open a
  WebSocket to the hub, submit a form to it, or load its images, and
  the request leaves from the user's device with the user's identity, as
  a cookie would. That is cross-site request forgery, cross-site
  WebSocket hijacking, and DNS rebinding of a hostile name onto the
  hub's address.
- **script in the app's own page** (XSS), from a note's content or a
  dependency. It can do anything the page can, while it runs.
- **a credential that leaks**: from storage, a URL, a log or a
  screenshot, and used from elsewhere.
- **other nodes on the tailnet.** Other nodes in the tailnet share the
  hub's site (`*.<tailnet>.ts.net`), so the browser counts them as the
  same site. Shared or tagged nodes may reach the hub too, as the
  tailnet's policy allows.

The device endpoints refuse any `Origin` but the device's own, and
devices carry a revocable token
([ADR-0016](0016-pair-devices-with-a-one-time-code-and-a-revocable-token.md)).
ADR-0016 also put approving a pairing on the hub's machine alone.

## Decision

- **Identity is the connection's tailnet peer, asked of the hub's own
  node.** For each browser request, notesd calls WhoIs on the request's
  remote address in its `tsnet` node. The answer is the WireGuard peer
  whose key Tailscale's coordination service bound to that node, and
  the node's owner. Nothing a client sends is consulted. Headers such as
  `Tailscale-User-Login`, `X-Forwarded-For` and `Forwarded` are
  ignored, and no cookie, query or token carries identity. The only
  ways to be someone else are to hold another node's WireGuard key, or
  to control the tailnet's coordination or identity provider. Both
  already give that node or user everything else on the tailnet.
- **Who may sign in is a list on the hub.** The hub keeps tailnet
  logins (`alice@example.com`, `alice@github`) in its database. It
  starts empty, so nobody signs in until the owner runs
  `notesctl hub allow LOGIN`. A peer may sign in when:
  - WhoIs names it;
  - its node is not tagged, since a tagged node belongs to no user;
  - its owner's login is on the list, compared exactly.

  A login from another tailnet, through a node shared with it, is
  refused unless listed. `notesctl hub disallow LOGIN` ends that login's
  sessions at once.
- **A session is one WebSocket connection,** and the browser holds no
  credential. `GET /api` upgrades to a WebSocket that serves JSON-RPC as
  caller `session` after these checks, each failure a 403 before any
  upgrade:
  - `Host` is the hub's name;
  - `Origin` is exactly `https://` and the hub's name;
  - `Sec-Fetch-Site`, when sent, is `same-origin`;
  - the peer may sign in.

  The hub keeps the session in memory while it lasts:
  - its ID, the login, the node's name and ID;
  - the client name given to `rpc.hello`, its start, its last request.

  It asks WhoIs again every minute, and ends the session when the peer
  no longer passes. There is no session cookie, no token in storage and
  nothing in a URL. Closing the page signs out. `hub.endSession` (the
  hub settings in the app, or `notesctl hub end`) ends one. A reconnect
  is a new sign-in, which costs one WhoIs.
- **Nothing outside the WebSocket changes anything.** The other browser
  routes (`/`, the web app's files, `/blobs/…`) answer `GET` and `HEAD`
  only, `405` otherwise, and read nothing from a query.
  - Forged requests: a cross-site form or fetch reaches the hub with the
    user's identity but finds nothing that writes. A cross-site
    WebSocket carries its page's `Origin`, which a browser sets and no
    page can change.
  - Reads across sites: every browser route checks `Host`, against DNS
    rebinding, and refuses `Sec-Fetch-Site` other than `same-origin` or
    `none` (a top-level navigation to `/` excepted). Its responses
    carry `Cross-Origin-Resource-Policy: same-origin`, `X-Frame-Options:
    DENY` and `Referrer-Policy: no-referrer`, and never a CORS header.
- **A session gets what the web app needs, method by method.** A method
  in `schema/rpc` may now narrow its domain's `callers`, and the router
  answers `methodNotFound` for the rest, as for a domain
  ([ADR-0008](0008-generate-the-rpc-contract-from-a-yaml-idl.md)).
  - Sessions may read and write notes (`note`), and list, read and
    configure vaults (`vault`), but not add or remove them: paths on
    the hub's disk stay the hub's socket's.
  - They may subscribe to events.
  - On the hub, they may list devices, pending pairings, MCP tokens,
    allowed logins and sessions, revoke devices and tokens, and end
    sessions.
- **Minting a credential stays on the hub's socket:** approving a
  pairing (as ADR-0016 has it), issuing an MCP token
  ([ADR-0028](0028-remote-mcp-serves-local-agents-with-scoped-tokens.md))
  and allowing a login. Script that runs in a page could use the
  session while the page is open, but could leave nothing behind that
  outlives it.
- **The hub, not the page, names a session's writes.** They are source
  `editor` with the node's name as author (`pixel (web)`), and the
  page's `source` and `author` parameters are ignored.
- **Sessions are bounded:** at most 8 at once per node and 32 per hub,
  each with the RPC limits of every connection (16 MiB messages, 32
  requests in flight).
- **Tests fake the identity, and never the binary.** WhoIs is an
  interface. Unit tests give the hub a fake one. The exit evidence's
  browser runs use a loopback listener with a fixed identity that exists
  only in a test binary built with the `e2e` tag. No flag or variable of
  a release build turns the checks off.

## Consequences

- Nothing in the browser can be phished, stolen or replayed: there are
  no passwords, no cookies and no tokens to manage, expire or rotate.
  A lost phone is removed from the tailnet, and its sessions end within
  a minute.
- The device is the credential. Anyone who uses an allowed person's
  unlocked phone or computer, in any browser on it, is signed in. So is
  any program on it that speaks WebSocket and sets the hub's `Origin`.
  The phone's lock screen and the tailnet's device approval are the
  guards.
- The web app works only at the hub's tailnet name, the one its
  certificate names.
- Sessions do not outlive a hub restart. The app reconnects, as it does
  when the desktop's notesd restarts.
- The hub settings in the web app cannot approve a pairing. The owner
  runs `notesctl hub approve` on the hub's machine, as during M2.
- Tailscale's coordination service is trusted to say who owns a node,
  as it already is for the rest of the tailnet.
- The IDL and the router gain method-level callers, an additive change
  to the generator.

## Alternatives considered

- **Tailscale's identity headers from `tailscale serve`:** forgeable on
  the bridge in today's layout, and only as trustworthy as the backend's
  listening address elsewhere
  ([ADR-0026](0026-the-hub-stays-on-the-tailnet-and-joins-it-itself.md)).
- **A session cookie issued after the tailnet check:** a second
  credential that needs `HttpOnly`, `SameSite`, CSRF tokens (the tailnet
  is one site, so `SameSite` does not separate its nodes), expiry and
  revocation. It gains nothing when every connection can be identified
  afresh.
- **Passwords or passkeys:** declined by the user. They are credentials
  to enroll, recover and rate-limit, for one person already identified
  by the tailnet.
- **Any tailnet user, or the hub node's owner, may sign in:** a tagged
  hub has no owner, and a share would admit another tailnet's users. An
  explicit list costs one command.
- **Approving pairings and issuing tokens in the web app:** convenient,
  but script in a page could then add a device or an agent that keeps
  its access after the page closes.

## References

- Shapes: [design/hub.md](../design/hub.md#browsers), [specs/hub-http.md](../specs/hub-http.md), [specs/rpc-protocol.md](../specs/rpc-protocol.md#caller-kinds), [design/app.md](../design/app.md#in-a-browser), [plan 0006](../plans/0006-m4-web-app.md)
- Builds on: [ADR-0004](0004-a-notes-daemon-with-thin-clients.md), [ADR-0008](0008-generate-the-rpc-contract-from-a-yaml-idl.md), [ADR-0016](0016-pair-devices-with-a-one-time-code-and-a-revocable-token.md), [ADR-0026](0026-the-hub-stays-on-the-tailnet-and-joins-it-itself.md)
- Related: [ADR-0007](0007-edit-in-codemirror-6-with-live-preview.md) (rendered notes never run script), [ADR-0028](0028-remote-mcp-serves-local-agents-with-scoped-tokens.md), [ADR-0029](0029-the-hub-serves-the-web-apps-build-from-a-directory.md)
