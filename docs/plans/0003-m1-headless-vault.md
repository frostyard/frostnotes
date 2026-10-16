# Plan 0003: M1 headless vault

M1 makes notesd a working engine for the user's vault before any UI
exists: it scans and watches the vault, indexes notes, resolves links as
Obsidian does, searches, and writes safely with local versions, driven
from `notesctl` and from an agent through `notesctl mcp`. It follows
[M0](0001-m0-foundations.md) and comes before M2's sync in the
[roadmap](0002-roadmap.md). Work is split per
[ADR-0010](../adr/0010-claude-plans-and-codex-runs-task-cards.md): the
planner owns the spec, the schema, scan and watch, the write path, merges
and the MCP surface; the executor owns cards T-0004 onward.
Status: **Phase 1 done** (2026-10-16): this plan,
[specs/markdown.md](../specs/markdown.md) and cards T-0004 to T-0008.

## Decisions taken for M1

- **Link resolution is measured, not guessed.** Obsidian's own answers for
  179 links from the user's vault (M0 spike 4) fit seven rules: relative
  to the linking note's folder first, then a whole-segment path suffix,
  case ignored, a folder path never falling back to the bare name, no
  fallback for `./` and `../`, and no alias resolution
  ([spec](../specs/markdown.md#link-resolution)). A reference resolver
  written from the spec agreed with all 179 on 2026-10-16. The tie order
  beyond "in the source's folder" has no example in the dump and is the
  spec's choice.
- **Aliases do not resolve links.** They name a note in search and link
  suggestions, as in Obsidian; [vault.md](../design/vault.md#link-resolution)
  said otherwise and is corrected.
- **goldmark parses notes for the index** (CommonMark and GFM, as ADR-0003
  planned), with a Frostnotes extension for the Obsidian syntax.
  CommonMark's code rules decide what is a link: 103 of the 268 wikilinks
  in the vault's note bodies are inside code, which is why a regex counted
  93 more links than Obsidian; outside code, with the local markdown links,
  there are 177 to Obsidian's 179, and the one note that differs has
  backslash-escaped backticks
  ([F03](../specs/markdown.md#f03--escaped-backticks)). M0's `Wikilinks`
  and `InlineTags` become the extension's inline parsers, and their 35
  cases join the spec's fixtures.
- **A write's base is the content hash** (SHA-256 hex) of the version the
  writer started from, which `note.read` returns. Revisions are M2's. A
  base whose content notesd no longer has (pruned after 30 days) is
  refused, and the writer reads again.
- **One writer per vault.** Writes to a vault are serialized. Each one:
  merge if the base is behind (`merge.Note`, head = the indexed version);
  re-read the file and merge an unscanned outside edit; put the result in
  `blobs/`; write a temporary dot-file in the same folder, fsync, rename,
  fsync the folder; then index it, append a version and emit
  `note.changed` in one `Tx` ([vault.md](../design/vault.md#writes)). A
  crash after the rename and before the `Tx` loses only the attribution:
  the next scan finds the new bytes as an outside version.
- **Scans commit in batches** of at most 200 files per `Tx`, so `kill -9`
  mid-index leaves a consistent index that the next scan continues. A
  missing file and a new file with the same hash are a move, keeping the
  file's ID.
- **Watching** uses fsnotify (already in the module, made direct) on every
  folder, debounced 100 ms per path, with rescans every 5 minutes, or 10 s
  when `statfs` says the vault is on a network file system (M0 spike 3).
  A folder that cannot be read fails the scan rather than deleting its
  notes from the index (T-0006).
- **Every version of every file** notesd writes or scans is kept in
  `blobs/` for 30 days, as ADR-0006 says, attachments included: M2 needs
  them as blobs anyway.
- **Search** is FTS5 with stored content, for snippets, over title,
  aliases, body and frontmatter text; `unicode61 remove_diacritics 2`; each
  query term quoted as a prefix phrase, as frostmail's `SearchQuery` does.
- **Obsidian's settings are read, not copied.** While `.frostnotes/` lacks
  a setting, notesd reads `.obsidian/daily-notes.json`, `templates.json`
  and `types.json` as defaults, and writes `.frostnotes/` only when a
  setting changes, so registering a vault still writes nothing to it
  ([vault.md](../design/vault.md#on-disk)). The user's vault has no
  `daily-notes.json`: Obsidian's defaults, the vault root and `YYYY-MM-DD`.
- **MCP** is `notesctl mcp`: a stdio server on the official Go SDK
  (`github.com/modelcontextprotocol/go-sdk`, a release at least two weeks
  old) that calls notesd over the socket like any client
  ([ADR-0012](../adr/0012-agents-reach-notes-through-mcp.md)). An edit tool
  reads the note, applies its change and writes with the hash it read, so a
  concurrent edit merges. Each write records the MCP client's name as its
  author.

## Phase 1 — Dialect and first cards (planner)

- [specs/markdown.md](../specs/markdown.md): notes and frontmatter, what
  the body's Obsidian syntax is and where it is not, `Parse`'s output, link
  resolution with a made-up fixture vault (35 cases) and 14 parse
  fixtures, each rule citing its evidence (the dump, CommonMark and GFM,
  Obsidian's docs, or a choice).
- Cards T-0004 to T-0008, each checked against a reference solution with
  its given files in place and `make verify` green, then the reference
  deleted.
- **Done when:** the spec is indexed and cross-linked, and each card has
  passed its reference.
- **Result (2026-10-16): done.** The resolution reference matched
  Obsidian on 179 of 179 links over the vault's 674 files. Checking cards
  caught five wrong word counts in the parse fixtures and six bugs in the
  planner's given tests: three wrong expectations, and three test names
  starting with `TestI`, which `make test` would have skipped.

## Phase 2 — Executor batch 1

| Card | Size | Package | What |
| --- | --- | --- | --- |
| T-0004 | S | `internal/markdown` | `Resolve`: the spec's link resolution, tested by its fixture table |
| T-0005 | M | `internal/blobs` | the content-addressed store: atomic, checked reads, listing, temp sweep |
| T-0006 | M | `internal/vault` | `Walk` and ignore patterns; never a partial listing |
| T-0007 | M | `internal/moment` | moment.js date formats for daily notes and templates |
| T-0008 | M | `internal/vaultgen` | generated vaults with a manifest of every link and its file |

- T-0008's given test calls `markdown.Resolve`, so it runs after T-0004
  merges; the others touch separate packages and may run in any order
  ([agent workflow](../design/agent-workflow.md#batching-cards)).
- After T-0004 merges, the planner adds `TestIObsidianLinkDump`
  (`//go:build integration`, `make obsidian-links`): with
  `FROSTNOTES_OBSIDIAN_LINKS` naming the dump and `FROSTNOTES_OBSIDIAN_VAULT`
  the vault, walked read-only for file names, it checks every resolution.
  Neither file enters the repository.
- **Done when:** the five cards are merged, `make check` is green, and
  `make obsidian-links` passes on the user's machine.

## Phase 3 — Index contracts (planner)

- `internal/markdown`: `Parse` and `Doc` as the
  [spec's output table](../specs/markdown.md#parse-output) says, goldmark
  (pinned) with the Obsidian extension's skeleton, and a fixture reader
  that runs the spec's `F` fixtures in Go.
- Migration `0001_init.sql`, edited in place (pre-release): `files` (vault,
  path, lower-cased name for `Resolve`'s lookup, kind, size, mtime, hash),
  `notes`, `props`, `tags`, `links` (raw target, subpath, alias, kind,
  embed, offsets, resolved file or null), `headings`, `blocks`, `tasks`,
  `versions` (file, hash, time, source, author) and the FTS5 table
  ([vault.md](../design/vault.md#index)).
- `schema/rpc/note.yaml` (callers `local`; `session` waits for M4): `list`,
  `read`, `search`, `backlinks`, `links`, `create`, `write`, `move`,
  `delete`, `versions`, `revert`, `daily`; durable `changed`, `moved` and
  `removed` events; `vault.reindex` and a scan status. Then `make gen`
  ([specs/rpc-api.md](../specs/rpc-api.md)).
- Store signatures as stubs with doc comments, and the batch-2 cards,
  each checked against a reference.
- **Done when:** `make check` is green with every stub in place, and each
  batch-2 card has passed its reference.

## Phase 4 — Engine (planner, alongside Phase 5)

- `internal/vault`: the scan (walk, compare size and mtime, hash, parse,
  index in batches, moves), the watcher and rescans with network
  detection, the path guard (vault-relative paths only; no `..`, no dot
  segments except `.frostnotes/`, no symlinks), atomic writes, the write
  path with merges and versions, link re-resolution when a file whose name
  could match is created, moved or deleted, settings and daily notes
  ([vault.md](../design/vault.md)).
- The `note` domain in `internal/engine`; notesd opens every vault at
  start, one actor per vault; the daily pass that prunes versions and
  blobs older than 30 days.
- Tests: the writer pairs of ADR-0006 that exist locally (an agent or
  `notesctl` against an outside edit, two writers on one base); a `kill -9`
  harness that re-runs the test binary as a child notesd, kills it at
  random points of writes and scans, and restarts it (`make vault-it`); a
  generated vault whose index holds exactly its manifest's links, each
  resolved to the manifest's file and none of its decoys.
- **Done when:** those tests pass, and `TestIObsidianLinkDump` also checks
  that `Parse` finds each note's links in the dump.

## Phase 5 — Executor batch 2

Planned cards, written in Phase 3:

- `tools/vaultgen`: the command over T-0008 (`-out`, `-notes`, `-seed`,
  `-manifest`; S).
- The parser: wikilinks, embeds and markdown links in the goldmark
  extension; tags, aliases and property links from frontmatter; headings,
  block IDs, tasks and words (three M cards against the spec's fixtures).
- Template expansion: `{{title}}`, `{{date}}`, `{{time}}` and
  `{{date:FORMAT}}` over T-0007 (S).
- Store queries: index one parsed note; search with snippets; backlinks,
  outgoing links and listing by folder, tag or property; versions (S and
  M).
- `notesctl ls`, `cat`, `search`, `backlinks`, `write`, `mv`, `versions`
  and `revert` (two M cards).
- **Done when:** every card is merged and `make check` is green.

## Phase 6 — MCP

- Planner: `internal/mcpserver` on the Go SDK: tool definitions and
  descriptions (note content is returned marked as data, and the
  descriptions say so), `--read-only`, author attribution, and
  `notesctl mcp` ([ADR-0012](../adr/0012-agents-reach-notes-through-mcp.md)).
- Executor: the handlers for `search_notes`, `read_note`, `list_notes`,
  `get_backlinks`, `create_note`, `edit_note` and `append_to_daily_note`.
- **Done when:** an SDK client test over an in-memory transport, against
  `rpctest`, runs every tool, and `--read-only` refuses every write.

## Phase 7 — Exit evidence

- **Done when**, each shown by a test or a recorded run:
  1. The user's vault (666 notes on SMB 3) indexes cold in under 5 s.
  2. Every link in Obsidian's dump (179 from 43 notes) is found by `Parse`
     in its note and resolves as Obsidian resolved it.
  3. A note saved in vim is searchable with `notesctl search` within 1 s.
  4. `kill -9` of notesd during writes and during indexing, 100 times
     each over a generated vault, never leaves a note that differs from
     every one of its versions, and the index after restart equals a fresh
     rebuild.
  5. Claude Code, through `notesctl mcp`, searches the user's vault, reads
     a note and appends to today's daily note; the append is in
     `notesctl versions`, attributed to the client, and `notesctl revert`
     undoes it.

## Later / ideas

- Checking that a heading or block subpath exists, for M3's link styling.
- Rewriting links when a folder moves (`note.move` handles files in M1).
- Substring search (FTS5 trigram) if prefix search proves too narrow.

## Open questions

- **Links in comments, and ties beyond the source folder:** does Obsidian
  index `[[x]]` inside `%%…%%`, and which of several suffix matches does it
  pick? A one-note test vault in the user's Obsidian answers both, by
  Phase 4; until then the spec's choices stand.
- **Deleting a note:** unlink it and keep its versions, or move it to
  `.trash/` as Obsidian can. By Phase 3.
- **`notesctl write` input:** stdin or `--file`, and whether `--base` may
  default to the indexed hash for a human at a terminal. By Phase 5.

## References

- Implements: [design/vault.md](../design/vault.md), [specs/markdown.md](../specs/markdown.md), [specs/rpc-api.md](../specs/rpc-api.md); [ADR-0003](../adr/0003-plain-markdown-files-are-the-source-of-truth.md), [ADR-0006](../adr/0006-every-write-names-the-version-it-changes.md), [ADR-0012](../adr/0012-agents-reach-notes-through-mcp.md)
- Evidence: [plan 0001, spikes 2–4](0001-m0-foundations.md#phase-5--spikes-go-or-no-go)
- Workflow: [design/agent-workflow.md](../design/agent-workflow.md)
- Roadmap: [plan 0002](0002-roadmap.md)
