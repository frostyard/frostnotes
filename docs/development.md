# Development

How to build Frostnotes from source, run it, and run its gates. The
rules for changes are in [AGENTS.md](../AGENTS.md); the architecture
starts at [design/overview.md](design/overview.md). The reasons for this
setup are in [ADR-0009](adr/0009-development-environment-and-gates.md).

## Requirements

- **On the host:** Go 1.27 (`go.mod` names the toolchain) and
  `mise install`, which installs the tools pinned in `mise.toml`
  (golangci-lint, svu, nfpm).
- **For the web app:** an [nsl](https://github.com/frostyard/nsl) machine. Its
  toolchain (Node, pnpm) lives only there (`dev/nsl/provision.sh`); never run
  pnpm or node on the host.

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

On a desktop, Frostnotes has no window: an editor of Markdown files,
Obsidian among them, opens the vault's folder, and notesd syncs what it
writes. The setup is `notesctl`'s, as in the Flatpak
([design/app.md](design/app.md#the-flatpak)):

```sh
build/notesctl hub join URL Notes       # pair with a hub (approve the code there) and sync its vault "Notes"
build/notesctl hub pair URL             # pair again, after the hub revoked this device
build/notesctl status                   # notesd, its vaults, their sync and the hubs
build/notesctl sync status              # one line per synced vault
```

## The Flatpak and the web build

Frostnotes has no desktop window ([ADR-0050](adr/0050-frostnotes-has-no-desktop-window.md)). The Flatpak carries notesd and `notesctl` only, and its launcher starts notesd, asks the desktop to start it at login, and prints `notesctl status` ([design/app.md](design/app.md#the-flatpak)). The released ref is `app/org.frostyard.Frostnotes/x86_64/master`.

Lodge's web app is the React app in `app/` ([ADR-0050](adr/0050-frostnotes-has-no-desktop-window.md)). `make web-build` builds it in nsl into `build/web` for `/app/notes/`, and `make lodge-app` lays it out with its manifest in `build/lodge/apps/notes`. The release Debian package (`make deb`), `make lodge-package` and the Flatpak workflow's Lodge archive all take that build; the package's check still fails on any file of the removed NSGo web build. `make m4-evidence` runs the build in Firefox inside Lodge's development build. To run it on what a package installs, extract the package and pass the tree:

```sh
dpkg-deb -x dist/frostyard-notesd_*_amd64.deb build/deb-x
make m4-evidence M4_NOTES_WEB=build/deb-x/usr/share/lodge/apps/notes
```

The Flatpak workflow builds notesd and `notesctl` offline from the root module's lock. Run `make flatpak-sources` after it changes. CI verifies the source revision in both downloaded artifacts, checks that the bundle holds notesd and `notesctl` and no window, and exports the same Flatpak ref as OCI. `make bump` publishes the next version from clean main. CI attests the OCI digest before moving `latest`. The existing Frostyard catalog reads that tag.

The Flatpak's smoke test runs without a Flatpak, on temporary directories and with stand-ins for the portals; it never touches your own notesd, config or keyring:

```sh
mkdir -p build/flatpak-bin
CGO_ENABLED=0 go build -o build/flatpak-bin/ ./cmd/notesd ./cmd/notesctl
install -m755 packaging/flatpak/frostnotes.sh build/flatpak-bin/frostnotes
FROSTNOTES_BIN_DIR=$PWD/build/flatpak-bin sh packaging/flatpak/smoke-test.sh
```

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
make ci             # make verify, the race gate and the app checks (nsl)
make race           # host: the race gate alone, in the 3 parts CI runs as shards
make ui-check       # nsl: Biome, typecheck and UI tests
make ui-dev         # nsl: notesd and Vite; open http://127.0.0.1:5173
```

`make help` lists every target. [AGENTS.md](../AGENTS.md#gates) says
which gate a change needs, and which integration runs a change to the
scan, the write path or parsing needs.
[ADR-0048](adr/0048-the-race-gate-runs-in-3-parts.md) says why the race
gate has 3 parts (`make race-rest`, `race-sim` and `race-sim-checkptr`).
