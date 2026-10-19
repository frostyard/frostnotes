# 0026 — The hub stays on the tailnet, and joins it itself

- **Status:** Proposed
- **Date:** 2026-10-18

## Context

M4 puts a browser and remote agents on the hub
([roadmap](../plans/0002-roadmap.md)). Until now only devices reach it,
with a device token over TLS
([ADR-0016](0016-pair-devices-with-a-one-time-code-and-a-revocable-token.md)).
ADR-0016 left the hub's TLS to M4, and the roadmap lists "the hub on the
internet" as top risk 3.

During M2's trial the hub is `notesd --hub 0.0.0.0:PORT` in a virtual
machine on an always-on host. The host runs `tailscale serve`, which
terminates TLS for a tailnet name and proxies over the virtual machines'
bridge to that port. So the hub listens on every address of its machine
in plain HTTP, and anything that reaches the bridge reaches it.

On 2026-10-18 the user decided:

- The hub stays on the tailnet. The web app (`/`) and the MCP endpoint
  (`/mcp`) are reachable only over Tailscale, and nothing goes on the
  public internet.
- The phone and the second machine join the tailnet to use the web app.
- Browsers sign in by their tailnet identity
  ([ADR-0027](0027-browsers-sign-in-with-their-tailnet-identity.md)),
  which notesd must learn in a way no client can forge.

Tailscale offers three ways for a Go service to be on a tailnet:

- behind `tailscale serve` on a machine that runs `tailscaled`;
- listening on a `tailscaled` node's tailnet address, and asking
  `tailscaled`'s local API who a connection's peer is (WhoIs);
- as a node of its own, with the `tsnet` library: a userspace network
  stack inside the process, with its own state, name, certificate and
  WhoIs.

notesd is pure Go (`CGO_ENABLED=0`,
[ADR-0004](0004-a-notes-daemon-with-thin-clients.md)), and so is
`tsnet`. M6 packages the hub as a container image and a systemd unit.

## Decision

- **The hub is reachable only on the tailnet.** It has no public
  endpoint: no Funnel, and no proxy to the internet. Who on the tailnet
  can reach it is the tailnet's access policy: the hub's docs say to
  grant only the owner's own devices port 443 on the hub's tag.
- **notesd joins the tailnet itself, with `tsnet`,** as a node of its
  own. `notesd --hub-tailnet NAME` makes the hub the node NAME and serves
  every hub route there: `/sync` and its blobs for devices, the browser
  routes, and `/mcp`. notesd never calls `ListenFunnel`, so no route
  reaches beyond the tailnet whatever the host's configuration.
- **TLS is the node's own, on port 443 only.** `tsnet`'s `ListenTLS`
  serves the certificate Tailscale obtains for the node's name under
  the tailnet's domain (HTTPS certificates must be on in the tailnet).
  The node listens on no other port. Browsers need HTTPS for a secure
  context, and devices already refuse a hub that is not `https://`.
- **Identity comes from the node.** For every request on the tailnet
  listener, notesd can ask its own node who the peer is
  (`LocalClient().WhoIs` on the connection's remote address). A peer the
  node cannot name is refused. ADR-0027 builds the browser sign-in on
  this; devices and agents keep their tokens.
- **The node's state lives in notesd's data directory,** in
  `tailnet/`, 0700, like the database: it holds the node's private key.
  The first start authenticates with an auth key in `TS_AUTHKEY`; the
  owner makes a tagged, single-use key, so the node belongs to no user
  and its key does not expire. Without one, notesd logs Tailscale's
  login URL and waits.
- **Tailscale's log upload is off** for the node (Tailscale's
  `TS_NO_LOGS_NO_SUPPORT`), so the hub sends Tailscale's log service
  nothing; its own logs stay in notesd's.
- **The plain listener is for loopback only.** `notesd --hub ADDR`
  stays, and accepts only a loopback address, as the development
  gateway does: tests, the simulation, the exit evidence, and a device
  on the hub's own machine use it.
