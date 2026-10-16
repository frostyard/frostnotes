# Design: vault

Living document. Rationale: [ADR-0003](../adr/0003-plain-markdown-files-are-the-source-of-truth.md),
[ADR-0006](../adr/0006-every-write-names-the-version-it-changes.md).
Contracts: [specs/markdown.md](../specs/markdown.md), [specs/rpc-api.md](../specs/rpc-api.md) (generated).

**Status: being built** in M1 ([plan 0003](../plans/0003-m1-headless-vault.md)).

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
- **An Obsidian vault's settings:** while `.frostnotes/` lacks a setting,
  notesd reads it from `.obsidian/daily-notes.json`, `templates.json` or
  `types.json`, and writes `.frostnotes/` only when a setting changes, so
  registering a vault writes nothing to it.

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

`internal/markdown` parses a note with goldmark (CommonMark and GFM) and
a Frostnotes extension for the Obsidian syntax, as
[specs/markdown.md](../specs/markdown.md) pins it, into: frontmatter (YAML; invalid YAML is
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

Obsidian's rules, as [specs/markdown.md](../specs/markdown.md#link-resolution)
pins them and Obsidian's own answers for 179 of the user's links confirm:
relative to the linking note's folder first, then a whole-segment path
suffix, ignoring case; among several suffix matches the note's own
folder wins first. Aliases
do not resolve links; they name a note in search and link suggestions.
`files` keeps each path's lower-cased last segment, which is all
`markdown.Resolve` asks for. Links are re-resolved when a file with a name
they could match is created, moved or deleted. Unresolved links are kept:
the app and the graph show them.

### Writes

`note.write(path, base, content)` and every other write:

`base` is the SHA-256 of the content the writer started from (`note.read`
returns it); M2 adds sync's revisions. A vault's writes run one at a time.

1. Compare `base` with the indexed version. If it differs, merge three ways
   (`internal/merge`) with the base's content from `blobs/`. A base whose
   content is gone (pruned) is refused, and the writer reads again.
2. Re-read the file. If it changed since the last scan, merge that change
   in too.
3. Put the result in `blobs/`, then write a temporary dot-file in the same
   directory (so scans skip it), fsync, rename, fsync the directory.
4. In one `Tx`: index the new content, append a version, emit
   `note.changed`.
5. Return the new revision and, if a merge happened, the merged content.

Moves (`note.move`) rename the file and, when asked, rewrite links in every
note that points at it (Obsidian's "update internal links"). Each rewrite is
an ordinary write with a base.

A crash after the rename and before the `Tx` loses only the attribution:
the next scan indexes the new bytes as an outside version.

### Daily notes

The daily-note settings are a folder, a moment.js date format and a
template, from `.frostnotes/vault.yaml` or else
`.obsidian/daily-notes.json`, and Obsidian's defaults without either (the
vault root, `YYYY-MM-DD`, no template). Today's note is the folder plus the
date formatted with `internal/moment`; a format may contain `/` and so
name subfolders. A missing daily note is created from the template, whose
`{{title}}`, `{{date}}`, `{{time}}` and `{{date:FORMAT}}` are expanded as
Obsidian's Templates plugin does. MCP's `append_to_daily_note` creates it
that way, then appends with a base like any write.

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
- Contracts: [specs/markdown.md](../specs/markdown.md), [specs/rpc-api.md](../specs/rpc-api.md)
- Built in: [plan 0003 — M1](../plans/0003-m1-headless-vault.md) ([roadmap](../plans/0002-roadmap.md))
