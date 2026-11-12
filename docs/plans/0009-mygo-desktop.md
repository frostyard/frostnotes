# Plan: the desktop app on MyGo and frostui

This plan moves the desktop app from Tauri and React to a native Go app
drawn with MyGo and frostui
([ADR-0038](../adr/0038-draw-the-desktop-app-natively-with-mygo-and-frostui.md)).
The React app stays as lodge's web app
([plan 0006](0006-m4-notes-inside-lodge.md)), and the Flatpak keeps its
own notesd ([plan 0008](0008-m6-the-standalone-desktop-app.md),
[ADR-0031](../adr/0031-the-desktop-app-ships-as-a-flatpak-with-its-own-notesd.md)).
The native app reaches parity with today's desktop app
([design/app.md](../design/app.md), [specs/ui.md](../specs/ui.md)) before
the Tauri shell is deleted. Frostmail is ported the same way in its own
repository; an orchestrating session keeps the two apps' chrome
consistent, and frostui is extracted from ChairLift at
`github.com/frostyard/frostui`.

Status (2026-11-10): Phase 1 is done: the inventory and the mapping
below, and ADR-0038. The orchestrator approved both on 2026-11-10 with
the rules both apps now share (the palette's keys, the status bar's
order, the top bar) and moved the shared pieces into frostui, built
once for Frostnotes and Frostmail; this plan follows those notes.
Phase 2, the parts that need no frostui piece, is done (below), and its
PR is open as a draft.

## The window

```
┌───────────────────────┬──────────────────────────────────────────────────────┬───────────────────┐
│ [▣] Frostnotes     [◧]│      ( ⌕ Search or run a command         ⌃K )    │ [▦] [✎] [◨] [⋮] –□✕│ top bar 52
│     Garden ▾          │                                                      │                   │ (frostui)
├───────────────────────┼──────────────────────────────────────────────────────┼───────────────────┤
│ [▭][⌕]                │ Garden plan ✕ │ Reading list ✕ │ +              ⌄ │ [↙][↗][#][▣][≡][☑]│ tab strip 36
│ ✎ ▭+ ⇅ ⇕              │ ← →   Projects / Garden plan          Live ▾   ⋯  │ Backlinks          │ view header 32
│ ▾ Projects            │                                                    │  Linked mentions 2 │
│     Garden plan       │   Garden plan                                      │  ▾ Weekly review   │
│     Seeds   PNG       │   ┌ Properties ──────────┐                         │                    │
│ ▸ Journal             │   Text of the note …                               │                    │
├───────────────────────┴──────────────────────────────────────────────────────┴───────────────────┤
│ ● Local   ☁ Synced   ⚠ 2 conflicts               2 backlinks   312 words   1,804 characters   Live │ status bar 28
└───────────────────────────────────────────────────────────────────────────────────────────────────┘
 left sidebar 260                 workspace (tab groups in splits)                   right sidebar 300
```

- **Top bar** (frostui's: 52 high, the `Chrome` color, the window's drag
  region, no system title bar). At the far left, the brand tile
  (Frostnotes' icon on the accent), "Frostnotes", and the vault's name
  as the tagline, which opens the vault menu (the vaults, Manage
  vaults…); then Toggle left sidebar. In the middle, frostui's palette
  field, the same widget in every frostyard app ("Search or run a
  command", ⌃K). At the end: Open today's daily note, Create new note,
  Toggle right sidebar, and the main menu (⋮), always the last item
  before the window buttons MyGo draws.
- **Main menu:** Command palette, Quick switcher, Manage vaults…,
  Settings (Ctrl + ,), Keyboard shortcuts, Help (F1), About Frostnotes,
  Close window (Ctrl + Shift + W), Quit (Ctrl + Q).
- **The palette** is frostui's: one prompt with two sources, asked
  lazily per query. Ctrl + K opens it in the main window, except inside
  a text editor, where Ctrl + K is Insert Markdown link; Ctrl + Shift +
  P opens it everywhere, editors included (the rule Frostnotes and
  Frostmail share). Ctrl + P opens it on commands and Ctrl + O on notes,
  for Obsidian's users ([plan 0005](0005-m3-desktop-app.md)'s exit
  criterion). Commands carry their key caps, a group, and a reason when
  they do not apply; notes are fuzzy-matched by name and alias, with
  recent files when the query is empty, Enter, Ctrl + Enter and Ctrl +
  Alt + Enter to open here, in a new tab or to the right, and Shift +
  Enter to create. The folder chooser (Move file to…) and the template
  chooser are the same prompt with their own source.
- **Status bar** (frostui's `StatusItem`s), in the order both apps
  share. First, always, the connection: a dot with "Local", or the
  hub's host for a synced vault, in the warning tone with the reason in
  its tooltip while notesd or the hub cannot be reached. Then the sync
  state (Synced, Syncing, Offline, Revoked, Reset; it opens Settings at
  Sync) and "N conflicts" (warning; it opens the conflicts menu). At the
  end, the app's items: "N backlinks" (it opens the pane), words and
  characters (of the selection when there is one) and the editing mode
  (it opens the mode menu).
- **Under the top bar**, Obsidian's layout as today
  ([specs/ui.md](../specs/ui.md#layout)): the left sidebar's panes (Files,
  Search) under icon pane tabs, the workspace of tab groups in splits,
  each group with its own tab strip and each view with its header, and
  the right sidebar's panes (Backlinks, Outgoing links, Tags, All
  properties, Outline, Tasks). Tab strips leave the title row: the top
  bar is frostui's, as ChairLift's and Frostmail's are.
- **Dialogs** are frostui's (`Dialogs.Ask` for the 7 confirmations, the
  danger color for deletes, `Info` for Keyboard shortcuts, Help and
  About). **Toasts** take frostui's look. In-tab notices (conflict,
  refused base, failed save, deleted) are frostui `Banner`s.

