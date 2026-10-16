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
- **Result (2026-10-16): done.** CI's Verify and Race jobs passed on PR
  frostyard/frostnotes#1. `internal/repocheck` fails any unit test named
  `TestI…`, which the ADR-0022 filter would skip silently; it caught
  frostmail's `TestImmutableReferences` and one of the new store tests.

## Phase 2 — Environment

- `dev/nsl/provision.sh` from frostmail's (Node, pnpm, Rust, WebKitGTK
  `-dev`, pinned and checksum-verified) for the nsl machine `frostnotes`
  ([ADR-0009](../adr/0009-development-environment-and-gates.md)).
- **Done when:** a fresh `nsl create frostnotes` plus the script builds the
  app shell.
- **Result (2026-10-16):** the machine provisioned on the first run (Node
  24.21.0, Go 1.27.1, Rust 1.99.0).

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
- **Result (2026-10-16):** `notesctl hello` works against a running notesd,
  and `make gen-check` passes. `vault.add` resolves symlinks and refuses
  vaults that contain or lie inside one another. The `notesctl vault`
  commands became calibration card T-0003 rather than planner work.

## Phase 4 — App shell

- The sibling notebook [application icon](../design/overview.md#application-icon)
  ([ADR-0013](../adr/0013-use-a-sibling-frostyard-notebook-icon.md)), with an
  editable SVG master and reproducible PNG exports.
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

Results (2026-10-16):

2. **Merge library shape: go.** `internal/merge` has `Text`, `Note` and
   `Create`; 30 golden cases and a 300-seed property test pass. Two
   findings changed the design ([sync.md](../design/sync.md#merge)): pure
   insertions at the same place must keep both sides, or two daily notes
   from one template conflict; and the diff must be linear-space, since a
   backtracking Myers diff of two unrelated 10,000-line files needs
   gigabytes. With the middle-snake algorithm, scattered edits in 10,000
   lines merge in about 2 ms; two unrelated 10,000-line files take 0.3 s
   and 3 MB.
3. **Watching the user's vault: go.** It is on an SMB 3 share (CIFS,
   `statfs` magic `0xfe534d42`). Local writes, including temp-and-rename,
   raise inotify events at once, and file and directory fsync succeed.
   Edits from other machines raise none, so notesd rescans: a walk with
   stat of 97 folders and 666 notes takes about 85 ms (reading every note:
   7 ms cached, 674 ms cold). The network rescan interval is 10 seconds
   ([vault.md](../design/vault.md#scan-and-watch)).
4. **Obsidian link resolution: pattern inventory done; Obsidian's answers
   pending.** In the vault, 125 file names are shared by two or more files
   (one by 13), so ties matter: 70 of 272 wikilinks are ambiguous by name,
   185 carry folder paths, 9 aliases, 5 heading links, and 150 name no
   existing file. 35 markdown links point at local files; 22 notes use
   callouts. Instead of 50 links checked by hand, the user runs a
   read-only snippet in Obsidian's developer console that records
   Obsidian's resolution of every link to a file outside the repository;
   M1's resolver is tested against it locally (an integration test gated on
   `FROSTNOTES_OBSIDIAN_LINKS`), and the patterns become `specs/markdown.md`
   fixtures with made-up names.

## Phase 6 — Task tooling and calibration

- `tools/taskrun` from frostmail, with the `make task`, `accept`,
  `task-verify` and `task-finish` targets, and `design/agent-workflow.md`.
- Three Codex calibration cards: a frontmatter splitter that keeps bytes
  exact, a wikilink and tag scanner against fixtures, and `notesctl vault`
  output formatting.
- **Done when:** Codex passes at least two of the three cards without help,
  and the results are recorded here.
- **Cards (2026-10-16):** T-0001 split frontmatter byte for byte, T-0002
  scan wikilinks and tags outside code, T-0003 `notesctl vault` commands;
  all size S. Writing them, the planner's own expected byte offsets were
  wrong four times and the reference solutions right, which is the case
  for checking given tests against a reference before a card ships.

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
