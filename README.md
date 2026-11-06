<!-- readme-kind: app -->
<img src="app/src-tauri/icons/icon.svg" alt="Frostnotes icon" width="112" align="right">

# Frostnotes

[![CI](https://github.com/frostyard/frostnotes/actions/workflows/ci.yml/badge.svg)](https://github.com/frostyard/frostnotes/actions/workflows/ci.yml)

**Plain markdown notes on your own computer.**

Frostnotes is a desktop app for people who keep notes as plain markdown
files, in the spirit of Obsidian. The notes stay in a folder you own; a
daemon, notesd, indexes them, syncs them through a hub you host, and
serves them to a web app and to agents.

**Status:** pre-release. The [roadmap](docs/plans/0002-roadmap.md) says what works and what comes next.

**Documentation:** [docs/README.md](docs/README.md)

## What it does

- **Your files are the notes.** A note is a `.md` file with YAML
  frontmatter. notesd writes it byte for byte and keeps its own state out
  of the vault. [ADR-0003](docs/adr/0003-plain-markdown-files-are-the-source-of-truth.md)
- **notesd owns your vaults.** It watches the files, keeps a SQLite index
  (full text, links, backlinks, tags, properties), and never writes over
  an edit it has not seen. [Vault design](docs/design/vault.md)
- **Every write is a version.** Each write, an agent's included, is kept
  for 30 days: `notesctl versions PATH` lists them and `notesctl revert`
  undoes one. [Versions](docs/design/vault.md#versions)
- **Sync is built in.** Each device syncs with a self-hosted hub, the same
  notesd in a hub role, which keeps every revision and merges concurrent
  edits. No conflict copies. [Sync design](docs/design/sync.md)
- **The desktop app** is Tauri v2 with a React UI and a CodeMirror 6
  editor with live preview, a sibling of
  [Frostmail](https://github.com/frostyard/frostmail). [App design](docs/design/app.md)
- **The web app** is the same UI, served by
  [lodge](https://github.com/frostyard/lodge), frostyard's tailnet-only
  web shell, with Obsidian's phone layout on a narrow screen.
  [notesd as a lodge app](docs/specs/lodge-app.md)
- **Agents** search, read and write notes through MCP: `notesctl mcp`
  locally, or lodge's `/mcp/notes` with a scoped token. [MCP tools](docs/specs/mcp.md)
- **Obsidian vaults open as they are:** wikilinks, embeds, tags, callouts
  and properties. [Markdown dialect](docs/specs/markdown.md)

## Install

Frostnotes is on the [Frostyard Flatpak remote](https://github.com/frostyard/flatpak-index),
with its own notesd inside. Add the remote and Flathub (for the runtime)
once, for your user, then install Frostnotes and update it with
`flatpak update`:

```sh
flatpak remote-add --user --if-not-exists frostyard https://frostyard.github.io/flatpak-index/frostyard.flatpakrepo
flatpak remote-add --user --if-not-exists flathub https://dl.flathub.org/repo/flathub.flatpakrepo
flatpak install --user frostyard org.frostyard.Frostnotes
```

Other ways: each [release](https://github.com/frostyard/frostnotes/releases)
has `frostyard-notesd`'s .deb, which runs notesd and notesctl as lodge's
notes app ([notesd as a lodge app](docs/specs/lodge-app.md)), and
[docs/development.md](docs/development.md) builds everything from source.

## Remove

```sh
flatpak uninstall --user org.frostyard.Frostnotes
```

Your vaults stay: they are your folders. notesd's index, versions and
sync state stay in `~/.var/app/org.frostyard.Frostnotes/` unless you add
`--delete-data`. A device's sync token stays in the Secret Service, in an
item labeled `Frostnotes: …`.

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

```sh
mise install        # host: golangci-lint, svu and nfpm, pinned
make check          # host: generated files, vet, lint, unit tests
nsl create frostnotes --distro debian:13
nsl run -m frostnotes --root bash dev/nsl/provision.sh "$(id -un)"
make ci             # make verify, the race tests and the app checks (nsl)
```

[docs/development.md](docs/development.md) covers the requirements,
running notesd from source, and the app's dev loops. The architecture
starts at [docs/design/overview.md](docs/design/overview.md).

## Where to go next

- [AGENTS.md](AGENTS.md): the rules for agents and contributors.
- [docs/README.md](docs/README.md): the decisions, designs, specs and plans.
- [SECURITY.md](SECURITY.md): report a vulnerability privately.
- [LICENSE](LICENSE): MIT.
