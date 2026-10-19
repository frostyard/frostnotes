# Plan 0006: M4 web app and remote MCP

M4 brings the notes to every machine and to the user's phone without a
local copy:

- the hub becomes a node of the user's tailnet and stays there;
- browsers on the user's own tailnet devices sign in by who they are on
  it, and run the desktop app's UI, which narrows to Obsidian's phone
  layout on a phone;
- Claude Code and other local agents reach the hub's MCP endpoint with a
  token scoped to one vault and to read or write.

It follows [M3](0005-m3-desktop-app.md) and comes before M5's graph and
queries in the [roadmap](0002-roadmap.md). Work is split per
[ADR-0010](../adr/0010-claude-plans-and-codex-runs-task-cards.md):

- **The planner owns** the ADRs and specs, the tailnet node
  (`internal/tailnet`), the hub's routes, checks, sessions and tokens,
  the generator's method-level callers, `/mcp`, the hub transport, the
  narrow layout's frame, the security review and the evidence.
- **The executor owns** cards from T-0084: the store's tables and
  queries for logins and tokens, the `notesctl hub` commands, the phone
  layout's components, the Hub settings' components, the manifest and
  icons, and the editing commands the phone toolbar lacks.

Status (2026-10-18): **Phase 1 is in review.** This plan,
[ADR-0026](../adr/0026-the-hub-stays-on-the-tailnet-and-joins-it-itself.md)
to [ADR-0031](../adr/0031-a-narrow-window-takes-obsidians-phone-layout.md)
(proposed), [design/hub.md](../design/hub.md),
[specs/hub-http.md](../specs/hub-http.md), and M4's sections of
[design/app.md](../design/app.md#in-a-browser),
[specs/ui.md](../specs/ui.md#narrow-windows),
[specs/rpc-protocol.md](../specs/rpc-protocol.md#caller-kinds) and
[specs/mcp.md](../specs/mcp.md#the-hubs-endpoint). No code or card
starts before the user accepts the ADRs.

## Decisions taken for M4

The user decided three things on 2026-10-18, which are ADRs:

- **The hub stays on the tailnet, and joins it itself**
  ([ADR-0026](../adr/0026-the-hub-stays-on-the-tailnet-and-joins-it-itself.md)).
  - The web app and `/mcp` are reachable only over Tailscale; nothing is
    public.
  - notesd embeds `tsnet` and becomes a node of its own
    (`--hub-tailnet NAME`), with TLS from the node's certificate on
    port 443. It serves every hub route there and never through Funnel.
    The plain `--hub` listener is for loopback only.
  - Devices relocate to the node's name and keep their tokens.
  - It is chosen over `tailscale serve` (whose identity headers would
    cross the virtual machines' bridge, or need a deployment notesd
    cannot check) and over WhoIs from `tailscaled`'s local API.
- **Browsers sign in with their tailnet identity**
  ([ADR-0027](../adr/0027-browsers-sign-in-with-their-tailnet-identity.md)).
  - The hub asks its own node who the connection's peer is. Untagged
    nodes of logins on the hub's list may sign in.
  - A session is one `/api` WebSocket, upgraded after `Host`, `Origin`
    and fetch-metadata checks, and checked again every minute.
  - The browser holds no credential, no other browser route writes, and
    minting a credential stays on the hub's socket.
  - A method may narrow its domain's callers.
- **Remote MCP serves local agents with scoped tokens**
  ([ADR-0028](../adr/0028-remote-mcp-serves-local-agents-with-scoped-tokens.md)).
  - `/mcp` speaks Streamable HTTP to Claude Code and local agents on the
    tailnet.
  - Each token (`fnm_…`, kept hashed) names one vault and `read` or
    `write`, is issued and revoked with `notesctl hub token`, and is the
    author of its writes.
  - The tools reach the engine as a new in-process caller, `agent`.
  - claude.ai and the Claude apps move to the roadmap's later work.

Three more follow from building the web app:

- **The hub serves the web app's build from a directory**
  ([ADR-0029](../adr/0029-the-hub-serves-the-web-apps-build-from-a-directory.md)).
  `make web-build` writes `build/web/` in nsl, and `notesd --hub-web DIR`
  serves it with the strict policy and headers of
  [specs/hub-http.md](../specs/hub-http.md#headers-on-browser-routes),
  checking its protocol at start.
- **The web app installs on a home screen, and needs the hub**
  ([ADR-0030](../adr/0030-the-web-app-installs-on-a-home-screen-and-needs-the-hub.md)).
  It has a manifest and no service worker. Offline, it keeps what was
  typed and says so.
- **A narrow window takes Obsidian's phone layout**
  ([ADR-0031](../adr/0031-a-narrow-window-takes-obsidians-phone-layout.md)).
  Below 600 pixels, the window shows one tab with drawers for sidebars,
  a bottom bar and an editing toolbar on the keyboard. A coarse pointer
  gets touch sizes at any width.

These follow from the ADRs and the user's setup, so the plan records
them without ADRs:

- **Nothing changes on the user's machines before 2026-10-26, nor
  without their go-ahead.**
  - M2's trial runs through 2026-10-24, and M3's through 2026-10-25,
    both through today's hub.
  - The hub's virtual machine keeps its M2 binary and address until
    Phase 8.
  - Each change on the hub's host or its virtual machine is asked first.
  - The repository names no address, tailnet, share or vault of the
    user's.
- **The API grows within protocol 1, additively** (Phase 3):
  - method-level `callers` and the `agent` caller;
  - `session` on `note`, `vault` and `hub`, with `vault.add`,
    `vault.remove` and the minting methods narrowed to `local`;
  - the `hub` methods of [specs/hub-http.md](../specs/hub-http.md#methods-on-the-hub);
  - `rpc.ping`, and `rpc.hello`'s caller and identity;
  - `sync.relocate`, and `/sync/hub`.
- **New dependencies are the planner's,** pinned, each release at least
  two weeks old:
  - in Go, `tailscale.com` (`tsnet`, and its test control server in
    tests);
  - in nsl, Firefox ESR and geckodriver for the evidence, checksum
    verified (frostyard/core ADR-0023), driven by plain WebDriver
    calls, as spike 1 drove WebKitWebDriver;
  - no npm package: the manifest is a file, and the drawers and sheets
    are the app's own.
- **The device protocol does not change** but for `/sync/hub`. M2's
  simulation and its regressions keep running on loopback.
- **The phone is the user's.** Which phone and browser decides how Add
  to Home screen behaves; Phase 6 checks on it, and the evidence's
  narrow size is 390 × 844 until then.

## Phase 1 — Decisions (planner)

- ADRs 0026 to 0031 (proposed), [design/hub.md](../design/hub.md),
  [specs/hub-http.md](../specs/hub-http.md), M4's sections of
  [design/app.md](../design/app.md#in-a-browser),
  [design/overview.md](../design/overview.md),
  [design/sync.md](../design/sync.md#pairing-and-sessions),
  [specs/rpc-protocol.md](../specs/rpc-protocol.md),
  [specs/mcp.md](../specs/mcp.md) and [specs/ui.md](../specs/ui.md),
  this plan, and the roadmap's M4 row, top risk and open questions.
- **Done when:** the docs are indexed and cross-linked both ways, and
  the user has accepted ADRs 0026 to 0031, or said what to change,
  before any of Phase 2's code merges.

## Phase 2 — The hub on the tailnet (planner)

Implements [design/hub.md](../design/hub.md#the-tailnet-node) and
[specs/hub-http.md](../specs/hub-http.md#listeners)
([ADR-0026](../adr/0026-the-hub-stays-on-the-tailnet-and-joins-it-itself.md)).

- `internal/tailnet` over `tsnet`:
  - start a node with a host name, its state in `tailnet/` (0700), log
    upload off, and the auth key from `TS_AUTHKEY`;
  - its TLS listener on `:443`, its name, and `WhoIs` returning a
    `Peer`;
  - shutdown with notesd's context.
- `notesd --hub-tailnet NAME` serves the hub's handler on the node.
  `--hub ADDR` refuses a non-loopback address, as the development
  gateway does. `hub.status` reports the node's name.
- Tests run real nodes against Tailscale's in-process test control
  server: WhoIs names a user's node and a tagged node, and refuses an
  address it does not know, and nothing listens off the tailnet. If the
  test control server issues no certificates, the TLS listener is
  checked on the real tailnet in Phase 8.
- [specs/sync-protocol.md](../specs/sync-protocol.md#endpoints) and
  [design/sync.md](../design/sync.md) say where the hub listens now.
- **Done when:** `make check` is green with `tailscale.com` in the
  module, the binary still builds with `CGO_ENABLED=0`, and the tests
  above pass; the binary's growth is recorded here.

## Phase 3 — Contracts (planner, with cards)

Implements [specs/rpc-protocol.md](../specs/rpc-protocol.md#caller-kinds)
and [specs/hub-http.md](../specs/hub-http.md#methods-on-the-hub).

- **The generator:** a method's own `callers` in `schema/rpc`, checked
  to narrow its domain's, carried into the Go router and the TypeScript
  types, with `tools/rpcgen` tests. `api.CallerAgent` joins the caller
  kinds.
- **The schema:**
  - callers on `note`, `vault`, `hub` and `events` as
    [specs/rpc-protocol.md](../specs/rpc-protocol.md#caller-kinds)
    lists them;
  - the new `hub` methods, `rpc.ping`, `rpc.hello`'s result, `hub.status`'s
    fields and `sync.relocate`;
  - `make gen`. New methods answer `unavailable` until their phase.
- **Attribution by connection:** a session's or an agent's connection
  carries its source and author, and the engine records them in place
  of the parameters, with tests for every `note` method that records a
  version.
- **Relocation:** `GET /sync/hub` and `sync.relocate`, which never sends
  the token to a URL whose hub ID differs (planner: it moves a token).
- **The mock** (`rpc/mock/`) can answer as caller `session`: hello says
  so, and narrowed methods answer `methodNotFound`, for the app's tests.
- **Cards:**
  - the store's migration, tables and queries for allowed logins and
    MCP tokens (`internal/store`);
  - `notesctl hub users`, `allow`, `disallow`, `tokens`, `token issue`,
    `token revoke`, `sessions`, `end` and `relocate`
    (`cmd/notesctl`), each against
    [specs/hub-http.md](../specs/hub-http.md#commands).

  Each card is checked against a reference, then deleted.
- **Done when:**
  - `make check`, `make ui-check` and `make gen-check` are green with
    every stub in place;
  - a router test sends every method as each caller kind and gets
    `methodNotFound` exactly where the spec says;
  - the cards are merged.

## Phase 4 — Browser sessions and the web build's routes (planner)

Implements [design/hub.md](../design/hub.md#browsers),
[design/hub.md](../design/hub.md#the-web-apps-files) and
[specs/hub-http.md](../specs/hub-http.md#browser-checks)
([ADR-0027](../adr/0027-browsers-sign-in-with-their-tailnet-identity.md),
[ADR-0029](../adr/0029-the-hub-serves-the-web-apps-build-from-a-directory.md)).

- **The hub's router by route kind.** The device routes keep their
  checks.
- **Browser checks and headers**, in one place.
- **`/api`:**
  - the upgrade, and the 426 answer for a plain `GET`;
  - the session registry, its limits and its minute's re-check;
  - `disallow` and `endSession` closing live sessions.
- **`/blobs/`:** the dev gateway's blob handler moves where both can use
  it, with the identity check in place of the token.
- **The web build's files:** the start check, paths, caching and types.
- **The `e2e` build tag's fixed identity on loopback,** and the
  `internal/repocheck` rule that only `e2e` files define it.
- **The `hub` methods** for logins and sessions.
- **Tests:**
  - every check of the spec, failing and passing;
  - forged `Tailscale-User-*` and `X-Forwarded-For` headers change
    nothing;
  - a tagged node, an unlisted login and a share from another tailnet
    are refused;
  - a fuzz test of the files route's paths;
  - every limit at and past its value;
  - a session reads, writes (its author the node's name) and receives
    events.
- **Done when:** `make check` is green with those tests, and a
  session over `httptest` with a fake WhoIs works end to end and is
  refused by each failed check.

## Phase 5 — Remote MCP (planner)

Implements [specs/mcp.md](../specs/mcp.md#the-hubs-endpoint),
[specs/hub-http.md](../specs/hub-http.md#mcp) and
[design/hub.md](../design/hub.md#agents)
([ADR-0028](../adr/0028-remote-mcp-serves-local-agents-with-scoped-tokens.md)).

- **`internal/mcpserver`:**
  - an HTTP handler over the SDK's Streamable HTTP server, one tool
    server per MCP session built from its token;
  - `Options.Author`;
  - sessions bound to their token, 8 per token, ended after an idle
    hour;
  - no SDK type in its exported API.
- **The hub:**
  - issuing tokens (`crypto/rand`, hashed) and checking them, recording
    their last use;
  - refusing `Origin`;
  - the in-process `agent` connection;
  - a revocation that ends the token's sessions.
- **Tests,** with the SDK's own HTTP client:
  - a `read` token lists the four read tools and cannot write;
  - a `write` token edits, with the token's name as the version's
    author;
  - a token revoked mid-session gets 401 at once;
  - another token's session ID is 404, and a request with `Origin` is
    403;
  - every tool call stays in the token's vault.
- **Done when:** `make check` is green with those tests, and Claude Code
  run against a hub on loopback with a token (`claude mcp add
  --transport http … --header`) lists the tools and reads a note.

## Phase 6 — The web app (planner, with cards)

Implements [design/app.md](../design/app.md#in-a-browser),
[specs/ui.md](../specs/ui.md#in-a-browser) and
[specs/ui.md](../specs/ui.md#narrow-windows)
([ADR-0030](../adr/0030-the-web-app-installs-on-a-home-screen-and-needs-the-hub.md),
[ADR-0031](../adr/0031-a-narrow-window-takes-obsidians-phone-layout.md)).

- **The planner's part:**
  - `make web-build` (`vite build --mode web`), `frostnotes-web.json`,
    and `main.tsx`'s choice;
  - `connectHub()` with its reconnects, heartbeat and refusal reason;
  - same-origin blob URLs;
  - the app reading its caller from `rpc.hello` and hiding what a
    session may not call;
  - leaving the page with unsaved text;
  - the layout's switch at 600 pixels, the workspace's narrow view, and
    touch sizes over the tokens;
  - the Hub settings' container.
- **First, the planner reads** Obsidian 1.14.4's phone styles and code
  in the user's install, as M3 read its desktop, and refines
  [specs/ui.md](../specs/ui.md#narrow-windows)'s measures before
  writing the cards.
- **Cards,** presentational, each with given tests:
  - the view header's narrow form;
  - drawers;
  - the bottom bar;
  - the tab switcher;
  - the editing toolbar;
  - menus as bottom sheets and long presses;
  - the Hub settings' lists (`features/hub/`);
  - the editing commands the toolbar names that the registry lacks;
  - the manifest and the icons' web sizes (`scripts/render-icons.py`).
- **Done when:**
  - `make ui-check` and `make web-build` are green;
  - in Firefox, at a desktop size and at 390 × 844, against a hub on
    loopback built with `e2e`, the web build opens a note, edits it
    (the edit reaches the hub's disk), uploads an image, comes back
    after the hub restarts, and shows why a disallowed login cannot
    sign in;
  - on the user's phone, against that hub on a machine of theirs, Add
    to Home screen opens the app standalone.

## Phase 7 — Security review (planner, an independent agent)

Checks [specs/hub-http.md](../specs/hub-http.md) and ADRs 0026 to 0029
against the code.

- An agent that did not write the code reviews the hub's surface,
  read-only:
  - identity: WhoIs alone; tagged, unknown and unlisted peers;
    re-checks;
  - each browser check;
  - sessions: limits, ending, what survives a restart;
  - forged requests: no writes outside the WebSocket; cross-site
    WebSocket hijacking; DNS rebinding;
  - the content security policy and the headers;
  - the files route: paths and links;
  - `/blobs/`;
  - MCP tokens: form, storage, comparison, revocation, session binding,
    `Origin`;
  - what `session` and `agent` can call, by trying every method;
  - attribution;
  - logs that might hold a secret;
  - limits against floods;
  - the `e2e` tag kept out of release builds;
  - the web app's XSS surfaces in Firefox and the phone's browser (both
    sanitizers, as M3's review read them);
  - `tsnet`'s settings: no Funnel, log upload off, the state's mode.
- Each finding is fixed with a test, as in
  [M3's review](0005-m3-desktop-app.md#phase-7--review-and-exit-evidence).
- **Done when:** the review's report lists no open XSS, CSRF or session
  finding, and every other finding is fixed with a test or accepted by
  the user in writing.

## Phase 8 — The user's hub and exit evidence

From 2026-10-26, and each step on the user's machines with their
go-ahead first. Checks [design/hub.md](../design/hub.md#operational-notes)
against the real tailnet.

- **Moving the hub** (the user's hands, the planner's steps):
  - a tagged auth key, and a grant in the tailnet's policy from the
    user's devices to the hub's port 443;
  - the hub's virtual machine runs `notesd --hub-tailnet NAME --hub-web
    DIR`, and the owner allows their own login;
  - framework, the laptop and the minideb VM relocate, and the old
    `tailscale serve` for the hub goes;
  - the phone and the second machine join the tailnet.
- **Done when**, each shown by a test or a recorded run:
  1. **The same UI on a second machine and on the phone.** The web app
     works in Firefox on a second machine and on the user's phone
     against the hub, on the notes the desktop app writes. The user
     opens, searches, creates and edits notes in both, sees the
     desktop's edits arrive and theirs reach the desktop, and opens the
     app standalone from the phone's home screen. `make m4-evidence`
     drives the same in Firefox at both sizes, against a hub on
     loopback.
  2. **A phone edit shows on the desktop within 2 s,** from the last
     keystroke in the web app to the text in the desktop app's editor,
     at p95 over 20 edits. The editor's 1 s save timer is inside the
     budget.
     - `make m4-evidence` measures it on loopback: the web app in
       Firefox at phone size, a hub, and a device notesd with the
       desktop app under Xvfb.
     - On the tailnet, an evidence client on framework, signed in as
       the user, writes as the web app does, and the time to
       framework's `note.changed` is recorded.
     - The user watches a phone edit arrive.
  3. **Claude Code uses the hub's `/mcp` with a scoped token.**
     - On a machine with no local copy of the vault, with a `write`
       token, Claude Code searches, reads and appends to today's daily
       note, whose history names the token.
     - With a `read` token, its tool list is the four read tools.
     - After `notesctl hub token revoke`, its next call fails.
  4. **The review finds no open issue:** no XSS, CSRF or session
     issue (Phase 7).
- `make m4-evidence` records evidence 1 and 2's loopback runs; the rest
  are the user's runs and the planner's notes, recorded here.

## Later / ideas

- claude.ai, Claude Desktop's connectors and the Claude apps as MCP
  clients: a public endpoint and OAuth 2.1
  ([ADR-0028](../adr/0028-remote-mcp-serves-local-agents-with-scoped-tokens.md)).
- Approving pairings and issuing tokens from the web app, with a
  confirmation that a script in the page cannot give, if the user wants
  them.
- Reading notes offline on the phone: a mobile app with its own notesd
  (the roadmap's Tauri mobile).
- The hub's flagged files (conflicts) in the web app, which today shows
  them on devices.
- Tailscale's app capabilities (grants) in place of the hub's list of
  logins, if the tailnet's policy should say who may sign in.
- Per-vault grants, for more than one person on a hub.

## Open questions

- **The phone and its browser:** Firefox for Android, Chrome or Safari
  on iOS changes how the page installs and the size the evidence
  drives. By Phase 6.
- **The hub's node name and tag:** the user's choice, kept out of the
  repository; the name appears in public certificate logs. By Phase 8.
- **Who else is on the tailnet:** whether other people's devices share
  it decides the policy's grant and the login list's first entries. By
  Phase 8.

## References

- Implements: [design/hub.md](../design/hub.md), [design/app.md](../design/app.md#in-a-browser), [design/overview.md](../design/overview.md), [specs/hub-http.md](../specs/hub-http.md), [specs/rpc-protocol.md](../specs/rpc-protocol.md), [specs/mcp.md](../specs/mcp.md#the-hubs-endpoint), [specs/ui.md](../specs/ui.md#narrow-windows), [specs/sync-protocol.md](../specs/sync-protocol.md) (`/sync/hub`, relocation)
- Decisions: [ADR-0026](../adr/0026-the-hub-stays-on-the-tailnet-and-joins-it-itself.md), [ADR-0027](../adr/0027-browsers-sign-in-with-their-tailnet-identity.md), [ADR-0028](../adr/0028-remote-mcp-serves-local-agents-with-scoped-tokens.md), [ADR-0029](../adr/0029-the-hub-serves-the-web-apps-build-from-a-directory.md), [ADR-0030](../adr/0030-the-web-app-installs-on-a-home-screen-and-needs-the-hub.md), [ADR-0031](../adr/0031-a-narrow-window-takes-obsidians-phone-layout.md); [ADR-0004](../adr/0004-a-notes-daemon-with-thin-clients.md), [ADR-0012](../adr/0012-agents-reach-notes-through-mcp.md), [ADR-0016](../adr/0016-pair-devices-with-a-one-time-code-and-a-revocable-token.md)
- Builds on: [plan 0003](0003-m1-headless-vault.md) (MCP), [plan 0004](0004-m2-sync.md) (the hub, devices), [plan 0005](0005-m3-desktop-app.md) (the app, its evidence harness)
- Workflow: [design/agent-workflow.md](../design/agent-workflow.md)
- Roadmap: [plan 0002](0002-roadmap.md)
