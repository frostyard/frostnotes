# Design: vault

Living document. Rationale: [ADR-0003](../adr/0003-plain-markdown-files-are-the-source-of-truth.md),
[ADR-0006](../adr/0006-every-write-names-the-version-it-changes.md),
[ADR-0005](../adr/0005-sync-through-a-hub-with-revisions-and-three-way-merges.md)
for deletes and conflict versions, and
[ADR-0014](../adr/0014-the-api-carries-text-and-keeps-bytes-it-cannot-show.md)
for content that is not UTF-8, and
[ADR-0015](../adr/0015-index-what-comments-hide-as-obsidian-does.md) for
comments, and
[ADR-0017](../adr/0017-revisions-number-the-hubs-log-and-hashes-name-content.md)
for what sync changes in writes.
Contracts: [specs/markdown.md](../specs/markdown.md), [specs/rpc-api.md](../specs/rpc-api.md) (generated), [specs/mcp.md](../specs/mcp.md) (agents' writes).

**Status: built** in M1 ([plan 0003](../plans/0003-m1-headless-vault.md),
Phases 3 to 5): `internal/vault` scans, watches and writes, and
`internal/engine` serves the `note` and `vault` domains over it. MCP
(Phase 6) is next.

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
  registering a vault writes nothing to it. M1 has no method that changes
  a setting, so it only reads them (`vault.LoadSettings`), at each scan
  and each `note.daily`:

  ```yaml
  # .frostnotes/vault.yaml; every key is optional
  daily:
    folder: Journal          # "" or absent: the vault root
    format: YYYY-MM-DD       # moment.js; may contain "/" to name subfolders
    template: Templates/Daily  # a note's path, ".md" optional
  templates:
    folder: Templates
    dateFormat: YYYY-MM-DD   # {{date}} in templates
    timeFormat: HH:mm        # {{time}}
  ignore: ["Archive/", "*.tmp"]  # gitignore-style; no negation
  ```

  A `vault.yaml` that does not parse fails the scan, as an unreadable
  folder does; an Obsidian file that does not parse is skipped, since it
  only gives defaults.

### Scan and watch

- **One actor per vault.** notesd opens every registered vault at start
  (`vault.Manager`; `vault.add` and `vault.remove` reload it, one at a
  time). Each vault has a goroutine that scans and watches it, and one lock
  that every change to the vault takes: a write, a scan's batch, the
  watcher's paths. Reads of the index take no lock. A vault being removed
  finishes the changes it is making and refuses new ones before its row
  goes.
- **Open:** walk the vault; compare each file's size and mtime with the
  index; read and hash those that differ; parse and index changed notes;
  treat a missing file and a new file of the same kind with the same hash
  as a move (paired in path order), keeping the file's ID. Reads, and the
  blobs they store, run 8 files at a time: on a network share each read
  waits on the server, and fsyncs made together cost less. Changes commit
  in batches of at most 200 files per `Tx`, each checked against the disk
  and the index again under the vault's lock, so a write that ran since
  the walk is never undone and `kill -9` leaves an index the next scan
  continues. A file that cannot be read keeps its rows; a folder that
  cannot be read fails the scan before anything changes, and so does a
  vault folder with no files while the index has some, which is what a
  network share that is not mounted looks like. A row whose file the walk
  leaves out although the disk has it (ignored by the settings now, or
  spelled otherwise on a file system that ignores case) leaves the index.
  A scan also removes the temporary files of writes a crash left behind
  (`.frostnotes-<16 hex digits>.tmp`). Its walk's folders are watched
  before it reads a file.
- **Watch:** fsnotify on every directory, debounced 100 ms per path; the
  paths due within half that window are handled together, so a rename's
  two paths are seen as one move. A path an event names is read and hashed
  whatever its stat says, since two edits of one size within the clock's
  granularity share an mtime. A folder created or moved in, a folder moved
  or deleted, a change under `.frostnotes/`, or an overflow of the event
  queue starts a scan instead. A periodic rescan catches what the watcher
  misses: every 5 minutes, or every 10 seconds when the vault is on a
  network file system (detected with `statfs`: SMB2 `0xfe534d42`, CIFS,
  SMB, NFS, 9p, Ceph and AFS), where edits from other machines raise no
  events. Edits made on this machine do raise events there. If the inotify
  watch limit is reached, notesd logs it and falls back to rescans.
  Measured in M0 on the user's vault over SMB 3 (97 folders, 666 notes,
  2.2 MiB): a walk with stat takes about 85 ms, and reading every note 7 ms
  cached or 674 ms cold, so a rescan that reads only changed files is cheap.
  A whole cold index of it takes 0.73 s (M1, Phase 7).

- An outside change is indexed like any write: a version is kept and a
  durable `note.changed` event is emitted.