ChairLift's patterns, kept and dropped:

| Kept | Dropped, and why |
| --- | --- |
| Hidden title bar and frostui's top bar (brand, palette field, actions, main menu) | The page rail: the left sidebar is a file tree and panes, not pages |
| The command palette (`Command`, `Match`, `Filter`, groups, key caps, disabled with a reason) | The padded page area (at most 1320 wide, cards): views are edge to edge |
| The status bar's clickable, toned items | The console (Ctrl + J) and its status item: the app runs no commands. If notesd's sync passes and agents' writes are shown later, they use frostui's console with typed entries that carry no note text, as Frostmail's maild activity does; not part of parity |
| `Dialogs`, `Info`, `ShortcutsBody`, `AboutBody`; toasts | The job queue, `JobCard`, `Pipeline`: notesd does the long work |
| The theme: `Adapt`, `Chrome`, `Panel`, metrics and tones; `Icons`; `SetAppID` | Charts and gauges (`LiveChart`, `Gauge`, `SegmentBar`), `CommandRow` |
| `SwitchRow`, `KeyValues`, `Section`, `Banner`, `Empty`, `Loading`, `Problem`, `Pill`, `Kbd` | The rail's host card |
| `Hero` and `StepDots` (the vault manager, joining a hub) | |
| `frostuitest.Loop`, `ui.NewTester` snapshots, production builds, the coalescing waker | |

## Inventory and mapping

What today's desktop app has
([design/app.md](../design/app.md), [specs/ui.md](../specs/ui.md)), what
draws it natively, and the phase that builds it. frostui's shared pieces
are built once, for Frostnotes and Frostmail, in frostui; "uikit" is the
app-local package for the rest (below).

### Screens and chrome

| Today | Native | Phase |
| --- | --- | --- |
| Connecting / Reconnecting, retry every 1 s; in the Flatpak, why notesd is not running (the end of `notesd.log`) | frostui's `ConnectionStatus` and its connecting screen; the log tail read in Go | 2 (the log), 3 |
| Vault manager (the window without a vault, and a dialog): list, open, Remove from list; Create new vault (name and folder); Open folder as vault (picker or typed path) | frostui `Hero`, list rows, `Dialogs.Ask`; MyGo's folder picker (`mygo.Dialog.Open` with `Directory`, the portal in a Flatpak) | 3 |
| Join a vault on a hub: form, approve (the code and `notesctl hub approve CODE`, 10 minutes), joining; Pair again; cancel removes a vault it made | a dialog with `StepDots`; the code as frostui `CodeDialog`-style monospace | 3 |
| Title bar row with tab strips, ribbon (switcher, daily note, template, palette), window buttons; the window's title "note - vault - Frostnotes" | frostui's top bar (above); the ribbon's buttons move to the top bar and the palette; the title set with MyGo's window | 3 |
| Status bar items | frostui status bar (above) | 3–4 |
| Notices (top right, 4 s, 10 s with an action) | frostui's toasts, with an action and a countdown | 3 |
| Window: 1200 × 780, at least 640 × 400, one instance (a second launch focuses it), zoom (Ctrl + = / - / 0) | MyGo window options, `RequestSingleInstanceLock`; zoom as a scale of the window's text, checked in Phase 3 | 3 |
| Narrow (phone) layout | not on the desktop: the web app's ([ADR-0027](../adr/0027-a-narrow-window-takes-obsidians-phone-layout.md)) | — |

### Workspace and panes

| Today | Native | Phase |
| --- | --- | --- |
| A tree of row and column splits holding tab groups, resizable (160 at least); tabs with back and forward (50 each), pinned tabs, closed tabs (20) and recent files (50); drag a tab to reorder, or to a group's edge to split | `workspace` (pure Go, ported from `app/src/workspace` with its tests); `ui.Split` per node; uikit's tab strips and split tree | 2 (the model), 4 |
| Tab views: empty (4 links), note (edit or read), history, merge, file (images inline, others in the default app), not-in-vault, error boundary | per-view packages; frostui `Empty`, `Problem` | 2–4 |
| View header: back, forward, breadcrumb, mode, More options | back and forward icon buttons, `ui.Breadcrumbs`, `ui.MenuButton` | 4 |
| Sidebars: 200–600 wide, collapse the right then the left when narrow; pane tabs | frostui's `Panes` (widths within bounds, keyboard separators, persisted widths, collapsible sides); uikit's icon pane tabs | 4 |
| Files: new note, new folder, sort (4), collapse/expand all; open here, in a tab, to the right, middle click; F2 rename inline; drag a row into a folder ("Update links?"); delete (confirmed); context menus; reveal; extension badges | frostui's tree on `ui.Outline` (it builds only the rows in view); MyGo drag and drop and context menus | 4 |
| Search (250 ms, `note.search`, `[key]` property search, foldable snippets) | `ui.SearchField`, `ui.List` | 4 |
| Backlinks (linked, unlinked), Outgoing links (unresolved dimmed, click creates), Tags (nested tree, sort), All properties (type icons, sort), Outline (headings, current marked, click jumps), Tasks (by note) | `ui.Outline` and `ui.List` rows; the outline from the shared parser | 4 |
| Menus: tab, tab list, More options, file row, folder row, files root, sort, editing mode, conflicts, vaults, vault options | `Element.ContextMenu`, `ui.MenuButton` | 4 |

