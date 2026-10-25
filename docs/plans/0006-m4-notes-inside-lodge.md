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

Status (2026-10-21):
- **Phase 1 is done.** M4's first drafts became lodge's ADRs 0004 to
  0008. The user accepted ADR-0026 and ADR-0027, with lodge's ADRs, on
  2026-10-21.
- **Phases 2 to 5 merged on 2026-10-19 (#45–#56).** The user asked on
  2026-10-18 for them to be built ahead of that acceptance, as PRs for
  review.
  - Phase 4's live run with Claude Code is still to come.
  - Phase 5's web app and phone components are built, by the planner
    rather than on cards. Some of ui.md's narrow and browser items are
    still to come, and so is `make m4-evidence`.
- **Phase 6.** The review ran on 2026-10-19, its notes findings are
  fixed, and its record is below. The user accepted the two findings left
  open in writing on 2026-10-21. Phase 6 still waits on XSS on the phone
  and Chrome's PDF viewer (Phase 7).
- **Phase 7.** The move script is merged (#55) and has not run. Phase 7
  starts on 2026-10-26 in the morning, with the user's go-ahead
  (2026-10-25). The script's dry run on 2026-10-25 found 2 problems; the
  script now handles both (#92), and the dry run passes (see
  [Phase 7](#phase-7--the-users-hub-in-lodge-and-exit-evidence)). The
  [run sheet](#run-sheet-for-the-move-2026-10-26) gives the morning's
  order.

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
  - *A second use, reported by the user on 2026-10-21:* on 2026-10-20
    the user tried Porter's daily brief through lodge and opened it in the
    web app on their phone
    ([ADR-0029](../adr/0029-inside-lodge-a-rooted-link-no-file-answers-opens-through-lodge.md)).
    The user reports that this was probably the M2 trial's vault, not the
    test vault, and is not sure. If so, Porter wrote into the trial's vault,
    and the phone looked at notes before Phase 7, both against the rule
    above. The user reports that sync is working well, and the M2 trial
    ([plan 0004](0004-m2-sync.md)) is not restarted. *Observed
    2026-10-25:* lodge's notesd on the hub's VM was paired with the trial
    hub as a third device on 2026-10-19 and syncs the trial's vault, so
    Porter's, Read's and the web app's writes went into it
    ([plan 0004, Phase 7](0004-m2-sync.md#phase-7--review-and-exit-evidence)).
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
  - ADR-0026 and ADR-0027 (accepted 2026-10-21);
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
- **Result (2026-10-21):** done, out of order. On 2026-10-18 the user
  asked for Phases 2 to 5 to be built before acceptance, as PRs for
  review; they merged on 2026-10-19, and the user accepted on 2026-10-21.

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
- **Result (2026-10-19): Claude Code through lodge, live.** Claude Code
  2.1.300 (`claude -p` with a temporary `--strict-mcp-config`: an HTTP
  server and an `Authorization: Bearer` header) ran on an allowed node
  over the tailnet against LODGE's `/mcp/notes` and `/mcp/mail`, with
  lodge 23c9f80, notesd af6fe45 and maild d73ad39. Its tokens were scoped
  `vault=1` (the test vault, read only) and `account=*`; both were
  revoked after, and the next request with each was 401.
  - **Origin (review 10): Claude Code sends none.** A local recorder saw
    `accept`, `accept-encoding`, `authorization`, `connection`,
    `content-length`, `content-type`, `host` and `user-agent` on every
    POST and GET, and no `Origin` or `Sec-Fetch-*`. Through lodge, which
    refuses any `Origin` on `/mcp`, every request was 200 or 202. Claude
    Code first sends a `server/discover` probe (MCP protocol
    2026-07-28); the kit answers 400 and Claude Code falls back to
    `initialize` at 2025-11-25. Nothing breaks; the kit does not speak
    the newer discovery yet.
  - **A session lost to eviction (review 11): Claude Code recovers on
    its own,** for notes and mail alike. After `list_notes`, 8 more
    sessions on the same token, each holding a GET stream, pushed past
    the cap; Claude Code's next `list_notes` met 404 on its stream and
    its POST, re-initialized, retried, and the model saw only the
    result. The kit evicts idle sessions first, and Claude Code holds
    its stream open, so a connected Claude Code is evicted only when 8
    other sessions on its token are active. While it recovers it holds
    two sessions for a moment, within the cap.

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
  - a rooted link no vault file answers, followed through `open`
    rather than made a note
    ([ADR-0029](../adr/0029-inside-lodge-a-rooted-link-no-file-answers-opens-through-lodge.md); added 2026-10-20, after the
    links in one of Porter's daily briefs made empty notes);
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
  writing the cards. *Read on 2026-10-19, after the cards (the overnight
  build wrote the narrow layout first, from the spec's structure):* the
  styles confirm the spec's 44 and 52 touch sizes, the drawers' 84% up to
  450, the 52-high bottom bar and editing toolbar, and notices at the
  top; a phone's view header is 44, not 52, and the narrow header now is.
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
- **Recorded (2026-10-19, `make m4-evidence`, `internal/webevidence`):**
  all of the above holds at 1280 × 800 and at 390 × 844 (notes' frame
  1232 and 390 wide), with Firefox ESR 153.4.0esr through geckodriver
  0.37.1 (dev/nsl/provision.sh, pinned and checksum-verified) and lodge's
  development build at c48d43a (`make lodge-e2e`):
  - the note from its canonical URL, an edit on the hub's disk, an
    outside edit in the editor, the quick switcher and Create new note;
  - an image put into the note, uploaded with `note.upload` and drawn
    from lodge-content (Chromium's loopback CORP refusal, lodge's to
    look at, does not happen in Firefox);
  - a link to the shell's origin followed through `open` back into
    notes, and Ctrl+Backquote opening lodge's switcher from the frame;
  - an edit after notesd restarts.

  How the run differs from a person's:
  - Firefox makes no window narrower than 500 CSS pixels, so the phone's
    width is a 509-wide window at a full zoom of 60/46, which reaches
    notes' frame as it does the shell's;
  - the image goes in by a made drop event, since a made
    `ClipboardEvent` keeps no file in Firefox; the editor takes a drop
    and a paste the same way;
  - a narrow window's bottom bar shows once no editor has the focus, as
    after a tap elsewhere.

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
- **Result (2026-10-19): the reviews of notes' surface.** Independent
  agents reviewed each of Phases 2 to 4 as it was built (on PRs #45, #46
  and #48), and the platform's review of lodge, notes and mail together
  (M1 to N8) read notes' side. Notes' findings:

  | Finding | Fix | Test |
  | --- | --- | --- |
  | Phase 2 (high) a device followed `log.hub` to any URL its hub recorded, and would give its token to whoever answered there | `follow` checks the host at the new URL with the relocation proof first, as `sync.relocate` does | `TestADeviceFollowsOnlyAProvenURL`, `TestRelocateChecksTheHubsProof` |
  | Phase 2 (low) hub URLs differing only in the host's case were different strings | `CheckURL` writes the host in lower case | `TestSameURL` |
  | Phase 3 (medium) `If-Range` fetched a blob whole past its limits | The handler drops `If-Range`: a blob never changes under its hash | `TestContentServesBlobsWithinTheirLimits` |
  | M1 / Phase 4 (medium) every MCP request without a session leaked a notesd connection; the SDK's `server/discover` opened one too | A request's server is built once, and only `initialize` opens a connection; a request that opened no session closes it; a session the SDK answers 404 for is forgotten | `TestASessionHoldsOneConnectionUntilItEnds` counts connections per `initialize`, stray request and malformed body |
  | L5 (low) content named by the file's hash let any same-site page in the tailnet probe whether a vault held a known file | Content URLs are `blobs/<mac>/<hash>.<ext>`, the MAC under notesd's content key, which `rpc.hello` gives a session | `TestContentServesBlobsWithinTheirLimits`, `TestTheContentKeyIsKept`, the app's MAC tests against notesd's |
  | N5 (nit) `POST /sync/hub` revealed which device IDs exist, and logged each call | Unknown and revoked devices get the same answer, a proof under a key no token makes; calls log at debug; a peer gets 30 requests at once and one a second after, across `POST /sync/hub` and `POST /sync/pair` ([lodge-app.md](../specs/lodge-app.md#limits)) | `TestTheRelocationProof`, `TestAPeerHasItsBucketOfRequests`, `TestTheBucketsAreBounded`, `TestUntokenedRequestsAreLimitedPerPeer` |
  | N6 (nit) any tailnet peer could fill the 8 pairing slots | One node (`Lodge-Node-ID`) may have 2 pairings waiting, a rate key and never an admission | `TestANodeMayHaveTwoPairingsWaiting` |
  | N7 (nit, the kit's) an MCP session was swept only when a request arrived | The kit sweeps on a ticker (lodge/kit 2017aec and since) | the kit's tests; notes' side, a session's end closing its connection, in `TestASessionHoldsOneConnectionUntilItEnds` |

  **Accepted, not fixed; the user accepted both in writing on
  2026-10-21:**
  - *The relocation proof is keyed by the stored token hash*
    ([lodge-app.md](../specs/lodge-app.md#device)): whoever can read the
    hub's database can make a valid proof. They could already serve the
    hub's data; the proof only stops a host without it.
  - *M2, notes' part:* notesd reads the bodies of `POST /sync/pair` and
    `POST /sync/hub` (1 KiB each) with no deadline of its own. It relies
    on lodge's per-read body deadline of 30 s on device requests, which
    is lodge's fix for M2.

  **The scope's other items,** checked on 2026-10-19:
  - *XSS in Firefox:* `TestIRenderedNotesRunNoScript` (`make
    m4-evidence`) opens a note of `<script>`, `onerror`, `onload`,
    `ontoggle`, `srcdoc` and `javascript:` links in live preview and in
    reading view, inside lodge's development build, clicks the links,
    and opens an SVG carrying a script at its lodge-content URL: no
    script runs in notes' frame, the shell, or the SVG, which
    lodge-content serves sandboxed.
  - *lodge-client's message handling:* the client takes a message only
    from its own origin and from `window.parent`, and posts to its own
    origin only; notes never sets its frame's location, and opens links
    through lodge's `open` (or `window.open` with `noopener` outside
    lodge), which refuses `javascript:`, `data:` and `blob:`.
  - *Logs holding a secret:* no log line names a device token, a
    pairing secret or code, the content key or an MCP token; MCP logs
    the token's name, which lodge sends, and pairing the device's name
    and address.
  - *What `session` and `agent` can call:* `TestEveryMethodAdmitsItsCallers`
    tries every method as each caller kind, and the engine's
    `TestAgentScope…` tests every `note` method on another vault and
    every write under a read scope.
  - *Each path's caller, the peer check and the headers' trust:*
    `TestTheSocketKeepsLodgesRules`, and the platform review's "checked
    and sound".

  **Pending:**
  - XSS in the phone's browser, on the user's phone (Phase 7);
  - Chrome's PDF viewer under lodge-content's sandbox (review 12, an
    open question).

  *Done:* lodge's own review of sign-in, sessions, tokens and headers
  lists no open XSS, CSRF or session finding
  ([lodge's roadmap, Phase 2](https://github.com/frostyard/lodge/blob/main/docs/plans/0001-roadmap.md#phase-2--lodge-v1-with-notes-as-its-only-app)).

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
  - the trial's devices move with `notesctl hub relocate OLD-URL
    NEW-URL` (the script stops the M2 hub before it records the URL, so
    no device can learn it from `log.hub`; dry run, 2026-10-25), and the
    old `tailscale serve` for the hub goes once `notesctl hub devices`
    shows all of them seen;
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
- **Loopback runs recorded (2026-10-19, `make m4-evidence`):**
  - evidence 1's: Phase 5's run above, at both sizes, inside lodge's
    development build; the desktop's edit reaches the web app at phone
    size through a device notesd paired at lodge's `/device/notes`.
    The phone's home screen is Phase 7's;
  - evidence 2's: 20 edits typed in Firefox at 390 × 844, timed from
    each one's last keystroke to its text in the desktop app's editor
    (the built app under Xvfb on a device notesd, the `watch` spike):
    median 1.016 s, p95 1.021 s, max 1.028 s, within the 2 s budget.
    The editor's 1 s save timer is nearly all of it.
- **Dry run (2026-10-25):** `scripts/move-hub-behind-lodge.sh` ran on the
  hub's VM as root, without `--apply`. It changed nothing and found 2
  problems:
  1. It said "lodge is not up", but `lodge.service` is active and
     `lodgectl status` shows both nodes and both apps up. `lodgectl` is
     in `/opt/lodge/bin`, not on root's PATH. Workaround: run the script
     with `LODGECTL=/opt/lodge/bin/lodgectl`. *Fixed 2026-10-25:* the
     script takes `LODGECTL`, then `lodgectl` on PATH, then the
     `lodgectl` beside `lodge.service`'s binary.
  2. It said `/var/lib/frostnotes` holds vaults other than the overnight
     test (the trial's vault and the test vault), and to move it aside by
     hand. The cause: lodge's notesd is a device of the trial hub and
     keeps a copy of the trial's vault there. Its pushes are all on the
     hub (pending 0, `verify` ok), so setting it aside loses nothing.
     Workaround: the user sets the copy aside by hand, or the script
     learns the case. *Fixed 2026-10-25:* the script sets the data
     directory aside whole when every vault in it but `Test` is a copy
     of one of the hub's vaults, in step with it (cursor and the hub's
     seq equal, 0 pending, 0 conflicted, nothing refused or skipped) and
     verified (`notesctl sync verify`), and syncs with the trial hub on
     this machine (the loopback address of the address the hub listens
     on). Otherwise it still refuses. Since lodge's notesd takes writes
     until nothing reaches it, the move then stops `lodge.service`,
     restarts lodge's notesd (`sync status` shows the counts of the loop's
     last cycle, so a fresh process counts from its database, conflict
     versions included), waits up to 60 s until each copy is idle and in
     step with the hub again and verifies it, and only then stops lodge's
     notesd and the trial hub.
     A copy that does not drain stops the move before anything moves
     (cross-review of #92). The move starts lodge again at its end.
  - *The dry run again (2026-10-25, the fixed script, on the hub's VM as
    root):* 0 problems. It found lodge, its `lodgectl` in `/opt/lodge/bin`,
    and lodge's copy of the trial's vault, pushed in full and verified at
    seq 763, so the whole data directory goes aside.
  - Other checks passed: notes' manifest and web build in
    `/usr/share/lodge/apps/notes`; the user `frostnotes` exists;
    `frostnotes.service` and `frostnotes-lodge.socket` are installed
    (binary `/opt/lodge/bin/notesd`, with the overnight drop-ins); the
    hub's data directory was found (14 MB, user `notes`); the trial notesd
    is the expected hub; the vault's CIFS mount was found (still
    `uid=notes`).
  - *Observed 2026-10-25:* the trial's devices run notesd builds from
    before Phase 2 (#45, 2026-10-19), which added `log.hub` and
    `notesctl hub relocate`. The laptop runs 8db47da (2026-10-18) and the
    device VM 40a3c56 (2026-10-17). Neither can move to lodge until it
    runs a current notesd and `notesctl`. Upgrading before the move is
    safe: a device asks `log.hub`, and the trial's hub answers
    `methodNotFound`, which the device takes as no move
    (`internal/syncer/relocate.go`). Read from the script and
    `cmd/notesd`, not yet run: the move stops the trial's unit before it
    records the URL, and `notesd --lodge` listens on no network address,
    so nothing answers at the old URL once the URL is recorded, and a
    device cannot learn it there. Each device then runs
    `notesctl hub relocate OLD-URL NEW-URL`
    ([design/hub.md](../design/hub.md#moving-devices-to-lodge)).
  - *Observed 2026-10-25* (`lodgectl tokens` on the hub's VM and the
    hub's database, read-only): lodge's agent tokens for notes name the vault
    by lodge's notesd's local vault ID. On 2026-10-25 that notesd holds the test
    vault as vault 1 and the trial's vault as vault 2. Two live tokens
    use `vault=2`: `porter-notes`
    (`vault=2;write=Porter/;except=Porter/Routines/;daily`) and
    `frostread-notes` (`vault=2;write=Reading/`). The move copies the
    hub's data directory in whole, and in the hub's database the trial's
    vault is vault 1, its only vault. After the move, `vault=2` names no
    vault. The user issues both tokens again with `vault=1` and the same
    rest of the scope (`lodgectl token issue`), and revokes the old ones.
    Porter and Read then take the new tokens.
  - After the move, lodge's own former device (device 3 on the hub) stays
    paired and is never seen again. Revoke it after the move, or
    `notesctl hub devices` never shows "all seen".
  - The hub's VM and the device VM did not synchronize their clocks;
    revision times come from the hub's clock. *Fixed 2026-10-25:* both
    run `systemd-timesyncd` now, synchronized, within 0.04 s of the
    laptop.
  - *Upgraded 2026-10-25, with the user's go-ahead:* the laptop and the
    device VM run notesd and `notesctl` at 9a51913, which have `log.hub`
    and `notesctl hub relocate`. Each kept its data directory and old
    binaries in a backup. After the upgrade, `notesctl sync verify` was
    ok on all 3 devices (687 files, cursor 763 = the hub's seq 763,
    0 pending, 0 conflicted), and the hub saw both devices connected.
    lodge's notesd stays at 8c096df, which has the move's commands.

### Run sheet for the move (2026-10-26)

The morning's order. About 30 minutes pass from the moment sync stops
(step 1) to both devices synced through lodge (step 9). Tokens appear by
name only, and are piped from `lodgectl` into their consumer, never
written down. Who does what:

- **[Brian]**: on the user's own devices, or a step only the user may
  take (a token shown on screen, a sudo password, a judgement).
- **[agent]**: the planner, through `incus exec` into the VMs and on the
  laptop, after the user's go-ahead that morning.

Names used below (the real ones stay out of the repository):

```sh
OLD=https://HUB.<tailnet>.ts.net                 # the M2 hub's URL now
NEW=https://lodge.<tailnet>.ts.net/device/notes  # its URL behind lodge
LODGECTL=/opt/lodge/bin/lodgectl                 # on the hub's VM
HUBCTL="runuser -u frostnotes -- env FROSTNOTES_SOCKET=/run/frostnotes-lodge/notesd.sock /opt/lodge/bin/notesctl"
```

**Before sync stops (about 10 minutes; nothing changes yet)**

0. **[Brian]** Merge the script's fix
   ([#92](https://github.com/frostyard/frostnotes/pull/92)) and say go.
   Name the iPhone's browser for evidence 1 (Safari is likely).
1. **[agent]** Copy `scripts/move-hub-behind-lodge.sh` from `main` to
   `/root/` on the hub's VM.
2. **[agent]** Checks, each must pass:
   - on the laptop, the device VM (as `notes`) and lodge's notesd
     (`$HUBCTL sync status`, `$HUBCTL sync verify --vault 2`):
     `notesctl sync status` idle with cursor = the hub's seq, 0 pending,
     0 conflicted; `notesctl sync verify` ok;
   - on the hub's VM: `notesctl hub devices` (as `notes`) shows devices
     1, 2 and 3 connected; `timedatectl` on both VMs says synchronized;
   - a backup of the laptop's copy of the vault:
     `tar -C ~/Notes -czf ~/obsidian-vault-before-move-$(date -u +%Y%m%dT%H%M%SZ).tar.gz "Obsidian Vault"`.
3. **[agent]** The dry run on the hub's VM, as root:
   `bash /root/move-hub-behind-lodge.sh --url "$NEW"`. It must end with
   "Dry run: nothing changed." and list 0 problems.

**Sync stops (about 30 minutes)**

4. **[Brian]** Edit no note anywhere until step 9 says done: not in the
   desktop app, the web app or Obsidian.
5. **[agent]** Stop the agents' writers on the hub's VM:
   `systemctl stop frostporter.socket frostporter.service frostread.socket frostread.service`.
   Then run step 2's checks on lodge's notesd again.
6. **[agent]** The move, on the hub's VM as root:
   `bash /root/move-hub-behind-lodge.sh --apply --url "$NEW"`. Keep the
   backup path it prints (`/var/backups/frostnotes-move-STAMP`). It stops
   lodge (mail, Porter and Read in lodge are down with it, for a few
   minutes), restarts lodge's notesd, waits until its copy is in the hub
   and verified ("✓
   lodge's copy of Obsidian Vault is in the hub at seq …"), stops notes
   in lodge and the M2 hub, moves the hub, and starts lodge again. It
   must print "✓ frostnotes wrote to …", "✓ frostnotes.service
   serves hub …" with the trial hub's ID, and "✓ its vaults are the
   hub's". From here, devices cannot sync until step 9.
7. **[agent]** The hub behind lodge:
   - `$HUBCTL hub status`: the trial hub's ID, "devices use $NEW",
     1 vault, 3 devices;
   - `$HUBCTL hub vaults`: "Obsidian Vault" at seq 763 or more;
   - `$HUBCTL vault list`: vault 1 is "Obsidian Vault" at the share's
     path.
   - **The SMB scratch note check,** before any device moves: write a
     scratch note as notesd, see it on the share from another of the
     share's clients (the laptop's CIFS mount of the share), then delete
     it.

     ```sh
     printf '# Phase 7 scratch check\n' | $HUBCTL write --create "Phase 7 scratch check.md"   # prints its hash
     ls -l "<the laptop's mount of the share>/<the vault's path>/Phase 7 scratch check.md"      # on the laptop
     $HUBCTL rm --base HASH "Phase 7 scratch check.md"
     ```

     **[Brian]**, if wanted: see it in Obsidian on another machine
     before the delete.
8. **[agent]** Agents' tokens, on the hub's VM. The old ones name
   `vault=2`, which after the move names no vault. Revoke each, then
   issue it again under the same name with `vault=1`, piped straight
   into its consumer:

   ```sh
   $LODGECTL token revoke porter-notes
   $LODGECTL token issue porter-notes --app notes \
     --scope 'vault=1;write=Porter/;except=Porter/Routines/;daily' --agent frostporter |
     runuser -u frostporter -- /usr/local/bin/porterd token set notes
   systemctl start frostporter.socket frostporter.service      # porterd reads its token at start
   systemctl start frostread.socket frostread.service
   $LODGECTL token revoke frostread-notes
   $LODGECTL token issue frostread-notes --app notes \
     --scope 'vault=1;write=Reading/' --agent frostread |
     runuser -u frostread -- /usr/local/bin/readctl notes-token   # "the notes token is set"
   $LODGECTL tokens   # porter-notes and frostread-notes live with vault=1; the old ones revoked
   ```

9. **The devices move.**
   - **[agent, on Brian's laptop]** `notesctl hub relocate "$OLD" "$NEW"`:
     "hub ID is at $NEW now".
   - **[agent]** On the device VM:
     `runuser -u notes -- env FROSTNOTES_SOCKET=/run/frostnotes/notesd.sock notesctl hub relocate "$OLD" "$NEW"`.
   - **[agent]** On each: `notesctl sync status` shows `$NEW`, idle,
     cursor = the hub's seq, 0 pending, 0 conflicted; `notesctl sync
     verify` is ok. `$HUBCTL hub devices` shows devices 1 and 2 seen
     after the move. Sync is back: the stop ends here.

**After sync is back**

10. **[agent]** Revoke lodge's former device, whose copy went aside:
    check that device 3 is "lodge" in `$HUBCTL hub devices`, then
    `$HUBCTL hub revoke 3`. It never connects again, and until it is
    revoked, `hub devices` never shows all devices seen. The M2 hub's own
    data, which a rollback restores, keeps device 3.
11. **[Brian]** Claude Code's tokens, for evidence 3. Run them in your
    own terminal: each token prints once, and only you see it.

    ```sh
    incus exec <the hub's VM> -- /opt/lodge/bin/lodgectl token issue claude-code-notes --app notes --scope 'vault=1;write'
    incus exec <the hub's VM> -- /opt/lodge/bin/lodgectl token issue claude-code-notes-read --app notes --scope 'vault=1'
    ```

    On the machine with no local copy of the vault:
    `claude mcp add --transport http notes https://lodge.<tailnet>.ts.net/mcp/notes --header "Authorization: Bearer …"`
    (never `--scope project`); the read token likewise under another
    name.
12. **[Brian]** Look: the desktop app on the laptop shows the notes;
    the web app in lodge on the laptop and on the iPhone opens "Obsidian
    Vault" (reload it; the vault's ID changed from 2 to 1, so its open
    tabs may reset); an edit on the phone shows on the laptop.
13. **[agent]** Porter and Read work: `journalctl -u frostporter -u
    frostread --since -10min` shows no token warning, and `$LODGECTL
    tokens` shows the new tokens' last use once each acts.
14. **[Brian]** On the server, once `$HUBCTL hub devices` shows devices
    1 and 2 seen at the new address: `tailscale serve status` shows only
    the hub's proxy; then `sudo tailscale serve --https=443 off`.
15. **[agent]** Record the run here, and start the 7 days of sync through
    lodge.

**Rollback**

The script's rollback compares the hub's log with the seq it recorded
when notes started in step 6 (`seqs=` in the backup's `manifest`), and
refuses when the log moved, unless `--force`. Run each rollback as a dry
run first, then with `--apply`. A rollback restores the M2 hub's data
from before the move, fstab and lodge's data directory (the test and its
copy of the vault), restarts both notesd and reloads lodge; devices
still at `$OLD` resume by themselves.

- *Step 6 stops on an error, or lodge's copy does not drain:* run the
  rollback it prints. It starts lodge, the M2 hub and lodge's notesd on
  its copy again, which pushes what it holds. Then find the cause and
  repeat from step 2.
- *After step 6, before the scratch note (step 7):* the plain rollback
  works.
- *After the scratch note, before step 8:* the note's create and delete
  moved the hub's log by 2, so the plain rollback refuses. Use `--force`
  only when both hold: `$HUBCTL hub vaults` shows the manifest's seq
  plus 2, and `$HUBCTL hub devices` shows no device seen since the move.
  The M2 hub never had the note, and the share no longer has it.
- *After step 8:* Porter and Read can write through the moved hub, and a
  rollback loses those writes (they stay in the backup's
  `data-after-move`). First stop them
  (`systemctl stop frostporter.socket frostporter.service frostread.socket frostread.service`),
  then roll back as above, with `--force` only when the seq is the
  manifest's plus 2. Otherwise fix forward. After a rollback, give both
  agents back their `vault=2` tokens:

  ```sh
  $LODGECTL token revoke porter-notes
  $LODGECTL token issue porter-notes --app notes \
    --scope 'vault=2;write=Porter/;except=Porter/Routines/;daily' --agent frostporter |
    runuser -u frostporter -- /usr/local/bin/porterd token set notes
  systemctl restart frostporter.socket frostporter.service   # porterd reads its token at start
  systemctl start frostread.socket frostread.service         # readctl needs readd running
  $LODGECTL token revoke frostread-notes
  $LODGECTL token issue frostread-notes --app notes \
    --scope 'vault=2;write=Reading/' --agent frostread |
    runuser -u frostread -- /usr/local/bin/readctl notes-token
  ```

- *After step 9 (a device relocated):* fix forward. A relocated device
  cannot relocate back (the M2 hub records no URL), and what it pushed
  through lodge is only in the moved hub. Use `--force` only if the user
  decides to; relocated devices then stay offline, their notes safe on
  them, until the move runs again.
- *After step 14:* also restore the old serve on the server:
  `sudo tailscale serve --bg --https=443 http://<the hub's VM>:7880`.

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
  drives. By Phase 7. Lodge's log shows the phone is an iPhone
  (2026-10-25); the browser is still the user's to say.
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
- Decisions: [ADR-0026](../adr/0026-notesd-runs-as-a-lodge-app.md), [ADR-0027](../adr/0027-a-narrow-window-takes-obsidians-phone-layout.md), [ADR-0029](../adr/0029-inside-lodge-a-rooted-link-no-file-answers-opens-through-lodge.md); [ADR-0004](../adr/0004-a-notes-daemon-with-thin-clients.md), [ADR-0012](../adr/0012-agents-reach-notes-through-mcp.md), [ADR-0016](../adr/0016-pair-devices-with-a-one-time-code-and-a-revocable-token.md); frostyard/core [ADR-0057](https://github.com/frostyard/core/blob/main/docs/adr/0057-run-first-party-apps-behind-lodge.md)
- lodge: [roadmap](https://github.com/frostyard/lodge/blob/main/docs/plans/0001-roadmap.md), [app contract](https://github.com/frostyard/lodge/blob/main/docs/specs/app-contract.md), [lodge-client](https://github.com/frostyard/lodge/blob/main/docs/specs/lodge-client.md)
- Builds on: [plan 0003](0003-m1-headless-vault.md) (MCP), [plan 0004](0004-m2-sync.md) (the hub, devices), [plan 0005](0005-m3-desktop-app.md) (the app, its evidence harness)
- Workflow: [design/agent-workflow.md](../design/agent-workflow.md)
- Roadmap: [plan 0002](0002-roadmap.md)
