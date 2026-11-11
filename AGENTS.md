# frostyard/frostnotes

Frostnotes is a notes app in the spirit of Obsidian: plain markdown files
with YAML frontmatter, a native desktop app (Go, MyGo, frostui), a web app
(React, CodeMirror 6 live preview), built-in sync through a self-hosted hub,
and an MCP server for agents. A Go daemon, `notesd`, owns every vault; the apps,
`notesctl` and MCP are thin clients. Start at [docs/README.md](docs/README.md);
the architecture entry point is [docs/design/overview.md](docs/design/overview.md).

This file (`AGENTS.md`) is the CANONICAL agent instructions — `CLAUDE.md`,
`GEMINI.md`, `.cursorrules` and `.github/copilot-instructions.md` are
relative symlinks to it, and `.claude/skills` symlinks to `.agents/skills/`
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
  Its evidence through lodge (Phase 4, `make porter-evidence`) was
  recorded on 2026-10-28. Porter's token on the hub, issued at the
  move on 2026-10-26, was checked live on 2026-10-29, and the user
  accepted it the same day. So Phase 4, and with it plan 0007, is
  done.
- **Phase 7** moved the user's hub behind lodge on 2026-10-26; the 7
  days of sync through lodge run to 2026-11-02.

Next, [plan 0009](docs/plans/0009-mygo-desktop.md) moves the desktop
app to a native Go app on MyGo and frostui, and keeps the React app as
lodge's web app ([ADR-0038](docs/adr/0038-draw-the-desktop-app-natively-with-mygo-and-frostui.md),
accepted on 2026-11-10). It is built on a draft pull request (2026-11-10):
session, source mode, reading view, history and merges, the window on
frostui's shell, the workspace and the panes. Phase 5's live preview draws
marks, blocks and embeds; heading sizes and completed-task strike-through use MyGo's per-line styles
([egoist/mygo#1651](https://github.com/egoist/mygo/issues/1651)). The draft
pull request's last commit replaces the Tauri shell.

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
  `internal/packagecheck` tests that tree. `make deb` packs it into
  `frostyard-notesd`'s .deb with nFPM (`packaging/nfpm.yaml`, checked by
  `packaging/deb-check.py`, which reads the archive's exact member names); `make bump` tags a release, and
  `.github/workflows/release.yml` builds, attests and publishes it.
  `internal/synctest`: sync's seeded simulation of a hub and devices
  (`make sync-sim`; reduced failures in its `testdata/regressions`).
  `internal/evidence`: real notesd processes for the milestones' exit
  evidence; `internal/webevidence`: M4's, Firefox ESR through
  geckodriver in lodge's development build (`make m4-evidence`,
  `scripts/lodge-e2e.sh`), and plan 0007's Porter run through lodge's
  listener and agent socket (`make porter-evidence`).
- `cmd/frostnotes`, `internal/desktop/`: the native desktop app (plan 0009).
  `window` joins the rest under frostui's shell; `session` is the notesd
  connection; `notes` documents, saves and merges (no MyGo import); `livepreview`
  what live preview hides and draws (no MyGo import); `editor` source mode and
  live preview on MyGo's code editor; `reading`, `history` and `panes` their
  views; `workspace`, `commands` and `prefs` plain data; `catalog` a vault's
  files; `system` paths, notesd's log, the background setting and the open
  rules; `uikit` tab strips and splits (imports only std, MyGo, purego and
  frostui); `desktoptest` the tests' UI loop and real notesd. `make
  desktop-build`, `desktop-run`, `desktop-smoke` (Xvfb) and `desktop-evidence`.
- `internal/sandbox`: a Flatpak sandbox's file grants (`/.flatpak-info`) and the
  explanation of a file error they cause; the engine uses it for `vault.add`
  ([design/app.md](docs/design/app.md#the-flatpak)).
- `internal/mcpserver`: the MCP tools `notesctl mcp` serves
  ([docs/specs/mcp.md](docs/specs/mcp.md)), a client of notesd's API; the
  only package that imports the MCP SDK.
- `internal/merge`: three-way merges (planner-owned; never a card's).
- `internal/problem`: the codes notesd records in the local journal and the
  `Recorder` interface the packages get in their configs; `problemtest`
  holds a fake and a test journal socket. notesd's recorder
  (`github.com/frostyard/std/problems`) is made in `cmd/notesd`, and the
  desktop app's in `cmd/frostnotes`
  ([ADR-0036](docs/adr/0036-notesd-and-the-app-record-problems-in-the-journal.md)).
- `internal/workflowcheck`, `internal/repocheck`: tests of repository rules
  (SHA-pinned actions; no test-name filter on a unit run; the baseline
  files, below).
- `app/`: lodge's web app, React + TypeScript. JSON-RPC multiplexing is in
  `app/src/rpc/transport.ts`, with WebSocket (`transport/ws.ts`) and
  mock (`rpc/mock/`) transports. `app/src/features/` is presentational;
  `app/src/app/` and `app/src/data/` connect it. `app/src/lodge/` runs the
  app inside lodge's frame (lodge-client, the lodge transport, content
  URLs), and `app/src/features/mobile/` holds the narrow layout's
  components. `make web-build` builds it for lodge (`/app/notes/`), and
  `make lodge-app` lays it out with `packaging/lodge/app.json`. `app/src/spike/` is M0's
  CodeMirror spike. The icon's master is `packaging/icons/icon.svg`
  (`scripts/render-icons.py`, ADR-0013).
  `app/patches/` holds pnpm patches of dependencies and `app/src/styles/preflight.css`
  the app's copy of Tailwind's preflight, each for a measured cost (ADR-0025).
- `packaging/flatpak/`: the desktop app's Flatpak (manifest, launcher,
  Background-portal helper, AppStream, smoke test and the generated
  `go-sources.json`); `scripts/flatpak-sources.sh` (`make flatpak-sources`)
  regenerates it with `tools/flatpakgo`;
  `stamp-version.sh` stamps a tag's version into the manifest and the
  metainfo; CI's `flatpak.yml` builds it and publishes the image to GHCR on a
  `v*` tag
  ([ADR-0031](docs/adr/0031-the-desktop-app-ships-as-a-flatpak-with-its-own-notesd.md)).
  Run `make flatpak-sources` after a change to `go.mod`.
- `dev/nsl/provision.sh`: the nsl machine for the web app's toolchain;
  `scripts/dev-app.sh` runs notesd beside Vite there;
  `scripts/move-hub-behind-lodge.sh` moves the M2 hub behind lodge
  (plan 0006, Phase 7; a dry run unless `--apply`).
  `tools/rpcgen`, `tools/taskrun`: the contract generator and the task-card
  runner.

## Gates

- `make check` formats, then runs `make verify`: tidy, `gen-check`, vet,
  gofmt, the pinned golangci-lint (`mise install`) and unit tests. Run it
  before claiming any Go change is complete. `make ui-check` (Biome, tsc,
  Vitest) runs in the nsl machine; `make ci` is verify plus that,
  `make desktop-build` and the race detector. Never run pnpm or node on the
  host. `make vault-it` (the `kill -9` harness) and
  `make obsidian-links` (Obsidian's link dump, local only) are the
  integration runs for changes to the scan, the write path or parsing.
  `make vulncheck` runs govulncheck (pinned in the Makefile; needs the
  network). It is not part of `check`, `verify` or `ci`; the CI job
  "Vulnerabilities (make vulncheck)" runs it.

- Unit and race runs filter no test names (frostyard/core ADR-0038, [ADR-0035](docs/adr/0035-unit-runs-filter-no-test-names.md)): integration tests carry the `integration` build tag instead; `internal/repocheck` fails if a unit run in the Makefile or a workflow gets `-run` or `-skip`.
- Fix a lint finding rather than loosening `.golangci.yml`.

## Releases and dependencies

- **Tags** are `v1.2.3` (`.svu.yml`: prefix `v`, v0 mode). `svu next` reads
  Conventional Commits; on v0 a breaking change gives a minor.
- **A release starts** when a `v*` tag is pushed. `.github/workflows/release.yml`
  checks the tag, builds the web app and `frostyard-notesd`'s .deb (`make deb`, nFPM),
  attests it, and makes the GitHub release with `gh release create`. `flatpak.yml`
  also runs on the tag and publishes the Flatpak image to GHCR.
- **Brian tags.** Agents never tag, push a tag or release. `make bump` is the
  only sanctioned path: from a clean `main` equal to `origin/main` it runs
  `make check`, tags `svu next` and pushes the tag.
- **Go modules** (`go.mod`): `github.com/frostyard/clix` v0.5.0 (the CLI shell of
  `notesctl`), `github.com/frostyard/lodge/kit` v0.4.0 (the RPC and app runtime),
  `github.com/frostyard/std` v0.5.0 (`problems`, notesd's problem records;
  also clix's), and for the native desktop app (plan 0009)
  `github.com/frostyard/frostui` v0.3.0 (frostyard's desktop UI kit) over
  `github.com/egoist/mygo` v0.9.0, replaced by the owner's fork
  `github.com/bketelsen/mygo` (v0.9.0 with egoist/mygo#1662, per-line styles
  and the Markdown view's task actions and anchors) until those are in
  a release (plan 0009). Run `make flatpak-sources` after a bump: it rewrites
  `packaging/flatpak/go-sources.json`.
- **App packages** (`app/package.json`): `@frostyard/lodge-client` as
  `github:frostyard/lodge#client/v0.3.2&path:client` and `@frostyard/lodge-ui` as
  `github:frostyard/lodge#ui/v0.6.0&path:ui`.
- **Tool pins** (`mise.toml`): golangci-lint 2.15.0, svu 3.4.1, nfpm 2.48.0
  (these are not frostyard modules). The only bketelsen module is the MyGo
  fork above.
- **Who uses this repository:** lodge hosts `notesd --lodge` as its `notes` app
  (`packaging/lodge/app.json`); Porter calls lodge's `/mcp/notes` with a
  folder-scoped token, and lodge forwards it to `/mcp` on notesd's app socket
  ([docs/specs/mcp.md](docs/specs/mcp.md)). Other users: not stated.

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

## Repository baseline

The repository follows core's baseline (frostyard/core ADR-0071, its spec
`docs/specs/repository-baseline.md`, rows B1 to B17; [org-adrs.md](docs/org-adrs.md)).
`internal/workflowcheck` fails `make verify` when one of these files breaks:

