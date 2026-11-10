# Development

How to build Frostnotes from source, run it, and run its gates. The
rules for changes are in [AGENTS.md](../AGENTS.md); the architecture
starts at [design/overview.md](design/overview.md). The reasons for this
setup are in [ADR-0009](adr/0009-development-environment-and-gates.md).

## Requirements

- **On the host:** Go 1.27 (`go.mod` names the toolchain) and
  `mise install`, which installs the tools pinned in `mise.toml`
  (golangci-lint, svu, nfpm).
- **For the app:** an [nsl](https://github.com/frostyard/nsl) machine. The
  app's toolchain (Node, pnpm, Rust) lives only there
  (`dev/nsl/provision.sh`); never run pnpm, node or cargo on the host.

## Run notesd and notesctl from source

```sh
make build                              # build/notesd and build/notesctl
build/notesd &                          # the daemon; one per user
build/notesctl vault add ~/Notes        # registering never writes to the vault
build/notesctl search quince
claude mcp add frostnotes -- "$PWD/build/notesctl" mcp   # or: mcp --read-only
```

Every write, an agent's included, is kept as a version for 30 days:
`notesctl versions PATH` lists them and `notesctl revert` undoes one
([design/vault.md](design/vault.md#versions)).

## Run the native desktop app

The native app ([plan 0009](plans/0009-mygo-desktop.md)) is pure Go, and needs
GTK 3, Pango and cairo at run time, not at build time. Its tests draw headless
and need only Pango, cairo and a font.

```bash
make desktop-run
```

`make desktop-build` makes the release binary, `make desktop-smoke` starts it
under Xvfb against a real notesd, and `make desktop-evidence` records the
10,000-note and phone-to-desktop evidence. `FROSTNOTES_SCREENS=DIR go test
-run TestEveryScreenDraws ./internal/desktop/window` writes each screen's
picture to `DIR`.

## Set up the nsl machine

```sh
nsl create frostnotes --distro debian:13
nsl run -m frostnotes --root bash dev/nsl/provision.sh "$(id -un)"
```

The Makefile's app targets run in the machine named `frostnotes`
(`NSL_MACHINE` changes it).

## Gates and dev loops

```sh
make check          # host: generated files, vet, lint, unit tests
make ci             # make verify, the race tests and the app checks (nsl)
make ui-check       # nsl: Biome, typecheck and UI tests
make app-run        # nsl: notesd plus the built app
make ui-dev         # nsl: notesd and Vite; open http://127.0.0.1:5173
```

`make help` lists every target. [AGENTS.md](../AGENTS.md#gates) says
which gate a change needs, and which integration runs a change to the
scan, the write path or parsing needs.
