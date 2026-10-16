# Design: vault

Living document. Rationale: [ADR-0003](../adr/0003-plain-markdown-files-are-the-source-of-truth.md),
[ADR-0006](../adr/0006-every-write-names-the-version-it-changes.md).
Contracts: `specs/markdown.md` (M1), `specs/rpc-api.md` (generated).

**Status: designed, not built** (M1).

## Overview

A vault is a directory of markdown notes and attachments. notesd scans and
watches it, parses each note into the index, keeps every version it sees,
and performs every write as a compare-and-merge. Nothing notesd keeps lives
in the vault except the settings in `.frostnotes/`.

```
 vault dir ──scan/watch──▶ vault ──parse──▶ markdown ──▶ index (notesd.db)
     ▲                       │                            versions (blobs/)
     └──── temp, fsync, rename ◀── write(base) ◀── merge ◀── app, web, MCP, sync
```

## Design

### On disk

- **Vault:** notes (`*.md`), attachments (everything else), folders, and
  `.frostnotes/`: `vault.yaml` (daily-note folder, format and template;
  template folder; attachment folder; ignore patterns) and `types.yaml`
  (property types). Ignored: every other dot-directory, including
  `.obsidian/`, `.git/`, `.trash/` and `.stfolder/`.
- **Data directory** (`FROSTNOTES_DATA_DIR`): `notesd.db`, one SQLite
  database for all vaults (WAL, `STRICT` tables, foreign keys on), and
  `blobs/`, content addressed by SHA-256.
- **First open of an Obsidian vault:** `.obsidian/daily-notes.json`,
  `templates.json` and `types.json` seed `.frostnotes/` if it does not exist.

### Scan and watch

- **Open:** walk the vault; compare each file's size and mtime with the
  index; hash those that differ; parse and index changed notes; treat a
  missing file and a new file with the same hash as a move.
- **Watch:** fsnotify on every directory, debounced 100 ms per path. A
  periodic rescan catches what the watcher misses: every 5 minutes, or
  every 10 seconds when the vault is on a network file system (detected
  with `statfs`; SMB2 is `0xfe534d42`), where edits from other machines
  raise no events. Edits made on this machine do raise events there. If the
  inotify watch limit is reached, notesd logs it and falls back to rescans.
  Measured in M0 on the user's vault over SMB 3 (97 folders, 666 notes,
  2.2 MiB): a walk with stat takes about 85 ms, and reading every note 7 ms
  cached or 674 ms cold, so a rescan that reads only changed files is cheap.

- An outside change is indexed like any write: a version is kept and a
  durable `note.changed` event is emitted.

### Parse

`internal/markdown` turns a note into: frontmatter (YAML; invalid YAML is
recorded as a property error and the body is still indexed), aliases,
tags (frontmatter and inline, nested `#a/b`), links (wikilinks and
markdown links to vault files: target, subpath `#heading` or `#^block`,
alias, embed flag, position), headings, block IDs, tasks (status character
and line), word count, and the plain text for full-text search. The title
is the file name without `.md`, as in Obsidian.

### Index

Planned tables (the M1 migration is the contract): `vaults`, `files` (path,
kind, size, mtime, hash), `notes` (title, frontmatter as JSON, word count),
`props` (key with text, number and date columns for queries), `tags`,
`links` (raw target, resolved file or null, subpath, alias, embed, position),
`headings`, `blocks`, `tasks`, `versions` (file, hash, time, source) and an
FTS5 table over title, aliases and plain text.

### Link resolution

Obsidian's rules, pinned with fixtures in `specs/markdown.md` and checked
against Obsidian itself: a vault path when the link has one, else the note
whose name or alias matches, case-insensitively, with ties broken as
Obsidian breaks them. Links are re-resolved when a file whose name could
match is created, moved or deleted. Unresolved links are kept: the app and
the graph show them.

### Writes

`note.write(path, base, content)` and every other write:

1. Compare `base` with the indexed version. If it differs, merge three ways
   (`internal/merge`) with the base's content from `blobs/`.
2. Re-read the file. If it changed since the last scan, merge that change
   in too.
3. Write a temporary file in the same directory, fsync, rename, fsync the
   directory.
4. In one `Tx`: index the new content, append a version, emit
   `note.changed`.
5. Return the new revision and, if a merge happened, the merged content.

Moves (`note.move`) rename the file and, when asked, rewrite links in every
note that points at it (Obsidian's "update internal links"). Each rewrite is
an ordinary write with a base.

### Versions

Every version notesd writes or scans is kept in `blobs/`, and listed in
`versions` with its source (editor, outside, agent, sync), for 30 days by
default. A daily pass prunes unreferenced blobs. `note.versions` and
`note.revert` expose them; on a synced vault, the hub's log holds the
long-term history ([sync.md](sync.md)).

## Operational notes

- The database must be on a local file system; the vault may be on a
  network file system.
- Byte-exact writes: notesd never normalizes line endings, BOMs or trailing
  newlines.
- Symlinks are not followed out of the vault.
- A note too large to index usefully (over 10 MB) is indexed by name only.
- `notesctl index rebuild` drops the index tables and rescans; nothing is
  lost. Deleting `notesd.db` itself also loses local versions and sync
  state: the vault re-syncs from its hub, and `blobs/` is pruned on the
  next pass.

## References

- Rationale: [ADR-0003](../adr/0003-plain-markdown-files-are-the-source-of-truth.md), [ADR-0006](../adr/0006-every-write-names-the-version-it-changes.md)
- Related: [overview.md](overview.md), [sync.md](sync.md)
- Built in: [roadmap — M1](../plans/0002-roadmap.md)
