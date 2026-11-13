# Organization ADRs binding frostnotes

Decisions from [frostyard/core](https://github.com/frostyard/core/tree/main/docs/adr) this repository follows:

- ADR-0071: the repository baseline (core's `docs/specs/repository-baseline.md`, rows B1 to B17); `internal/workflowcheck` checks the files of rows B1, B2, B6 and B13.
- ADR-0002, ADR-0018, ADR-0029: `AGENTS.md` is canonical; `CLAUDE.md`, `GEMINI.md`, `.cursorrules` and `.github/copilot-instructions.md` are relative symlinks to it, and `.claude/skills` is a symlink to `.agents/skills/`. `.memory/corrections/` is the corrections inbox.
- ADR-0012, ADR-0053: svu versions with `.svu.yml`; `make bump` tags `svu next`.
- ADR-0016: reverse-DNS identifiers; the app ID is `org.frostyard.Frostnotes`.
- ADR-0019, ADR-0068, ADR-0078: risk tiers in the PR template; `.github/risk-paths.json` (schema version 2: each glob has a level `L1` to `L4`, and the default is `L3`) and the PR triage caller `.github/workflows/risk-paths.yml` label a PR with `level:L1` to `level:L4` and with `risk:high` at L1 and L2. ADR-0078 amends ADR-0068 in part.
- ADR-0076: every default branch requires status checks.
- ADR-0077: `ready-for-human` is bound to the head commit; the PR triage caller removes the label when the head moves after the label, or when CI is not green at it.
- ADR-0021: CI actions pinned to full commit SHAs; least-privilege workflow permissions; `persist-credentials: false` (`internal/workflowcheck`).
- ADR-0038 (supersedes ADR-0022): unit and race runs filter no test names ([ADR-0035](adr/0035-unit-runs-filter-no-test-names.md)).
- ADR-0023: downloads are version-pinned and checksum-verified (`dev/nsl/provision.sh`).
- ADR-0025: one `docs/` tree in the four-category shape.
- ADR-0043, ADR-0044: tools pinned in `mise.toml` with `mise.lock`; the `make verify` / `check` / `ci` gate triad.
- ADR-0058: the desktop app ships as a Flatpak, notesd for lodge as a package that stages under `usr/`.
- ADR-0070: a tool documentation site in `site/` on GitHub Pages (not built yet).
- ADR-0072: the root `README.md` follows core's README spec (`docs/specs/readme.md`, baseline row B17) as the `app` kind; until the site exists, its documentation line links `docs/README.md`.
- ADR-0081: each correction is 1 file in `.memory/corrections/`, in canonical form (amends ADR-0018 and ADR-0071 in part). `internal/workflowcheck` (`make verify`) checks each file's name, content and form, and fails while the old single corrections file exists. It reads only the working tree, so it does not catch a removed, renamed or changed record.
