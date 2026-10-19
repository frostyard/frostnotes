# Plan 0006: M4 notes inside lodge

M4 brings the notes to every machine and to the user's phone, and to
agents on the tailnet, as an app of lodge, frostyard's tailnet-only web
shell ([frostyard/lodge](https://github.com/frostyard/lodge);
[core ADR-0057](https://github.com/frostyard/core/blob/main/docs/adr/0057-run-first-party-apps-behind-lodge.md)):

- lodge signs browsers in, issues agents' tokens, serves notes' web
  build and frames it beside the other apps;
- notesd, on lodge's machine, becomes lodge's `notes` app: an HTTP
  server on a Unix socket that only lodge may open, taking its callers
  from lodge's headers
  ([ADR-0026](../adr/0026-notesd-runs-as-a-lodge-app.md));
- the hub's devices move behind lodge with their tokens;
- the UI narrows to Obsidian's phone layout on a phone
  ([ADR-0027](../adr/0027-a-narrow-window-takes-obsidians-phone-layout.md)).

It follows [M3](0005-m3-desktop-app.md) and comes before M5's graph and
queries in the [roadmap](0002-roadmap.md). It depends on
[lodge's roadmap](https://github.com/frostyard/lodge/blob/main/docs/plans/0001-roadmap.md):

- **lodge's phase 1, the kit:** `lodge/kit` (the socket, the peer check,
  the headers) and `@frostyard/lodge-client`. Phases 3 to 5 need it.
- **lodge's phase 2, lodge v1 with notes:** the node, sign-in, tokens,
  the shell and lodge-content. Its development build runs Phases 4 and
  5 on loopback, and its deployment is Phase 7's.

Phases 1 and 2 need nothing from lodge, and can start once the ADRs are
accepted.

Work is split per
[ADR-0010](../adr/0010-claude-plans-and-codex-runs-task-cards.md):

- **The planner owns** the ADRs and specs, the generator's method-level
  callers and `writes` mark, the engine's scope, attribution by
  connection, relocation, `internal/lodgeapp`, `/mcp`, the lodge
  transport and lodge-client wiring, the narrow layout's frame, the
  security review and the evidence.
- **The executor owns** cards from T-0084: the `notesctl hub url` and
  `relocate` commands, the phone layout's components, the Hub settings'
  lists, and the editing commands the phone toolbar lacks.

Status (2026-10-18): **Phase 1 is in review,** with frostyard/core
[ADR-0057](https://github.com/frostyard/core/blob/main/docs/adr/0057-run-first-party-apps-behind-lodge.md) and lodge's first docs, all three to be published together.
M4's first drafts (a notesd of its own on the tailnet, with its own
sign-in, tokens and web serving) became lodge's ADRs 0004 to 0008
before anyone accepted them. No code or card merges before the user
accepts the ADRs. On 2026-10-18 the user asked for the build to go
ahead overnight as stacked PRs for review: Phases 2, 3, 4 and 5 are in
review on branches `m4/phase2` to `m4/phase5`, each on the one before:
agents through `/mcp`, then the web app at desktop widths first (the
narrow layout and its cards follow).

## Decisions taken for M4

Two ADRs here:

- **notesd runs as a lodge app**
  ([ADR-0026](../adr/0026-notesd-runs-as-a-lodge-app.md)).
  - `notesd --lodge` serves lodge's app socket as `frostnotes`, and no
    network address.
  - Callers come from `Lodge-*` headers, trusted because only lodge's
    user can connect, and each path admits one caller kind: `/api`
    session, `/mcp` agent, `/content/` content, `/device/` device.
  - What stays notes' own:
    - which methods each caller kind may call, now per method, with a
      `writes` mark;
    - the agent's scope, `vault=<id>[;write]`, enforced by the engine;
    - attribution by connection;
    - the JSON-RPC, blobs and the device protocol.
  - Devices move to lodge's `/device/notes/` with their tokens, by a
    URL the hub records, or by a relocation proof bound to that URL.
- **A narrow window takes Obsidian's phone layout**
  ([ADR-0027](../adr/0027-a-narrow-window-takes-obsidians-phone-layout.md)):
  - below 600 pixels of frame width, one tab with drawers for
    sidebars, a bottom bar and an editing toolbar on the keyboard;
  - a coarse pointer gets touch sizes at any width.

lodge decides the rest, in its own ADRs and specs:

- the tailnet node and sign-in, by allowed devices and logins;
- sessions and their liveness;
- MCP tokens, `lodgectl token`;
- serving the web build, and the home screen;
- frames and lodge-client;
- lodge-content.

These are linked from [ADR-0026](../adr/0026-notesd-runs-as-a-lodge-app.md#references).

These follow from the ADRs and the user's setup, so the plan records
them without ADRs:

- **Nothing changes on the user's machines before 2026-10-26, nor
  without their go-ahead.**
  - M2's trial runs through 2026-10-24, and M3's through 2026-10-25,
    both through today's hub.
  - The hub keeps its M2 binary and address until Phase 7.
  - Each change on the hub's host or its virtual machine is asked first.
  - Every run before Phase 7 uses loopback and lodge's development
    build, never a node on the user's tailnet. So no phase but the last
    touches the user's machines, and the phone's first look at notes is
    Phase 7's.
  - *One exception, at the user's go-ahead (2026-10-19):* an overnight
    test deployment put lodge and notes' notesd, as `frostnotes`, on the
    hub's virtual machine with a test vault of its own, beside the M2
    trial hub, which nothing touched; Phase 4's live Claude Code run
    used it. Phase 7's move script finds that deployment and sets its
    data aside, never merging it into the hub's.
  - M4's docs name no address, host, tailnet, device, share or vault of
    the user's.
- **The API grows within protocol 1, additively** (Phase 2):
  - method-level `callers` and the `writes` mark;
  - the `agent` caller and its scope;
  - `session` on `note`, `vault` and `hub`, with `vault.add`,
    `vault.remove`, `hub.serve`, `hub.approve` and `hub.setURL` narrowed
    to `local`;
  - `hub.setURL`, `hub.status`'s `url`, `log.hub` and `sync.relocate`;
  - `rpc.ping`, and `rpc.hello`'s caller and identity.
- **New dependencies are the planner's,** pinned, each release at least
  two weeks old where it has releases:
  - in Go, `lodge/kit`, a nested module free of `tailscale.com`, pinned
    to a published lodge commit or release, never a local path;
  - in the app, `@frostyard/lodge-client`, pinned the same way;
  - in nsl, Firefox ESR and geckodriver for the evidence, checksum
    verified (frostyard/core ADR-0023) and driven by plain WebDriver
    calls, as spike 1 drove WebKitWebDriver.
- **The device protocol keeps its paths** under the new `/device`
  prefix. Only `/sync/hub` and `log.hub` are new. M2's simulation and
  its regressions run unchanged on loopback.
- **The phone is the user's.** Which phone and browser decides how
  lodge's home-screen page behaves. Until Phase 7 the evidence's narrow
  size is 390 × 844.

## Phase 1 — Decisions (planner)

- **This phase's documents:**
  - ADR-0026 and ADR-0027 (proposed);
  - [specs/lodge-app.md](../specs/lodge-app.md) and
    [design/hub.md](../design/hub.md);
  - M4's sections of [design/overview.md](../design/overview.md#notesd-as-a-lodge-app),
    [design/app.md](../design/app.md#in-a-browser),
    [design/sync.md](../design/sync.md#pairing-and-sessions),
    [specs/rpc-protocol.md](../specs/rpc-protocol.md#caller-kinds),
    [specs/mcp.md](../specs/mcp.md#through-lodge) and
    [specs/ui.md](../specs/ui.md);
  - this plan and the roadmap's M4 row.
- **Reviewed together** with frostyard/core [ADR-0057](https://github.com/frostyard/core/blob/main/docs/adr/0057-run-first-party-apps-behind-lodge.md) and lodge's ADRs
  and specs, and published with them.
- **Done when:** the docs are indexed and cross-linked both ways, and
  the user has accepted ADR-0026 and ADR-0027, with the platform's ADRs,
  or said what to change, before any of Phase 2's code merges.

## Phase 2 — Contracts, scope and relocation (planner, with cards)

Implements [specs/rpc-protocol.md](../specs/rpc-protocol.md#caller-kinds),
[specs/lodge-app.md](../specs/lodge-app.md#methods-and-callers) and
[specs/lodge-app.md](../specs/lodge-app.md#device). It needs nothing
from lodge.

- **The generator:**
  - a method's own `callers`, checked to narrow its domain's;
  - a method's `writes` mark;
  - both carried into the Go router and the TypeScript types, with
    `tools/rpcgen` tests;
  - `api.CallerAgent` joins the caller kinds.
- **The schema:**
  - callers and `writes` marks as
    [specs/lodge-app.md](../specs/lodge-app.md#methods-and-callers)
    lists them;
  - `hub.setURL`, `hub.status`'s `url`, `log.hub` and `sync.relocate`;
  - `rpc.ping`, and `rpc.hello`'s result;
  - `make gen`. New methods answer `unavailable` until their phase.
- **An agent's scope in the engine:** a connection may carry a scope.
  A `note` call that names another vault answers `notFound`, and a
  method marked `writes` answers `methodNotFound` under a read scope.
  Tests call every `note` method both ways.
- **Attribution by connection:** a session's or an agent's connection
  carries its source and author, and the engine records them in place
  of the parameters. Tests cover every `note` method that records a
  version.
- **Relocation** (planner: it moves tokens), on today's loopback hub:
  - the hub's recorded URL, and devices moving to it through `log.hub`;
  - `/sync/hub` and its proof;
  - `sync.relocate`, with tests of a relayed proof, a wrong hub, a URL
    that differs and a forged HMAC.
- **The mock** (`rpc/mock/`) can answer as caller `session`, for the
  app's tests.
- **Cards:** `notesctl hub url` and `notesctl hub relocate`
  (`cmd/notesctl`), against
  [specs/lodge-app.md](../specs/lodge-app.md#commands), each checked
  against a reference that is then deleted.
- **Done when:**
  - `make check`, `make ui-check` and `make gen-check` are green;
  - a router test sends every method as each caller kind and gets
    `methodNotFound` exactly where the spec says;
  - the scope and relocation tests pass;
  - the cards are merged.

## Phase 3 — notesd on the lodge socket (planner)

Implements [specs/lodge-app.md](../specs/lodge-app.md#listeners) and
[design/hub.md](../design/hub.md#device-routes-through-lodge). Needs
lodge's phase 1 (the kit).

- **`internal/lodgeapp` over `lodge/kit`:**
  - the peer check and the caller headers;
  - each path's caller kind, and `/health`;
  - `/api` as caller `session`;
  - `/content/blobs/`, the dev gateway's blob handler shared;
  - `/device/` mounting `internal/hub`'s handler.
- **The flags:** `notesd --lodge`, and `--hub` refusing a non-loopback
  address. The user's hub keeps its M2 binary until Phase 7.
- **`packaging/`:**
  - the manifest, `lodge/app.json`;
  - a systemd unit and a `sysusers.d` entry for `frostnotes`, with its
    runtime and state directories.

  M6 packages them.
- **Tests over a real Unix socket:**
  - another uid is refused;
  - a missing caller, or the wrong caller for a path, is 403;
  - display headers change nothing;
  - a session's writes name its node;
  - blob headers and limits hold;
  - the device routes work under `/device/` exactly as on loopback;
  - every limit is tested at and past its value.
- **Done when:**
  - `make check` is green;
  - a session over the socket reads, writes and receives events;
  - a device pairs, pushes and pulls through `/device/`;
  - `make sync-sim` passes.

## Phase 4 — Agents through lodge (planner)

Implements [specs/mcp.md](../specs/mcp.md#through-lodge) and
[specs/lodge-app.md](../specs/lodge-app.md#mcp). Needs lodge's phase 1.
Its live run needs lodge's phase 2's development build.

- **`internal/mcpserver`:**
  - an HTTP handler over the SDK's Streamable HTTP server, one tool
    server per MCP session, built from `Lodge-Scope`;
  - `Options.Author`;
  - sessions bound to their token's name and scope, 8 per token, the
    least recently used evicted, ended after an idle hour;
  - no SDK type in its exported API.
- **The in-process `agent` connection,** carrying the scope to the
  engine.
- **Tests,** with the SDK's own HTTP client over the socket and lodge's
  headers:
  - a read scope lists the four read tools;
  - a write sent straight down the agent connection is
    `methodNotFound`;
  - another vault is `notFound`;
  - a write scope's version names the token;
  - another token's session ID is 404;
  - a ninth session evicts the least recently used.
- **Done when:** `make check` is green with those tests, and Claude
  Code, through lodge's development build on loopback with a lodge
  token, lists the tools and reads a note. The run also records whether
  Claude Code sends an `Origin`, for lodge (review 10).

## Phase 5 — The web app inside lodge (planner, with cards)

Implements [design/app.md](../design/app.md#in-a-browser),
[specs/ui.md](../specs/ui.md#in-a-browser) and
[specs/ui.md](../specs/ui.md#narrow-windows)
([ADR-0027](../adr/0027-a-narrow-window-takes-obsidians-phone-layout.md)).
Needs lodge's phase 1 (lodge-client) and phase 2's development build.

- **The planner's part:**
  - `make web-build` with the base `/app/notes/`, and `main.tsx`'s
    choice;
  - startup: lodge-client's `connectLodge({app: "notes"})` handshake
    (`ready`, then the shell's `init` and `visible`), then notes' own
    `connectApi()` WebSocket transport in `rpc/transport/ws.ts`;
  - lodge-client's other messages: `route`, `context`, `open`,
    `shortcut`, `navigate`, `visible`, `theme`;
  - canonical routes, `contentURL` blobs, and PDFs in a new tab, with
    the download URL as the fallback;
  - the app hiding what a session may not call;
  - leaving the page with unsaved text;
  - `viewport-fit=cover`;
  - the layout's switch at 600 pixels, the workspace's narrow view,
    and touch sizes over the tokens;
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
  - the Hub settings' lists of devices and pending pairings
    (`features/hub/`);
  - the editing commands the toolbar names that the registry lacks;
  - `icon.svg` for lodge's rail.
- **Done when:**
  - `make ui-check` and `make web-build` are green;
  - in Firefox, inside lodge's development build on loopback, at a
    desktop size and at 390 × 844, the web build:
    - opens a note from its canonical URL;
    - edits it, and the edit reaches the hub's disk;
    - uploads an image and shows it from lodge-content;
    - comes back after notesd restarts;
    - follows a lodge link through `open`;
    - lets the switcher's key reach the shell from inside the frame.

## Phase 6 — Security review (planner, an independent agent)

Checks [specs/lodge-app.md](../specs/lodge-app.md) and
[ADR-0026](../adr/0026-notesd-runs-as-a-lodge-app.md) against the code.
lodge's own review of sign-in, sessions, tokens and headers belongs to
lodge's phase 2.

- An agent that did not write the code reviews notes' surface,
  read-only:
  - the socket's peer check, and the headers' trust;
  - each path's caller;
  - what `session` and `agent` can call, by trying every method;
  - the engine's scope;
  - attribution;
  - `/content/` headers;
  - the device routes under `/device/`, and relocation (a relay, a
    forged proof, a wrong URL), with display headers never deciding
    anything;
  - the web app's XSS surfaces in Firefox on loopback, and in the
    phone's browser at Phase 7 (both sanitizers, as M3's review read
    them);
  - lodge-client's message handling: origins, and no message that
    navigates the frame out of lodge;
  - logs that might hold a secret;
  - limits.
- Each finding is fixed with a test, as in
  [M3's review](0005-m3-desktop-app.md#phase-7--review-and-exit-evidence).
- **Done when:** neither notes' review nor lodge's lists an open XSS,
  CSRF or session finding, and every other finding is fixed with a test
  or accepted by the user in writing.

## Phase 7 — The user's hub in lodge, and exit evidence

Needs lodge's phase 2 deployed on the hub's machine. From 2026-10-26,
and each step on the user's machines with their go-ahead first. Checks
[design/hub.md](../design/hub.md#operational-notes) against the real
tailnet.

- **Moving the hub** (the user's hands, the planner's steps):
  - lodge v1 on the hub's machine (lodge's plan);
  - the user `frostnotes` on the hub's virtual machine: the system user
    of the `sysusers.d` entry, or the dedicated user that runs notesd
    there now, renamed, since it runs nothing else;
  - the vault's network mount (CIFS) with `uid=` and `gid=` set to
    `frostnotes`. Its files' local owner comes from those options, and
    the file server sees every write as the mount's one credential, so
    nothing changes on the file server;
  - the hub's data directory moved, whole, to `/var/lib/frostnotes` on
    the virtual machine's local disk, so its hub ID, log and blobs stay
    and SQLite never lives on the share;
  - a check before any device moves: a scratch note that notesd,
    running as `frostnotes`, writes appears on the share, read from
    another of the share's clients, and is then deleted;
  - `notesd --lodge` as `frostnotes` (`packaging/systemd/`), then
    `notesctl hub url https://lodge.<tailnet>.ts.net/device/notes`. The
    M2 hub's own unit is stopped and disabled first: both use
    `/run/frostnotes` for the private socket, and only one notesd owns
    the hub's data directory;
  - the trial's devices move on their own, and the old `tailscale
    serve` for the hub goes once `notesctl hub devices` shows all of
    them seen;
  - the steps from stopping the M2 unit to `notesctl hub url` are one command,
    [`scripts/move-hub-behind-lodge.sh`](../../scripts/move-hub-behind-lodge.sh),
    run as root on the hub's machine
    ([design/hub.md](../design/hub.md#operational-notes)). It takes the
    `sysusers.d` user, prints what it finds and would do, changes
    nothing without `--apply`, refuses a state it cannot read
    unambiguously, keeps a backup of what it moves or edits, and prints
    the command that rolls it back. A test deployment's data in
    `/var/lib/frostnotes` goes aside, never merged into the hub's;
  - the phone and the second machine allowed in lodge.
- **Done when**, each shown by a test or a recorded run:
  1. **The same UI on a second machine and on the phone.** Notes
     works in Firefox on a second machine and on the user's phone,
     through lodge, on the notes the desktop app writes. The user opens,
     searches, creates and edits notes in both, sees the desktop's edits
     arrive and theirs reach the desktop, and opens lodge from the
     phone's home screen and switches to notes. `make m4-evidence`
     drives the same in Firefox at both sizes, inside lodge's
     development build on loopback.
  2. **A phone edit shows on the desktop within 2 s,** from the last
     keystroke in the web app to the text in the desktop app's editor,
     at p95 over 20 edits. The editor's 1 s save timer is inside the
     budget.
     - `make m4-evidence` measures it on loopback: Firefox at phone
       size in lodge's development build, notesd as the hub, and a
       device notesd with the desktop app under Xvfb.
     - On the tailnet, an evidence client on the desktop, signed in
       through lodge as the user, writes as the web app does, and the
       time to the desktop's `note.changed` is recorded.
     - The user watches a phone edit arrive.
  3. **Claude Code uses `/mcp/notes` with a scoped token.**
     - On a machine with no local copy of the vault, with
       `--scope 'vault=<id>;write'`, Claude Code searches, reads and
       appends to today's daily note, whose history names the token.
     - With `vault=<id>`, its tool list is the four read tools.
     - After `lodgectl token revoke`, its next call fails.
  4. **No open issue in the reviews:** Phase 6's reviews find no open
     XSS, CSRF or session issue.
- The trial's devices sync through lodge for 7 days after the move
  with no lost edit, as M2's rule asks.
- `make m4-evidence` records evidence 1 and 2's loopback runs; the rest
  are the user's runs and the planner's notes, recorded here.

## Later / ideas

- claude.ai, Claude Desktop's connectors and the Claude apps as MCP
  clients, which need a public endpoint and OAuth 2.1: lodge's later
  work.
- Approving pairings from the web app, with a confirmation that a
  script in the page cannot give, if the user wants it.
- Reading notes offline on the phone: a mobile app with its own notesd
  (the roadmap's Tauri mobile).
- The hub's flagged files (conflicts) in the web app, which today shows
  them on devices.
- The desktop app as a lodge client too (the platform's open question);
  in M4 it stays local-first against its own notesd.

## Open questions

- **The phone and its browser:** Firefox for Android, Chrome or Safari
  on iOS changes how lodge's page installs, and the size the evidence
  drives. By Phase 7.
- **Chrome's PDF viewer under lodge-content's sandbox** (the platform
  review's L12): does Chrome show a PDF that lodge-content serves with
  `sandbox`? The docs say the download URL serves where a browser will
  not show a file in a sandboxed page, but a file's view offers both
  Open in new tab and Download for every file the app does not draw
  itself. The code stays as it is until a check in Chrome says which.
- *Answered 2026-10-18:* **the vault's mount for `frostnotes`.** The
  mount's `uid=` and `gid=` give the user the files, with no change on
  the file server; Phase 7 sets them.

## References

- Implements: [specs/lodge-app.md](../specs/lodge-app.md), [design/hub.md](../design/hub.md), [design/overview.md](../design/overview.md#notesd-as-a-lodge-app), [design/app.md](../design/app.md#in-a-browser), [specs/rpc-protocol.md](../specs/rpc-protocol.md), [specs/mcp.md](../specs/mcp.md#through-lodge), [specs/ui.md](../specs/ui.md#narrow-windows), [specs/sync-protocol.md](../specs/sync-protocol.md) (the device routes under `/device/`)
- Decisions: [ADR-0026](../adr/0026-notesd-runs-as-a-lodge-app.md), [ADR-0027](../adr/0027-a-narrow-window-takes-obsidians-phone-layout.md); [ADR-0004](../adr/0004-a-notes-daemon-with-thin-clients.md), [ADR-0012](../adr/0012-agents-reach-notes-through-mcp.md), [ADR-0016](../adr/0016-pair-devices-with-a-one-time-code-and-a-revocable-token.md); frostyard/core [ADR-0057](https://github.com/frostyard/core/blob/main/docs/adr/0057-run-first-party-apps-behind-lodge.md)
- lodge: [roadmap](https://github.com/frostyard/lodge/blob/main/docs/plans/0001-roadmap.md), [app contract](https://github.com/frostyard/lodge/blob/main/docs/specs/app-contract.md), [lodge-client](https://github.com/frostyard/lodge/blob/main/docs/specs/lodge-client.md)
- Builds on: [plan 0003](0003-m1-headless-vault.md) (MCP), [plan 0004](0004-m2-sync.md) (the hub, devices), [plan 0005](0005-m3-desktop-app.md) (the app, its evidence harness)
- Workflow: [design/agent-workflow.md](../design/agent-workflow.md)
- Roadmap: [plan 0002](0002-roadmap.md)
