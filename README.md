# Frostnotes

A notes app in the spirit of Obsidian, from Frostyard. Notes are plain
markdown files with YAML frontmatter, in a folder you own
([roadmap](docs/plans/0002-roadmap.md)).

- **notesd** is a Go daemon that owns your vaults: it watches the files,
  keeps a SQLite index (full text, links, backlinks, tags, properties), and
  never writes over an edit it has not seen.
- **Sync is built in.** Each device syncs with a self-hosted hub, the same
  notesd in a hub role, which keeps every revision and merges concurrent
  edits. No conflict copies.
- **The desktop app** is Tauri v2 with a React UI and a CodeMirror 6 editor
  with live preview, a sibling of [Frostmail](https://github.com/frostyard/frostmail).
- **The web app** is the same UI, served by
  [lodge](https://github.com/frostyard/lodge), frostyard's tailnet-only
  web shell, with Obsidian's phone layout on a narrow screen.
- **Agents** search, read and write notes through MCP: `notesctl mcp`
  locally, or lodge's `/mcp/notes` with a scoped token.
- **Obsidian vaults open as they are:** wikilinks, embeds, tags, callouts
  and properties.

Pre-release. M0 (foundations), M1 (the headless vault), sync (M2) and the
desktop app (M3) are done; M2's and M3's 7-day trials ended on 2026-10-24
and 2026-10-25. M4, notes inside lodge, is in progress: notesd
runs as lodge's `notes` app, and the hub moved behind lodge on
2026-10-26. See the [roadmap](docs/plans/0002-roadmap.md).

## Try it

```sh
make build                              # build/notesd and build/notesctl
build/notesd &                          # the daemon; one per user
build/notesctl vault add ~/Notes        # registering never writes to the vault
build/notesctl search quince
claude mcp add frostnotes -- "$PWD/build/notesctl" mcp   # or: mcp --read-only
```

Every write, an agent's included, is kept as a version for 30 days:
`notesctl versions PATH` lists them and `notesctl revert` undoes one.

## Problem records

notesd and the desktop app write their panics, and a few coded errors of
notesd (a database that does not migrate, a write that does not reach the
disk, a failed merge, a lost watch event, a sync protocol fault), to the
local systemd journal. They send nothing over a network. A record holds the
app ID (`org.frostyard.Frostnotes`), its release version, the runtime (Go
or Rust), the part (`daemon` or `gui`), the class (`panic` or `error`), the
error's code or the panic's frames or place in the source, and a message
that can name a note's path and stays on your machine. A development build
records nothing. Only chairlift can send a record off the machine, and only
with your consent
([ADR-0036](docs/adr/0036-notesd-and-the-app-record-problems-in-the-journal.md)).
To see them:

```sh
journalctl MESSAGE_ID=8915bdd215c84cbab0c31c05a1a33e7e
```

## Develop

Requirements: Go 1.27 and `mise install` (golangci-lint) on the host;
[nsl](https://github.com/frostyard/nsl) for the app toolchain.

```sh
nsl create frostnotes --distro debian:13
nsl run -m frostnotes --root bash dev/nsl/provision.sh "$(id -un)"
make check          # host: generated files, vet, lint, unit tests
make ui-check       # nsl: Biome, typecheck and UI tests
make app-run        # nsl: notesd plus the built app
make ui-dev         # nsl: notesd and Vite; open http://127.0.0.1:5173
```

`make help` lists every target. Architecture starts at
[docs/design/overview.md](docs/design/overview.md); agents start at
[AGENTS.md](AGENTS.md).

## License

MIT.
