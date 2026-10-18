# Plan 0005: M3 desktop app

M3 makes the desktop app the user's Obsidian: Obsidian's layout and
shortcuts in frostmail's look, a CodeMirror editor with live preview that
saves as the user types without ever overwriting another writer, a
reading view notesd renders, and the panes the user's Obsidian shows. It
follows [M2](0004-m2-sync.md), whose 7-day trial runs alongside it, and
comes before M4's web app in the [roadmap](0002-roadmap.md). Work is
split per [ADR-0010](../adr/0010-claude-plans-and-codex-runs-task-cards.md):
the planner owns the ADRs, the specs, the schema additions, notesd's new
methods, `internal/render` and its sanitizer, the blob scheme, the data
layer, the workspace model, the editor core (documents, saves, merges),
the live-preview framework, the reading view, the merge view, the command
registry and the containers; the executor owns cards T-0038 onward: the
panes, the quick switcher, the command palette, the widgets, settings,
history, daily notes and templates, and the keymap.
Status (2026-10-18): **Phases 1 to 5 done**, and the user accepted
ADRs 0021 to 0023 as written: this plan, the ADRs (accepted),
[design/app.md](../design/app.md), [specs/ui.md](../specs/ui.md), the
spec's tokens in the app's styles, and the batch-1 cards T-0038 to
T-0042, which Codex passed and which are merged. Phase 3, the contracts,
is done too, in three PRs against `main`: the API's contract with the
store's cards T-0043 and T-0044; the renderer's contract with its cards
T-0045 and T-0046; and the blob scheme. The mock's vault moved to Phase 5.
Codex has passed cards T-0043 to T-0046, the first of Phase 6's batch 3,
and they are merged. Phase 4, notesd's additions, is merged in two PRs.
Phase 5 is merged in four PRs: the data layer, the workspace and
commands, the editor core, and the reading view, merge view and history
with the exit evidence. Phase 6 is under way: batch 2 is merged in three
PRs (T-0047 to T-0063 and their wiring); batch 3's first part, the
editor's syntax, formatting commands, word counts and completion
(T-0064 to T-0069), its second, the live-preview widgets (T-0070 to
T-0075), and its last, properties, the inline title, templates and daily
notes (T-0076 to T-0079), are merged. What the spec still asked of Phase
6 (opening links and files outside the app, zoom, the Editor settings,
aliases, page previews, indentation guides and the reading view's
properties; T-0080 and T-0081) is merged. Phase 7, the review's fixes
and evidence 1 to 3, is in review: typing, the hotkeys and the writer
pairs meet their targets in the built app, and scrolling falls just
short of its own (a follow-up). Evidence 4 is the user's 7-day trial.
It waits for Phase 8, which lets the app open and create vaults
([ADR-0024](../adr/0024-the-app-opens-and-creates-vaults-and-a-window-switches-between-them.md),
accepted 2026-10-18): batch 2's vault switcher only ever switched
vaults.

## Decisions taken for M3

Three decisions extend the architecture, so they are ADRs, written in
Phase 1 before any code:

- **The workspace and app preferences stay on each device**
  ([ADR-0021](../adr/0021-keep-the-workspace-and-app-preferences-on-each-device.md)).
  The app keeps the layout (a tree of splits and tab groups, the
  sidebars, each tab's history, recent files) and its preferences in
  local storage, one versioned document per vault, validated on load,
  never in the vault and never in notesd. Tabs follow moves, even those
  made while the app was closed, by replaying events from the last
  sequence number it applied. Vault settings stay in
  `.frostnotes/vault.yaml`, which notesd now writes (`vault.configure`).
- **The editor saves as you type and applies merges as remote changes**
  ([ADR-0022](../adr/0022-the-editor-saves-as-you-type-and-applies-merges-as-remote-changes.md)).
  One document per open note, shared by its tabs; saves 1 s after the
  last change and at most 10 s after the first, one write in flight;
  a merged answer is diffed against what was sent, mapped over the edits
  typed since, and dispatched as one transaction outside the undo
  history, so the cursor holds and Ctrl+Z undoes only the user's own
  edits. Other writers' changes arrive the same way. A conflict shows a
  notice whose Compare opens the merge view, `@codemirror/merge`,
  against the conflict version notesd kept; a base notesd refuses opens
  it at once.
- **Files show by their hash and upload through notesd**
  ([ADR-0023](../adr/0023-the-app-shows-files-by-their-hash-and-uploads-them-through-notesd.md)).
  `notesblob://localhost/<sha256>.<ext>` serves notesd's blob store from
  the Tauri side, the dev gateway serves `/blobs/` in a browser, and the
  hub will in M4; pasted and dropped files go up through `note.upload`,
  base64, at most 10 MB.

A fourth came with Phase 8 (2026-10-18):

- **The app opens and creates vaults, and a window switches between
  them**
  ([ADR-0024](../adr/0024-the-app-opens-and-creates-vaults-and-a-window-switches-between-them.md),
  accepted 2026-10-18). Open folder as vault, Create new vault and Remove from
  list go through notesd's `vault.add` (which gains `create`) and
  `vault.remove`; the folder comes from the system's picker
  (`tauri-plugin-dialog`, open only); one window shows one vault and
  switches in place; without a vault the window is the vault manager.

These follow from ADRs 0003 to 0023 and from what the user's Obsidian
does, so the plan records them without ADRs:

- **The user's Obsidian is Obsidian's defaults.** Their vault's
  `.obsidian/` has an empty `app.json` and `appearance.json` and no
  `hotkeys.json`, `daily-notes.json`, `templates.json` or
  `bookmarks.json`. Its workspace shows Files, Search and Bookmarks on
  the left (200 wide) and Backlinks, Outgoing links, Tags, All
  properties and Outline on the right, collapsed, with one note in live
  preview. Its core plugins on: file explorer, search, quick switcher,
  graph, backlinks, canvas, outgoing links, tags, properties, page
  preview, daily notes, templates, note composer, command palette,
  editor status, bookmarks, outline, word count, file recovery, sync and
  bases; no community plugins. M3 builds what that layout and those
  plugins use day to day; graph and bases are M5's; canvas, note
  composer and bookmarks (none exist) wait.
- **Obsidian's behavior is read from Obsidian.** The default hotkeys,
  settings, the file explorer's sort, the quick switcher's matching and
  the supported file types in [specs/ui.md](../specs/ui.md) come from
  Obsidian 1.14.4's own code in the user's install, not from memory.
- **Rendering follows Obsidian's defaults:** Strict line breaks is off,
  so a single newline inside a paragraph is a line break in the reading
  view; embeds of notes render three levels deep; comments do not
  render; remote images load
  ([ADR-0007](../adr/0007-edit-in-codemirror-6-with-live-preview.md)).
- **The API grows within protocol 1, additively** (Phase 3): `note.files`
  (files and folders, empty ones included, with hashes), `note.resolve`
  (link targets from a note, by `markdown.Resolve`), `note.render`,
  `note.upload`, `note.readVersion`, `note.tags` and `note.properties`
  (with counts), `note.template` (a template expanded by
  `internal/templates`), folder operations (create, move with link
  updates, delete), `vault.settings` and `vault.configure`, and
  `sync.readRevision` (a hub revision's content, which the device fetches
  for the merge view when it is another device's); `WriteResult` gains
  the conflict version's ID and who made the side that won, for the
  conflict notice; search gains `path:`, `file:` and `tag:`.
- **A folder delete deletes what is unchanged.** `note.deleteFolder`
  takes the files the caller listed, each with its base, deletes those
  still at their base, keeps any changed since or made since, and removes
  the folder only when nothing is left: per file, an edit beats a delete
  ([ADR-0005](../adr/0005-sync-through-a-hub-with-revisions-and-three-way-merges.md)),
  and every deleted file's versions stay for `note.revert`. (Phase 3
  settled what was open until then.)
- **One window shows one vault.** A switcher at the bottom of the left
  sidebar changes it. More windows and pop-out tabs wait.
- **New dependencies are the planner's,** pinned, each release at least
  two weeks old: `@codemirror/merge`, `@codemirror/autocomplete`,
  `@codemirror/search`, `dompurify`, `katex` and `@tanstack/react-virtual`;
  in Rust, `tauri-plugin-opener` (frostmail's), which opens web links
  and files in the system's applications; in Go, nothing new (the sanitizer walks `golang.org/x/net/html`, already
  in the module). Cards never add one.
- **M0's spike is the live-preview framework's first draft.** Its
  decorations become the first providers; `make spike-cm6` stays until
  the editor's own latency harness replaces it.
- **The mock grows a vault.** `MockTransport` serves the `note` and
  `vault` domains over an in-memory vault of made-up notes with notesd's
  semantics for bases, events and errors, and takes a scripted merge for
  editor tests, so components and containers test without notesd.
- **App tests drive the built app** in WebKitGTK through
  `WebKitWebDriver`, as spike 1 did, against a real notesd (and a hub,
  for the writer pairs across devices).

## Phase 1 — Decisions and first cards (planner)

- ADRs 0021 to 0023 (proposed), [design/app.md](../design/app.md) (the
  design ADR-0007 and ADR-0011 named), [specs/ui.md](../specs/ui.md),
  this plan, and the roadmap's M3 row.
- The spec's new tokens and type roles in `app/src/styles/tokens.css` and
  `app.css`, and the `data-theme` rules the Theme preference will set.
- Batch-1 cards, T-0038 to T-0042: pure helpers and one presentational
  component that need nothing the planner has not built, each with a
  stub and given tests, checked against a reference that is then
  deleted.
- **Done when:** the ADRs, the design, the spec and this plan are indexed
  and cross-linked both ways; each card has passed its reference with
  `make verify` and `make ui-check` green; and the user has accepted the
  ADRs, or said what to change, before Phase 5's code merges.
- **Result (2026-10-17): done.** The user accepted ADRs 0021 to 0023 as
  written, after this plan merged (PR frostyard/frostnotes#19). Reading Obsidian
  1.14.4's own code in the user's install settled what the spec pins:
  the hotkeys (Ctrl+L toggles a checkbox, Ctrl+; adds a property,
  Ctrl+Shift+N makes a note to the right, and the editor drops Alt+↑ and
  Alt+↓), the explorer's sort (folders first and always A to Z; files by
  a numeric, case-free collator), the switcher's matching (tokens first,
  then single characters; a part inside a word costs a tenth), its outline
  (a stack, as T-0041 states), and the defaults (spellcheck on, indent
  with tabs, Strict line breaks off, links not updated without asking).
  Each card was checked with its given tests and a reference in place,
  alone (`make ui-vitest`) and then all five together, with `make
  ui-check` (14 files, 98 tests) and `make verify` green; the references
  are deleted. Against the stubs, every given test that expects a match
  fails (keymap 7 of 10, fuzzy 10 of 12, file tree 14 of 15, outline 9
  of 11, tab strip 14 of 14); the rest check that nothing extra matches,
  or the spec's own table. The fuzzy card's expected scores were worked
  out by hand from Obsidian's code before the reference ran, and every
  one agreed. Checking changed
  two things: the keymap's uniqueness test, which only checks the spec's
  own table, now says so; and the file tree's first reference grew its
  lists by copying them, so the card now says to group by folder once,
  and its test builds a 10,101-row vault. An independent review (a
  separate agent, read-only) found no card at odds with its tests, and
  these, each fixed: `text-badge` would have taken `--color-badge`'s
  color instead of its size (Tailwind tries colors first), so the role is
  `text-keycap`; the Theme preference had no `data-theme` rules to set;
  the spec's window size was not the shell's; the API could not yet give
  a conflict notice its writer and version, nor a hub revision's content
  (so `WriteResult` grows and `sync.readRevision` joins Phase 3);
  ADR-0007's remote-images setting and `tauri-plugin-opener` were
  missing; KaTeX's own markup needed an exception to the no-`style` rule;
  the agent workflow did not yet allow two executor worktrees at once;
  and some back-links and phases' links were missing.

