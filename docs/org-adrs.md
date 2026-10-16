# Organization ADRs binding frostnotes

Decisions from [frostyard/core](https://github.com/frostyard/core/tree/main/docs/adr) this repository follows:

- ADR-0002, ADR-0029: `AGENTS.md` is canonical; `CLAUDE.md`, `GEMINI.md` and `.github/copilot-instructions.md` are symlinks.
- ADR-0016: reverse-DNS identifiers; the app ID is `org.frostyard.Frostnotes`.
- ADR-0021: CI actions pinned to full commit SHAs; least-privilege workflow permissions.
- ADR-0022: the `TestI` test-name prefix is reserved for integration tests.
- ADR-0023: downloads are version-pinned and checksum-verified (`dev/nsl/provision.sh`).
- ADR-0025: one `docs/` tree in the four-category shape.
- ADR-0043, ADR-0044: tools pinned in `mise.toml` with `mise.lock`; the `make verify` / `check` / `ci` gate triad.
