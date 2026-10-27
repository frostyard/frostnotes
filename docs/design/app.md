# Design: the app

Living document. Rationale: [ADR-0004](../adr/0004-a-notes-daemon-with-thin-clients.md),
[ADR-0007](../adr/0007-edit-in-codemirror-6-with-live-preview.md) (the
editor and the reading view),
[ADR-0011](../adr/0011-look-like-frostmails-sibling-with-obsidians-layout.md)
(the look and the layout),
[ADR-0030](../adr/0030-the-look-comes-from-lodges-ui-package.md) (the
shared look comes from lodge's UI package),
[ADR-0021](../adr/0021-keep-the-workspace-and-app-preferences-on-each-device.md)
(the workspace and preferences),
[ADR-0022](../adr/0022-the-editor-saves-as-you-type-and-applies-merges-as-remote-changes.md)
(saves and merges),
[ADR-0023](../adr/0023-the-app-shows-files-by-their-hash-and-uploads-them-through-notesd.md)
(attachments),
[ADR-0026](../adr/0026-notesd-runs-as-a-lodge-app.md),
[ADR-0027](../adr/0027-a-narrow-window-takes-obsidians-phone-layout.md) and
[ADR-0029](../adr/0029-inside-lodge-a-rooted-link-no-file-answers-opens-through-lodge.md)
(the web app inside lodge),
[ADR-0031](../adr/0031-the-desktop-app-ships-as-a-flatpak-with-its-own-notesd.md)
(the Flatpak).
Contracts: [specs/ui.md](../specs/ui.md), [specs/render.md](../specs/render.md) (the reading view's HTML), [specs/rpc-protocol.md](../specs/rpc-protocol.md),
[specs/rpc-api.md](../specs/rpc-api.md), [specs/markdown.md](../specs/markdown.md)
(the syntax both parsers read).

**Status: M0's shell is built** ([plan 0001](../plans/0001-m0-foundations.md#phase-4--app-shell)):
the transports, the mock, the title bar, the status bar, and the
CodeMirror spike (`app/src/spike`); so are M3's `notesblob://` and the
dev gateway's `/blobs/` (Phase 3), and M3's data layer, workspace, layout,
commands, editor core, reading view, merge view and history below
(Phase 5), and Phase 6's menus, notices, prompts, status bar and
sidebar panes.
M3's design is built through its Phase 8 ([plan 0005](../plans/0005-m3-desktop-app.md)).
[In a browser](#in-a-browser) is M4's design
([plan 0006](../plans/0006-m4-notes-inside-lodge.md)), rewritten as each phase
lands.

## Overview

The app is a Tauri v2 window running a React UI, a thin client of notesd
like `notesctl` and MCP. It holds no notes of its own: it lists a vault's
files from notesd, edits a note's exact text in CodeMirror, saves it with
its base, and shows what notesd renders. The same UI runs in a browser
against the dev gateway, and in M4 as the web app the hub serves
([In a browser](#in-a-browser)).

```
 features/ (presentational) ◄── app/ (containers, layout) ◄── workspace/ (tabs, splits, sidebars)
                                     │
                                     ├── editor/  documents, saves, merges, live preview, merge view
                                     ├── reading/ notesd's HTML, sanitized again, embeds, math
                                     ├── commands/ the command registry and the window's keys
                                     ▼
                       data/ (Session, stores) ◄── Client (rpc/gen/api.ts)
                                                     │ Transport: tauri (bridge.rs ⇄ notesd.sock)
                                                     │            ws (devgw; lodge's /app/notes/api in M4), mock
 <img src="notesblob://localhost/<sha256>.png"> ◄── protocol.rs ◄── notesd's blobs/
```

## Source layout (`app/src`)

| Path | What | Owner |
| --- | --- | --- |
| `main.tsx` | starts the `web` build from `lodge/main.tsx` and every other from `desktop.tsx`, each branch's imports out of the other's bundle | planner |
| `desktop.tsx` | the desktop app's and the development browser's start: picks the transport (`VITE_NOTESD_WS`, `VITE_MOCK=1`, else Tauri), runs a spike (`FROSTNOTES_SPIKE`), renders `App` | planner |
| `lodge/` | the web build inside lodge: lodge-client's handshake and `connectApi` (`main.tsx`), the `LodgeBridge` (route, context, navigate, theme, open), and canonical routes (`routes.ts`) | planner |
| `rpc/` | `transport.ts` (JSON-RPC over lines), `transport/tauri.ts`, `transport/ws.ts`, `gen/api.ts` (generated) | planner |
| `rpc/mock/` | `MockTransport`: an in-memory notesd with notesd's semantics and a vault of made-up notes | planner |
| `data/` | `Session`, the stores (vaults, files, sync, settings), the document registry | planner |
| `workspace/` | the workspace model: its tree, operations, validation and persistence | planner |
| `editor/` | documents and saves, the editor's extensions, the live-preview framework, the merge view | planner |
| `editor/syntax/` | Lezer extensions for Obsidian's syntax, one per file, against the shared fixtures | cards |
| `editor/widgets/` | live-preview widgets, one kind per file | cards |
| `reading/` | the reading view: notesd's HTML, DOMPurify, links, embeds, math | planner |
| `commands/` | the command registry and the window's key listener | planner |
| `app/` | `App`, the layout and splitters, containers that connect features to data | planner |
| `features/<area>/` | presentational components and their tests: `chrome`, `workspace`, `explorer`, `search`, `links`, `outline`, `properties`, `tags`, `switcher`, `palette`, `status`, `settings`, `history`, `menu`, `notice`, and `mobile` (a narrow window's view header, drawers, bottom bar, tab switcher and editing toolbar) | cards |
| `lib/` | pure helpers: the keymap, fuzzy matching, the file tree, the outline, formatting | cards |
| `styles/` | `tokens.css` (Frostnotes' own two tokens), `app.css` (Tailwind entry, imports the shared look), `editor.css` (the editor's and the reading view's styles) | planner |
| `testing/` | Testing Library setup and render helpers | planner |
| `spike/` | M0's CodeMirror spike (`make spike-cm6`), its measurement of the app's editor, and the drivers of M3's exit evidence (`make m3-evidence`) | planner |

Presentational components take data and callbacks as props, hold only
transient state (hover, an open menu, a field being typed), and import
nothing from `data/`, `rpc/`, `workspace/` or `editor/` except types.
Containers in `app/` connect them. Rust stays small: `bridge.rs` moves
lines, `protocol.rs` serves blobs, `main.rs` sets up the window.

## Data

- **Connection.** `Session` connects, calls `rpc.hello`, subscribes to
  events from the last durable sequence number the app applied
  ([ADR-0021](../adr/0021-keep-the-workspace-and-app-preferences-on-each-device.md)),
  loads the stores, and reconnects every second when the connection
  ends. After the first connection the window stays while it reconnects,
  with the closed client, whose calls fail until the new one arrives, so
  no editor goes away with what was typed; the status bar says why. On
  reconnect it reloads the stores, and each open document saves what is
  unsaved or reads its note again, applying any difference as another
  writer's change.
- **Events.** Every event, replays included, goes first to the event
  feed (`data/events.ts`), whose listeners keep state across runs (saved
  workspaces follow moves), and which records the last sequence number
  in `frostnotes.seq`. The stores then skip replays, which their loads,
  made after subscribing, already reflect. The files store applies each
  note event after its listing's starting point over the listing, in
  order, and queues those that arrive while it loads, so it ends where
  the disk is; a changed file's time is the event's arrival, until the
  next listing.
- **One vault per window** ([ADR-0024](../adr/0024-the-app-opens-and-creates-vaults-and-a-window-switches-between-them.md)). The window shows one
  vault; the vault menu at the foot of the left sidebar switches it in
  place, and the app reopens the last one. The vault manager
  (`app/VaultsView.tsx` over `features/vaults/`, and `data/vaults.ts`)
  opens a folder as a vault (`vault.add` of a folder from the system's
  picker, `pickFolder`, or a typed path in a browser), creates one
  (`vault.add` with `create`, notesd making the folder) and removes one
  from the list (`vault.remove`, which keeps its files) after asking
  what goes. It is the whole window while there is no vault, and
  otherwise a dialog from the menu's Manage vaults… or the command Open
  another vault. notesd's refusals show as notices in the person's
  words. The window's title names the active note and the vault.
- **Files:** `note.files` lists a vault's files and folders (path, kind,
  hash, size, modification time) once, and `note.changed`, `note.moved`
  and `note.removed` keep the list current. Folders have no events: after
  its own `note.mkdir`, `note.moveFolder` or `note.deleteFolder` the app
  lists again, and an empty folder made elsewhere shows at the next
  listing. It feeds the file tree, the
  quick switcher, link suggestions and attachments' hashes. 666 notes is
  about 100 KB of JSON.
- **Sync:** `sync.status` per vault, kept current by `sync.changed`.
- **Settings:** the vault's settings (`vault.settings`, changed with
  `vault.configure`, loaded again on `vault.configured`) and the app's
  preferences (local storage, `data/prefs.ts`: a stored value a
  preference cannot take falls back to Obsidian's default), both per
  [ADR-0021](../adr/0021-keep-the-workspace-and-app-preferences-on-each-device.md).
- **Documents:** the registry of open notes (below).

## Workspace

- **Model** (`workspace/`): a tree of splits (`row` or `column`, child
  sizes as fractions) and tab groups; a tab group holds tabs and the
  index of its active one; a tab holds a view state and its back and
  forward stacks (at most 50 each). View states: `empty`, `note` (path,
  mode `edit` or `read`, scroll and cursor), `history` (path), `merge`
  (path and the other side), `file` (an attachment's path). The left and
  right sidebars are tab groups of panes, with a width and an open flag.
  One tab group is active, and its active tab is the active tab. The
  Files pane's sort order and open folders are the workspace's too.
- **Operations** are pure functions from a workspace to a workspace, so
  each is a unit test: open (here, in a new tab, to the right, below),
  close, close others, reopen the last closed (the last 20), activate,
  next and previous, the nth, move a tab within or between groups or to a
  group's edge (a new split), split, resize, back and forward, pin, apply
  a move or a delete, and validate. Empty splits collapse; the last tab of
  the last group becomes an empty tab rather than closing. Closing a tab
  activates the one that takes its place; opening a note in an empty tab
  keeps no history of the empty view; a delete drops the file from history
  and the recent files, while a tab showing it stays and says so. IDs are
  `t`, `g` and `s` with a counter the workspace keeps, so operations are
  deterministic.
- **Opening links** follows Obsidian: a click opens in the current tab,
  or in a new tab when the current one is pinned; Ctrl+click opens in a
  new tab; Ctrl+Alt+click opens to the right; middle click opens in a new
  tab. A new tab goes right after the active one.
- **Persistence** ([ADR-0021](../adr/0021-keep-the-workspace-and-app-preferences-on-each-device.md)):
  local storage, one document per vault, written 500 ms after the last
  change and when the page goes away; recently opened files (the last 50)
  ride along. Validation (`workspace/validate.ts`) keeps what it
  understands of a stored workspace and drops the rest, so no stored
  layout wedges the window. `wireWorkspace` listens to the event feed
  before `Session` connects, so moves and deletes, replays included,
  reach the shown vault's workspace and every other vault's stored one.
- **The panes** (`app/Panes.tsx`, containers in `app/panes/`) draw
  batch 2's presentational panes over the stores. Files: `lib/fileTree.ts`'s
  rows from the files store, its commands through notesd (renames and
  drops move with `note.move` or `note.moveFolder`, links updated as the
  vault's setting says, else after asking; New folder makes "Untitled"
  and renames it in place; Delete asks first while Confirm file deletion
  is on); notesd sends no event for an empty folder, so a window that
  makes, moves or deletes one lists the folders again. Search:
  `note.search` 250 ms after the query last changed, at most 100 notes,
  Obsidian's `[key]` listed with `note.list`; the query lives in
  `data/panes.ts`, where the Tags pane (`tag:#name`), All properties
  (`[key]`) and Ctrl+Shift+F (the editor's selection) set it. Backlinks:
  `note.backlinks` grouped by note, each link's line from its
  `context`, and unlinked mentions from a search for the title and
  aliases as phrases, leaving out the notes that link; a line opens its
  note at the line. Outgoing links: `note.links` through
  `lib/outgoing.ts`; an unresolved one is created where new notes go.
  Tags and All properties: `note.tags` and `note.properties`, sorted by
  name or count. Outline: the active editor's headings from its syntax
  tree (`editor/outline.ts`), read again 150 ms after the text changes,
  the heading under the cursor marked; the editors map tells its
  subscribers about every change. Each pane reads again when the vault's
  files change. F2 renames the active file in the Files pane until batch
  3's inline title.
- **Layout** (`app/MainWindow.tsx`, `app/WorkspaceView.tsx`): the ribbon,
  the sidebars and the workspace are columns, each drawing its part of
  the title bar row, so the row's segments meet the panes' edges without
  measuring; groups whose top edge is the workspace's draw their strips
  there, and the top-right one keeps clear of the window controls while
  the right sidebar is closed. Dragging a tab over a group shows where it
  lands (between tabs on the strip; the group's end in its middle; a new
  split in its outer quarter) and drops it there with `moveTab`.

## Editor

### Documents, saves and merges

Per [ADR-0022](../adr/0022-the-editor-saves-as-you-type-and-applies-merges-as-remote-changes.md),
the document registry (`editor/registry.ts`) holds one `NoteDoc`
(`editor/document.ts`) per vault and path while any tab shows it, and
after the last tab goes until what it holds is saved:

- `base`, the hash notesd last confirmed, and `baseText`, the text it
  names, and the text itself; each view keeps its own state and history,
  and a user's change in one is forwarded to the others with the
  `forwarded` annotation and outside their histories, so each tab's
  Ctrl+Z undoes only its own typing.
- **States:** clean (the text is `baseText`), dirty, saving (one write in
  flight, holding the text it sent). A change marks it dirty and arms the
  timers: 1 s after the last change, 10 s after the first. Ctrl+S, blur,
  closing or leaving the tab, a move, a delete and the window's close save
  at once.
- **An answer** sets `base` and `baseText`. A merged answer's content
  differs from what was sent: the registry diffs the two
  (`@codemirror/merge`'s diff, with a time budget), maps the change over
  the edits made since the send, and dispatches it to every view with
  `addToHistory: false` and the `remote` annotation. With conflicts, the
  document records a notice for its tabs, naming the writer whose side
  won and the conflict version, both of which the answer carries.
- **Events:** `note.changed` with a hash it does not know reads the note
  (clean) or saves at once (dirty); `note.moved` renames the document and
  its tabs; `note.removed` creates it again with unsaved edits, else marks
  its tabs deleted.
- **Errors:** a base notesd refuses ("has no version … to merge from",
  "… is gone") makes the document take the file's text and keep the
  editor's, whose notice's Compare opens the merge view against it; a
  write that fails otherwise keeps the document dirty, retries after 2,
  4, 8 … 60 s, and shows the error in the tab; nothing is dropped while
  the window is open, and closing it with unsaved text saves first and
  asks if that fails.
- **Tests:** every writer pair (an outside edit while typing, a pull, an
  agent's write, a second tab) runs against the mock's scripted merges
  with headless views (`editor/document.test.ts`).

### Extensions

- **Syntax:** `@lezer/markdown` with GFM and Frostnotes' extensions for
  Obsidian's syntax ([specs/markdown.md](../specs/markdown.md)), one
  file each in `editor/syntax/`: wikilinks, embeds (an `Embed` around a
  `WikiLink`), tags (`Hashtag`), highlights, block IDs, inline and
  block math (`InlineMath`, `BlockMath`, by notesd's rules, so neither
  holds a link or tag), and footnote references and one-line
  definitions; the frontmatter is one node, none of it markdown, for the
  properties block to draw. Two of the spec's syntaxes are not nodes.
  Comments are ranges (`editor/syntax/comments.ts`), found as notesd
  finds them: a `%%` outside code, math and HTML opens one that the next
  `%%` closes, across blocks, so a node could not hold one; they show in
  the tertiary color in both modes. Callouts are blockquotes whose first
  line `readCallout` reads (type, fold, title), their content parsed as
  usual. `extract.ts` reads links, tags (not those in a link's text) and
  block IDs (alone only after a list, quote or table) as Parse reports
  them. Where the two parsers must agree (links, tags, block IDs, and
  where code and math hide them), Vitest runs every one of the spec's
  `F` fixtures through Lezer as Go runs them through goldmark
  (`editor/syntax/fixtures.test.ts`), and the word count
  (`editor/words.ts`, the body outside comments) against each fixture's
  `words`.
- **Formatting:** Obsidian's editing commands are `StateCommand`s
  (`editor/format.ts`) the registry runs on the active editor: toggling
  bold, italics, strikethrough, highlight, code and comments around the
  selection or the word at the cursor, inserting a link, toggling a
  task's checkbox, deleting lines and setting headings. Follow link
  under cursor (and its new tab and split forms) follows the link at
  the cursor as a click would.
- **Keys:** CodeMirror's default keymap without the bindings Obsidian
  removes (Alt+↑ and Alt+↓ with and without Shift, Alt+L, Alt+V, Ctrl+M)
  and without any key the app's keymap binds (CodeMirror's Ctrl+D,
  Ctrl+I, Ctrl+Enter and Ctrl+/ would otherwise run first), its history
  keymap, Tab and Shift+Tab to indent, and its search panel restyled. The
  app's commands (Ctrl+B, Ctrl+L, …) are commands, not editor keymaps
  ([Commands and keys](#commands-and-keys)).
- **Settings:** the Editor section's switches reach the view through a
  compartment: line numbers, folding (headings with the markdown
  language, by indentation with Fold indent, behind a fold gutter), smart
  lists (the markdown keymap), auto-pairing brackets and Markdown's marks
  around a selection, indentation, and the indentation guides.
- **Page previews:** Ctrl over a link in the editor or the reading view
  shows the note it leads to (`features/preview/PagePreview.tsx`),
  rendered by notesd as an embed of it; its title opens the note.
- **Completion:** `@codemirror/autocomplete` suggests files and notes'
  aliases after `[[`, ranked as the quick switcher ranks them, headings
  (`[[note#`) and blocks (`[[note#^`), and tags after a tag's `#`, by
  `lib/fuzzy.ts`, never in code or math (`editor/complete.ts`). A note's
  editor gives it a catalog (`editor/catalog.ts`): the files store's
  files, link text by the vault's New link format, another note's
  headings and block IDs from its open document or `note.read`, and
  `note.tags`, kept 10 s, and the notes' aliases from `note.list`
  (`data/aliases.ts`), which the quick switcher reads too.
- **Links in the editor:** `note.resolve` resolves the links of the
  visible lines from the note's folder, as notesd's index does, in one
  batched call cached until a file is added, moved or removed (the files
  store's `pathsVersion`; `editor/links.ts`); unresolved links show
  dimmed, and following one creates the note under its own folders when
  it names any, else where new notes go (inside lodge, a rooted one
  opens through lodge instead: [In a browser](#in-a-browser)). A web
  link opens outside the app.
- **Modes:** live preview (the default), source mode (the same editor
  without the preview decorations) and the reading view. Ctrl+E switches
  between editing and reading; source and live preview are a preference
  and a command.

### Live preview

Live preview is decoration over the source, never a transformation
([ADR-0007](../adr/0007-edit-in-codemirror-6-with-live-preview.md)).
The framework (`editor/preview/`) owns:

- **Providers.** A widget kind is a provider: the syntax nodes it
  handles, whether it is inline or block, and a function from a node and
  a context (the document, link resolution, blob URLs, preferences) to
  decorations. Inline providers run over the visible ranges in a view
  plugin; block providers, which replace line breaks, run in a state
  field over the whole document, incrementally by changed ranges and by
  what each step of the background parse adds.
- **The reveal rule.** Syntax shows as typed on every line a selection
  touches; a block widget (a callout, a table, an embed, display math, the
  properties block) shows its source when a selection enters its range.
- **Events.** A widget's clicks go to its provider, which answers with a
  transaction (a checkbox toggles one character; a click on a block
  shows its source), a command (follow a link, search for a tag, copy a
  code block), or that it handled the click itself (a callout's fold).
- **Caching.** Widgets compare equal by their source text and inputs, so
  a keystroke elsewhere redraws nothing.
- **Cost.** A fling makes the view render a screen's worth of new lines
  every other frame, so what a line costs to build, style and lay out is
  the frame's budget ([plan 0005, Phase 7](../plans/0005-m3-desktop-app.md#phase-7--review-and-exit-evidence)).
  Hidden syntax is a mark that does not display, not a replacement, which
  the view would pad with two buffer images. A block widget estimates its
  height from its source, so the view renders no more than a scroll needs.
  KaTeX draws each formula once and widgets clone the drawing; its MathML,
  read only by assistive technology, is attached a frame later. Two
  changes in dependencies complete it
  ([ADR-0025](../adr/0025-patch-what-costs-the-editor-its-frames.md)):
  the app's copy of Tailwind's preflight leaves the editor's lines out of
  its pseudo-element reset, and a patch makes CodeMirror's height map walk
  only what is outdated.
- **The environment** (`PreviewEnv`) is what the app gives the
  providers: link resolution, where commands go, a vault file's URL
  (its hash's blob URL), the Load remote images preference, and
  rendering markdown into an element.

M0's spike (`app/src/spike/livePreview.ts`) is its first draft: hidden
marks, headings, links, wikilinks and checkboxes, measured at 9 ms at p95
a keystroke on a 10,000-line note. Its decorations are the framework's
first providers (`editor/preview/basic.ts`); the framework is
`editor/preview/framework.ts`, and `editor/preview/index.ts` lists every
provider: highlights, tags as pills, inline code and footnote references
(`inline.ts`); code blocks with their language and a copy button
(`code.ts`); quotes, bullets, rules and done tasks (`lists.ts`); math
with KaTeX (`math.ts`); embedded images, audio and video (`media.ts`);
and callouts, tables and embeds of notes (`rendered.ts`). Those last
three are blocks drawn exactly as the reading view draws them: the
environment renders a block's source with `note.render` as the note
would render it, sanitizes it as the reading view does, and fills in its
files and math (`editor/rendered.ts`), so no provider builds HTML from
the note's text. A note's editor keeps those renderings by markdown and
renders what is on screen again a second after the vault changes, so an
embedded note follows its edits. The framework keeps the typing budget
with every provider on: providers are measured together in the exit
evidence.

### Properties

The note's frontmatter shows in live preview as the properties block (a
block widget, `editor/preview/properties.tsx`, holding the React
component `features/properties/PropertiesBlock.tsx`): a row per key, its
type's icon and its value, editable. An edit there is a transaction on
the YAML text that changes only that key's lines and keeps every other
byte, as MCP's property edits do in Go ([specs/mcp.md](../specs/mcp.md)):
`editor/properties.ts` ports them over the `yaml` package, writing a
string that needs quotes in double quotes and a date plain, as Obsidian
writes them. A key's type is the vault's (`note.properties`), else what
its key or value says. The widget keeps its DOM while the frontmatter
changes, so a field keeps its focus, and takes its own events. Add file
property (Ctrl+;) makes an empty frontmatter where there is none and
opens the block's new row. The Properties in document preference shows
the block, hides the frontmatter, or leaves its YAML, as source mode
does. A note opens with the cursor where its body starts. The reading
view draws the same block over the rendered note and writes its edits
into the note's document.

The inline title (`editor/title.tsx`, a block widget above the first
line in both editing modes, and above the reading view) is the file's
name; renaming it moves the file as the Files pane does, asking about
links. Insert template lists the template folder's notes
(`features/prompt/TemplateChooser.tsx`), has notesd expand the chosen one
(`note.template`), adds its properties the note lacks, and puts its body
at the cursor. Open today's daily note opens `note.daily`'s note, which
notesd makes from the daily template when missing.

## Reading view

notesd renders a note (`note.render`, from the document's text when it
is open) with goldmark and the Obsidian extension, then sanitizes it by
an allowlist (`internal/render`; [specs/render.md](../specs/render.md)
pins the HTML and the allowlist): no script, event
handler, form, frame, `style` or `javascript:` URL; raw HTML in a note
keeps listed elements and attributes only. In the HTML, an internal link
is `a.internal-link` with `data-href` (as written) and `data-path` (the
file it resolves to, absent when unresolved); a vault file is an `img`,
`audio`, `video` or `a.file-embed` with `data-blob` (its hash and
extension) instead of a URL; an embedded note is `div.embed` with its
rendered content, three levels deep at most; a callout is
`div.callout[data-callout]`; math is `span.math` or `div.math` with
`data-tex`; a tag is `a.tag[data-tag]`; a task's checkbox carries
`data-line`.

The app sanitizes again with DOMPurify and the same allowlist
(`reading/sanitize.ts`), sets each `data-blob` element's `src` to its
blob URL, renders math with KaTeX, enables each task's checkbox (notesd
renders it disabled), and inserts the result into the view's element,
styled by `styles/editor.css`. It renders again 300 ms after the
document's text changes, so the view shows what was typed, saved or
not. A DOMPurify that fails its first sanitization (as under happy-dom,
whose DOM it does not trust) is replaced by escaping: the view then
shows the HTML as text, never unsanitized. Clicks on links and tags
become commands; a checkbox toggles its line in the document, as live
preview does. The CSP forbids inline script
([ADR-0007](../adr/0007-edit-in-codemirror-6-with-live-preview.md)).

## Attachments

Per [ADR-0023](../adr/0023-the-app-shows-files-by-their-hash-and-uploads-them-through-notesd.md),
the transport's `blobURL(hash, ext)` gives a file's URL:
`notesblob://localhost/…` in Tauri, the dev gateway's `/blobs/…` in a
browser, a `data:` URL from the mock. Images, audio and video show
inline in live preview, in the reading view and in a `file` tab; a PDF
shows with a button that opens it with the system's application (the
shell's `open_file`; [Window](#window)). Rendered HTML is finished
(`reading/finish.ts`'s `fill`) in a template, which loads nothing, before
the page has it: with the Load remote images preference off, a remote
`img` loses its source there and is never requested. `notesblob://`
answers a range at most 8 MiB long and a file without a range only up to
64 MiB, so a large video plays without sitting whole in memory. Pasting
or dropping a file into an editor, or the command Attach file (the
phone's toolbar has it), calls `note.upload` with the file, at most 10 MB,
under the vault's Default location for new attachments and a name no
file has (a nameless pasted image is `Pasted image YYYYMMDDHHmmss.png`),
then inserts `![[name]]` (a markdown image without wikilinks) at the
cursor, on a line of its own at a line's start (`editor/attach.ts`, M4).

Both servers of a hash (`protocol.rs` in Tauri, `internal/devgw` in a
browser; [specs/rpc-protocol.md](../specs/rpc-protocol.md#transports))
name the content type by extension, from one table:

| Extensions | Content type |
| --- | --- |
| `png`, `jpg` and `jpeg`, `gif`, `webp`, `avif`, `bmp`, `svg` | `image/png`, `image/jpeg`, `image/gif`, `image/webp`, `image/avif`, `image/bmp`, `image/svg+xml` |
| `mp3`, `wav`, `m4a`, `3gp`, `flac`, `ogg` and `oga`, `opus` | `audio/mpeg`, `audio/wav`, `audio/mp4`, `audio/3gpp`, `audio/flac`, `audio/ogg`, `audio/opus` |
| `mp4`, `webm`, `ogv`, `mov`, `mkv` | `video/mp4`, `video/webm`, `video/ogg`, `video/quicktime`, `video/x-matroska` |
| `pdf`, `md` | `application/pdf`, `text/markdown; charset=utf-8` |
| anything else | `application/octet-stream` |

Under `make app-dev` the page comes from Vite's server, and WebKitGTK may
not load a custom scheme into it (frostmail found that with `mailpart:`
and serves `/mailpart/` from Vite in development); Phase 5 checks, and
serves `/blobs/` from Vite there if it must.

## History and merges

- **History** (a `history` tab): the file's versions from
  `note.versions`, each readable with `note.readVersion`, and for a
  synced vault the hub's revisions from `sync.history`, each readable
  with `sync.readRevision`, which fetches another device's content from
  the hub. `app/HistoryView.tsx` draws them with the history list
  (`features/history/`); selecting one shows it read-only with the
  editor's styles, or, with Show changes, `@codemirror/merge`'s unified
  view of how it differs from the current text. Restore is `note.revert`
  with the document's base, and Compare opens the merge view. The
  command Open version history and More options open the tab.
- **Merge** (a `merge` tab, `editor/MergePane.tsx`): `@codemirror/merge`'s
  side-by-side view, the document on the left, a view of it like any
  other, editable, and the other version on the right, read-only: a
  conflict version, a hub revision, a version from the history, or the
  text the editor held when notesd refused its base. Taking a chunk from
  the right is an edit of the document, so it saves with its base and
  Ctrl+Z undoes it. It opens from a conflict notice (which names the
  conflict version notesd kept; from a notesd that does not, Compare
  opens the history), from the history, and from the status bar's
  conflicts (`sync.conflicts`). Done dismisses the notice, calls
  `sync.resolve` when the vault syncs (a file not flagged on the hub
  answers an error, which is ignored), and closes the tab.

## Commands and keys

- **The registry** (`commands/`) lists every command once: its ID
  (Obsidian's, where Obsidian has the command: `switcher:open`,
  `editor:toggle-bold`), its name, when it applies (an editor is
  active, a note is open, a sidebar exists) and what it does. The command
  palette, the menus and the keys all run commands through it.
- The registry (`commands/registry.ts`) holds the workspace's commands
  now (`app/commands.ts`): tabs, groups, history, sidebars, creating a
  note where the vault's settings say (`Untitled.md`, then `Untitled 1.md`
  …), the quick switcher and the palette, and Reset workspace layout
  (`frostnotes:reset-workspace-layout`, Frostnotes' own); a button whose
  command is not registered yet (settings, daily notes, templates) is
  disabled until its card lands. The registry tells its subscribers when
  commands come and go, and the window draws its chrome again then.
- **Over the layout** (`data/overlays.ts`, drawn by `app/Overlays.tsx`):
  one menu at a time, the quick switcher or the command palette, and the
  notices. The menus (`app/menus.ts`) are the tab's, the tab list, More
  options, the vault menu, the status bar's editing mode and conflicts,
  the Files pane's sort order and its rows'; Open in default app waits
  for the opener plugin. Move file to and Move folder to open the folder
  chooser (a folder never into itself). Questions are the confirmation
  dialog (`ask`, a promise of the answer): Delete asks while Confirm
  file deletion is on, a move asks whether to update links while the
  vault does not update them on its own, and closing a window with
  unsaved notes asks. The settings dialog (`app/Settings.tsx`; Ctrl+,,
  F1 at Hotkeys, the vault row's button) builds its sections from the
  preferences, the vault's settings (changed with `vault.configure`),
  the keyboard map and the sync state, and applies each change at once;
  the theme sets `data-theme` on the root and the font size the editor's
  and the reading view's body. The quick switcher lists the
  vault's notes and the workspace's recent files (aliases join when
  `note.list` carries them); Shift+Enter creates the note where new notes
  go. The palette lists the commands that apply as it opens, the ten run
  from it last first. A menu or prompt gives focus back to what had it
  when it closes. The status bar (`app/status.ts`) shows the sync state
  in words, the active note's backlinks (`note.backlinks`, again as the
  vault changes), its words and characters, or its selection's, counted
  150 ms after the last change, and its mode.
- **The window has one key listener,** in the bubble phase, so
  CodeMirror and fields handle their own keys first. It skips events
  already handled, maps the rest with `lib/keymap.ts` (Obsidian's default
  hotkeys, [specs/ui.md](../specs/ui.md#keyboard-map)), and runs the
  command when it applies. In a text field outside the editor, only
  presses with Ctrl or Alt, and F1 and F2, reach it.

## Window

- Undecorated. The title bar row holds the sidebar toggles, the
  sidebars' pane tabs and the top row of tab strips; its empty space
  drags the window, and a double click toggles maximize. The capability
  file grants the window permissions the controls use and the app's own
  commands.
- The webview's own context menu is off (`system.ts`'s
  `suppressNativeMenu`, from the first line of the page); the app draws
  its own menus. The webview downloads nothing (`on_download`).
- A view that fails while it draws shows why in its tab
  (`ViewBoundary`); the rest of the window works on.
- Links to the web open in the system browser (`http`, `https`, `mailto`
  only); the webview never navigates away from the app. The shell's two
  commands (`src-tauri/src/open.rs`) are the only ways the page reaches
  the system: `open_url` checks a link's scheme, and `open_file` (Open in
  default app) opens a vault file only when it is a regular,
  non-executable file inside the vault folder, of a document or media
  type, never a script, a `.desktop` entry or HTML. Both run `xdg-open`
  with one argument and no shell. In a browser, a web link opens in a new
  tab and files do not open.
- View's Zoom in, Zoom out and Reset zoom step through a browser's zoom
  levels with the webview's own zoom, kept as a preference.

## In a browser

M4 runs the app inside lodge, in a frame of lodge's shell, in Firefox on
another machine and on the user's phone
([ADR-0026](../adr/0026-notesd-runs-as-a-lodge-app.md),
[lodge ADR-0009](https://github.com/frostyard/lodge/blob/main/docs/adr/0009-apps-switch-in-frames-that-stay-loaded.md),
[specs/ui.md](../specs/ui.md#in-a-browser)). It is the same code as the
desktop's, with a different transport and frame:

- **The build.** `make web-build` runs `vite build --mode web` in nsl
  into `build/web/`, with Vite's `base` set to `/app/notes/`.
  - lodge serves it from `/usr/share/lodge/apps/notes/web/`
    ([lodge ADR-0007](https://github.com/frostyard/lodge/blob/main/docs/adr/0007-lodge-serves-every-apps-web-build-from-disk.md)),
    with notes' manifest, `packaging/lodge/app.json`, beside it.
    `make lodge-app` lays that tree out in `build/lodge/apps/notes/`,
    the manifest at the build's version. `make lodge-package` stages
    the same tree with the binaries and units under `usr/`
    ([specs/lodge-app.md](../specs/lodge-app.md#what-notesd-installs)).
  - In that mode `main.tsx` connects through lodge, and the mock, the
    spikes and the shell's commands stay out of the bundle.
  - The build carries `icon.svg`, from the icon's master, for lodge's
    rail.
  - `index.html`'s viewport has `viewport-fit=cover`, so the safe-area
    insets the narrow layout uses are not zero on iOS.
  - There is no web manifest of notes' own: lodge installs once, for
    every app
    ([lodge ADR-0008](https://github.com/frostyard/lodge/blob/main/docs/adr/0008-lodge-installs-as-one-home-screen-app.md)).
- **The shell, through `@frostyard/lodge-client`**
  ([lodge-client](https://github.com/frostyard/lodge/blob/main/docs/specs/lodge-client.md)):
  - **Startup.** `main.tsx` first calls lodge-client's
    `connectLodge({app: "notes"})`.
    - lodge-client posts `ready {contract: 1}`.
    - The shell answers with `init {contract, contentOrigin, shortcut,
      theme}`, then `visible`.
    - `connectLodge()` resolves on `init`.

    Only then does the app open its WebSocket, with `connectApi()`.
  - **Routes.** A note's canonical URL is `/notes/<vault id>/<path>`,
    each segment percent-encoded as `encodeURIComponent` does, and its
    frame's URL `/app/notes/<vault id>/<path>`.
    - MCP's results through lodge give the same URL as `url`
      ([specs/mcp.md](../specs/mcp.md#results)). Both sides test against
      `src/lodge/note-routes.json`.
    - The app sends `route {path, title}` when the active tab changes.
    - It opens a note on `navigate {route}`, and opens its own route on
      load.
    - A route whose note is gone opens the quick switcher with its
      name.
  - **Context.** `context {kind: "note", ref: "notes/<vault id>/<path>",
    title}` follows the active note, for lodge's companion.
  - **Links.** A link to a lodge URL (the shell's origin) goes to
    `open {url}`; other web links open in a new tab.
    - A rooted link no vault file answers goes to `open` too, and is
      never made a note
      ([ADR-0029](../adr/0029-inside-lodge-a-rooted-link-no-file-answers-opens-through-lodge.md)). `followLink` and the Outgoing links
      pane ask `openInLodge` (`app/commands.ts`), which encodes the
      target with `lodgeAddress` (`src/lodge/routes.ts`); a first
      segment no lodge app can have gets a notice instead.
    - A rooted link that resolves opens its file, and outside lodge an
      unresolved one still creates the note.
  - **The switcher's key.** lodge-client catches the key `init` names,
    in the capture phase, before notes' key listener, and posts
    `shortcut {key}` itself; the window's key listener never binds it.
  - **Visibility and theme.** The heartbeat follows the page's
    visibility, not the frame's
    ([lodge-client](https://github.com/frostyard/lodge/blob/main/docs/specs/lodge-client.md#visibility-and-theme)):
    a hidden frame keeps its session. On `visible {true}` a lost
    connection is tried at once. `theme` sets the light or dark scheme
    while the Theme preference is System.
- **The connection** is notes' own `connectApi()` in
  `rpc/transport/ws.ts`, beside the dev gateway's `connectWS()`. It
  opens a WebSocket to `wss://<the page's host>/app/notes/api`, with no
  token, once lodge-client's `connectLodge()` has resolved.
  lodge signed the browser in before the shell loaded, and passes who it
  is to notesd ([specs/lodge-app.md](../specs/lodge-app.md#api)).
  A phone drops connections when it sleeps or changes networks, so:
  - `Session` reconnects after 1 s, doubling to 30 s with jitter, and at
    once when the frame becomes visible or the browser comes back
    online;
  - while visible it calls `rpc.ping` every 30 s, and drops a connection
    that has not answered in 10 s. lodge closes a session after 90 s
    without a frame;
  - each reconnect resumes events from the last sequence number and
    saves or re-reads open documents, as on the desktop.

  Sign-in failures are the shell's to show. The app only says that
  notes is out of reach, and keeps trying.
- **Who is signed in** comes from `rpc.hello`: the caller kind, the
  login, the display name and the node. The status bar's first item,
  notes' connection, names them in its tooltip and its menu
  (`StatusBar`'s `identity`, `openIdentityMenu`). With caller `session` the app
  hides what a session may not call: opening, creating and removing
  vaults, sync's status and conflicts, approving pairings, and the
  shell's commands.
- **Files by hash** load from lodge-content: the transport's `blobURL`
  is `contentURL("blobs/<mac>/<hash>.<ext>")`, the `mac` an HMAC of the
  hash under the content key `rpc.hello` gives the session, so no page
  without it can make a file's URL ([lodge-app.md](../specs/lodge-app.md#contentblobs)). Open in new browser tab,
  the menus' Open in default app in a browser (`system.ts`'s
  `openOutside`), opens an image, audio, video or PDF at its content URL
  and is hidden for other files; a file's view opens it at that URL too; where a browser will not show a file in a sandboxed
  page, the app offers the `?download=1` URL instead.
- **Offline**, the status bar says notes is out of reach. Editors keep
  their text and save it with its base on reconnect (the registry's
  retries), and leaving the page with unsaved text asks first where the
  browser allows. Nothing is stored in the browser but ADR-0021's
  workspace and preferences.
- **Narrow windows**
  ([ADR-0027](../adr/0027-a-narrow-window-takes-obsidians-phone-layout.md),
  [specs/ui.md](../specs/ui.md#narrow-windows)):
  - `app/` chooses the layout from `matchMedia("(max-width: 599px)")`,
    the frame's width.
  - The workspace model gains a narrow view, the active group's active
    tab, which leaves the stored tree as it is.
  - The view header, drawers, bottom bar, tab switcher, editing toolbar
    and bottom sheets are presentational components in
    `features/mobile/`, connected in `app/`.
  - Touch sizes come from a `(pointer: coarse)` media query over the
    tokens.
  - The editing toolbar follows the visual viewport to sit on the
    on-screen keyboard.
- **Hub settings** (`app/Settings.tsx`'s `hubSection`, drawn as the
  settings dialog's rows) list the hub's devices and pending pairings,
  with Revoke where the caller may ([hub.md](hub.md#hub-settings-in-the-app)).
  lodge manages sign-in, tokens and sessions.
- **Tests:**
  - Vitest drives the lodge transport's reconnect and heartbeat with a
    fake WebSocket and timers.
  - It drives lodge-client's messages with a fake shell that posts
    them.
  - It runs the narrow components in happy-dom with `matchMedia`
    stubbed.
  - `make m4-evidence` drives the web build in Firefox (geckodriver in
    nsl) inside lodge's development build on loopback, at a desktop size
    and at 390 × 844
    ([plan 0006](../plans/0006-m4-notes-inside-lodge.md#phase-7--the-users-hub-in-lodge-and-exit-evidence)).

## The Flatpak

Rationale: [ADR-0031](../adr/0031-the-desktop-app-ships-as-a-flatpak-with-its-own-notesd.md)
and [core ADR-0058](https://github.com/frostyard/core/blob/main/docs/adr/0058-ship-suite-desktop-apps-as-flatpaks-and-services-as-sysexts.md);
built in [M6](../plans/0002-roadmap.md) (core's
[Plan 0010, Phase 5](https://github.com/frostyard/core/blob/main/docs/plans/0010-suite-packaging.md)).
The desktop app installs as `org.frostyard.Frostnotes`, from `packaging/flatpak/`:

- **What is inside.** `/app/bin/frostnotes` is the launcher, `/app/bin/notesd`
  and `/app/bin/notesctl` are the Go binaries, and `/app/libexec/frostnotes-app`
  is the Tauri shell. `flatpak run --command=notesctl org.frostyard.Frostnotes`
  is the CLI on a desktop.
- **notesd** runs in its own sandbox of the same app, once per data
  directory: it takes an exclusive lock on `<data dir>/notesd.lock`, and a
  second notesd on that directory exits at once.
  **The launcher** (`frostnotes.sh`): `frostnotes --background` runs
  `exec notesd -log $XDG_STATE_HOME/frostnotes/notesd.log`, with the shell
  also appending notesd's output to that file, also when the Background
  portal starts it at login. notesd rotates the log itself, once it holds
  the data directory's lock: a log over 1 MiB moves to `notesd.log.1`, then
  notesd appends its output there. A shell check cannot do this, since 2
  launchers can both pass it, and a duplicate would move the log of the
  notesd that runs. The shell's redirect keeps what notesd says before the
  lock, such as "another notesd uses".
  `frostnotes [ARGS]`, when `notesctl hello`
  fails, runs `flatpak-spawn frostnotes --background`, so the Flatpak portal
  starts notesd in a new sandbox that outlives the window (until logout),
  and waits for about 10 s for it. Then it ends `flatpak-spawn` with
  `SIGKILL`: `flatpak-spawn` waits for its command and would keep the
  window's sandbox alive, it forwards every signal it can catch to notesd,
  and without `--watch-bus` the portal keeps notesd when it goes. With no
  portal the launcher logs an error and
  starts no notesd of its own (one in the window's sandbox would die with a
  duplicate window), so the app shows that notesd is not running. It then starts the
  Background request in the background and `exec`s the app.
  The request is `frostnotes-request-background`, a PyGObject helper that
  keeps its bus connection open until the portal's `Response`: the
  portal closes a request when its caller leaves the bus, so a one-shot
  `gdbus call` cannot ask the user. Allowed once, the portal runs
  `frostnotes --background` at login, so sync goes on with no window.
- **The socket** is `$XDG_RUNTIME_DIR/app/org.frostyard.Frostnotes/frostnotes/notesd.sock`
  when `FLATPAK_ID` is the app ID, the one runtime directory Flatpak shares
  between the app's sandboxes. `internal/config` and `src-tauri/src/paths.rs`
  compute it the same way. The data directory is the app's `XDG_DATA_HOME`,
  shared by the shell and notesd, so `notesblob://` reads `blobs/` as usual.
- **Permissions:** Wayland or X11, the GPU, the network, the Secret Service
  and the home directory (vaults are watched and written in place; the
  document portal cannot do that). Where `HOME` is `/var/home/<user>`
  (Snow, Fedora's atomic desktops), the host's `/home` does not exist in
  the sandbox, so notesd reads a vault path under `/home` as the same
  path under `/var/home` when only that exists.
- **The build** is offline: `make flatpak-sources` writes `cargo-sources.json`,
  `node-sources.json` and `go-sources.json` from the lockfiles (with a
  patched node generator and hash-pinned Python dependencies), and the
  `Flatpak` workflow (`.github/workflows/flatpak.yml`) checks they are
  current, then builds and tests the manifest (the smoke test starts notesd
  through the launcher with a stand-in `flatpak-spawn`, checks that the
  launcher ends it, that notesd lives on and that the log turns over, and
  checks the data directory's lock). Run `make flatpak-sources`
  after any change to `Cargo.lock`, `pnpm-lock.yaml`, `go.mod` or `go.sum`.
  A `vX.Y.Z` tag builds a release: `packaging/flatpak/stamp-version.sh`
  stamps the version and the commit into the manifest and the version, with
  its date, into the metainfo, and the workflow then publishes the OCI image
  to `ghcr.io/frostyard/frostnotes` as `vX.Y.Z` and `sha-<commit>`, moving
  `latest` only after it attested the digest. The Frostyard remote
  (`frostyard/flatpak-index`) lists only attested digests
  ([core ADR-0058](https://github.com/frostyard/core/blob/main/docs/adr/0058-ship-suite-desktop-apps-as-flatpaks-and-services-as-sysexts.md)).

## Dev and test

- `VITE_MOCK=1` runs the UI against `MockTransport` and its made-up
  vault in any browser; `make ui-dev` runs a real notesd (`-devgw`) and
  Vite, and the same UI in a browser on the host.
- `make ui-check`: Biome, the TypeScript check and Vitest (happy-dom,
  Testing Library). Editor logic is tested on `EditorState` without a view
  where it can be; views run in happy-dom where a test needs one, and in
  jsdom (`// @vitest-environment jsdom`) where DOMPurify must sanitize:
  the sanitizer's tests and the reading, merge and history views'.
- `make m3-evidence` drives the built app in WebKitGTK
  ([plan 0005, Phase 7](../plans/0005-m3-desktop-app.md#phase-7--review-and-exit-evidence)):
  the app runs with `FROSTNOTES_SPIKE` naming a driver in `spike/`. The
  editor's measurement (`editor`, after spike 1's `measure.ts`) times
  typing and scrolling in a generated 10,000-line note with every
  live-preview provider. The hotkeys (`hotkeys`, over the mock) and the
  writer pairs (`writers`, over the Tauri bridge to a real notesd) ask a
  harness, `internal/appevidence`, for what the page cannot do itself,
  in lines the shell prints (`spike_say`): xdotool presses keys under
  Xvfb, so they arrive through X, GTK and WebKit, and vim, notesctl's
  MCP server and a second notesd through a hub write the open note.

## Operational notes

- Local storage holds (in the web build, after lodge's `notes:` prefix,
  since every app shares lodge's origin) `frostnotes.workspace.v1:<vault path>`,
  `frostnotes.prefs.v1`, `frostnotes.seq` and `frostnotes.recentCommands`
  (the palette's). The command Reset workspace
  layout clears the current vault's; clearing the app's site data clears
  them all, and no note is lost.
- If notesd is not running, the window shows the connection state and
  keeps retrying; a notesd started later is picked up.
- A note whose document cannot save keeps its text in the window and
  shows why; closing the window then asks before discarding it.

## References

- Rationale: [ADR-0031](../adr/0031-the-desktop-app-ships-as-a-flatpak-with-its-own-notesd.md) (the Flatpak), [ADR-0004](../adr/0004-a-notes-daemon-with-thin-clients.md), [ADR-0007](../adr/0007-edit-in-codemirror-6-with-live-preview.md), [ADR-0011](../adr/0011-look-like-frostmails-sibling-with-obsidians-layout.md), [ADR-0021](../adr/0021-keep-the-workspace-and-app-preferences-on-each-device.md), [ADR-0022](../adr/0022-the-editor-saves-as-you-type-and-applies-merges-as-remote-changes.md), [ADR-0023](../adr/0023-the-app-shows-files-by-their-hash-and-uploads-them-through-notesd.md), [ADR-0024](../adr/0024-the-app-opens-and-creates-vaults-and-a-window-switches-between-them.md), [ADR-0025](../adr/0025-patch-what-costs-the-editor-its-frames.md); for the web app, [ADR-0026](../adr/0026-notesd-runs-as-a-lodge-app.md), [ADR-0027](../adr/0027-a-narrow-window-takes-obsidians-phone-layout.md), [ADR-0029](../adr/0029-inside-lodge-a-rooted-link-no-file-answers-opens-through-lodge.md) and lodge's [ADR-0007](https://github.com/frostyard/lodge/blob/main/docs/adr/0007-lodge-serves-every-apps-web-build-from-disk.md) to [ADR-0009](https://github.com/frostyard/lodge/blob/main/docs/adr/0009-apps-switch-in-frames-that-stay-loaded.md); frostmail's `docs/design/app.md`, which this copies
- Related: [overview.md](overview.md), [vault.md](vault.md) (writes, settings), [sync.md](sync.md) (conflicts), [hub.md](hub.md) (the hub behind lodge, its devices)
- Contracts: [specs/ui.md](../specs/ui.md), [specs/render.md](../specs/render.md), [specs/rpc-api.md](../specs/rpc-api.md), [specs/markdown.md](../specs/markdown.md), [specs/lodge-app.md](../specs/lodge-app.md) (what notesd serves to lodge), lodge's [lodge-client](https://github.com/frostyard/lodge/blob/main/docs/specs/lodge-client.md)
- Built in: [M0 app shell](../plans/0001-m0-foundations.md#phase-4--app-shell); [plan 0005 — M3](../plans/0005-m3-desktop-app.md); [plan 0006 — M4](../plans/0006-m4-notes-inside-lodge.md) ([roadmap](../plans/0002-roadmap.md))