### Parse

`internal/markdown` parses a note with goldmark (CommonMark and GFM) and
a Frostnotes extension for the Obsidian syntax, as
[specs/markdown.md](../specs/markdown.md) pins it. `markdown.Parse`
returns a `Doc`: frontmatter (YAML; invalid YAML is recorded as a
property error and the body is still indexed), aliases, tags (frontmatter
and inline, nested `#a/b`), links (wikilinks, property links and markdown
links to vault files: target, subpath `#heading` or `#^block`, alias,
embed flag, position), headings, block IDs, tasks (status character and
line) and word count. Comments are found after goldmark parses, so a
comment may span blocks. They hide text when a note is rendered, not from
the index: what lies inside one is indexed like the rest, as Obsidian
indexes it, and only the word count leaves it out
([ADR-0015](../adr/0015-index-what-comments-hide-as-obsidian-does.md)). The
title is the file name without `.md`, as in Obsidian.

### Index

The tables are in `internal/store/migrations/0001_init.sql`, each column
commented: `vaults`; `files` (path, lower-cased name for `Resolve`'s
lookup, kind, size, mtime in nanoseconds, hash), keeping its ID across
moves; `notes` (title, frontmatter as JSON, property error, aliases, word
count); `props` (one row per scalar value or list item, with its
lower-cased text and number, for listing by property); `tags`; `links`
(as parsed, with the file each resolves to or null, and the lower-cased
name of its target that decides when it must resolve again); `headings`,
`blocks`, `tasks`; `versions`; and `notes_fts`, an FTS5 table with stored
content over the title, aliases, the body as written and the frontmatter
text, so search returns snippets. `internal/store` reads and writes them
(`PutFile`, `IndexNote`, `Search`, `ListNotes`, `Backlinks`, …).

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
returns it). Sync keeps it so: revisions number the hub's log, and a
device applies what it pulls through `vault.Apply`, which compares the
file's hash instead of merging; on a hub, each write also appends a
revision in its `Tx` (`vault.logRevision`;
[ADR-0017](../adr/0017-revisions-number-the-hubs-log-and-hashes-name-content.md),
[sync.md](sync.md#device-loop)). A device's push on a hub is `vault.Push`,
which applies the protocol's rules through this same write path. A vault's writes run one at a time,
under the vault's lock. Paths pass the guard first: vault-relative and
slash-separated, no empty, `.` or `..` segment, no hidden segment, no
name over 255 bytes and no path over 4,000 (what file systems hold), and no
symlink on the way (every file operation goes through an `os.Root` of the
vault, and each parent is checked with `lstat`). The note domain also
refuses `.frostnotes/`, which holds settings, not notes, and paths the
settings ignore. A path spelled otherwise than the file on a file system
that ignores case is not a second file.

1. Re-read the file. If it changed since the last scan, index it first as
   an outside version, so it becomes the head the write merges into.
   `note.read` does the same, so the hash it returns is always a version.
2. Compare `base` with the indexed version. If it differs, merge three ways
   (`internal/merge`: `merge.Note` for a note, `merge.Text` for other
   text) with the base's content from `blobs/`. A base that is no version
   of the file, or whose content is gone (pruned), is refused, and the
   writer reads again; so is a stale base of a file that is not text. A
   base that is not valid UTF-8 first gets its stray bytes back where the
   writer left the words around them alone (ADR-0014). When the merge
   conflicts, the content the writer sent is kept as a conflict version
   at once, in a `Tx` of its own, before the file changes.
3. Put the result in `blobs/`, then write a temporary dot-file in the same
   directory (so scans skip it), keeping the file's mode, and fsync it.
   Read the file once more just before the rename: if it changed, discard
   the temporary file and start again from step 1, so the edit is merged.
   Then rename and fsync the directory. A create renames with
   `RENAME_NOREPLACE` where the file system has it (SMB does not; there it
   checks first).
4. In one `Tx`: index the new content, append a version, emit
   `note.changed`. Once the file has changed, this runs to the end even if
   the client has gone away.
5. Return the new hash and, if a merge happened, the merged content.

A write records its version's source (`editor` for a client, `agent` for
MCP) and author (the client's name, or the MCP client's). When a merge
leaves a conflicting hunk, the file's newer side stands and the content
the writer sent is kept as a conflict version, so nothing typed is lost.

Moves (`note.move`) rename the file and, when asked, rewrite links in every
note that points at it (Obsidian's "update internal links"), and the moved
note's own links that the move would point elsewhere, such as `./` links.
Each link gets the shortest target that resolves to its file, as
Obsidian's "shortest path when possible" writes it: the file name, else
the vault path, else the path from the root; a wikilink leaves out `.md`, a
markdown link keeps it, percent-encoded. Subpath, alias and embed mark stay.
Each rewrite replaces only the link's target, so links that nest, as a
clickable image's do, are rewritten both, and is an ordinary write with a
base. A move emits `note.moved`; the file keeps its ID and versions, and a
note's title follows its new name.

Deletes (`note.delete`) name their base too. A note changed since is not
deleted (an edit beats a delete, as on the hub). Otherwise notesd removes
the file, never moving it to `.trash/`, after reading it once more, and
records a deletion version;
the file's versions stay, so `note.revert` brings it back with its ID at
its last path. A file deleted by another program is recorded the same way,
as an outside deletion.

A crash after the rename and before the `Tx` loses only the attribution:
the next scan indexes the new bytes as an outside version.

### Daily notes

The daily-note settings are a folder, a moment.js date format and a
template, from `.frostnotes/vault.yaml` or else
`.obsidian/daily-notes.json`, and Obsidian's defaults without either (the
vault root, `YYYY-MM-DD`, no template). Today's note is the folder plus the
date formatted with `internal/moment`; a format may contain `/` and so
name subfolders. A missing daily note is created from the template, whose
`{{title}}`, `{{date}}`, `{{time}}`, `{{date:FORMAT}}` and offsets such as
`{{date-1d:YYYY-MM-DD}}` are expanded as Obsidian does
(`internal/templates`): `{{title}}` is the note's name, `{{date}}` the date
in the daily-note format, and the date carries the time of day it is now.
A template that cannot be read gives an empty note, as in Obsidian. MCP's
`append_to_daily_note` ([specs/mcp.md](../specs/mcp.md)) creates it that way, as the agent's version, then appends with a base like
any write.

### Versions

Every version notesd writes or scans is kept in `blobs/`, and listed in
`versions` with its source (editor, outside, agent, sync) and author, for
30 days by default; a deletion is a version too. A file's newest version
is kept however old, as the base of its next merge, and so is the version
holding a file's current content when a conflict version is newer. A daily
pass prunes
older versions and removes the blobs no version or file names, those a
crash left unnamed included, with every vault's lock held so no write can
name a blob it removes. `note.versions` and
`note.revert` expose them; on a synced vault, the hub's log holds the
long-term history ([sync.md](sync.md)).

## Operational notes

- The database must be on a local file system; the vault may be on a
  network file system.
- Byte-exact writes: notesd never normalizes line endings, BOMs or trailing
  newlines.
- Symlinks are not followed out of the vault.
- A note too large to index usefully (over 10 MB) is indexed by name only,
  and the API neither reads nor writes a file that large.
- `make vault-it` kills a child notesd at random points of writes and of
  scans over a generated vault, 100 times each, and checks after each
  restart that every file holds a complete version notesd knows and that
  the index equals a fresh rebuild (`store.IndexSnapshot`).
- `vault.reindex` reads every file again and rebuilds the index, keeping
  file IDs and versions; nothing is lost. Deleting `notesd.db` itself also loses local versions and sync
  state: the vault re-syncs from its hub, and `blobs/` is pruned on the
  next pass.
- `schema_migrations` records each applied migration's SHA-256, and
  `store.Open` compares them with the migrations notesd embeds. Pre-release,
  `0001_init.sql` is edited in place, so a database that applied an earlier
  text of it would run without the tables the code expects; notesd refuses
  to start on it, naming the database and the migration. Move such a
  database aside (with any `-wal` and `-shm` files) and notesd creates a new
  one, losing what deleting it loses. A database from before checksums were
  recorded adopts them once its schema equals what its migrations create in
  an empty database, and is refused otherwise.

## References

- Rationale: [ADR-0003](../adr/0003-plain-markdown-files-are-the-source-of-truth.md), [ADR-0005](../adr/0005-sync-through-a-hub-with-revisions-and-three-way-merges.md), [ADR-0006](../adr/0006-every-write-names-the-version-it-changes.md), [ADR-0014](../adr/0014-the-api-carries-text-and-keeps-bytes-it-cannot-show.md), [ADR-0015](../adr/0015-index-what-comments-hide-as-obsidian-does.md), [ADR-0017](../adr/0017-revisions-number-the-hubs-log-and-hashes-name-content.md)
- Related: [overview.md](overview.md), [sync.md](sync.md)
- Contracts: [specs/markdown.md](../specs/markdown.md), [specs/rpc-api.md](../specs/rpc-api.md), [specs/mcp.md](../specs/mcp.md)
- Built in: [plan 0003 — M1](../plans/0003-m1-headless-vault.md); sync's apply path in [plan 0004 — M2](../plans/0004-m2-sync.md) ([roadmap](../plans/0002-roadmap.md))
