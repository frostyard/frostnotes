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
Status (2026-10-17): **Phase 1 done** but for the user's acceptance of
ADRs 0021 to 0023: this plan, the ADRs (proposed),
[design/app.md](../design/app.md), [specs/ui.md](../specs/ui.md), the
spec's tokens in the app's styles, and the batch-1 cards T-0038 to
T-0042, each checked against a reference.

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
  edits. Other writers' changes arrive the same way. Conflicts show a
  notice and open the merge view, `@codemirror/merge`, against the
  conflict version notesd kept.
- **Files show by their hash and upload through notesd**
  ([ADR-0023](../adr/0023-the-app-shows-files-by-their-hash-and-uploads-them-through-notesd.md)).
  `notesblob://localhost/<sha256>.<ext>` serves notesd's blob store from
  the Tauri side, the dev gateway serves `/blobs/` in a browser, and the
  hub will in M4; pasted and dropped files go up through `note.upload`,
  base64, at most 10 MB.

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
  updates, delete), and `vault.settings` and `vault.configure`; search
  gains `path:`, `file:` and `tag:`.
- **One window shows one vault.** A switcher at the bottom of the left
  sidebar changes it. More windows and pop-out tabs wait.
- **New dependencies are the planner's,** pinned, each release at least
  two weeks old: `@codemirror/merge`, `@codemirror/autocomplete`,
  `@codemirror/search`, `dompurify`, `katex` and `@tanstack/react-virtual`;
  in Go, nothing new (the sanitizer walks `golang.org/x/net/html`, already
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
  `app.css`.
- Batch-1 cards, T-0038 to T-0042: pure helpers and one presentational
  component that need nothing the planner has not built, each with a
  stub and given tests, checked against a reference that is then
  deleted.
- **Done when:** the ADRs, the design, the spec and this plan are indexed
  and cross-linked both ways; each card has passed its reference with
  `make verify` and `make ui-check` green; and the user has accepted the
  ADRs, or said what to change, before Phase 5's code merges.
- **Result (2026-10-17): done, but for the acceptance.** Reading Obsidian
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
  and its test builds a 10,101-row vault.

## Phase 2 — Executor batch 1 (alongside Phase 3)

| Card | Size | File | What |
| --- | --- | --- | --- |
| [T-0038](../tasks/todo/0038-keymap.md) | S | `app/src/lib/keymap.ts` | Obsidian's default hotkeys: a key press to a command ID, a command's hotkeys, and their display |
| [T-0039](../tasks/todo/0039-fuzzy-search.md) | S | `app/src/lib/fuzzy.ts` | Obsidian's fuzzy search for the quick switcher, the palette and link suggestions: scores and matched ranges |
| [T-0040](../tasks/todo/0040-file-tree.md) | M | `app/src/lib/fileTree.ts` | the Files pane's rows: folders and files nested, sorted as Obsidian sorts, filtered by type, flattened by what is open |
| [T-0041](../tasks/todo/0041-outline.md) | S | `app/src/lib/outline.ts` | the Outline pane's tree of headings, its visible rows, and the heading a position is under |
| [T-0042](../tasks/todo/0042-tab-strip.md) | M | `app/src/features/workspace/TabStrip.tsx` | a tab group's strip: tabs, close, new tab, the tab list, keys, and the commands it sends |

- The cards touch separate files and use nothing of each other's, so
  they may run in any order, in two executor worktrees at once
  ([agent workflow](../design/agent-workflow.md#batching-cards)).
- **Done when:** every card is merged and `make ui-check` is green.

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
  scripted merges.
- The batch-3 cards whose contracts these settle, each checked against a
  reference.
- **Done when:** `make check` and `make ui-check` are green with every
  stub in place, `make gen-check` passes, and each card has passed its
  reference.

## Phase 4 — notesd's additions (planner, alongside Phase 5)

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

## Phase 5 — The app's core (planner)

- `data/`: `Session` from the stored sequence number; the files, sync
  and settings stores.
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
  undoing only the typing; a forced conflict opens the merge view; the
  reading view shows the same note; and typing in a generated
  10,000-line note with the framework on stays under 16 ms a keystroke at
  p95 on the host.

## Phase 6 — Executor batches 2 and 3, and wiring

Cards are numbered as each batch is written; the planner wires each one
into its container as it merges.

- **Batch 2** (presentational, each written once its props' types
  exist: most need only the spec and today's API types; Tags and All
  properties wait for Phase 3's): the Files pane, the search pane,
  Backlinks, Outgoing links, Tags, All properties and the Outline pane;
  the quick switcher and the command palette over T-0039; the status
  bar; menus and notices; the settings dialog's sections; the history
  list; the vault switcher.
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

## Phase 7 — Review and exit evidence

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

## Open questions

- **Deleting a folder whose files changed since it was listed:** refuse
  the whole delete, or delete what is unchanged and keep the rest? By
  Phase 3.
- **The latency budget with every provider on:** if 16 ms breaks, which
  providers update when typing pauses instead of per keystroke? Phase 5
  measures.
- **The two trials:** M3's needs M2's hub on the user's two machines.
  Run M2's 7 days first and M3's after, or overlap them once Phase 6
  ends? The user decides by Phase 7.

## References

- Implements: [design/app.md](../design/app.md), [specs/ui.md](../specs/ui.md), [specs/rpc-api.md](../specs/rpc-api.md) (`note` and `vault` additions), [specs/markdown.md](../specs/markdown.md) (the fixtures both parsers run)
- Decisions: [ADR-0021](../adr/0021-keep-the-workspace-and-app-preferences-on-each-device.md), [ADR-0022](../adr/0022-the-editor-saves-as-you-type-and-applies-merges-as-remote-changes.md), [ADR-0023](../adr/0023-the-app-shows-files-by-their-hash-and-uploads-them-through-notesd.md); [ADR-0007](../adr/0007-edit-in-codemirror-6-with-live-preview.md), [ADR-0011](../adr/0011-look-like-frostmails-sibling-with-obsidians-layout.md)
- Builds on: [plan 0001](0001-m0-foundations.md) (the app shell, spike 1), [plan 0003](0003-m1-headless-vault.md) (the write path, versions), [plan 0004](0004-m2-sync.md) (conflicts, the hub)
- Cards: [T-0038 to T-0042](../tasks/todo/)
- Workflow: [design/agent-workflow.md](../design/agent-workflow.md)
- Roadmap: [plan 0002](0002-roadmap.md)
