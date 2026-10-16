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
- **The web app** is the same UI, served by the hub.
- **Agents** search, read and write notes through MCP: `notesctl mcp`
  locally, `/mcp` on the hub.
- **Obsidian vaults open as they are:** wikilinks, embeds, tags, callouts
  and properties.

Pre-release: M0 (foundations) is done; the vault engine (M1) is next.

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