- **One package imports Tailscale.** `internal/tailnet` wraps `tsnet`
  (start, listen, WhoIs, the name, shutdown), as `internal/mcpserver`
  wraps the MCP SDK; nothing else imports `tailscale.com`. Its tests run
  real nodes against Tailscale's in-process test control server
  (`tstest/integration/testcontrol`), as `tsnet`'s own tests do; other
  packages' tests give the hub a fake WhoIs.
- **Devices follow the hub to its new name.** Their hub URL changes from
  the host's name to the node's. `notesctl hub relocate URL NEW-URL`
  changes a paired hub's URL on a device after checking that `NEW-URL`
  answers with the same hub ID, so the device token never goes to
  another hub, and keeps the token, the vaults' sync state and their
  cursors.

## Consequences

- The hub's exposure is a property of the binary, not of a proxy's
  configuration: it serves nothing outside the tailnet, and nothing on
  the virtual machine's own addresses.
- notesd depends on `tailscale.com` (pinned; the userspace stack and
  WireGuard come with it). The binary grows by tens of megabytes, and a
  Tailscale security fix reaches the hub only in a notesd release, so
  dependency updates watch Tailscale's bulletins.
- The tailnet gains a node, and the hub a new URL. The three trial
  devices relocate once; the host's `tailscale serve` for the hub goes.
  Nothing changes on the user's machines before M2's and M3's trials end
  (2026-10-25), and nothing without the user's go-ahead.
- `tsnet`'s userspace stack is slower than the kernel's. Notes are
  small; a 100 MB attachment takes longer, which M2's evidence can
  measure again.
- Certificates for the node's name appear in public Certificate
  Transparency logs, as they do for `tailscale serve` now; the node's
  name should say nothing private.
- The hub needs Tailscale's coordination service to start and to learn
  new peers. If it is down, existing peers keep working.
- M6's container image needs no `tailscaled` beside notesd, only its
  data directory and, once, an auth key.

## Alternatives considered

- **`tailscale serve` on the host, as during M2's trial:** its identity
  headers would cross the bridge, where any machine on it can send its
  own. Tailscale's documentation says to trust them only from a backend
  that listens on loopback.
- **Tailscale inside the hub's virtual machine, with `tailscale serve`
  proxying to notesd on loopback or a 0600 Unix socket:** sound while
  the backend is reachable only from serve, and it keeps Tailscale out
  of notesd. But notesd could not check that its deployment stays so,
  Funnel would be one command away, the headers name a user and not the
  device, and M6's image would need `tailscaled` beside notesd. The
  close second.
- **notesd on the node's tailnet address, with WhoIs from `tailscaled`'s
  local API:** notesd would trust that packets from tailnet addresses
  arrive only through Tailscale's interface, which rests on the host's
  firewall rules, and it would need operator rights on `tailscaled` to
  fetch certificates.
- **A public hub behind a reverse proxy:** rejected by the user; it
  would need passwords or passkeys, rate limits and the hardening the
  roadmap's risk 3 lists.

## References

- Shapes: [design/hub.md](../design/hub.md), [design/overview.md](../design/overview.md), [design/sync.md](../design/sync.md#pairing-and-sessions), [specs/hub-http.md](../specs/hub-http.md), [plan 0006](../plans/0006-m4-web-app.md)
- Builds on: [ADR-0004](0004-a-notes-daemon-with-thin-clients.md), [ADR-0005](0005-sync-through-a-hub-with-revisions-and-three-way-merges.md), [ADR-0016](0016-pair-devices-with-a-one-time-code-and-a-revocable-token.md) (TLS left to M4)
- Related: [ADR-0027](0027-browsers-sign-in-with-their-tailnet-identity.md), [ADR-0028](0028-remote-mcp-serves-local-agents-with-scoped-tokens.md)
