# frostyard/frostnotes

Frostnotes is a notes app in the spirit of Obsidian: plain markdown files
with YAML frontmatter, a desktop app (Tauri v2, React, CodeMirror 6 live
preview), a web app, built-in sync through a self-hosted hub, and an MCP
server for agents. A Go daemon, `notesd`, owns every vault; the apps,
`notesctl` and MCP are thin clients. Start at [docs/README.md](docs/README.md);
the architecture entry point is [docs/design/overview.md](docs/design/overview.md).

This file (`AGENTS.md`) is the CANONICAL agent instructions — `CLAUDE.md`,
`GEMINI.md`, and `.github/copilot-instructions.md` are symlinks to it, and
`.claude/skills` symlinks to `.agents/skills/`
([ADR-0002](docs/adr/0002-agent-portable-instruction-surface.md)). Edit only
the canonical paths; keep content tool-agnostic.

## Status

M0 (foundations) is done ([plan 0001](docs/plans/0001-m0-foundations.md)),
and so is M1, the headless vault ([plan 0003](docs/plans/0003-m1-headless-vault.md)).
M2, sync ([plan 0004](docs/plans/0004-m2-sync.md)), and M3, the desktop
app ([plan 0005](docs/plans/0005-m3-desktop-app.md); the app's design is
[docs/design/app.md](docs/design/app.md)), are done: their 7-day trials
ended on 2026-10-24 and 2026-10-25, and the user accepted both on
2026-10-25. M4, notes
inside lodge (frostyard's tailnet web shell), is in progress
([plan 0006](docs/plans/0006-m4-notes-inside-lodge.md); notesd's side
of lodge's app contract is
[docs/specs/lodge-app.md](docs/specs/lodge-app.md)):
- **Built:** Phases 2 to 5 (notesd as lodge's notes app, `/mcp` and the
  web app), merged on 2026-10-19, with their evidence and the security
  review recorded in plan 0006.
- **Also built:** [plan 0007](docs/plans/0007-folder-scopes-for-agents.md)'s
  Phases 2 and 3, which limit an agent's writes to folders and let it
  append to today's daily note, for Porter, frostyard's assistant
  ([ADR-0028](docs/adr/0028-an-agents-scope-can-limit-writes-to-folders.md)).
  Its evidence through lodge (Phase 4) is still to come.
- **Phase 7** moved the user's hub behind lodge on 2026-10-26; the 7
  days of sync through lodge run to 2026-11-02.

The roadmap is [plan 0002](docs/plans/0002-roadmap.md). Frostnotes copies
its machinery from [frostyard/frostmail](https://github.com/frostyard/frostmail)
(a sibling checkout at `../frostmail` when present): read the frostmail
file a step names before writing its frostnotes counterpart.

## Two kinds of agent work

- **Planning and core work** (ADRs, designs, specs, schemas, the merge
  library, sync, security boundaries, reviews): follow this file and the
  docs ([ADR-0010](docs/adr/0010-claude-plans-and-codex-runs-task-cards.md)).
- **Task cards** (`docs/tasks/`, run by `make task T=NNNN` with Codex):
  follow [docs/tasks/EXECUTOR.md](docs/tasks/EXECUTOR.md) and the card. The
  card's `touch` list is the only set of files you may change.
  [docs/design/agent-workflow.md](docs/design/agent-workflow.md) explains
  the loop.

## Map

- `cmd/notesd`: the daemon. `cmd/notesctl`: the CLI (clix/cobra; one
  `newXxxCmd(opts)` per file).
- `api/`: the RPC contract. `zz_generated.go` comes from `schema/rpc/*.yaml`
  via `tools/rpcgen` (`make gen`); `api.go`, `client.go` and `conn.go`
  name lodge kit's runtime (`lodge/kit/jsonrpc`: Router, Client, Error)
  and keep notes' caller kinds, scope and identity. Never edit
  generated files (`api/zz_generated.go`, `app/src/rpc/gen/api.ts`,
  `docs/specs/rpc-api.md`).
- `internal/store`: SQLite (modernc, pure Go): migrations, the single-writer
  `Tx`, the durable `changes` log, and the index (files, notes, links,
  versions, `notes_fts`) and its queries.
- `internal/markdown`: `Parse` (goldmark with an Obsidian extension) into
  a `Doc`, `SplitFrontmatter`, `Resolve`; the dialect and its fixtures are
  [docs/specs/markdown.md](docs/specs/markdown.md). `internal/events`: fans
  committed events out and replays the log. `internal/rpcserver`:
  `rpc.hello`, `events.subscribe` and the private socket over the kit's
  server, and `ServeConn` for authenticated connections.
  `internal/engine`: the API domains over the store and the vaults.
  `internal/rpctest`: a real server for tests, on `lodge/kit/rpctest`.
  `internal/devgw`: the loopback WebSocket gateway for UI work in a
  browser. `internal/config`: paths.
- `internal/vault`: one actor per vault (`Manager`): `Walk`, the scan, the
  fsnotify watcher, the path guard, atomic compare-and-merge writes,
  moves, settings, daily notes and the daily prune. Every change to a
  vault takes its lock (docs/design/vault.md#scan-and-watch).
  `internal/blobs`: the content-addressed store of every version.
- `internal/hub`: the hub role (pairing, the token check, the `/sync`
  WebSocket, the `log` and `hub` domains); `internal/syncer`: a device's
  sync loops and the `sync` domain; `internal/blobxfer`: blobs over HTTP
  by hash; `internal/secrets`: device tokens in the Secret Service
  ([docs/specs/sync-protocol.md](docs/specs/sync-protocol.md)).
  `internal/lodgeapp`: `notesd --lodge`, lodge's app socket over
  `lodge/kit` (`/health`, `/api` as caller `session`, `/content/blobs/`,
  `/mcp` for agents, `/device/`; [docs/specs/lodge-app.md](docs/specs/lodge-app.md));
  `internal/blobhttp`: files by hash for
  browsers. `packaging/`: the manifest, systemd units and `sysusers.d`, and
  `packaging/lodge/stage.sh` (`make lodge-package`), which stages them with
  `notesd` and `notesctl` in the `/usr` layout
  ([core ADR-0058](https://github.com/frostyard/core/blob/main/docs/adr/0058-ship-suite-desktop-apps-as-flatpaks-and-services-as-sysexts.md));
  `internal/packagecheck` tests that tree.
  `internal/synctest`: sync's seeded simulation of a hub and devices
  (`make sync-sim`; reduced failures in its `testdata/regressions`).
  `internal/evidence`: real notesd processes for the milestones' exit
  evidence; `internal/appevidence`: M3's, over the built app under Xvfb
  (`make m3-evidence`); `internal/webevidence`: M4's, Firefox ESR through
  geckodriver in lodge's development build (`make m4-evidence`,
  `scripts/lodge-e2e.sh`).
- `internal/mcpserver`: the MCP tools `notesctl mcp` serves
  ([docs/specs/mcp.md](docs/specs/mcp.md)), a client of notesd's API; the
  only package that imports the MCP SDK.
- `internal/merge`: three-way merges (planner-owned; never a card's).
- `internal/workflowcheck`, `internal/repocheck`: tests of repository rules
  (SHA-pinned actions; no unit test named `TestI…`).
- `app/`: Tauri v2 + React + TypeScript. `app/src-tauri/src/bridge.rs` only
  moves lines to notesd's socket; JSON-RPC multiplexing is in
  `app/src/rpc/transport.ts`, with Tauri, WebSocket (`transport/ws.ts`) and
  mock (`rpc/mock/`) transports. `app/src/features/` is presentational;
  `app/src/app/` and `app/src/data/` connect it. `app/src/lodge/` runs the
  app inside lodge's frame (lodge-client, the lodge transport, content
  URLs), and `app/src/features/mobile/` holds the narrow layout's
  components. `make web-build` builds it for lodge (`/app/notes/`), and
  `make lodge-app` lays it out with `packaging/lodge/app.json`. `app/src/spike/` is M0's
  CodeMirror spike and the drivers of M3's exit evidence. The icon's master is `app/src-tauri/icons/icon.svg`
  (`scripts/render-icons.py`, ADR-0013).
  `app/patches/` holds pnpm patches of dependencies and `app/src/styles/preflight.css`
  the app's copy of Tailwind's preflight, each for a measured cost (ADR-0025).
- `dev/nsl/provision.sh`: the nsl machine for the app toolchain;
  `scripts/dev-app.sh` runs notesd beside the app or Vite there;
  `scripts/move-hub-behind-lodge.sh` moves the M2 hub behind lodge
  (plan 0006, Phase 7; a dry run unless `--apply`).
  `tools/rpcgen`, `tools/taskrun`: the contract generator and the task-card
  runner.

## Gates

- `make check` formats, then runs `make verify`: tidy, `gen-check`, vet,
  gofmt, the pinned golangci-lint (`mise install`) and unit tests. Run it
  before claiming any Go change is complete. `make ui-check` (Biome, tsc,
  Vitest), `make app-build` and `make app-test` run in the nsl machine;
  `make ci` is verify plus those and the race detector. Never run pnpm,
  node or cargo on the host. `make vault-it` (the `kill -9` harness) and
  `make obsidian-links` (Obsidian's link dump, local only) are the
  integration runs for changes to the scan, the write path or parsing.
  `make vulncheck` runs govulncheck (pinned in the Makefile; needs the
  network). It is not part of `check`, `verify` or `ci`; the CI job
  "Vulnerabilities (make vulncheck)" runs it.

- Unit tests run with `-run '^Test[^I]'` (frostyard/core ADR-0022): never
  name a unit test `TestI…`; `internal/repocheck` fails if you do.
- Fix a lint finding rather than loosening `.golangci.yml`.

## Invariants (decided; binding from the first line of code)

- **Files are the truth.** A note is a `.md` file with YAML frontmatter.
  notesd writes byte for byte (no reformatting), atomically (temp file,
  fsync, rename), and keeps its own state (index, versions, sync state) in
  its data directory, never in the vault
  ([ADR-0003](docs/adr/0003-plain-markdown-files-are-the-source-of-truth.md)).
- **Every write names its base.** No API writes blind; a writer that is
  behind gets a three-way merge, never an overwrite
  ([ADR-0006](docs/adr/0006-every-write-names-the-version-it-changes.md)).
- **Between devices, only the hub merges**
  ([ADR-0005](docs/adr/0005-sync-through-a-hub-with-revisions-and-three-way-merges.md)).
- **Contract first:** API changes start in `schema/rpc/*.yaml`; generated
  files are never edited by hand
  ([ADR-0008](docs/adr/0008-generate-the-rpc-contract-from-a-yaml-idl.md)).
- **Rendered notes never run script:** notesd sanitizes, the app sanitizes
  again, and the CSP forbids inline script
  ([ADR-0007](docs/adr/0007-edit-in-codemirror-6-with-live-preview.md)).
- **SQLite never lives on a network file system;** vaults may.

## Skills (follow these for common tasks)

Step-by-step procedures live in [.agents/skills/](.agents/skills/); follow
them rather than improvising, whichever agent you are:

- *(none yet — add the first one when a procedure repeats)*

## Code conventions (live — the code exists)

- Go: `ctx` first; errors are lowercase, wrapped with `%w`; `slog` for logs;
  `encoding/json/v2`; no mutable package state; modern idioms
  (`t.Context()`, `t.Chdir`, `wg.Go`, `omitzero`, `b.Loop`). Tests use temp
  dirs and `rpctest`; no sleeps for synchronization.
- Every write goes through `store.DB.Tx`; emit durable events with `Tx.Emit`
  in the same transaction as the change (`internal/store/vault.go`).
- API changes: edit `schema/rpc/*.yaml` (every domain lists its `callers`),
  run `make gen`, implement. Changes within protocol 1 are additive only.
  Quote doc strings inside inline YAML `{...}` mappings.
- A method a caller kind may not use answers `methodNotFound`, exactly like
  one that does not exist; the router enforces it from the domain's
  `callers` (`api.Router.Dispatch`).
- SQLite databases never live on a network file system or `/mnt/host`.
- Downloads in scripts are version-pinned and checksum-verified
  (frostyard/core ADR-0023).

## Repository boundary

Never commit `build/`, `app/node_modules`, `app/dist`, Rust targets,
`.envrc`, credentials, device tokens, or real notes. Tests use generated
vaults (`tools/vaultgen`) and hand-written fixtures. Fixtures modeled on the
user's vault reproduce its link and folder patterns with made-up names;
nothing from it, file names included, is committed.
Use conventional commit subjects.

## Documentation rules (enforced)

Docs live in `docs/` in four categories (frostyard/core ADR-0025), plus
`docs/tasks/` for task cards from M0. **Every new doc starts from its
category's `TEMPLATE.md`** and follows its structure:

- `docs/adr/` — why we decided. Immutable once Accepted; reversals are new
  ADRs that mark the old one Superseded. Frostnotes is pre-release: a
  Proposed ADR may still be edited in place.
- `docs/design/` — how it fits together. Living; updated in place to match
  reality.
- `docs/specs/` — exact contracts. Change only alongside implementing code.
- `docs/plans/` — order of work. Phases with "Done when" outcomes.

### Cross-linking is mandatory

A doc without its required links is incomplete — do not finish a docs change
until they exist, in both directions:

- **ADR** → links every design doc/spec it shapes, and prior ADRs it builds on.
- **Design doc** → links the ADR(s) providing its rationale, the spec(s)
  pinning its contracts, and the roadmap phase that builds it.
- **Spec** → links its motivating ADR(s) and the design doc showing where it
  fits.
- **Plan** → every phase links the design docs/specs it implements; resolved
  open questions become ADRs.

When you touch a doc, verify its links still hold (targets exist, section
anchors valid) and add the back-links on the targets. Use relative paths.

### Housekeeping

- New doc ⇒ add a line to the index in [docs/README.md](docs/README.md).
- New significant decision ⇒ new ADR *first*, then update the affected design
  docs/specs in the same change.
- Convert relative dates ("next weekend") to absolute dates in all docs.
