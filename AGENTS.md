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

Designed, not built. ADRs 0003–0012 are accepted;
[M0](docs/plans/0001-m0-foundations.md) is in progress. Frostnotes
copies its machinery from [frostyard/frostmail](https://github.com/frostyard/frostmail)
(a sibling checkout at `../frostmail` when present): read the frostmail
file a step names before writing its frostnotes counterpart.

## Two kinds of agent work

- **Planning and core work** (ADRs, designs, specs, schemas, the merge
  library, sync, security boundaries, reviews): follow this file and the
  docs ([ADR-0010](docs/adr/0010-claude-plans-and-codex-runs-task-cards.md)).
- **Task cards** (`docs/tasks/`, from M0 Phase 6): follow the card and
  `docs/tasks/EXECUTOR.md`. The card's `touch` list is the only set of
  files you may change.

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

No code exists yet. Rules graduate here as M0 lands the code that enforces
them, starting from frostmail's `AGENTS.md` conventions (Go: `ctx` first,
lowercase wrapped errors, `slog`, `encoding/json/v2`, no mutable package
state; every write through `store.DB.Tx` with its events emitted in the
same transaction).

- Run the project's full check (`make check`, from M0 Phase 1) before
  calling any change done. CI runs the same recipe, so a local pass is a CI
  pass.

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
