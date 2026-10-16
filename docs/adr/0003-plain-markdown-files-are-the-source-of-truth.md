# 0003 — Plain markdown files are the source of truth

- **Status:** Accepted
- **Date:** 2026-10-16

## Context

Frostnotes is a notes app in the spirit of Obsidian: a desktop app, a web
app, built-in sync and an MCP server for agents. The user's notes today are
an Obsidian vault of 666 notes, 577 of them with YAML frontmatter, synced by
Obsidian Sync. A second vault, once synced with Syncthing, still holds a
Syncthing conflict copy. Notes must outlive the app and stay editable by
other tools: a text editor, git, grep, Obsidian itself. The user's vault
lives on a network home directory today.

## Decision

- **A vault is a directory.** A note is a UTF-8 `.md` file with optional
  YAML frontmatter at its top, which holds all of its metadata. Every other
  file is an attachment. Folders are folders.
- **Frostnotes adds one directory to a vault:** `.frostnotes/`, holding
  YAML settings that travel with the vault (daily-note folder and format,
  template folder, property types, ignore patterns). It syncs like notes.
- **notesd changes only what it is asked to change, byte for byte.** It
  never reformats frontmatter, line endings or trailing newlines. A write is
  a temporary file in the same directory, fsync, then rename.
- **notesd's own state lives outside the vault**, in its data directory:
  the SQLite index (full text, links, tags, properties, headings, blocks,
  tasks), local versions
  ([ADR-0006](0006-every-write-names-the-version-it-changes.md)) and sync
  state ([ADR-0005](0005-sync-through-a-hub-with-revisions-and-three-way-merges.md)).
  The index is derived: notesd can drop and rebuild it from the files at
  any time without losing anything.
- **The dialect is Obsidian-flavored markdown:** CommonMark and GFM plus
  wikilinks (`[[Note]]`, `[[Note|alias]]`, `[[Note#Heading]]`,
  `[[Note#^block]]`), embeds (`![[…]]`), inline `#tags` and frontmatter
  `tags` and `aliases`, block IDs, callouts, `==highlights==`, `%%comments%%`
  and math. Links resolve by Obsidian's rules. The exact grammar is pinned
  in a spec with shared fixtures (M1).
- **An Obsidian vault opens as it is.** `.obsidian/` is ignored, except that
  its daily-notes and templates settings seed `.frostnotes/` on first open.
  Other dot-directories (`.git/`, `.trash/`, `.stfolder/`) are ignored too.

## Consequences

- Any tool can read and edit notes. notesd must notice their edits (a
  watcher, and a scan at start) and treat them like its own
  ([ADR-0006](0006-every-write-names-the-version-it-changes.md)).
- Two parsers read the markdown: goldmark in notesd for the index and
  rendering, Lezer in the editor for live preview
  ([ADR-0007](0007-edit-in-codemirror-6-with-live-preview.md)). Shared
  fixtures keep them agreeing on links, tags, headings and blocks.
- A vault may sit on a network file system; the database never does (WAL
  needs shared memory). Remote edits on a network file system raise no
  inotify events, so notesd also rescans; with built-in sync the user can
  keep each device's vault on local disk instead.
- Identity across renames cannot come from the file. It is sync state
  ([ADR-0005](0005-sync-through-a-hub-with-revisions-and-three-way-merges.md)).
- Property types are vault settings, as Obsidian keeps them in
  `.obsidian/types.json`; YAML alone does not say whether `2026-10-16` is a
  date or text.

## Alternatives considered

- **A database as the truth** (Notion, Apple Notes, Bear): faster queries
  and simpler sync, but notes become hostage to the app and no other tool can
  edit them.
- **Our own markdown dialect:** loses the user's vault and every Obsidian
  vault.
- **An ID in every note's frontmatter:** makes renames trivial, but writes
  our bookkeeping into the user's files and fails for files other tools
  create.
- **The index inside the vault** (as Obsidian keeps its cache in
  `.obsidian/`): breaks on network file systems and syncs a derived file.

## References

- Shapes: [design/overview.md](../design/overview.md), [design/vault.md](../design/vault.md)
- Builds on: [ADR-0001](0001-record-architecture-decisions.md)
- Built in: [roadmap — M1](../plans/0002-roadmap.md)