### Prompts and dialogs

| Today | Native | Phase |
| --- | --- | --- |
| Quick switcher, command palette (10 recent commands), folder chooser, template chooser | frostui's palette with its lazy sources, and frostui's `CommandTable` | 4 |
| Settings dialog: Editor, Files and links, Appearance, daily notes, templates, Sync (Join, Pair again, Leave), Hub (devices, pending, revoke), Background (Flatpak), Hotkeys (read only) | frostui's settings dialog, with `SwitchRow` and rows of choices and numbers | 3 |
| Confirmations: delete file, delete folder, update links, remove vault, stop syncing, revoke device, close with unsaved notes | `Dialogs.Ask` | 3–4 |
| Page preview: Ctrl + hover on a link, 400 × 300, notesd's render of `![[target]]` | the code editor's hover (`ShowHover` with content), a `ui.Popover` elsewhere, drawn by the reading view | 2 |

### Preferences and state

| Today | Native | Phase |
| --- | --- | --- |
| 23 local preferences (`frostnotes.prefs.v1`: views, editing mode, inline title, readable line length, spellcheck, properties, line numbers, guides, folding, auto-pairs, smart lists, tabs, confirm delete, remote images, unsupported files, theme, font size, ribbon, zoom, last vault) | `$XDG_CONFIG_HOME/frostnotes/prefs.json`, versioned; the ribbon's preference goes, the inactive spellcheck setting is removed ([ADR-0040](../adr/0040-remove-the-inactive-native-spellcheck-setting.md)) | 2 |
| The workspace per vault (`frostnotes.workspace.v1:<path>`, 500 ms after a change and on close; moves and removals applied to other vaults' workspaces), the event resume point, recent commands | `$XDG_STATE_HOME/frostnotes/workspace/<hash of the path>.json` and `state.json`; not migrated from WebKitGTK ([ADR-0038](../adr/0038-draw-the-desktop-app-natively-with-mygo-and-frostui.md)) | 2 |
| Vault settings (notesd's: `vault.settings`, `vault.configure`, `vault.configured`) | unchanged | 3 |

### Editor

| Today (CodeMirror 6) | Native | Phase |
| --- | --- | --- |
| One document per note shared by its tabs; saves 1 s after the last change and 10 s after the first, at once on Ctrl + S, blur, the last view closing, a move, a delete or the window closing; one write in flight; edits made meanwhile composed | one `editor.Buffer` per note shared by its tabs' editors (MyGo shares a buffer between editors); the save loop in Go | 2 |
| A merged answer or another writer's change applied as a remote change, outside the undo history; deleted notes re-created with unsaved text; refused bases kept for the merge view; retries at 2–60 s | the same loop; MyGo's buffer puts every edit in its undo history (an ask of MyGo, below) | 2 |
| Highlighting of Markdown in both modes; no per-language highlighting of code | MyGo's Markdown lexer with Obsidian's syntax laid over it (frontmatter, wikilinks and embeds, tags, highlights, comments, math, callouts, footnotes, block IDs), the lines in view only | 2 |
| Live preview: the table of constructs in [design/app.md](../design/app.md#live-preview) (headings, emphasis, highlights, inline code, fences with a header and Copy, links, wikilinks with resolution, images and media, note embeds, callouts, quotes, tables, tasks, bullets, guides, rules, math, footnotes, tags, comments, block IDs, properties, inline title), revealed on the selection's lines | not possible on MyGo's editors today: Phase 5 | 5 |
| Completion of `[[`, `[[t#`, `[[t#^`, `#tag` (fuzzy, by count) | `OnCompletion` and `ShowCompletions`, sources from notesd and the shared parser | 2 |
| Folding of headings and by indentation (gutter only) | a `Folder` from the parser; the gutter's markers | 2 |
| Keys: Obsidian's map ([specs/ui.md](../specs/ui.md#keyboard-map)), CodeMirror's defaults without what Obsidian removes, Tab indents, smart lists, auto-pair brackets and Markdown | `Bindings` with `NoCommand` for the app's keys; smart lists and auto-pairs in `OnInput` | 2 |
| Find and replace (case, regexp, whole word) | the editor's find bar, drawn with frostui's look (`RenderFindBar`) | 2 |
| Follow a link: click in live preview, Alt + Enter, Ctrl + Enter, Ctrl + Alt + Enter; unresolved creates; web links open outside | `OnDefinition` (Ctrl + click) and the commands; frostui's link opening behind allow-lists | 2 |
| Paste and drop files: `note.upload` (10 MB), the attachment folder, "Pasted image …", the link inserted | MyGo's clipboard and drag and drop | 2 |
| Inline title (Enter or blur renames); properties block (text, number, checkbox, date, date and time, lists of pills; rename a key, remove, add with Ctrl + ;, fold) | a title field and a properties panel above the editor: `ui.TextInput`, `ui.DateInput`, `ui.TimeInput`, `ui.TokenField`, `ui.Checkbox` | 2 |

### Reading view, history and merges

| Today | Native | Phase |
| --- | --- | --- |
| Reading view: notesd's HTML (`note.render`) sanitized twice, blob URLs, remote images by preference, KaTeX, task checkboxes toggle their line, callouts fold, links and tags follow, footnotes jump, audio and video play | MyGo's Markdown view over notesd's new Markdown format; `LoadImage` from blobs (and https when allowed); `Block` draws callouts and embeds; `OnLink` follows; tasks toggle mapped source lines with undo; native vector math; local audio/video controls | 2 |
| History: `note.versions` and the hub's `sync.history`; a version as read-only source; Show changes against the current text; Restore; Compare | `ui.List`; a read-only `editor.State`; `editor.DiffView` (unified) | 2 |
| Merge view: side by side, the left the note (editable, saving), the right a version, a revision or the held text; revert a chunk; Done resolves | `editor.DiffView` (split, `Editable`, `HunkActions`) | 2 |

### Shell and system (the Rust side)

| Today (`app/src-tauri`) | Native | Phase |
| --- | --- | --- |
| `notesd_connect` / `notesd_send`: lines over the socket (16 MiB), the multiplexing in TypeScript | the generated Go client (`api/`) over `lodge/kit/jsonrpc`, caller `local`; `rpc.hello`, then `events.subscribe` from the stored sequence | 2 |
| `paths.rs` (socket, data directory, log; the Flatpak's shared directory) | `internal/config`, which `paths.rs` mirrors | 2 |
| `notesblob://` (blobs by hash, ranges, content types) | read `<data dir>/blobs/<hh>/<hash>` directly; decode with `ui.DecodeBitmap` | 2 |
| `open_url` (http, https with a host, mailto) and `open_file` (inside the vault, a regular non-executable file, 35 extensions) with `xdg-open` | frostui's opening behind allow-lists, given today's rules | 3 |
| `background_get` / `background_set` (the off-file, the generation, `frostnotes-request-background`) and `stop_on_exit` | the same files and helper, from Go | 2 |
| `notesd_log` (the last 8 KiB, the `PORTAL_FAILURE` line) | Go, with `internal/packagecheck`'s test moved to it | 2 |
| `problems.rs` (`frostyard-problems`, the panic hook) | `github.com/frostyard/std/problems`, `RecoverPanic` in each goroutine | 3 |
| Single instance (focus the window), the folder picker, the window's CSP and navigation limits | MyGo's single instance and dialogs; no web content, so no CSP | 3 |
| `spike_*`: the hooks of M3's evidence | the Tester, and MyGo's automation in a development build | 6 |

### Packaging, CI and evidence

| Today | Native | Phase |
| --- | --- | --- |
| Flatpak module `frostnotes`: Rust and Node SDK extensions, `cargo-sources.json`, `node-sources.json`, the pnpm tarball, `pnpm tauri build`, `cargo test` | the Go module builds `cmd/frostnotes` from `go-sources.json` (now with MyGo and purego) as a MyGo production build, installed as `/app/libexec/frostnotes-app` so the launcher is unchanged | 6 |
| Icons in `app/src-tauri/icons/` (also copied by `make web-build`) | moved to `packaging/icons/` | 6 |
| CI's `app` job (WebKitGTK, Node, Rust); `ci` = verify, ui-check, app-build, app-test, race | the Tester runs in `verify`'s unit run, with Pango, cairo and a font installed in CI; `app-build` and `app-test` become `desktop-build`; the required checks' names change with the owner | 6 |
| `internal/workflowcheck` (the cargo entry, `TestDependabotLeavesInternalGitCratesAlone`, `TestCargoIgnores`, risk-path globs of `tauri.conf.json`, capabilities, `open.rs`), `internal/packagecheck` (`pnpm tauri build` in the manifest, `notesd_log.rs`), the PR template | updated in the change that removes them; the Go port of the open rules joins the risk paths at L1 | 6 |
| `internal/appevidence` (hotkeys and concurrent writers through X, GTK and WebKit), M3's typing and scrolling spike, `TestIPhoneEditReachesTheDesktop`, `dev/screenshots` (WebKitWebDriver) for the metainfo | Tester tests with a real notesd, a typing benchmark on a 10,000-line note, an automation build under Xvfb for one end-to-end run, and screenshots from `Tester.Image()` | 6 |
| `dev/nsl/provision.sh`: WebKitGTK, `webkit2gtk-driver`, Rust | the WebKitGTK packages go; Rust stays (frostread's `make app-check` uses the same machine) | 6 |

## The hard parts

- **Live preview.** Neither of MyGo's editors can draw it. The code
  editor highlights, folds and decorates backgrounds and inlay text, but
  draws one monospaced font and cannot hide a range or put a widget in
  place of one. The rich text editor draws headings and images in place,
  but holds runs, not the file, and saves its own Markdown. So the app
  starts with source mode on the code editor and the reading view
  (Phase 2), and live preview comes in Phase 5, as decorations on MyGo's
  editor, which its author accepted ([egoist/mygo#1651](https://github.com/egoist/mygo/issues/1651)); Frostnotes' own editor on
  MyGo's text-input client, shaped text and the code editor's rope stays
  the fallback should that stall:
  laying out only the lines in view with cached heights (CodeMirror's
  height map, which [ADR-0025](../adr/0025-patch-what-costs-the-editor-its-frames.md)
  had to patch), hiding marks off the selection's lines, drawing widgets
  in place, with its own undo, IME and accessible text.
- **Large vaults.** The file tree is a `ui.Outline`, which builds only
  the rows in view (`ui.Tree` builds every item); search results and
  panes are `ui.List`s. The notes' source of the palette ranks names off
  the UI thread and is asked only while the palette is open, where
  frostui's palette takes a slice of every command each frame. The code
  editor lays out and highlights only the lines in view, a line at a
  time, so a keystroke never waits for a parse of the whole note.
  Events from notesd wake the window through a coalescing waker, as
  ChairLift's console does. Images decode once and stay cached while the
  view that drew them is open, up to 256 megapixels a view; past that
  they show as their descriptions. Tested on a `tools/vaultgen` vault of 10,000 notes.
- **Accessibility.** MyGo gives its widgets AT-SPI roles: the outline is
  a tree, tabs a tab list, the editor a multi-line text field, the
  Markdown view headings, lists, links, tables and check boxes, dialogs
  dialogs. What the app draws itself takes frostui's rules: clickable
  rows are buttons, icon buttons have names and tooltips with their
  keys, and the status bar is a status. Today's keyboard paths stay: the
  files tree (arrows, Home, End, Enter, F2), tab strips (arrows, Delete,
  Shift + F10), menus and prompts; the Tags and Outline trees gain the
  keyboard they lack today. The live-preview editor exposes the note's
  text and selection, as the code editor does. Tests assert roles and
  names with `Tester.AccessByRole`; Phase 5 ends with a run of Orca.

### Original MyGo gaps

These were the gaps at the start of the port. The 2026-11-11 issue sweep
implements the editor's replacement/line-style hooks, remote edits, reading task
actions and block anchors on the fork, native math and AVIF decoding in
Frostnotes, and playback through MyGo's existing APIs. Native Spellcheck is
removed under [ADR-0040](../adr/0040-remove-the-inactive-native-spellcheck-setting.md).
See [the app design](../design/app.md) and [native rendering contract](../specs/render.md#markdown-for-native-views).

- Hiding or replacing source in the code editor, widgets between its
  lines, and more than one font size in it (live preview).
- An edit outside the buffer's undo history (a merge applied as a remote
  change, [ADR-0022](../adr/0022-the-editor-saves-as-you-type-and-applies-merges-as-remote-changes.md)).
- Showing HTML outside a web view (notesd's rendered HTML).
- TeX (KaTeX), audio and video codecs, AVIF note images, and a spell
  checker.
- A click on a task's box in the Markdown view (the reading view shows
  tasks but cannot check them; the editor's Ctrl + L does).

Requests to MyGo's author go through the owner, whom the orchestrator
asks; Frostnotes does not contact the author. Two were filed on
2026-11-10 with the owner's approval, and the author welcomed both;
other sessions are writing their pull requests:

- [egoist/mygo#1651](https://github.com/egoist/mygo/issues/1651): hidden or replaced ranges first (Zed's folds with placeholders),
  then elements between lines, and per-line text styles after a design
  note. Phase 5 builds live preview on them.
- [egoist/mygo#1652](https://github.com/egoist/mygo/issues/1652): an edit outside the undo history that rebases it. MyGo's undo
  keeps steps as inverse edits moved over a remote change, and drops a
  step that overlaps one. The app applies remote changes with
  `Buffer.EditRemote` ([egoist/mygo#1654](https://github.com/egoist/mygo/pull/1654))
  and uses MyGo's own undo.

**A fork until the rest lands.** MyGo v0.9.0 (2026-11-11) has #1654 and
[egoist/mygo#1655](https://github.com/egoist/mygo/pull/1655). `go.mod`
requires v0.9.0 and replaces it with `github.com/bketelsen/mygo`, branch
`frostyard/next` at `d97fd2e1`: v0.9.0 with
[egoist/mygo#1662](https://github.com/egoist/mygo/pull/1662) (elements between
lines), per-line styles (headings' sizes and done tasks) and the Markdown
view's task actions and anchors, nothing else. The fork has no v0.9.0 tag, so
its pseudo-version reads v0.8.4. The owner approved the fork on 2026-11-10 and
the move to v0.9.0 on 2026-11-11. Once all of these are in a MyGo release,
`go.mod` requires that release and the `replace` goes. The fork also changes the MyGo frostui builds against here, and
frostui's CI tests only released MyGo, so `make verify` runs
`go test github.com/frostyard/frostui/...` while the `replace` stays; the
step goes with it. A failure there is reported to frostui.

Not filed yet: a hook for a click on a task's box in the Markdown view.

## Generic pieces

**In frostui, built once for Frostnotes and Frostmail** by the
orchestrator (Frostnotes builds none of them; it pins the frostui
pseudo-version the orchestrator names, in `go.mod`, with no `go.work` or
`replace` in a commit):

1. A frame without the rail: full-height content, segments of the top
   bar, the status bar, and an optional console split.
2. `Panes`: sides within minimum and maximum widths, keyboard
   separators, persisted widths, collapsible sides (the left and right
   sidebars).
3. The keymap (`CommandTable`) and palette sources asked lazily per
   query.
4. A toast with an action and a countdown.
5. `ConnectionStatus` with the "connecting to the daemon" screen.
6. A tree (source list) on MyGo's virtualized outline.
7. A settings dialog or window layout.
8. Embedded icons for systems without a freedesktop theme.
9. ChairLift's coalescing waker.
10. Opening links and files behind allow-lists.

**In `internal/desktop/uikit`, for now**, which imports the standard
library, MyGo, purego and frostui only (a test enforces it, as
ChairLift's `TestFrostUIImportsNoChairLift` does), so that a piece moves
into frostui by a change of import path:

1. Tab strips (closable, pinnable, draggable, middle click, a tab list)
   and the split tree of tab groups (drop on an edge to split).
2. The side panels' icon pane tabs.

## Packages

- `cmd/frostnotes`: the program (app ID, one instance, problems, MyGo's
  run loop).
- `internal/desktop/uikit`: the pieces above.
- `internal/desktop/session`: the client, events, reconnecting and the
  stores (vaults, files, tags), with no MyGo import.
- `internal/desktop/workspace` and `internal/desktop/commands`: the
  workspace model, and the commands' table of IDs, titles and keys,
  pure Go, ported with their Vitest cases.
- `internal/desktop/notes`: documents, saves and merges, pure Go.
- `internal/desktop/editor`, `reading`, `history`, `panes`, `catalog`,
  `prefs`, `system` (paths, background, notesd's log, the open rules) and
  `window` (the frame, the vault manager, settings and their wiring);
  `livepreview` (what live preview shows, pure Go).

## Phase 1 — Inventory and plan (done 2026-11-10)

- The inventory and mapping above, from the code of `app/src`,
  `app/src-tauri`, `packaging/flatpak`, the workflows and the evidence,
  and from MyGo v0.8.3's documentation and source.
- [ADR-0038](../adr/0038-draw-the-desktop-app-natively-with-mygo-and-frostui.md),
  accepted on 2026-11-10.
- **Done when:** this plan and ADR-0038 are committed and the
  orchestrator has the chrome, the patterns and the generic pieces to
  check against the Frostmail port.

## Phase 2 — Session, source mode, reading view, history and merges (done 2026-11-10)

The parts that need no frostui piece, in the order the orchestrator set.

- MyGo v0.8.3 pinned in `go.mod`, with `go-sources.json` regenerated.
- The session, with no MyGo import: connect to the socket with the
  generated Go client, `rpc.hello`, events from the stored sequence,
  reconnect each second, the stores (vaults, files, tags)
  ([design/app.md](../design/app.md#data)); notesd's log tail and the
  background setting
  ([ADR-0033](../adr/0033-a-setting-keeps-notesd-running-without-a-window.md)).
- The view models, pure Go: the workspace model, its operations and its
  persistence ([ADR-0021](../adr/0021-keep-the-workspace-and-app-preferences-on-each-device.md),
  [design/app.md](../design/app.md#workspace)), the preferences, and
  the commands' table ([specs/ui.md](../specs/ui.md#keyboard-map)).
- The documents, saves and merges
  ([ADR-0022](../adr/0022-the-editor-saves-as-you-type-and-applies-merges-as-remote-changes.md),
  [design/app.md](../design/app.md#documents-saves-and-merges)) over
  MyGo's buffer.
- Source mode: the highlighter, folding, completions, hovers (the page
  preview), links, formatting commands, smart lists, auto-pairs, find
  and replace, paste and drop
  ([ADR-0023](../adr/0023-the-app-shows-files-by-their-hash-and-uploads-them-through-notesd.md)),
  the inline title and the properties panel
  ([design/app.md](../design/app.md#properties)).
- notesd: `note.render`'s Markdown format, specified with its fixtures
  in [specs/render.md](../specs/render.md) beside the HTML, through
  `schema/rpc` first.
- The reading view on MyGo's Markdown view
  ([design/app.md](../design/app.md#reading-view)), with attachments read
  from the blob store by hash; history and the merge view on
  `editor.DiffView`
  ([design/app.md](../design/app.md#history-and-merges),
  [specs/ui.md](../specs/ui.md#merge-view)); tab notices.
- Each view tested headless with `ui.NewTester`, in `make verify`'s unit
  run.
- **Built (2026-11-10):** `internal/desktop/notes` (a change set with
  CodeMirror's compose and map; the save loop; CRLF notes), `editor` (source mode,
  Obsidian's keys and formatting commands with the web app's test
  cases, smart lists, links, completions of `[[` and `#`), `reading`,
  `history` (and the merge view), `session`, `workspace` (the model and
  its operations, with the web app's test cases), and
  `note.renderMarkdown` in notesd. The evidence: three writers lose no token and the caret
  stays (`TestNoTokenIsLostWithThreeWriters`); a keystroke in a
  10,000-line note takes about 3 ms with a CPU-drawn frame
  (`BenchmarkTypingInALongNote`, an AMD Ryzen AI MAX+ 395); the render
  rules pass in `internal/render` and every link of the generated
  vault's manifest in `internal/engine`.
- **Also built (2026-11-10):** the workspace's and the preferences'
  files, the commands' table (held to the keyboard map by a test), the
  vault catalog, completion of headings and block IDs, the inline
  title and the properties panel, paste and drop of attachments, tab
  notices on frostui's banner, notesd's log tail and the background
  setting. Phase 2 is done.
- **Done when:** M3's writers evidence passes against the native
  document and editor in a test with a real notesd (vim, MCP and a hub
  writing while the app types lose no token, and the caret stays),
  typing in a 10,000-line note stays under 16 ms a keystroke in source
  mode, the render fixtures pass for both formats, and `make check`
  passes. The branch is then pushed and its PR opened as a draft.

## Phase 3 — The frame (done 2026-11-10)

- `cmd/frostnotes` on frostui's frame: the top bar, the palette, the
  status bar in the shared order, the main menu, dialogs, toasts, About,
  Help, Keyboard shortcuts; one instance; problems
  ([ADR-0036](../adr/0036-notesd-and-the-app-record-problems-in-the-journal.md)).
- frostui's `ConnectionStatus` with notesd's cause.
- The vault manager and joining a hub
  ([ADR-0024](../adr/0024-the-app-opens-and-creates-vaults-and-a-window-switches-between-them.md),
  [specs/ui.md](../specs/ui.md#vault-manager)); the settings dialog with
  every preference, vault setting, Sync, Hub and Background
  ([specs/ui.md](../specs/ui.md#settings-batch-2)).
- `make desktop-build`, `desktop-run`.
- **Built (2026-11-10):** `cmd/frostnotes` and `internal/desktop/window` on
  frostui v0.2.0's `PaneShell`, `Connection`, `Settings` and palette
  sources. The sidebar toggle is at the far left, before the brand: frostui
  draws it there in every app. Why notesd is not running moves from a
  block under the text to the connecting screen's Details. The open rules
  of `open.rs` are `internal/desktop/system/open.go`, in front of
  frostui's `Opener`, and joined the risk paths at L1.
- **Evidence:** `internal/desktop/window`'s tests, against a real notesd:
  Connecting, a lost connection keeping the workspace, creating, opening
  and removing vaults, joining a loopback hub, and each screen's PNG in
  light and dark (`FROSTNOTES_SCREENS` keeps them). `make desktop-smoke`
  starts the release build under Xvfb.
- **Done when:** against a real notesd in a test, the app shows
  Connecting, lists, creates, opens and removes vaults and joins a
  loopback hub, and the Tester writes each screen's PNG, light and dark.

## Phase 4 — Workspace and panes (done 2026-11-10)

- Tab strips, splits and view headers over Phase 2's workspace model;
  frostui's `Panes` for the sidebars, with uikit's pane tabs.
- Files (frostui's tree), Search, Backlinks, Outgoing links, Tags, All
  properties, Outline and Tasks
  ([specs/ui.md](../specs/ui.md#sidebars-and-panes)); every menu; the
  quick switcher, the folder and template choosers; daily notes and
  templates; file tabs; opening outside the app.
- Every command and key of [specs/ui.md](../specs/ui.md#keyboard-map)
  through frostui's `CommandTable`
  ([design/app.md](../design/app.md#commands-and-keys)).
- **Built (2026-11-10):** tab strips, splits and view headers; the panes,
  with Files on frostui's `SourceList`; the quick switcher, the folder,
  template and heading choosers as palette sources; daily notes,
  templates and file tabs; the link hover's page preview.
- **Evidence:** `TestEveryCommandRunsFromThePalette` and
  `TestEveryKeyRunsItsCommand`; `make desktop-evidence`'s
  `TestALargeVaultStaysWithinAFrame`, on an AMD Ryzen AI MAX+ 395 with a
  CPU-drawn frame (the numbers are in the PR).
- **Done when:** every command ID of the keyboard map runs from the
  palette and from its keys in Tester tests, and a generated vault of
  10,000 notes shows its tree and ranks the switcher's notes with no frame
  over 16 ms.

## Phase 5 — Live preview (about 3 weeks)

**Built (2026-11-10), on the fork above:** the logic,
`internal/desktop/livepreview`, recorded in
[ADR-0039](../adr/0039-live-preview-is-replacements-over-the-source.md): the
marks of headings, emphasis, highlights, inline code, links and wikilinks,
footnotes, comments and block IDs; bullets, tasks, quotes and rules; the
frontmatter while the properties panel shows; and the reveal rule. The editor
shows them as MyGo's `Replacement`s and `Block`s
(`internal/desktop/editor/live.go`): callouts, tables, note embeds and images
as the reading view draws them (`reading.Blocks`), fences' headers with Copy,
and links, tags, footnotes and task boxes a click reaches. Undo is MyGo's. A
keystroke in a 10,000-line note with blocks, with a frame drawn on the CPU,
costs 8 ms in live preview and 2.3 ms in source mode, on an AMD Ryzen AI MAX+
395. The gate's reference machine is the CI runner: the `desktop` job's
`make desktop-bench` prints the figure on every run. The runner's CPU
changes from run to run, so a figure names its CPU. With per-line styles,
on 2026-11-12 ([#193](https://github.com/frostyard/frostnotes/issues/193)),
once live preview's scan hashed each line once and the highlighter tried
each pattern only on a line that can match it, it measured 10.4 ms in live
preview and 2.4 ms in source mode on an AMD EPYC 9V45, where `main` before
them measured 12.9 ms in live preview. With the scan's change alone it
measured 14.2 ms on an AMD EPYC 9V74 (`main`: 15.1 and 15.4 ms), and 17.7
ms, over the gate, on an Intel Xeon Platinum 8573C, the slowest CPU seen,
where source mode took 5.1 ms. MyGo's cost in
`view.replacedLines` was fixed on the fork, with egoist/mygo#1655.

**Built:** heading sizes, completed-task strike-through, native inline/display
math and local audio/video playback ([app design](../design/app.md)). Selected
lines restore their source styles. The owner used it with their real vault on Snow on
2026-11-10, and waived the run of Orca that day; the Tester's checks of roles
and names stand in for it.

- Live preview on MyGo's editor, in the order its author is building
  [egoist/mygo#1651](https://github.com/egoist/mygo/issues/1651): hidden and replaced ranges first (the marks hidden off the
  selection's lines; a checkbox, a bullet, an image, math or a link's
  text in place of its source), then elements between lines (tables,
  callouts, note embeds, the properties block), then per-line styles
  (headings' sizes) once their design note settles. An ADR records the
  shape when the phase starts; Frostnotes' own editor widget is the
  fallback should the pull requests stall.
- The app's undo gives way to MyGo's, with remote changes applied outside
  the history ([egoist/mygo#1652](https://github.com/egoist/mygo/issues/1652)). Done.
- The constructs of [design/app.md](../design/app.md#live-preview), in
  M3's order: headings, emphasis, highlights and inline code; links and
  wikilinks; tasks and bullets; quotes and rules; code fences; images;
  callouts; tables; note embeds; math as TeX; footnotes, tags, comments
  and block IDs; the properties block in place; indentation guides. Each
  reveals its source when the selection's lines reach it, as today.
- IME, accessible text, and undo that leaves out remote changes.
- **Done when:** each construct has a Tester snapshot in both states,
  typing in a 10,000-line note in live preview stays under 16 ms a
  keystroke, every note of the user's vault opens in live preview, and
  Orca reads and edits a note.

## Phase 6 — Packaging, CI and evidence (about 1 week)

**Built (2026-11-10), on the draft pull request:** the Flatpak is the native
app with no Rust or Node, the icons are in `packaging/icons/`, the `app` CI job
is gone, and the metainfo's screenshots come from the Tester. The owner
installed the Flatpak on Snow and used it with their vault on 2026-11-10.

- The Flatpak's Go module, the icons' move, the version stamp, and the
  tests of `internal/packagecheck` and `internal/workflowcheck`
  ([ADR-0031](../adr/0031-the-desktop-app-ships-as-a-flatpak-with-its-own-notesd.md),
  [design/app.md](../design/app.md#the-flatpak)).
- CI: Pango, cairo and a font for the unit run; the automation build
  under Xvfb; the `app` job replaced.
- The evidence: hotkeys, writers, typing, and an edit on the phone
  reaching the desktop within 2 s, against the native app; metainfo
  screenshots from the Tester.
- The docs: `AGENTS.md`, [design/app.md](../design/app.md) and
  [specs/ui.md](../specs/ui.md) split into what both UIs do and how each
  draws it, [design/overview.md](../design/overview.md),
  `docs/development.md`, the README.
- **Built so far (2026-11-10):** CI's `desktop` job; `make desktop-evidence`
  with an edit on the hub reaching the open editor; the Flatpak also
  builds the native app, as `/app/libexec/frostnotes-native`, which
  `flatpak run --env=FROSTNOTES_NATIVE=1 org.frostyard.Frostnotes` runs
  for a trial. The Tauri module stays the default until parity.
- **Done when:** the Flatpak built from the branch installs on Snow,
  opens the user's vault and syncs it, with no Rust or Node in its
  build, and `make ci` and `flatpak.yml` pass.

## Phase 7 — The switch

**Built (2026-11-10), as the draft pull request's last commit:** `app/src-tauri`,
the Tauri transport, the window buttons and `@tauri-apps/*` are gone, and the
React app builds only for lodge. The owner accepted ADR-0038 and ADR-0039, and
changed the required checks, on 2026-11-10. frostui v0.4.0 has the
`mailto` rule (`SanitizeMailto`, frostui's ADR-0010), and on 2026-11-11
`system.WebURL`'s own filter went: `WebURL` calls `SanitizeMailto`, which
also drops the fragment (#200). The window checks every link with
`WebURL`, and `Opener` checks it again.

- The parity list above, checked item by item in the PR's body.
- Delete `app/src-tauri`, the Tauri transport, the window buttons and
  `@tauri-apps/*`, the cargo entry of `dependabot.yml`, and the risk
  paths that named them; the React app builds only for lodge.
- ADR-0038 accepted, and ADR-0004, ADR-0007, ADR-0011, ADR-0030 and
  ADR-0031 marked superseded in part.
- **Done when:** the owner merges a PR in which the desktop app is the
  native one and the repository has no Tauri.

## Later / ideas

- Several windows (Open in new window), and the graph view of M5 drawn
  with MyGo's painter.

## Open questions

- **A trial before the switch,** as M3's 7 days? The owner decides by
  Phase 7.
- *Answered 2026-11-10:* **MyGo's answer.** Its author welcomed both
  asks ([egoist/mygo#1651](https://github.com/egoist/mygo/issues/1651), [egoist/mygo#1652](https://github.com/egoist/mygo/issues/1652)); Phase 5 builds live preview on MyGo's editor, as
  above.
- *Answered 2026-11-10 by the orchestrator:* **Ctrl + K** opens the
  palette except inside a text editor, where it is Insert Markdown link;
  Ctrl + Shift + P opens it everywhere.

## References

- Implements: [design/app.md](../design/app.md), [specs/ui.md](../specs/ui.md),
  [specs/render.md](../specs/render.md),
  [ADR-0038](../adr/0038-draw-the-desktop-app-natively-with-mygo-and-frostui.md)
- Replaces the desktop half of: [plan 0005](0005-m3-desktop-app.md) (M3);
  packaging follows [plan 0008](0008-m6-the-standalone-desktop-app.md)
- Roadmap: [plan 0002](0002-roadmap.md)
- ChairLift's [ADR-0017](https://github.com/frostyard/chairlift/blob/feat/mygo/docs/adr/0017-mygo-command-center-window.md)
  and [design/frostui.md](https://github.com/frostyard/chairlift/blob/feat/mygo/docs/design/frostui.md)

Native math rendering follows [ADR-0041](../adr/0041-render-native-math-as-vector-paths.md),
with the reveal rule and source fallback described in the
[UI spec](../specs/ui.md#in-the-native-desktop-app).

The window moved to frostui v0.5.0 and its Frostyard look on 2026-11-11
(frostui's [plan, phase 5](https://github.com/frostyard/frostui/blob/main/docs/plans/frostyard-look.md#phase-5--frostnotes-small);
[ADR-0042](../adr/0042-the-desktop-app-takes-frostuis-frostyard-look.md),
[the native look](../design/app.md#the-native-look)): the Theme setting
picks Frostyard or Frostyard light, live preview sets headings in Inter and
code on the code color, and the app's literal font sizes and corners became
frostui's styles and metrics. `TestTheWindowDrawsInTheFrostyardThemes`
checks the window in both themes.
