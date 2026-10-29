# Plan: Folder scopes for agents

This plan builds [ADR-0028](../adr/0028-an-agents-scope-can-limit-writes-to-folders.md):
- an agent's scope can limit its writes to folders and take folders out
  (`write=`, `except=`);
- it can allow appending to today's daily note and nothing more
  (`daily`).

It comes after M4 ([plan 0006](0006-m4-notes-inside-lodge.md)), whose
agent scope it extends additively in protocol 1, and it is what Porter,
frostyard's assistant, needs from notes. Porter's token is
`vault=<id>;write=Porter/;except=Porter/Routines/;daily` (Porter's
[ADR-0005](https://github.com/frostyard/porter/blob/main/docs/adr/0005-memory-and-routines-live-in-the-vault.md)).

Porter reaches `/mcp/notes` over lodge's local agent socket (lodge's
[ADR-0014](https://github.com/frostyard/lodge/blob/main/docs/adr/0014-agents-on-lodges-host-use-a-local-agent-socket.md)).
That is lodge's work. lodge passes notes the same `Lodge-Caller: agent`,
`Lodge-Token` and `Lodge-Scope` it passes from its tailnet listener, so
nothing in notes changes for it.

Work is split per
[ADR-0010](../adr/0010-claude-plans-and-codex-runs-task-cards.md):
- **The planner owns** every piece: the grammar, the engine's checks,
  the new method and the MCP tools' scope handling are one security
  boundary and its contract.
- **No cards.**

Status (2026-10-28):
- **Phase 1 is done.** The user approved building this plan on
  2026-10-19, ahead of acceptance, and accepted ADR-0028 on 2026-10-21.
- **Phases 2 and 3 are built, in two PRs.**
  - The first PR has the grammar, the engine's checks on every writing
    method, the router's `daily`, the tests at the boundaries, and the
    specs.
  - The second has `note.appendDaily` and Phase 3's tools.
  - `append_to_daily_note` is offered under `daily` or a bare `write`,
    not under "a write right that covers today's daily note". An agent
    cannot learn the daily note's path without writing it, so the
    session could not tell. Under `write=` alone, `edit_note` and
    `create_note` reach a daily note in their folders.
  - This was confirmed on 2026-10-19. A whole-vault writer can already
    write the daily note, so taking the tool from `vault=<id>;write`
    tokens would be a regression rather than a limit. Under folder
    writes, `daily` is the only way to append, which is what Porter
    needs.
  - Two rules were added to ADR-0028 while building:
    - a prefix is written one way, in NFC and `url.PathEscape`'s
      encoding;
    - a path that reaches an existing file by a name its folder does
      not list is refused (a share's 8.3 names, trimmed dots and other
      foldings).
- **Phase 4's run is recorded** (2026-10-28,
  [below](#phase-4--evidence-through-lodge-planner)), through lodge's
  listener and its agent socket. Its "done when" still waits for the
  user to issue Porter's token on the hub.
- **Today's time zone,** the open question below, is checked when the
  hub runs it.

## Phase 1 — Decisions (planner)

- [ADR-0028](../adr/0028-an-agents-scope-can-limit-writes-to-folders.md),
  proposed: the grammar, how a path is judged (after the path guard,
  segment by segment, NFC and case folded, `except` winning), what each
  write method needs, `daily`, and `invalidParams` for a refused write.
- **Done when:** the user accepts ADR-0028.

## Phase 2 — The grammar and the engine's checks (planner, contract first)

- **The contract:**
  - `schema/rpc/note.yaml` gains `note.appendDaily {vaultId, text,
    date?, source?, author?}`, marked `writes`, for `local`, `session`
    and `agent`. It appends `text` at the end of a day's daily note,
    after a line break when the note does not end in one, and creates
    the note first from the vault's template when it is missing. It
    answers the write's result and whether it created the note.
  - Then `make gen`.
- **`api.Scope`:**
  - it gains the write prefixes, the except prefixes and `daily`;
  - `ParseScope` reads the grammar strictly: parts once each and in
    order, prefixes percent-decoded, each ending in `/` and passing the
    path guard without it, no empty prefix, and `except` only beside a
    write right;
  - `String` writes the canonical form;
  - today's two forms parse and print as they do now.
- **The engine:**
  - one check, *may this connection write this path*, judged after the
    path guard: segment by segment, after NFC normalization
    (`golang.org/x/text/unicode/norm`, already in the module graph) and
    case folding, with `except` winning;
  - the check runs in every method marked `writes`, as
    [ADR-0028](../adr/0028-an-agents-scope-can-limit-writes-to-folders.md)
    lists them. That covers both ends of a move, folders whole for
    `moveFolder` and `deleteFolder`, a move's link updates (refused
    whole when any would write outside), and `revert` at a deleted
    file's last path;
  - a refused path answers `invalidParams`, with a message naming the
    scope's writable folders and `daily`.
- **The router** admits `note.daily` and `note.appendDaily` under
  `daily`. Every other writing method still needs a write right, and is
  `methodNotFound` without one.
- **`daily`:**
  - it admits those two methods for today only, in notesd's time zone;
  - `except` still applies to the daily note's path.
- **The specs, with the code:**
  - [lodge-app.md](../specs/lodge-app.md#mcp): the scope's grammar and
    its rules;
  - [rpc-protocol.md](../specs/rpc-protocol.md#caller-kinds): an
    agent's scope binding its connection;
  - [design/vault.md](../design/vault.md#daily-notes): the append;
  - each gains ADR-0028 in its references.
- **Tests at the boundaries:**
  - the grammar: every valid form, and each malformed one, refused;
  - every method marked `writes`, tried just inside and just outside
    each prefix, under `except` inside `write=`, and in another case and
    in NFD. Like `TestEveryMethodAdmitsItsCallers`, the test walks the
    generated table, so a new writing method without a case fails it;
  - moves across the boundary in both directions;
  - folder moves and deletes over an excepted folder;
  - a move whose link updates would reach outside;
  - `daily` for yesterday and tomorrow, refused;
  - an append after the user edited the note meanwhile, which keeps the
    edit and leaves earlier text as it was;
  - a daily note under `except`, refused.
- **Done when:** `make check` and `make vault-it` are green with those
  tests, and an agent connection with Porter's scope can write
  `Porter/Memory.md` and append to today's daily note, and can write
  nothing else in the vault.

## Phase 3 — The MCP tools under a limited scope (planner)

- **The tools a session offers** follow its scope:
  - the read tools always;
  - `create_note` and `edit_note` under a write right, their
    descriptions naming the writable folders when `write=` or `except=`
    limits them;
  - `append_to_daily_note` under `daily` or a bare `write`. It takes a
    `date` only under a bare `write`.
- **`append_to_daily_note` calls `note.appendDaily`** under every scope,
  so an append never rewrites earlier text.
- **`read_note` and `list_notes` cope with notes an agent may read but
  not write,** such as `Porter/Routines/`:
  - under a limited scope, each note they return says whether the
    session may write it (`writable`);
  - they read those notes as they read any other.
- **[mcp.md](../specs/mcp.md#through-lodge)** gains all of this, with
  ADR-0028 in its references.
- **Tests:** through `lodgeapp`'s socket with Porter's scope:
  - the tool list;
  - an edit of `Porter/Memory.md`, which succeeds;
  - an edit of `Porter/Routines/brief.md` and of a note outside
    `Porter/`, each refused with the message naming the folders;
  - an append that creates today's note from the template, then a
    second append;
  - `writable` in `list_notes`.
- **Done when:** those tests are green, and an MCP client with Porter's
  scope cannot write a routine however it asks.

## Phase 4 — Evidence through lodge (planner)

- `make m4-evidence` gains a run with a token of Porter's scope, issued
  by lodge's development build's `lodgectl`:
  - an MCP client writes `Porter/Memory.md` and
    `Porter/Briefs/<date>.md`;
  - it is refused at `Porter/Routines/` in two spellings of its case,
    and at a user's note;
  - it appends to today's daily note;
  - each version names the token.
- Once lodge ships its local agent socket, the same run goes through
  it.
- `make porter-evidence` runs this alone, without Firefox or the
  desktop app.
- **Done when:** that run is recorded here, and the user issues
  Porter's token on the hub after plan 0006's Phase 7.
  *Note (2026-10-21):* a token of Porter's scope was already in use on
  the overnight test deployment from 2026-10-19, and on 2026-10-20
  probably on the trial's vault (owner-reported;
  [plan 0006](0006-m4-notes-inside-lodge.md#decisions-taken-for-m4)).
- **Recorded (2026-10-28, `make porter-evidence` and `make
  m4-evidence`, `TestIPorterThroughLodge` in `internal/webevidence`):**
  the run holds through both of lodge's routes to `/mcp/notes`.
  - **Where:** in the nsl machine `frostnotes` (Debian 13, nsl v0.11.0,
    `dev/nsl/provision.sh`), with lodge's development build at
    `e76a17c` (`make lodge-e2e`). The pin moved from `c48d43a`, which
    has no agent socket.
  - **The tokens:** `lodgectl token issue` gave 2 tokens of Porter's
    scope, `vault=1;write=Porter/;except=Porter/Routines/;daily`:
    - `porter-notes-tailnet`, for lodge's listener (the tailnet's
      rules);
    - `porter-notes`, issued `--agent` for the test's user, for the
      agent socket.
  - **The client:** the MCP SDK's client, first on the listener's
    `/mcp/notes`, then on the agent socket's.
  - **Each route:**
    - writes `Porter/Memory.md` and `Porter/Briefs/2026-10-28.md` (the
      first route creates them, the second appends);
    - is refused at `Porter/Routines/` in two spellings of its case:
      - `edit_note` of `Porter/Routines/Morning.md` and of
        `porter/routines/Spelled.md`;
      - `create_note` of `Porter/Routines/New.md` and of
        `porter/routines/New.md`;
    - is refused at a user's note, `edit_note` of `Garden.md`;
    - appends to today's daily note, `Daily/2026-10-28.md` (the first
      route's append creates it from the template).
  - **Every refusal** is `invalidParams`, and says "outside this
    token's writes" and "this token may write under Porter/ except
    Porter/Routines/, and append to today's daily note". The refused notes are byte for byte as before
    on the hub's disk, and no new note is in `Porter/Routines/` in any
    case.
  - **Every version** of the notes the run wrote has source `agent`
    and names the token of the route that wrote it, and there are no
    others. Newest first:
    - `Porter/Memory.md` and the brief: `porter-notes`, then
      `porter-notes-tailnet`;
    - the daily note: `porter-notes`, then `porter-notes-tailnet` twice
      (the template's version and the first line's). It holds the
      template's heading and both lines in order.
  - **A token works only on its own route:** `porter-notes` on the
    listener and `porter-notes-tailnet` on the agent socket each answer
    401.
  - **The rest of M4's evidence** also passes at `e76a17c`, in the same
    `make m4-evidence` run:
    - `TestIWebAppInLodge` (frames 1232 and 390 wide);
    - `TestIPhoneEditReachesTheDesktop`, with a p95 of 1.035 s over 20
      edits;
    - `TestIRenderedNotesRunNoScript`.

  How the run differs from Porter's on the hub:
  - lodge runs from `--root` and admits its own uid on the agent socket,
    so the run does not show the socket's peer check (lodge's tests
    hold it).
  - The vault is on a case-sensitive disk. There, `porter/routines/`
    is a folder of its own, which the case-folded check also refuses.
    An edit of `porter/routines/Morning.md` answers `notFound`: no note
    has that path. So the run edits a note seeded under that
    spelling.
  - The run's notesd and hub are a loopback vault, not the user's.
  - What stays open is the user issuing Porter's token on the hub.

## Later / ideas

- A `read=` part, for a token that reads only some folders.
- Showing an agent's writable folders in the web app's Hub settings, beside
  lodge's tokens.

## Open questions

- **Today's date:** notesd decides "today" in its own time zone. The
  hub's must be the user's, or capture lands in the wrong day's note.
  Phase 2 checks the hub's, or the scope learns a time zone.
- **lodge's agent socket** *(answered, 2026-10-28)*: it passes the
  same three headers. Phase 4's run through the socket shows it:
  - the session is an agent's;
  - the versions name the token;
  - the scope holds.

  It passes no `Lodge-Node-Name`. notesd reads none, and lodge's own
  log names the agent's user in place of a node (lodge's
  [lodge-http.md](https://github.com/frostyard/lodge/blob/main/docs/specs/lodge-http.md#the-agent-socket)).

## References

- Implements: [ADR-0028](../adr/0028-an-agents-scope-can-limit-writes-to-folders.md);
  [specs/lodge-app.md](../specs/lodge-app.md#mcp),
  [specs/mcp.md](../specs/mcp.md#through-lodge),
  [specs/rpc-protocol.md](../specs/rpc-protocol.md#caller-kinds),
  [design/vault.md](../design/vault.md#daily-notes)
- Builds on: [plan 0006](0006-m4-notes-inside-lodge.md) (the agent
  scope, `/mcp`), [ADR-0026](../adr/0026-notesd-runs-as-a-lodge-app.md)
- Elsewhere: lodge's
  [ADR-0006](https://github.com/frostyard/lodge/blob/main/docs/adr/0006-agents-reach-each-app-through-its-own-mcp-endpoint.md)
  and [ADR-0014](https://github.com/frostyard/lodge/blob/main/docs/adr/0014-agents-on-lodges-host-use-a-local-agent-socket.md);
  Porter's [ADR-0005](https://github.com/frostyard/porter/blob/main/docs/adr/0005-memory-and-routines-live-in-the-vault.md)
- Workflow: [design/agent-workflow.md](../design/agent-workflow.md)
- Roadmap: [plan 0002](0002-roadmap.md)
