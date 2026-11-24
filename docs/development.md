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
  pnpm or node on the host. The native desktop app builds on the host.

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

## Try the shared NSGo editor

From the repository root on x86-64 Linux, build the native app and browser assets from the same source.

```sh
make nsgo-check
(cd cmd/frostnotes-nsgo && go tool nsgo build -target linux_x64,web -installers=false -channel none -output ../../build/nsgo)
go build -o build/ ./cmd/notesctl ./cmd/notesd
```

The Linux executable uses the same socket as `notesctl`. For a disposable trial, run the following together in Bash. The daemon, native client and static server run as jobs in this shell. The vault and database live under the printed temporary directory.

```bash
trial=$(mktemp -d)
mkdir "$trial/Notebook"
printf '# Field notes\n\nFirst line.\n' > "$trial/Notebook/Field notes.md"
export FROSTNOTES_DATA_DIR="$trial/db" FROSTNOTES_SOCKET="$trial/notesd.sock"
export FROSTNOTES_DEVGW_TOKEN="$(openssl rand -hex 16)"
build/notesd -devgw 127.0.0.1:46321 &
daemon=$!
for i in {1..100}; do [[ -S "$FROSTNOTES_SOCKET" ]] && break; sleep 0.1; done
build/notesctl vault add "$trial/Notebook"
build/nsgo/FrostNotes-0.1.0-linux_x64/FrostNotes &
native=$!
python3 -m http.server 5173 --bind 127.0.0.1 --directory build/nsgo/FrostNotes-0.1.0-web &
web=$!
printf 'Vault: %s\nOpen: http://127.0.0.1:5173/#token=%s\n' "$trial/Notebook" "$FROSTNOTES_DEVGW_TOKEN"
```

Open **Field notes** on both screens. Edit it in one window and press **Save**. The other editor should update in place. Reverse the direction. If both editors change the same line from the same starting version, the later writer should see the conflict notice. Inspect its version with `build/notesctl versions 'Field notes.md'`. To finish the trial, run `kill "$web" "$native" "$daemon"` in the original shell.

To try inline editing, add a heading, a task such as `- [ ] prune`, and a link to a second note such as `[[Garden|orchard]]`. Choose **Live preview**. Click the heading to edit its Markdown and use the arrow keys to move between lines. Click a rendered checkbox to toggle it, or the link text to open the note. **Source** returns to whole-note selection and editing. The mode carries over when you open another note in the same window.

The loopback hostname and port match the development gateway’s origin check. On a Lodge origin the browser opens the same-origin `/app/notes/api` WebSocket. The trial above exercises the loopback gateway. See the [preview decision](adr/0046-test-a-shared-nsgo-source-editor.md).

### Run the preview inside Lodge

With the FrostNotes nsl machine provisioned, run `make nsgo-lodge-evidence`. It stages the preview in `build/nsgo-lodge/apps/notes` and starts the pinned Lodge development gateway and a real `notesd` with a disposable vault. Firefox opens Notes from the Lodge shell. The test removes its fixture node from the allow-list, types a draft offline, writes to the same line over the Unix socket, and readmits the node. It checks the editor, the file and the exact conflict version, then writes a screenshot under `build/nsgo-lodge/evidence`.

The same evidence target checks inline editing in Firefox. It compares the checkbox edit, heading edit and undo with the file on disk, then follows a wiki link.

For a separate Lodge test installation, `make nsgo-lodge-app` builds the app tree alone. The gateway needs [Lodge’s Wasm frame policy](https://github.com/frostyard/lodge/pull/116). The preview uses the existing Notes manifest and app ID inside this separate staging tree.

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
make desktop-run    # host: the native app against the running notesd
make ui-dev         # nsl: notesd and Vite; open http://127.0.0.1:5173
```

`make help` lists every target. [AGENTS.md](../AGENTS.md#gates) says
which gate a change needs, and which integration runs a change to the
scan, the write path or parsing needs.
