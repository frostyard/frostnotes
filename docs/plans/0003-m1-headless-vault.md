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
Status: **Phases 1 to 3 done** (2026-10-16): this plan,
[specs/markdown.md](../specs/markdown.md), cards T-0004 to T-0008
merged, and the index contracts with cards T-0009 to T-0020.

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
- **Deleting a note unlinks it and keeps its versions.** `note.delete`
  removes the file and records a deletion version; its earlier versions
  stay in `blobs/` for 30 days, and `note.revert` brings it back, with its
  file ID, at its last path. notesd never moves a note to `.trash/`: that
  would put its own state in the vault
  ([ADR-0003](../adr/0003-plain-markdown-files-are-the-source-of-truth.md)),
  and `.trash/` stays ignored. A delete names its base like any write; a
  note changed since is not deleted, because an edit beats a delete
  ([ADR-0005](../adr/0005-sync-through-a-hub-with-revisions-and-three-way-merges.md),
  as the hub does in [sync.md](../design/sync.md#hub)). These follow from
  ADR-0003, ADR-0005 and ADR-0006, so no new ADR records them.
- **`notesctl write` reads stdin, or `--file`, and `--base` is required.**
  A `--base` that defaulted to the indexed hash would make the write blind:
  it would overwrite whatever changed since the person last read the note.
  `notesctl cat --json` shows the hash; `--create` makes a new note.
- **A write's version records who made it:** `source` (`editor` or
  `agent`) and `author` are optional write parameters. The author defaults
  to the client name the connection gave `rpc.hello`, which Phase 4 keeps
  per connection (`api.Conn`); `notesctl mcp` passes the MCP client's name.
- **A writer's losing side is kept.** When a merge's conflicting hunk
  keeps the file's newer side, the content the writer sent is stored as a
  conflict version (`versions.conflict`), so nothing typed is lost
  ([ADR-0005](../adr/0005-sync-through-a-hub-with-revisions-and-three-way-merges.md)'s
  conflict revision, locally).
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
- **Result (2026-10-16): done.** Codex passed all five cards on the first
  attempt (the user ran T-0004; the planner ran the rest and reviewed each
  before merging it into `m1/cards`). `make obsidian-links` passes: the
  merged `Resolve` agrees with Obsidian on 179 of 179 links.

  | Card | Attempts | Files | Review |
  | --- | --- | --- | --- |
  | T-0004 resolve | 1 | 1 | matches the spec rule for rule |
  | T-0005 blobs | 1 | 1 | correct; atomic publish with the shard fsync |
  | T-0006 walk | 1 | 2 | correct; its `**` matcher avoids the reference's backtracking |
  | T-0007 moment | 1 | 1 | correct; longest match over four-byte prefixes |
  | T-0008 vaultgen | 1 | 1 | links correct, but the vaults are not realistic: see below |

  T-0008 met every given test, yet at 666 notes it writes 9 folders,
  notes of about 300 bytes that all share one paragraph, and two
  attachments, where the user's vault has 97 folders and notes of about
  3.3 KB. The given tests checked that each pattern appears and each link
  resolves, not scale or variety: a card's tests must pin every property
  its goal needs. The `tools/vaultgen` card in batch 2 also makes vaults
  realistic, with tests for folder count, note size and distinct text.

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
- **Result (2026-10-16): done.** `internal/markdown` has `Parse`, `Doc`,
  goldmark v1.8.6 (GFM, footnotes, an empty Obsidian extension, and its
  link parser wrapped to record where links end), and `fixtures_test.go`,
  which reads the spec's 14 `F` fixtures and compares a `Doc` part by part.
  T-0002's 35 cases moved into T-0011's given test, over `Parse`. The
  migration has the index tables; `note.yaml` the 12 methods and 3
  durable events; `vault.yaml` `status`, `reindex` and a `scan` event; the
  engine answers each new method `unavailable` until Phase 4, and
  `rpctest.Options.Note` lets client tests use a fake. Store signatures
  are stubs in `files.go`, `index.go`, `search.go`, `queries.go` and
  `versions.go`. The twelve cards were checked in order, each with its
  given tests and a reference in place on top of the batch-1 cards, and
  `make verify` green at every step; the references are deleted. Writing
  references settled three spec rules: a tag may start a block's text
  (`>#tag`, `|#tag|`), as goldmark reports it; a markdown link with an
  empty destination is not a link; a multi-line setext heading's text is
  its lines joined with a space. Checking caught, in the planner's given
  tests, four test names starting with `TestI` (`repocheck` refused
  them), a YAML comment taken for a tag, a link's end off by one, three
  expected rows with an extra column, two setups that added links after
  their targets, and a prune case that gave the kept file a newer
  version. `go mod tidy -diff` caught goldmark required as indirect.

## Phase 4 — Engine (planner, alongside Phase 5)

- `internal/vault`: the scan (walk, compare size and mtime, hash, parse,
  index in batches, moves), the watcher and rescans with network
  detection, the path guard (vault-relative paths only; no `..`, no dot
  segments except `.frostnotes/`, no symlinks), atomic writes, the write
  path with merges and versions, link re-resolution when a file whose name
  could match is created, moved or deleted, settings and daily notes
  ([vault.md](../design/vault.md)).
- The `note` domain in `internal/engine`, replacing the stubs that answer
  `unavailable`, and `vault.status` and `vault.reindex`; notesd opens
  every vault at start, one actor per vault; the daily pass that prunes
  versions (`store.PruneVersions`) and the blobs it frees, older than 30
  days. `api.Conn` gains the client name `rpc.hello` gave, the default
  author of a write.
- Daily notes expand their template with `internal/templates` (T-0013) and
  the vault's date format from `.frostnotes/vault.yaml` or
  `.obsidian/daily-notes.json`.
- Tests: the writer pairs of ADR-0006 that exist locally (an agent or
  `notesctl` against an outside edit, two writers on one base); a `kill -9`
  harness that re-runs the test binary as a child notesd, kills it at
  random points of writes and scans, and restarts it (`make vault-it`); a
  generated vault whose index holds exactly its manifest's links, each
  resolved to the manifest's file and none of its decoys.
- **Done when:** those tests pass, and `TestIObsidianLinkDump` also checks
  that `Parse` finds each note's links in the dump.

## Phase 5 — Executor batch 2

Cards written in Phase 3; each runs after the batch-1 cards it builds on
are merged.

| Card | Size | Package | What | Runs after |
| --- | --- | --- | --- | --- |
| T-0009 | M | `internal/vaultgen`, `tools/vaultgen` | realistic vaults (folders, note sizes, prose, attachments, unquoted dates) and the command (`-out`, `-notes`, `-seed`, `-manifest`) | T-0008 |
| T-0010 | M | `internal/markdown` | frontmatter: props, property errors, aliases, tags, property links | |
| T-0011 | M | `internal/markdown` | the goldmark extension: wikilinks, embeds, tags, math; comments; replaces `scan.go` | T-0010 |
| T-0012 | M | `internal/markdown` | markdown links, headings, block IDs, tasks, words; every spec fixture | T-0011 |
| T-0013 | S | `internal/templates` | `{{title}}`, `{{date}}`, `{{time}}`, formats and offsets | T-0007 |
| T-0014 | M | `internal/store` | files, and re-resolving the links a file's arrival, move or deletion affects | T-0004 |
| T-0015 | M | `internal/store` | `IndexNote`: one parsed note's rows | T-0014 |
| T-0016 | S | `internal/store` | `SearchQuery` and `Search` with snippets | T-0015 |
| T-0017 | M | `internal/store` | listing by folder, tag and property; note info; links and backlinks | T-0015 |
| T-0018 | S | `internal/store` | versions: add, list, a deleted file's, prune | T-0014 |
| T-0019 | M | `cmd/notesctl` | `--vault`, `ls`, `cat`, `search`, `backlinks` | |
| T-0020 | M | `cmd/notesctl` | `write`, `mv`, `rm`, `versions`, `revert` | T-0019 |

- Cards in one package run one after another, in the order above
  ([agent workflow](../design/agent-workflow.md#batching-cards)). The
  store cards' given tests build `markdown.Doc` values by hand, so they do
  not wait for the parser cards; the notesctl cards test against a fake
  note service, so they do not wait for Phase 4.
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

## References

- Implements: [design/vault.md](../design/vault.md), [specs/markdown.md](../specs/markdown.md), [specs/rpc-api.md](../specs/rpc-api.md) (`note` and `vault`); [ADR-0003](../adr/0003-plain-markdown-files-are-the-source-of-truth.md), [ADR-0005](../adr/0005-sync-through-a-hub-with-revisions-and-three-way-merges.md), [ADR-0006](../adr/0006-every-write-names-the-version-it-changes.md), [ADR-0012](../adr/0012-agents-reach-notes-through-mcp.md)
- Cards: [T-0009 to T-0020](../tasks/todo/)
- Evidence: [plan 0001, spikes 2–4](0001-m0-foundations.md#phase-5--spikes-go-or-no-go)
- Workflow: [design/agent-workflow.md](../design/agent-workflow.md)
- Roadmap: [plan 0002](0002-roadmap.md)