## Phase 2 — Executor batch 1 (alongside Phase 3)

| Card | Size | File | What |
| --- | --- | --- | --- |
| [T-0038](../tasks/done/0038-keymap.md) | S | `app/src/lib/keymap.ts` | Obsidian's default hotkeys: a key press to a command ID, a command's hotkeys, and their display |
| [T-0039](../tasks/done/0039-fuzzy-search.md) | S | `app/src/lib/fuzzy.ts` | Obsidian's fuzzy search for the quick switcher, the palette and link suggestions: scores and matched ranges |
| [T-0040](../tasks/done/0040-file-tree.md) | M | `app/src/lib/fileTree.ts` | the Files pane's rows: folders and files nested, sorted as Obsidian sorts, filtered by type, flattened by what is open |
| [T-0041](../tasks/done/0041-outline.md) | S | `app/src/lib/outline.ts` | the Outline pane's tree of headings, its visible rows, and the heading a position is under |
| [T-0042](../tasks/done/0042-tab-strip.md) | M | `app/src/features/workspace/TabStrip.tsx` | a tab group's strip: tabs, close, new tab, the tab list, keys, and the commands it sends |

- They implement [specs/ui.md](../specs/ui.md)'s keyboard map, Prompts,
  Files pane, Outline and Tab strip, and
  [design/app.md](../design/app.md#commands-and-keys)'s keymap.
- The cards touch separate files and use nothing of each other's, so
  they may run in any order, in two executor worktrees at once
  ([agent workflow](../design/agent-workflow.md#batching-cards)).
- **Done when:** every card is merged and `make ui-check` is green.
- **Result (2026-10-17): done.** Codex (codex-cli 0.161, the user's
  default model) passed all five cards on the first attempt, the app's
  first cards, in two executor worktrees at once (`exec-m3`: T-0038,
  T-0039, T-0041; `exec-m3b`: T-0040, T-0042), each in one to two and a
  half minutes; the planner reviewed each before merging it into
  `m3/cards`. On the merge, `make ui-check` (14 files, 98 tests) and `make
  verify` are green. Each card's given tests were checked by mutation
  against Codex's code, and each mutation failed them: letters matched
  by case (T-0038), a character inside a word right after the last one
  counted (T-0039), folders sorted in reverse with the files (T-0040),
  headings of one level nested (T-0041), and focus left behind after a
  key activated a tab (T-0042).

  | Card | Attempts | Wall time | Review |
  | --- | --- | --- | --- |
  | T-0038 keymap | 1 | 2 m 0 s | matches the spec; builds its key-name table per call, harmless |
  | T-0039 fuzzy search | 1 | 1 m 10 s | matches Obsidian's rules; its classes held the characters themselves, some invisible, so the planner wrote them as the card's escapes (the same code points) |
  | T-0040 file tree | 1 | 2 m 18 s | correct, linear grouping; two files whose names differ only in case and extension order by the whole name a step before the card's plain order would, deterministic either way |
  | T-0041 outline | 1 | 54 s | correct |
  | T-0042 tab strip | 1 | 1 m 32 s | correct; un-hides the pin's name for assistive technology and drops a pending focus whose tab is gone |

## Phase 3 — Contracts (planner)

- `schema/rpc/note.yaml` and `vault.yaml`: the methods in the decisions
  above, each with its errors; `make gen`
  ([specs/rpc-api.md](../specs/rpc-api.md)). The engine answers the new
  methods `unavailable` until Phase 4.
- `internal/render`: `Render(doc, opts)` and the sanitizer's allowlist as
  a table, with stubs for the Obsidian syntax's HTML that cards fill
  ([design/app.md](../design/app.md#reading-view)).
- Store signatures for tags and properties with counts, and search's
  filters, as stubs with doc comments.
- `app/src-tauri/src/protocol.rs` (`notesblob://`, with Rust tests) and
  `internal/devgw`'s `/blobs/`
  ([ADR-0023](../adr/0023-the-app-shows-files-by-their-hash-and-uploads-them-through-notesd.md));
  the CSP's `notesblob:` and `https:` for images.
- `MockTransport`'s vault: the `note` and `vault` domains, events, and
  scripted merges. *Moved to Phase 5,* with the containers that use it: no
  card needs it, since batch 2's components take props.
- The batch-3 cards whose contracts these settle, each checked against a
  reference.
- **Done when:** `make check` and `make ui-check` are green with every
  stub in place, `make gen-check` passes, and each card has passed its
  reference.
- **Result, part 1 (2026-10-17): the API's contract.** `note.yaml` gains
  `files`, `resolve`, `render`, `upload`, `readVersion`, `tags`,
  `properties`, `template`, `mkdir`, `moveFolder` and `deleteFolder`, and
  `WriteResult` the conflict version's ID and who made the side a write
  merged into; `vault.yaml` gains `settings`, `configure` and the durable
  `vault.configured` event; `sync.yaml` gains `readRevision`; `note.search`
  documents its filters. All are additive within protocol 1, generated
  by `make gen`, and answer `unavailable` until Phase 4
  (`internal/engine/m3.go`). The store has stubs for the counts and the
  filters (`counts.go`, `ParseQuery` and `SearchNotes`), and two cards,
  each checked against a reference that is then deleted, with `make
  check` green; against the stubs every one of their tests fails.
  Checking caught one bug in a given test (an empty result compared as
  nil) and settled one rule: property keys fold as Go folds them, so
  "Été" and "été" are one key, which SQLite's ASCII `lower()` would not
  do.

  | Card | Size | File | What | Runs after |
  | --- | --- | --- | --- | --- |
  | [T-0043](../tasks/done/0043-tag-and-property-counts.md) | S | `internal/store/counts.go` | every tag and frontmatter key of a vault, with how many notes have it | |
  | [T-0044](../tasks/done/0044-search-filters.md) | M | `internal/store/search.go` | `path:`, `file:` and `tag:` in searches, and listing by filters alone | T-0043 |

  Both cards are in `internal/store`, so they run one after the other.
- **Result, part 2 (2026-10-17): the renderer's contract.**
  [specs/render.md](../specs/render.md) pins the HTML for each piece of
  syntax, the sanitizer's allowlist, a fixture vault and 18 fixtures, read
  by `internal/render`'s tests. `internal/markdown` gives the renderer the
  index's goldmark, the Obsidian nodes and the body to render (comments
  out, their newlines kept, so `data-line` counts the note's lines);
  `internal/render` has `Render`, `HTML`, embeds' depth and cycles, a
  fallback that keeps unknown nodes from crashing goldmark, and a
  sanitizer that escapes everything until Phase 4 builds it. Writing the
  references settled two rules: a note embed alone in its paragraph
  replaces the paragraph as a block, since a `div` in a `p` is split
  apart by every HTML parser, and one among text is a link; and an
  embedded note names no lines. Each card was checked against a reference
  that is then deleted, T-0045's alone and then both, with `make check`
  green; against the stubs every `[links]` and `[syntax]` fixture fails.

  | Card | Size | File | What | Runs after |
  | --- | --- | --- | --- | --- |
  | [T-0045](../tasks/done/0045-render-links.md) | M | `internal/render/links.go` | links, note embeds and their sections, files by hash, markdown images | |
  | [T-0046](../tasks/done/0046-render-syntax.md) | M | `internal/render/syntax.go` | tags, highlights, math, callouts, checkboxes and headings with lines, block IDs | T-0045 |

  The render cards share a package with neither store card, so the two
  pairs may run at once in separate executor worktrees.
- **Result, part 3 (2026-10-17): files by hash.** `notesblob://` is built
  (`app/src-tauri/src/protocol.rs`, registered in `main.rs`, the data
  directory found as `internal/config` finds it), with Rust tests of its
  path check, content types, ranges and responses; the dev gateway serves
  `/blobs/` behind its token with `Range` (`internal/devgw/blobs.go`, and
  notesd passes it the blob store); the CSP admits `notesblob:` for images
  and media and `https:` for remote images. Both servers share one table
  of content types ([design/app.md](../design/app.md#attachments)) and
  the headers [specs/rpc-protocol.md](../specs/rpc-protocol.md#transports)
  now pins.

## Phase 4 — notesd's additions (planner, alongside Phase 5)

Implements [design/vault.md](../design/vault.md) (written settings,
folders), [design/app.md](../design/app.md#reading-view) (rendering) and
the Phase 3 methods in [specs/rpc-api.md](../specs/rpc-api.md).

- The new methods over the vault's actor: the listing walks folders, so
  empty ones show; `note.resolve` runs `markdown.Resolve` from the given
  note's folder; `note.render` renders the given text or the file;
  `note.upload` creates through the write path; folder moves move each
  file under the vault's lock with `note.move`'s link rewriting; a folder
  delete deletes each file with its base; `vault.configure` writes
  `vault.yaml` atomically, the first write notesd makes to
  `.frostnotes/`, and the settings M3 adds are read from
  `.obsidian/app.json` while unset.
- The sanitizer's table test: script and `on*` attributes, `javascript:`
  and `vbscript:` URLs in every attribute that takes one, `data:` outside
  images, SVG with script, `style` elements and attributes, forms,
  frames, `base`, `meta` refresh, `object` and `embed`, and nested and
  malformed markup.
- **Done when:** every method works against a generated vault through
  `rpctest`, the sanitizer's table passes, and `make vault-it` still
  passes.
- **Result, part 1 (2026-10-17): the sanitizer and the reads.**
  `render.Sanitize` is the spec's allowlist over `golang.org/x/net/html`'s
  tokenizer, with the table above and every fixture sanitizing to itself
  but R02's `onclick`; a table column's alignment is now an `align`
  attribute, which it keeps, not a style (fixture R11). `note.files`,
  `note.resolve`, `note.render`, `note.readVersion`, `note.tags`,
  `note.properties` (property types from `types.yaml` or Obsidian's
  `types.json`), `note.search`'s filters and `vault.settings` (the
  Files and links settings from `vault.yaml` or `.obsidian/app.json`,
  and where each comes from) work against a generated vault through
  `rpctest` ([design/vault.md](../design/vault.md#on-disk)). The writes
  (`note.upload`, `note.mkdir`, `note.moveFolder`, `note.deleteFolder`,
  `note.template`, `vault.configure`, `sync.readRevision`) still answer
  `unavailable`: part 2.
- **Result, part 2 (2026-10-17): the writes.** `note.upload` creates a
  file from base64 through the write path, at most 10 MB. `note.mkdir`
  makes folders; `note.moveFolder` renames a folder once and moves each
  indexed file's row in one `Tx`, a revision and `note.moved` each, with
  `note.move`'s link rewriting, now planned for many files at once;
  `note.deleteFolder` deletes each listed file with its base and keeps the
  rest; `note.template` expands a template note. `vault.configure` writes
  `vault.yaml` atomically, keeping its other keys and comments, and
  `vault.configured` follows it or a hand edit a scan finds.
  `sync.readRevision` needed the hub's `log.revision`, an additive
  method, since a revision cannot be fetched by its sequence number
  alone. Every method works through `rpctest`, against a generated vault
  for folder moves (every manifest link still leads to its file), and a
  hub and two devices for `sync.readRevision`; `make vault-it` passes
  ([design/vault.md](../design/vault.md#writes),
  [specs/sync-protocol.md](../specs/sync-protocol.md)). Phase 4 is done.

## Phase 5 — The app's core (planner)

Implements [design/app.md](../design/app.md) (data, workspace, editor,
live preview, reading view, history and merges, commands) and the parts
of [specs/ui.md](../specs/ui.md) not marked for a batch.

- `data/`: `Session` from the stored sequence number; the files, sync
  and settings stores; `MockTransport`'s vault (the `note` and `vault`
  domains with notesd's semantics, events, scripted merges), moved here
  from Phase 3.
- `workspace/`: the model, its operations, validation and persistence;
  the layout (ribbon, sidebars, splits, strips from T-0042, view
  headers) and drag and drop between groups.
- `commands/`: the registry and the window's key listener over T-0038.
- `editor/`: the document registry with saves and merges
  ([ADR-0022](../adr/0022-the-editor-saves-as-you-type-and-applies-merges-as-remote-changes.md)),
  views sharing a document, the modes, the keymap, link resolution, and
  the live-preview framework with the spike's decorations as its first
  providers; the Lezer extensions' skeleton and a Vitest reader of the
  spec's `F` fixtures.
- `reading/`: `note.render`, DOMPurify, blob URLs, math, link clicks.
- The merge view and the history's plumbing.
- Tests: each writer pair with the editor (an outside edit, a pull, an
  agent's append, a second tab) as Vitest tests over the mock's scripted
  merges and as app tests against a real notesd.
- **Done when:** in the built app against a real notesd, a note opens
  from the Files pane, takes typing, saves, and takes an outside edit
  made meanwhile, merged, with the cursor where it was and Ctrl+Z
  undoing only the typing; a forced conflict shows its notice, whose
  Compare opens the merge view; the reading view shows the same note; and typing in a generated
  10,000-line note with the framework on stays under 16 ms a keystroke at
  p95 on the host.
- **Result so far (2026-10-17): the data layer.** `Session` subscribes
  from the last sequence number the window applied (`frostnotes.seq`),
  loads the vaults and the window's vault (the last one it showed,
  `lastVault` in `frostnotes.prefs.v1`), and passes every event, replays
  included, to the event feed (`data/events.ts`), which is how saved
  workspaces will follow moves made while the app was closed. The files
  store (`data/files.ts`) applies note events after its listing's
  starting point over the listing, in order, and queues those that
  arrive while it loads; the vault's settings, sync and index state are
  `data/vaultState.ts`; the preferences, with Obsidian's defaults, are
  `data/prefs.ts`. Each transport gives a blob's URL (`notesblob://`,
  the gateway's `/blobs/` with its token, or a `data:` URL from the
  mock). `MockTransport` gained a vault per vault ID
  (`rpc/mock/vault.ts`): the note and vault domains with notesd's bases,
  merges (scripted with `scriptMerges`, else the newer side stands with a
  conflict version), versions, folders, events, and a replay after
  `sinceSeq`; `VITE_MOCK=1` shows a made-up vault (`rpc/mock/fixture.ts`).
- **Result so far (2026-10-17): the workspace and commands.** The model,
  its pure operations, validation and per-vault persistence
  (`workspace/`), following moves and deletes from the event feed into
  every vault's saved workspace; the command registry and the window's
  key listener over T-0038's keymap (`commands/`); and the layout
  (`app/`): the ribbon, the sidebars with their pane tabs, splitters and
  the vault row, the split tree of tab groups with T-0042's strips in the
  title bar row and view headers, drag and drop between groups and to
  their edges, and sidebars that give way, then collapse, in a narrow
  window ([specs/ui.md](../specs/ui.md#layout)). Until batch 2, the Files
  pane is a plain list of paths, a note tab shows its text read-only
  until the editor core, and the menus (tab menu, tab list, More
  options) and the panes' contents are not drawn.
- **Result so far (2026-10-17): the editor core.** The document registry
  (`editor/document.ts`, `editor/registry.ts`): saves 1 s after the last
  change and at most 10 s after the first, one write in flight, merged
  answers and other writers' changes applied as one remote transaction
  mapped over what was typed since, out of the history; conflict,
  refused-base, failed-save and deleted notices; a document kept until
  it saves. Every writer pair of ADR-0022 is a Vitest test over the
  mock's scripted merges. The editor (`editor/NoteEditor.tsx`): views
  sharing a document, live preview and source mode, the keymap without
  what Obsidian removes and what the app binds, search, link resolution
  batched through `note.resolve`, and links that follow or create their
  note. The live-preview framework (`editor/preview/`): inline and block
  providers, the reveal rule, clicks to providers, with the spike's
  decorations as its first providers; typing in a generated 10,000-line
  note took 3.4 ms at p95 a keystroke in a browser over the mock (the
  exit evidence measures WebKitGTK). The Lezer skeleton
  (`editor/syntax/`): wikilinks with subpaths and the frontmatter as one
  node, and the `F` fixtures through Lezer, 8 agreeing and 7 pending
  batch 3's extensions. `Session` now keeps the window through a
  reconnect, so no editor goes away with what was typed.
  `@codemirror/merge` 6.12.2, `@codemirror/autocomplete` 6.20.3 and
  `@codemirror/search` 6.7.2 joined, pinned.
- **Result (2026-10-18): the reading view, the merge view, the history,
  and the exit evidence.** The reading view (`reading/`) renders the
  document's text with `note.render` and again 300 ms after it changes,
  sanitizes it again with DOMPurify and render.md's allowlist (every
  fixture sanitizes to itself; an attack table checks the rest), gives
  each `data-blob` element its blob URL, drops remote images unless they
  load, draws math with KaTeX, follows links as the editor does, and
  toggles a task's line in the document. Under happy-dom DOMPurify keeps
  `<script>`, so the sanitizer checks itself and falls back to escaping,
  and its tests run in jsdom. The merge view (`editor/MergePane.tsx`)
  attaches its left editor to the document; the history
  (`app/HistoryView.tsx`, plain until batch 2) lists versions and hub
  revisions, restores with the document's base, and compares; the
  command Open version history opens it. DOMPurify 3.4.16, KaTeX 0.18.11
  and, for tests, jsdom 30.1.1 joined, pinned. The exit evidence:
  - Against a real notesd (a scratch vault, its own data directory and
    socket, `-devgw`) with the app from Vite in a browser over the dev
    gateway, a note opened from the Files pane took typing and saved it;
    an outside edit (`sed -i` on another line) made while typing was
    unsaved landed 2 ms before the save, which notesd merged with the
    cursor where it was; Ctrl+Z took back all the typing and left both
    outside edits, and the undo saved. An outside edit of the line being
    typed on showed the conflict notice with nothing lost (the conflict
    version held all 53 typed characters); Compare opened the merge view
    on it, taking the right side's chunk saved, and Done closed it. The
    reading view showed the same note; its checkbox toggled the line on
    disk and its link opened the other note; a callout, highlight, a
    tag, inline and display math, a table, code, an embedded note and a
    footnote rendered. The run found two bugs, fixed here with tests:
    notesd's write answer never named its conflict version (`Compare`
    asked for version 0), and a task's checkbox, which notesd renders
    disabled, took no clicks in a browser.
  - Typing in a generated 10,000-line note in the built app with the
    framework on (`FROSTNOTES_SPIKE=editor build/frostnotes-app`): 10 ms
    at p95 from keystroke to paint on the host's GPU (dispatch 5 ms;
    scrolling at 59 fps, frame p95 18 ms), and 17 ms under Xvfb in nsl,
    which renders in software (dispatch 9 ms).
  - Not yet run by hand: the same editor flow in the built app's window
    over the Tauri bridge (it reads `FROSTNOTES_SOCKET`), and whether
    WebKitGTK loads `notesblob://` into Vite's page under `make app-dev`
    (see [Attachments](../design/app.md#attachments)). Phase 7's
    `make m3-evidence` drives the built app.

## Phase 6 — Executor batches 2 and 3, and wiring

Cards are numbered as each batch is written; the planner wires each one
into its container as it merges.

- **Batch 2** (presentational, each written once its props' types
  exist: most need only the spec and today's API types; Tags and All
  properties wait for Phase 3's): the Files pane, the search pane,
  Backlinks, Outgoing links, Tags, All properties and the Outline pane;
  the quick switcher and the command palette over T-0039; the status
  bar; menus and notices; the settings dialog's sections; the history
  list; the vault switcher (its menu came with the vault row; opening,
  creating and removing vaults never got a card, and moved to
  [Phase 8](#phase-8--vaults-from-the-app) on 2026-10-18).
- **Batch 3** (after Phase 3's contracts and Phase 5's framework): the
  Lezer extensions (embeds, tags, highlights, comments, callouts, block
  IDs, math, footnotes) against the spec's fixtures; live-preview
  widgets, one kind per card (callouts, images and embeds, math, marks
  for highlights, tags and comments, code blocks, quotes, lists and
  rules, tables shown read-only with their source on click, the
  properties block, footnotes); Obsidian's editor commands (bold,
  italics, highlight, strikethrough, code, comment, link, checkbox,
  delete paragraph, headings); link and tag completion; property edits
  on YAML text that keep every other byte, against MCP's cases (T-0022);
  word counts as notesd counts them; notesd's tag and property counts,
  search filters, and the renderer's HTML for Obsidian's syntax against
  golden files; Open today's daily note and Insert template.
- **Done when:** every card is merged, `make check` and `make ui-check`
  are green, and each section of [specs/ui.md](../specs/ui.md) works in
  the built app.
- **Batch 2 runs as three PRs against `main`,** one at a time, each
  with its cards and their wiring: the shared pieces (menus, notices,
  the prompt frame, the quick switcher, the command palette, the status
  bar's items); the sidebar panes (Files, Search, Backlinks, Outgoing
  links, Tags, All properties, Outline); and the settings dialog, the
  history list and the dialogs they need.
- **Batch 3 runs as three PRs against `main`,** like batch 2: the
  editor's syntax (the Lezer extensions), Obsidian's formatting
  commands, word counts and link and tag completion; the live-preview
  widgets; and properties (the block and its edits), daily notes and
  templates.
- **Result so far (2026-10-17): batch 3's first four cards.** Codex
  (codex-cli 0.161, the user's default model) passed T-0043 to T-0046 on
  the first attempt each, in two executor worktrees at once: the store
  pair in `exec-m3c` (`m3/batch3`), the render pair in `exec-m3d`
  (`m3/batch3-b`), each card reviewed and merged before its partner
  started. T-0046's card was corrected before it ran: T-0045 had named
  its helpers `linkAttr` and `escapeLink`, not the `attr` and `writeText`
  the card offered. On the merge, `make check` is green. Each card's
  given tests were checked by mutation against Codex's code, and each
  mutation failed them: property keys left unfolded (T-0043), the `file:`
  filter ignored (T-0044), note embeds never made blocks (T-0045, 5
  fixtures), and callouts transformed after embeds are looked for
  (T-0046, 2 fixtures).

  | Card | Attempts | Wall time | Review |
  | --- | --- | --- | --- |
  | T-0043 counts | 1 | 2 m 0 s | correct; one tag row per note makes `COUNT(*)` the note count, and property keys fold in Go |
  | T-0044 search filters | 1 | 1 m 43 s | correct; words split at `"` as `SearchQuery` splits them, and filters run after SQL so the limit counts admitted notes |
  | T-0045 links and embeds | 1 | 2 m 41 s | matches the spec; also makes a markdown image of a note alone in its paragraph a block embed, which the spec's "an embed of it as above" allows |
  | T-0046 syntax | 1 | 1 m 45 s | correct; runs its callout transformer before T-0045's embed transformer, which R16 needs, and says why |

- **Result so far (2026-10-18): batch 2's shared pieces.** Codex
  (codex-cli 0.161, the user's default model) passed T-0047 to T-0052 on
  the first attempt each: the menu, the notices, the prompt frame and
  the status bar at once in four executor worktrees (`exec-m3e` to
  `exec-m3h`, from `m3/batch2`), then the quick switcher and the command
  palette, whose given tests render T-0049's frame, once it merged. Each
  card's given tests were checked by mutation against Codex's code: a
  menu that stops at its ends (3 tests failed), keys that reach the
  window from the prompt (1), a status bar without singulars (2), a
  switcher without aliases (4) and a palette without its recent
  commands (4) failed them; timers restarted on every render passed
  the notices' tests, so a test of a notice drawn again joined them.
  The wiring (`data/overlays.ts`, `app/Overlays.tsx`, `app/menus.ts`,
  `app/status.ts`) opens the tab menu, the tab list, More options, the
  vault menu, the editing mode and conflicts menus, both prompts from
  their keys and the ribbon, and the notices; it found that the ribbon
  never enabled a command registered after the window drew (the
  registry now tells the window), and the planner made a menu or prompt
  give focus back when it closes and drew key caps in the interface
  font. A `--backdrop` token dims the window behind a prompt. Left for
  later: aliases in the switcher (`note.list` carries none yet), word
  counts (batch 3), Rename, Move file to and Open in default app (their
  prompts and the opener plugin).

  | Card | Attempts | Wall time | Review |
  | --- | --- | --- | --- |
  | T-0047 menu | 1 | 3 m 50 s | correct; nested lists in one portal, a press outside the portal closes, ← and Escape close only a submenu |
  | T-0048 notices | 1 | 2 m 12 s | correct; a notice closed early keeps its timer entry until it leaves, so a render cannot restart it |
  | T-0049 prompt frame | 1 | 2 m 35 s | correct; a row's `mousedown` keeps focus in the field |
  | T-0050 status bar | 1 | 2 m 16 s | correct |
  | T-0051 quick switcher | 1 | 2 m 16 s | correct |
  | T-0052 command palette | 1 | 1 m 21 s | correct; its key caps took the monospace font Tailwind gives `kbd`, which the planner set back |

- **Result so far (2026-10-18): batch 2's sidebar panes.** Codex passed
  T-0053 to T-0059 on the first attempt each, four at a time in the
  executor worktrees (`exec-m3e` to `exec-m3h`, from `m3/batch2-panes`),
  the three small cards taking the worktrees as the first four merged.
  A mutation of each card's code failed its given tests: an outline
  that marks no heading (1 test), outgoing links listed twice (1), tag
  and property sorts that ignore counts (1 and 2), unlinked mentions
  open at first (3), a search without its singular (1), and ← that does
  not reach a row's folder (1). The planner's part: `note.backlinks`
  carries each link's line (`context`, additive; the vault reads the
  source's indexed content from the blob store), the workspace keeps
  the Files pane's sort order and open folders, the mock's search and
  list take notesd's filters, and the containers (`app/panes/`) with
  the Outline reading the editor's syntax tree (the editors map tells
  subscribers of every change). Wiring found that notesd sends no event
  for an empty folder: a window that makes, moves or deletes a folder
  lists the folders again, and another window sees it at its next
  listing (a folder event is left for later). Rename joins More options
  and F2 (in the Files pane until the inline title); Move file to and
  Open in default app wait for their prompt and the opener plugin.

  | Card | Attempts | Wall time | Review |
  | --- | --- | --- | --- |
  | T-0053 Outline | 1 | 1 m 48 s | correct |
  | T-0054 Outgoing links | 1 | 1 m 21 s | correct |
  | T-0055 Tags | 1 | 3 m 16 s | correct; adds each count to every node on its path, which sums as the contract says |
  | T-0056 All properties | 1 | 1 m 33 s | correct; guards the icon table with `Object.hasOwn` |
  | T-0057 Backlinks | 1 | 2 m 42 s | correct; some rows lack the 8px inset, left as is |
  | T-0058 Search | 1 | 2 m 19 s | correct; opens on a middle click too |
  | T-0059 Files | 1 | 3 m 2 s | correct; drag targets found from the event at the tree; Enter sends a blank name, which the container ignores |

- **Result so far (2026-10-18): batch 2's settings, history and
  dialogs.** Codex passed T-0060 to T-0063 on the first attempt each, all
  four at once in the executor worktrees (from `m3/batch2-settings`). A
  mutation of each failed its given tests: a text setting sent when
  unchanged (1 test), a history that never says Yesterday (3), Escape
  that confirms (1), a folder chooser without the root (3). The
  planner's part: the confirmation replaces every `window.confirm`
  (deleting files and folders, the link question on moves, closing with
  unsaved notes); Move file to and Move folder to open the folder
  chooser; the settings dialog's container maps rows to the preferences
  and `vault.configure`, lists the keyboard map's commands by name
  (`HOTKEY_NAMES` joined `lib/keymap.ts`), and the theme and font size
  preferences now reach the page; the history tab shows a version with
  the editor's styles and its changes from the current text. Batch 2 is
  done but for Open in default app, which waits for the opener plugin.

  | Card | Attempts | Wall time | Review |
  | --- | --- | --- | --- |
  | T-0060 Settings dialog | 1 | 2 m 10 s | correct; one draft field serves text and number settings |
  | T-0061 History list | 1 | 1 m 42 s | correct; also sorts entries newest first itself |
  | T-0062 Confirmation | 1 | 2 m 2 s | correct |
  | T-0063 Folder chooser | 1 | 1 m 23 s | correct |

- **Result so far (2026-10-18): batch 3's syntax, commands, counts and
  completion.** Codex passed T-0064 to T-0069, four at once in the
  executor worktrees (from `m3/batch3-syntax`), then two. The planner
  gave each syntax its file and node names first, registered as stubs,
  and made the fixture test hold a fixture pending while a node it needs
  is undefined, so the cards landed in any order; with them, every one
  of the spec's 14 fixtures agrees in Lezer, block IDs and each
  fixture's word count included. Comments are ranges found as notesd
  finds them (a node could not cross blocks), and callouts are read from
  a quote's first line. A mutation of each card failed its given tests:
  all-digit tags (1 test), block IDs after any character (1), math
  closed after a space (1), U+0085 not white space (1), italics read as
  any star run (1), and completion that always adds `]]` (1). T-0069's
  first run failed all three attempts on the planner's mistake, a line
  of `MainWindow.tsx` committed unformatted, which Biome flagged in a
  file outside the card; T-0068's first run was stopped for the same
  reason, and both passed from the fixed base. Review found that a task
  whose box ends its line was not read as one; the planner fixed it with
  a test. The planner's part: the formatting commands and Follow link
  under cursor in the registry, the status bar's words and characters
  (150 ms after the last change; about 8 ms on a 10,000-line note), the
  completion catalog (`editor/catalog.ts`), and the styles of tags,
  highlights, comments and the completion list. In a browser over the
  mock, `[[` and a tag's `#` listed and wrote completions, Ctrl+B and
  Ctrl+L formatted, and the counts followed typing. Aliases in
  completion wait with the switcher's for note listings that carry them.

  | Card | Attempts | Wall time | Review |
  | --- | --- | --- | --- |
  | T-0064 tags and highlights | 1 | 1 m 34 s | correct |
  | T-0065 embeds, block IDs, footnotes | 1 | 1 m 48 s | correct; a footnote definition's leaf parser is a plain object |
  | T-0066 math and callouts | 1 | 2 m 5 s | correct |
  | T-0067 comments and words | 1 | 2 m 50 s | correct; caches comments per tree and document, as the empty tree is shared |
  | T-0068 formatting commands | 1 (a first run stopped) | 2 m 40 s | correct but for a task box at a line's end, fixed by the planner |
  | T-0069 completion | 1 (a first run failed 3, the planner's fault) | 1 m 48 s | correct |

- **Result so far (2026-10-18): batch 3's live-preview widgets.** Codex
  passed T-0070 to T-0075 on the first attempt each, four at once and
  then two (from `m3/batch3-preview`). The planner first gave each kind
  its file in `editor/preview/`, registered in `PROVIDERS`, and gave the
  preview's environment what widgets need: a vault file's URL, the Load
  remote images preference, and rendering markdown into an element.
  Callouts, tables and embeds of notes are drawn by that rendering:
  `note.render` with the block's source, sanitized as the reading view
  sanitizes it, its files and math filled in, cached by markdown and
  filled again a second after the vault changes
  (`editor/rendered.ts`), so no card builds HTML from a note's text and
  the blocks look exactly like the reading view, whose content styles
  became a shared `rendered` class. A mutation of each card failed its
  given tests: tags in a link's text drawn as pills (1 test), a closing
  fence left showing (2), bullets drawn over tasks (1), no text when
  KaTeX throws (1), callouts drawn inside a line (1), and web images
  loaded against the preference (1). KaTeX draws nothing in happy-dom,
  which it takes for a page in quirks mode, so the math card's tests and
  the window's run in jsdom, and a math widget shows its TeX when KaTeX
  throws. The planner's part also styled the widgets, made a tag's click
  search (in the reading view too) and a foldable callout's title fold
  it, added search and copy commands, and taught the mock's renderer
  callouts, tables and embeds. In a browser over the mock, a callout, a
  table, math, inline code, a footnote, a code block, a bullet, a done
  task and a rule drew as the spec says, and a click on the callout
  showed its source; the copy button's command ran, but the browser pane
  refused the clipboard, so copying waits for the built app.
  Indentation guides are left for later.

  | Card | Attempts | Wall time | Review |
  | --- | --- | --- | --- |
  | T-0070 highlights, tags, code, footnotes | 1 | 1 m 42 s | correct |
  | T-0071 code blocks | 1 | 1 m 26 s | correct |
  | T-0072 quotes, lists, rules, done tasks | 1 | 1 m 36 s | correct |
  | T-0073 math | 1 | 1 m 52 s | correct; its two providers share an id and a click |
  | T-0074 callouts, tables, note embeds | 1 | 1 m 40 s | correct |
  | T-0075 images, audio, video | 1 | 1 m 44 s | correct; leaves an image whose text spans lines as source |

- **Result so far (2026-10-18): batch 3's properties, inline title,
  templates and daily notes.** Codex passed T-0076 to T-0079 on the first
  attempt each, all four at once (from `m3/batch3-props`). `yaml` 2.9.1
  (2026-09-11, ISC, no dependencies) joined, pinned, for T-0076's port of
  MCP's property edits, which keeps MCP's cases but writes strings and
  dates as Obsidian does. A mutation of each card failed its given tests:
  CRLF lost in edits (2 tests), a tag kept with its `#` (1), a forbidden
  name accepted (1), and repeated templates listed twice (3); the
  reference for T-0077 was itself caught by its test letting Escape's
  blur commit. The planner's part: the properties block as a block
  widget holding the React component, its DOM kept while the
  frontmatter changes; Add file property (Ctrl+;), whose request waits
  for a new block's first render; the Properties in document
  preference; the cursor starting after the frontmatter; the inline
  title in the editor and the reading view, renaming as the Files pane
  does; Insert template with the chooser and the template's properties
  merged; Open today's daily note; and a `note.daily` in the mock. A
  window test runs each over the mock, and in a browser the block drew
  text, tag and alias rows, added a property, and wrapped its pills.
  Left for later: the properties block in the reading view, and
  indentation guides.

  | Card | Attempts | Wall time | Review |
  | --- | --- | --- | --- |
  | T-0076 properties in the frontmatter's text | 1 | 1 m 46 s | correct |
  | T-0077 properties block | 1 | 2 m 15 s | correct; ignores Enter while an IME composes; the planner let its pills wrap |
  | T-0078 inline title | 1 | 1 m 21 s | correct; the planner styled its alert |
  | T-0079 template chooser | 1 | 1 m 22 s | correct |

- **Result (2026-10-18): Phase 6's last pieces.** An audit of the spec
  against the app found what Phase 6 still owed: web links did not open
  in the built app (the shell denies `window.open`), Open in default app
  waited, the zoom keys ran nothing, six Editor settings were never read
  (line numbers, folding, smart lists, auto-pairing Markdown's marks,
  indentation guides), aliases were missing from the quick switcher and
  completion, Ctrl+hover showed no page preview, and the reading view
  drew no properties. Instead of `tauri-plugin-opener`, the shell has two
  commands of its own (`open.rs`) that check what they open, so a page
  cannot reach anything but web links and vault documents (Rust tests
  for both). `note.list`'s entries carry aliases (additive). Codex passed
  T-0080 (guides) and T-0081 (the page preview box) on the first
  attempt each; a mutation failed T-0081's tests, but one of T-0080's
  (a guide on a line no deeper than its bullet) passed its given tests,
  so the planner added the test that fails it. A window test covers each
  piece over the mock.

  | Card | Attempts | Wall time | Review |
  | --- | --- | --- | --- |
  | T-0080 indentation guides | 1 | 2 m 2 s | correct; its given tests missed a lazy continuation line, added since |
  | T-0081 page preview | 1 | 1 m 19 s | correct |

## Phase 7 — Review and exit evidence

Checks [specs/ui.md](../specs/ui.md) (the keyboard map, the rules) and
[design/app.md](../design/app.md) (saves and merges, attachments)
against the built app.

- An independent review of what renders and loads: both sanitizers, the
  CSP, link handling (the webview never navigates; only `http`, `https`
  and `mailto` reach the system), `notesblob://`'s inputs and headers,
  `note.upload`'s limits, paste (plain text only in M3), and the Tauri
  capabilities. Each finding is fixed with a test.
- **Done when**, each shown by a test or a recorded run:
  1. Typing in a generated 10,000-line note in the built app, with every
     live-preview provider on, stays under 16 ms a keystroke at p95 on
     the host's GPU, and the note scrolls at 60 fps (frame p95 under
     20 ms); the same runs in nsl are recorded.
  2. Every hotkey in the spec's keyboard map runs its command in the
     built app.
  3. Each writer pair with the editor loses no token and keeps the
     cursor: vim saving the open note, a pull from another device
     through a hub, an agent's append through MCP, and a second tab.
  4. The user uses Frostnotes instead of Obsidian on two machines, synced
     through M2's hub, for 7 days, finds no edit missing, and names
     anything they went back to Obsidian for.
- `make m3-evidence` records evidence 1 to 3; evidence 4 is the user's.

- **Result (2026-10-18): the review.** An agent that had not written the
  code read what renders and loads (13 findings: 1 high, 3 medium, 4
  low, 5 for information) and found the shell's checks, the CSP, the
  navigation guard and DOMPurify sound. Each finding is fixed with a
  test, but three noted below; a fuzz test of notesd's sanitizer, added
  for finding 1, found a second way past it.

  | Finding | Fix | Test |
  | --- | --- | --- |
  | 1 (high) notesd's sanitizer wrote everything after a self-closing raw-text tag (`<xmp/>`, `<script/>`, `<iframe/>`) unsanitized; the fuzz test then found text joined across a dropped tag opening a new one (`<<x>script>`) | A non-void element that goes, written self-closing, goes with its content as a browser reads it; a `<` in text is written `&lt;` | Table rows, every raw-text name, a check that a browser's parse of the output holds nothing off the allowlist (the table and every fixture), and `FuzzSanitize` (9.3 million inputs in 90 s; its find kept as a seed); a mutation fails them |
  | 2 (medium) note embeds fanned out without bound; replies and the bridge's lines had no size limit | A render draws at most 100 note embeds and 4 MiB of their notes, every level together, then links; a note's existence is asked of the index, and a note larger than that is never read; notesd answers a reply over 16 MiB with `internal`; the bridge ends a connection that sends a longer line | Render tests of a 20×20×20 fan-out and of 1 MiB notes, an engine test, an rpcserver test, a bridge test; mutations fail the first and the third |
  | 3 (medium) `notesblob://` read whole files, also for open-ended ranges | A 206 carries at most 8 MiB; a request without a range is answered whole up to 64 MiB, else 413 | A protocol test with a small window and limit |
  | 4 (medium) `class`, `id` and `role` let a note's HTML use the app's styles and name window properties | Both sanitizers keep only the classes the renderer draws, footnotes' ids and roles; `.rendered` contains its layout | Rows in both sanitizers' tests |
  | 5 (low) the webview's own context menu showed, offering downloads | The page keeps it from showing from its first line; the webview downloads nothing | A test of the listener |
  | 6 (low) KaTeX's errors other than parse errors went uncaught, and nothing caught a view's failure | `finish` shows such math as its TeX (KaTeX's `maxSize` 50 em); each tab's view has an error boundary | Tests of both |
  | 7 (low) `mailto:` links passed any query (`attach=`) to the mail client | Only `to`, `cc`, `bcc`, `subject` and `body` stay | A shell test |
  | 8 (low) a remote image's request could start before `finish` removed its source | Rendered HTML is finished in a template, which loads nothing, then placed | A test that the page never has the HTML |
  | 9 (info) `open_file` takes the vault's folder from the page | Not changed: a page that could run script already drives notesd through the bridge; the shell's checks stand | — |
  | 10 (info) a CIFS mount's default `file_mode` makes every file look executable | The user's share mounts with `file_mode=0644`; `open.rs` says so, and that its type list guards types, not handlers | — |
  | 11 (info) `xdg-open`'s processes were never reaped | Each is waited for in a thread | — (a process's zombie) |
  | 12 (info) `core:default` granted menus, tray, images and paths | The capability lists only what the page uses; the audit found `destroy` missing, which `onCloseRequested` calls, so Close window never closed the app (evidence 2 confirmed it) | A shell test of the list, and evidence 2 |
  | 13 (info) docs and code disagreed (paste and drop, where rendered HTML goes, the message limit), and `[x](//host)` in live preview made a note | The docs say what the code does; a protocol-relative link makes no note | A test of following it |

- **Result (2026-10-18): evidence 1, typing met, scrolling not quite.**
  `FROSTNOTES_SPIKE=editor` measures the app's editor with every
  live-preview provider on a generated 10,000-line note (678 KB), which
  now holds batch 3's syntax too: callouts, tables, display and inline
  math, highlights, tags, comments, footnotes, embeds and rules. On the
  host's GPU (scale 2) typing is 12 ms at p95 from keystroke to paint
  (dispatch 8 ms; 11 and 13 ms in two more runs), within 16 ms, and
  live preview's checks hold (a task's checkbox toggles one character
  and back). A fling
  of 300 px a frame through the whole note runs at 52 to 54 fps over
  three runs, its frame p95 34 ms: 5 to 8 percent of frames take two
  vsyncs, where the target is 60 fps and a p95 under 20 ms. The same
  editor with no provider scrolls at 59.6 fps (p95 18 ms), as M0's spike
  does on this note (59.8 fps), and Phase 5 measured 59 fps with the
  first providers. No group of providers is the cause: without any one
  of them, or with any half, the editor scrolls at 57 to 59 fps (p95 18
  or 19 ms; 28 ms with only the basic, inline, code, lists and guides
  groups); only all of them together tip a frame now and then into a
  second vsync. Rebuilding every provider's decorations costs 0.4 ms a
  viewport (V8), so the time goes to drawing and laying out what they
  make. Under Xvfb in nsl, which renders in software, typing is 17 to
  25 ms at p95 and scrolling 22 fps; leaving out one group there gains
  at most 6 fps (basic). The follow-up profiles the scroll in WebKit and
  makes what live preview draws cheaper.

- **Result (2026-10-18): evidence 2 and 3** (`make m3-evidence`, the
  built app under Xvfb in nsl; `internal/appevidence`, whose drivers are
  in `app/src/spike/`). xdotool presses the keys, so they reach the page
  through X, GTK and WebKit as a person's do; the page asks for them in
  lines the shell prints (`spike_say`).
  - **Evidence 2:** before each binding of the keyboard map the driver
    lays out a workspace where its command applies (a note in the editor
    with history both ways, its cursor on a link, more tabs, a closed
    tab), over the mock's vault. 45 of the 46 bindings run their command;
    the 46th, Ctrl+G, opens the graph view, which is M5's. Its first runs
    found two bugs, fixed here with tests: WebKitGTK names Shift+Tab
    (X's ISO_Left_Tab) `Unidentified`, so Ctrl+Shift+Tab ran nothing
    (the key listener now reads its code, `Tab`, as CodeMirror and React
    already do); and Close window left the app open, because the
    capability file did not grant `destroy`, which `onCloseRequested`
    calls. The run ends by closing the window, and the app exits.
  - **Evidence 3:** three notesd processes, a hub and devices a and b,
    share `Writers.md`; the built app, on a over the Tauri bridge, opens
    it in two tabs side by side and types 40 tokens at the end of a line
    in each, by turns. Meanwhile vim saves the note on a's disk (17
    times, in place and by vim's rename by turns), an agent appends to it
    through `notesctl mcp`'s `edit_note` (17), and b edits its own line
    (17), which reaches a through the hub. Before every token each tab's
    cursor was still at the end of its line; a's file and b's hold every
    token of every writer, in order, and a kept no conflict version. This
    is also Phase 5's hand check of the editor in the built app's own
    window over the Tauri bridge.

  **The user's steps for evidence 4** (the real vault, so the human runs
  them, ADR-0010), once [Phase 8](#phase-8--vaults-from-the-app) is
  merged:

  1. Decide the open question of the two trials (below): M3's 7 days
     alongside M2's, or after.
  2. On each of the two machines, whose notesd is joined to the hub (M2's
     trial): from this repository, `make app-build`, then
     `install -m 0755 build/frostnotes-app ~/.local/bin/frostnotes`, and
     start `frostnotes`; it connects to the running notesd.
  3. For 7 days, write in Frostnotes on both machines instead of
     Obsidian.
  4. Each day, on both: `notesctl sync verify`; note the result, any edit
     found missing, and anything you went back to Obsidian for.
  5. Once, check by hand what no test reaches: copy from a code block with
     its button and paste elsewhere; open a web link (the browser opens)
     and Open in default app on a PDF and an image; play and seek a video
     over 8 MiB embedded in a note (`notesblob://` now answers 8 MiB at a
     time); type with an input method, if you use one; and Open folder as
     vault, which opens the system's folder picker (Phase 8).

## Phase 8 — Vaults from the app

The app opens, creates and forgets vaults, so the user can begin with it
alone
([ADR-0024](../adr/0024-the-app-opens-and-creates-vaults-and-a-window-switches-between-them.md)).
Today only `notesctl vault add` registers one, and a window without a
vault says to run it. Batch 2 listed the vault switcher; the vault
row's menu switches between registered vaults, but opening, creating
and removing them never got a card, and the plan did not say so. It
comes before evidence 4, since a day's use starts here. Builds
[design/app.md](../design/app.md#data) (one vault per window) and the
vault manager in [specs/ui.md](../specs/ui.md#sidebars-and-panes), over
`vault.add` and `vault.remove` in [specs/rpc-api.md](../specs/rpc-api.md).

Obsidian is the model. Its start screen lists the vaults it knows (name
and path, each with a menu) beside Create new vault (a name and a
location picked with Browse) and Open folder as vault (the system's
folder picker). The vault menu at the foot of its left sidebar lists
the vaults and ends with Manage vaults…, which opens the same screen
as a dialog. Choosing a vault there switches the window in place
(Obsidian opens another window).

- **Planner, first** (the contract, the spec and the shell, so the
  cards have what they draw):
  - `schema/rpc/vault.yaml`: `vault.add` gains `create` (bool,
    optional): "Make the directory first; its parent must exist and it
    must not." notesd implements it with the same overlap check, made
    before the folder is; `notesctl vault add` gains `--create`; the
    mock does the same. Tests: a made folder registers; an existing
    one answers `conflict`; a missing parent `invalidParams`; an
    overlap answers `conflict` and makes no folder; `vault.remove`
    leaves the folder as it was.
  - `specs/ui.md`: the vault manager (as the whole window, and as a
    dialog), the vault menu's Manage vaults…, the create form, Remove
    from list's confirmation, and the window's title.
  - The shell: `tauri-plugin-dialog` and `@tauri-apps/plugin-dialog`
    2.8.1 (released 2026-10-01, Apache-2.0 OR MIT, frostmail's pin;
    the crate brings `tauri-plugin-fs`'s, which the app does not
    register), registered in `main.rs`. The capability adds
    `dialog:allow-open` and `core:window:allow-set-title`, and its test
    lists them. `app/system.ts` gains `pickFolder(title)`, the open
    dialog choosing a folder; it returns null in a browser, where the
    manager asks for a path instead.
- **Cards (batch 4)**, presentational, over the spec:
  - **T-0082 the vault manager** (`features/vaults/VaultManager.tsx`):
    the vaults by name, each with its path, the window's vault marked;
    a row opens its vault, and its menu holds Remove from list. Beside
    the list are Create new vault, Open folder as vault, and how to
    join a vault on a hub (`notesctl hub join`). It draws as the whole
    window or as a dialog with Close.
  - **T-0083 the create form** (`features/vaults/CreateVault.tsx` and
    `lib/vaultName.ts`): a name, checked as notesd and Obsidian check a
    folder's name (not empty, no `/`, `\` or leading `.`, none of
    Obsidian's forbidden characters); a location, a folder chosen with
    Browse, or typed where there is no picker; Create, enabled once
    both are good; Back.
- **Planner wiring:**
  - `data/vaults.ts`: Open folder as vault, Create new vault and Remove
    from list over `vault.add` and `vault.remove`. The window switches
    to a vault it opens or creates (`openVault`); notesd's errors show
    as notices in the person's words; Remove asks first (`ask`),
    naming what goes (notesd's history of the vault and, if it syncs,
    its place on the hub; never its files).
  - `MainWindow`: the manager in place of today's notesctl hint when
    there is no vault. The vault menu's Manage vaults…, and the command
    Open another vault (Obsidian's `app:open-vault`), open it as a
    dialog.
  - The window's title, `<note> - <vault> - Frostnotes` (the vault
    alone when no note is active): `setTitle` in the desktop app, the
    document's title in a browser.
- **Done when:**
  - With no vault, the window is the manager. Create new vault makes
    `<location>/<name>`, registers it and shows it. Open folder as
    vault registers a folder and shows it. A folder inside a vault,
    one holding a vault, or a name that exists shows notesd's reason
    as a notice and changes nothing. Each is a window test over the
    mock.
  - Manage vaults… lists the vaults and opens one in place. Remove from
    list asks, then forgets the vault and shows another, or the
    manager. Window tests, and notesd's test that the folder stays.
  - The window's title names the note and the vault (a window test).
  - In the built app, Open folder as vault opens the system's folder
    picker and its choice becomes the window's vault: a hand check in
    the user's steps for evidence 4.
  - Evidence 4 starts once this phase is merged.

## Later / ideas

- Bookmarks, canvas, note composer, and workspaces (the user has none
  saved), each when the user asks.
- More windows and pop-out tabs; stacked tabs.
- Pasting HTML as Markdown (Obsidian's Convert pasted HTML, on by
  default there); M3 pastes plain text.
- A PDF viewer (pdf.js); M3 opens PDFs with the system's application.
- Editable tables in live preview.
- Changing hotkeys, and reading `.obsidian/hotkeys.json` as M1 reads
  Obsidian's other settings.
- Syncing `.frostnotes/` (plan 0004's later work), so vault settings
  follow the user.
- Saves that send changes instead of whole text, if large notes prove
  slow (ADR-0022).
- Unlinked mentions' Link action, which writes to another note.
- Obsidian's other vault actions: Rename vault, Move vault, Reveal in
  the file manager, Open in new window (with more windows), and joining
  a vault on a hub from the app (Obsidian's Open vault from Sync).

## Open questions

- *Answered 2026-10-18:* **the latency budget with every provider on.**
  Typing holds it (12 ms at p95 on the host with batch 3's providers), so
  no provider waits for typing to pause. Scrolling is what misses its
  target, by a frame now and then (Phase 7, evidence 1); that is the
  follow-up's.
- **The two trials:** M3's needs M2's hub on the user's two machines.
  Run M2's 7 days first and M3's after, or overlap them once Phase 6
  ends? The user decides by Phase 7.

## References

- Implements: [design/app.md](../design/app.md), [specs/ui.md](../specs/ui.md), [specs/rpc-api.md](../specs/rpc-api.md) (`note` and `vault` additions), [specs/markdown.md](../specs/markdown.md) (the fixtures both parsers run)
- Decisions: [ADR-0021](../adr/0021-keep-the-workspace-and-app-preferences-on-each-device.md), [ADR-0022](../adr/0022-the-editor-saves-as-you-type-and-applies-merges-as-remote-changes.md), [ADR-0023](../adr/0023-the-app-shows-files-by-their-hash-and-uploads-them-through-notesd.md), [ADR-0024](../adr/0024-the-app-opens-and-creates-vaults-and-a-window-switches-between-them.md); [ADR-0007](../adr/0007-edit-in-codemirror-6-with-live-preview.md), [ADR-0011](../adr/0011-look-like-frostmails-sibling-with-obsidians-layout.md)
- Builds on: [plan 0001](0001-m0-foundations.md) (the app shell, spike 1), [plan 0003](0003-m1-headless-vault.md) (the write path, versions), [plan 0004](0004-m2-sync.md) (conflicts, the hub)
- Cards: [T-0038 to T-0042](../tasks/done/), [T-0043 to T-0046](../tasks/done/), [T-0047 to T-0081](../tasks/done/)
- Workflow: [design/agent-workflow.md](../design/agent-workflow.md)
- Roadmap: [plan 0002](0002-roadmap.md)