- The instruction symlinks above (B1). Edit `AGENTS.md`, never a link.
- `.memory/corrections.jsonl` (B2): 1 JSON object per line with exactly
  `date`, `scope`, `correction`, `evidence` and `promoted_to`. Append only:
  never edit or remove a line (no test checks this). A fix of a blocking
  review point adds its guard and 1 line.
- `.github/risk-paths.json` (B6, core ADR-0068, ADR-0078): schema version
  2, where each glob has a level, `L1` to `L4`, and the default level is
  `L3`. Its caller `.github/workflows/risk-paths.yml` runs core's
  `pr-triage.yml` (ADR-0077, ADR-0078): it adds `level:L1` to `level:L4`,
  adds `risk:high` at L1 and L2, and removes `ready-for-human` when the
  head moves after the label or CI is not green at the label. A PR body
  names each matched path and why the change is safe. A new release
  workflow, install script, privileged helper, secrets store, auth check or
  sanitizer joins the list in the PR that adds it, at a level by the
  grading rule (`L1` for the risk categories, `L2` for governance); a glob
  that matches no file fails.
- `.github/dependabot.yml` (B13): 1 entry for each manifest (`go.mod`,
  `app/package.json`). A new manifest gets an entry.

`.github/pull_request_template.md` (B7) holds the risk tier. Vulnerabilities
are reported privately ([SECURITY.md](SECURITY.md), B14). No `CODEOWNERS`
file (B15), and no other agent framework's tree (B16): the task-card loop
(`docs/tasks/`, ADR-0010) and `opencode.json` are this repository's own.
The root `README.md` (B17, core ADR-0072) follows core's README spec as
the `app` kind: a short front page that links into `docs/`. Build and
development detail goes in [docs/development.md](docs/development.md), not
the README. No test here checks it: core's
`.agents/skills/frostyard-readme/scripts/check-readme.mjs` must print `ok`.

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
