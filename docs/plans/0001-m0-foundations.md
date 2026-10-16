# Plan 0001: M0 foundations

Goal: everything later milestones stand on, proven before any notes
feature is built. Most of it comes from frostmail, copied and then owned
here ([ADR-0008](../adr/0008-generate-the-rpc-contract-from-a-yaml-idl.md),
[ADR-0011](../adr/0011-look-like-frostmails-sibling-with-obsidians-layout.md)).
Status: **in progress** (ADRs 0003–0012 accepted 2026-10-16).

## Phase 1 — Repository

- Done: the repository starts from `bketelsen/agentic-template`, with
  ADRs 0001–0012, [overview](../design/overview.md),
  [vault](../design/vault.md) and [sync](../design/sync.md) designs, and
  the [roadmap](0002-roadmap.md).
- `frostyard/frostnotes` on GitHub (public, as frostmail is), MIT.
- [org-adrs.md](../org-adrs.md); `docs/tasks/` with `TEMPLATE.md` and
  `EXECUTOR.md` adapted from frostmail.
- `go.mod` (Go 1.27.1), `mise.toml` and `mise.lock` (golangci-lint 2.13.1),
  `.golangci.yml`, and the `Makefile` gate triad: `verify`, `check`, `ci`.
- CI: SHA-pinned actions, `permissions: {}`, `persist-credentials: false`
  (frostyard/core ADR-0021), running `make verify` and the app checks.
- **Done when:** `make check` is green on an empty Go module, and CI is
  green on the first push.

## Phase 2 — Environment

- `dev/nsl/provision.sh` from frostmail's (Node, pnpm, Rust, WebKitGTK
  `-dev`, pinned and checksum-verified) for the nsl machine `frostnotes`
  ([ADR-0009](../adr/0009-development-environment-and-gates.md)).
- **Done when:** a fresh `nsl create frostnotes` plus the script builds the
  app shell.

## Phase 3 — Contracts and runtime

- `tools/rpcgen` from frostmail, with per-domain caller kinds (`local`,
  `session`, `device`) enforced by the router.
- `api/` runtime, `internal/store` (migrations, `Tx`, the changes log),
  `internal/events`, `internal/rpcserver` (socket, peer-UID check),
  `cmd/notesd` and `cmd/notesctl` skeletons.
- Protocol-1 schema: `rpc.hello`, `events.subscribe`, `vault.list`,
  `vault.add`, `vault.remove`.
- `specs/rpc-protocol.md`.
- **Done when:** `notesctl hello` and `notesctl vault add/list` work against
  a running notesd, and `make gen-check` passes.

## Phase 4 — App shell

- `app/` from frostmail's shell, with the mail stripped out: `bridge.rs`,
  `transport.ts`, the mock transport, tokens, window chrome, Biome,
  Vitest, and `make ui-check`, `app-dev` and `app-build`.
- A WebSocket transport, and notesd serving `/api` on loopback in a
  development mode, so the same shell runs in a browser.
- **Done when:** the app shows `rpc.hello` from notesd in the Tauri window
  and in Firefox.

## Phase 5 — Spikes (go or no-go)

1. **CodeMirror 6 in WebKitGTK:** live preview with wikilinks, hidden
   syntax and a checkbox widget, in the Tauri window, on a generated
   10,000-line note. Keystroke-to-paint under 16 ms, scrolling at 60 fps,
   and IME input works. Measured natively on the host GPU and in nsl.
2. **Merge library shape:** `internal/merge`'s interface, and diff3 by line
   and by word over 30 hand-written cases, including frontmatter keys and
   two daily notes from one template. The cases become M2's golden tests.
3. **Watching the user's vault where it lives:** fsnotify on the network
   home directory. Measure which events arrive and the rescan cost for 666
   notes, to set the network rescan interval.
4. **Obsidian link resolution:** 50 links from the user's vault, checked
   against Obsidian on the user's machine. Their patterns (duplicate names
   in different folders, aliases, heading and block links, case
   differences) become the first `specs/markdown.md` fixtures with made-up
   names; nothing from the vault is committed.

- **Done when:** each spike has a recorded result in this plan, and any
  no-go has an ADR that changes the design.

## Phase 6 — Task tooling and calibration

- `tools/taskrun` from frostmail, with the `make task`, `accept`,
  `task-verify` and `task-finish` targets, and `design/agent-workflow.md`.
- Three Codex calibration cards: a frontmatter splitter that keeps bytes
  exact, a wikilink and tag scanner against fixtures, and `notesctl vault`
  output formatting.
- **Done when:** Codex passes at least two of the three cards without help,
  and the results are recorded here.

## Done when

`make check` is green on the host and `make ui-check` in nsl; the app shows
data from notesd on the desktop and in a browser; the spikes are go; and
Codex has passed its calibration cards. Next: the M1 plan and its cards.

## Open questions

- **Copy or vendor frostmail's code:** copy with history notes in the
  commit message, or a one-time `git subtree`? Copying is the default.

## References

- Implements: [design/overview.md](../design/overview.md); [ADR-0008](../adr/0008-generate-the-rpc-contract-from-a-yaml-idl.md), [ADR-0009](../adr/0009-development-environment-and-gates.md), [ADR-0010](../adr/0010-claude-plans-and-codex-runs-task-cards.md), [ADR-0011](../adr/0011-look-like-frostmails-sibling-with-obsidians-layout.md)
- Roadmap: [plan 0002](0002-roadmap.md)
