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
below, and ADR-0038, proposed. Phase 2 waits for the orchestrator's
consistency notes from the Frostmail port.

## The window

```
┌───────────────────────┬──────────────────────────────────────────────────────┬───────────────────┐
│ [▣] Frostnotes     [◧]│      ( ⌕ Search notes or run a command   Ctrl P )    │ [▦] [✎] [◨] [≡] –□✕│ top bar 52
│     Garden ▾          │                                                      │                   │ (frostui)
├───────────────────────┼──────────────────────────────────────────────────────┼───────────────────┤
│ [▭][⌕]                │ Garden plan ✕ │ Reading list ✕ │ +              ⌄ │ [↙][↗][#][▣][≡][☑]│ tab strip 36
│ ✎ ▭+ ⇅ ⇕              │ ← →   Projects / Garden plan          Live ▾   ⋯  │ Backlinks          │ view header 32
│ ▾ Projects            │                                                    │  Linked mentions 2 │
│     Garden plan       │   Garden plan                                      │  ▾ Weekly review   │
│     Seeds   PNG       │   ┌ Properties ──────────┐                         │                    │
│ ▸ Journal             │   Text of the note …                               │                    │
├───────────────────────┴──────────────────────────────────────────────────────┴───────────────────┤
│ ☁ Synced   ⚠ 2 conflicts                         2 backlinks   312 words   1,804 characters   Live │ status bar 28
└───────────────────────────────────────────────────────────────────────────────────────────────────┘
 left sidebar 260                 workspace (tab groups in splits)                   right sidebar 300
```

- **Top bar** (frostui's: 52 high, the `Chrome` color, the window's drag
  region, no system title bar). Over the left sidebar: the brand tile
  (Frostnotes' icon on the accent), "Frostnotes", and the vault's name
  as the tagline, which opens the vault menu (the vaults, Manage
  vaults…); then Toggle left sidebar. In the middle, the palette field.
  At the end: Open today's daily note, Create new note, Toggle right
  sidebar, the main menu, and the window buttons MyGo draws.
- **Main menu:** Command palette, Quick switcher, Manage vaults…,
  Settings (Ctrl + ,), Keyboard shortcuts, Help (F1), About Frostnotes,
  Close window (Ctrl + Shift + W), Quit (Ctrl + Q).
- **The palette** is frostui's: one prompt with two sources. Ctrl + P
  opens it on commands and Ctrl + O on notes (Obsidian's keys, which
  [plan 0005](0005-m3-desktop-app.md)'s exit criterion keeps); Ctrl + K
  opens it too, except in the editor, where Ctrl + K stays Obsidian's
  Insert Markdown link. Commands carry their key caps, a group, and a
  reason when they do not apply; notes are fuzzy-matched by name and
  alias, with recent files when the query is empty, Enter, Ctrl + Enter
  and Ctrl + Alt + Enter to open here, in a new tab or to the right, and
  Shift + Enter to create. The folder chooser (Move file to…) and the
  template chooser are the same prompt with their own source.
- **Status bar** (frostui's `StatusItem`s). At the start: notesd's state
  when it is not connected ("Reconnecting…", warning), the sync state
  (Synced, Syncing, Offline, Revoked, Reset; it opens Settings at Sync)
  and "N conflicts" (warning; it opens the conflicts menu). At the end:
  "N backlinks" (it opens the pane), words and characters (of the
  selection when there is one) and the editing mode (it opens the mode
  menu).
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
| The status bar's clickable, toned items | The console (Ctrl + J) and its status item: the app runs no commands |
| `Dialogs`, `Info`, `ShortcutsBody`, `AboutBody`; toasts | The job queue, `JobCard`, `Pipeline`: notesd does the long work |
| The theme: `Adapt`, `Chrome`, `Panel`, metrics and tones; `Icons`; `SetAppID` | Charts and gauges (`LiveChart`, `Gauge`, `SegmentBar`), `CommandRow` |
| `SwitchRow`, `KeyValues`, `Section`, `Banner`, `Empty`, `Loading`, `Problem`, `Pill`, `Kbd` | The rail's host card |
| `Hero` and `StepDots` (the vault manager, joining a hub) | |
| `frostuitest.Loop`, `ui.NewTester` snapshots, production builds, the coalescing waker | |

## Inventory and mapping

What today's desktop app has
([design/app.md](../design/app.md), [specs/ui.md](../specs/ui.md)), what
draws it natively, and the phase that builds it. "uikit" is the app-local
package of generic pieces (below).

### Screens and chrome

| Today | Native | Phase |
| --- | --- | --- |
| Connecting / Reconnecting, retry every 1 s; in the Flatpak, why notesd is not running (the end of `notesd.log`) | uikit's daemon-connection screen; the log tail read in Go | 2 |
| Vault manager (the window without a vault, and a dialog): list, open, Remove from list; Create new vault (name and folder); Open folder as vault (picker or typed path) | frostui `Hero`, list rows, `Dialogs.Ask`; MyGo's folder picker (`mygo.Dialog.Open` with `Directory`, the portal in a Flatpak) | 2 |
| Join a vault on a hub: form, approve (the code and `notesctl hub approve CODE`, 10 minutes), joining; Pair again; cancel removes a vault it made | a dialog with `StepDots`; the code as frostui `CodeDialog`-style monospace | 2 |
| Title bar row with tab strips, ribbon (switcher, daily note, template, palette), window buttons; the window's title "note - vault - Frostnotes" | frostui's top bar (above); the ribbon's buttons move to the top bar and the palette; the title set with MyGo's window | 2 |
| Status bar items | frostui status bar (above) | 2–4 |
| Notices (top right, 4 s, 10 s with an action) | toasts; one with an action needs uikit's toast | 2 |
| Window: 1200 × 780, at least 640 × 400, one instance (a second launch focuses it), zoom (Ctrl + = / - / 0) | MyGo window options, `RequestSingleInstanceLock`; zoom as a scale of the window's text, checked in Phase 3 | 2–3 |
| Narrow (phone) layout | not on the desktop: the web app's ([ADR-0027](../adr/0027-a-narrow-window-takes-obsidians-phone-layout.md)) | — |

### Workspace and panes

| Today | Native | Phase |
| --- | --- | --- |
| A tree of row and column splits holding tab groups, resizable (160 at least); tabs with back and forward (50 each), pinned tabs, closed tabs (20) and recent files (50); drag a tab to reorder, or to a group's edge to split | `workspace` (pure Go, ported from `app/src/workspace` with its tests); `ui.Split` per node; uikit's tab strip | 3 |
| Tab views: empty (4 links), note (edit or read), history, merge, file (images inline, others in the default app), not-in-vault, error boundary | per-view packages; frostui `Empty`, `Problem` | 3–4 |
| View header: back, forward, breadcrumb, mode, More options | back and forward icon buttons, `ui.Breadcrumbs`, `ui.MenuButton` | 3 |
| Sidebars: 200–600 wide, collapse the right then the left when narrow; pane tabs | uikit's side panel | 3 |
| Files: new note, new folder, sort (4), collapse/expand all; open here, in a tab, to the right, middle click; F2 rename inline; drag a row into a folder ("Update links?"); delete (confirmed); context menus; reveal; extension badges | `ui.Outline` (builds only the rows in view) in uikit's tree; MyGo drag and drop and context menus | 3 |
| Search (250 ms, `note.search`, `[key]` property search, foldable snippets) | `ui.SearchField`, `ui.List` | 3 |
| Backlinks (linked, unlinked), Outgoing links (unresolved dimmed, click creates), Tags (nested tree, sort), All properties (type icons, sort), Outline (headings, current marked, click jumps), Tasks (by note) | `ui.Outline` and `ui.List` rows; the outline from the shared parser | 3–4 |
| Menus: tab, tab list, More options, file row, folder row, files root, sort, editing mode, conflicts, vaults, vault options | `Element.ContextMenu`, `ui.MenuButton` | 3 |

### Prompts and dialogs

| Today | Native | Phase |
| --- | --- | --- |
| Quick switcher, command palette (10 recent commands), folder chooser, template chooser | frostui's palette with uikit's sources (above) | 3 |
| Settings dialog: Editor, Files and links, Appearance, daily notes, templates, Sync (Join, Pair again, Leave), Hub (devices, pending, revoke), Background (Flatpak), Hotkeys (read only) | uikit's settings dialog with frostui `SwitchRow` and rows of choices and numbers | 2 |
| Confirmations: delete file, delete folder, update links, remove vault, stop syncing, revoke device, close with unsaved notes | `Dialogs.Ask` | 2–4 |
| Page preview: Ctrl + hover on a link, 400 × 300, notesd's render of `![[target]]` | the code editor's hover (`ShowHover` with content), a `ui.Popover` elsewhere, drawn by the reading view | 4 |

### Preferences and state

| Today | Native | Phase |
| --- | --- | --- |
| 23 local preferences (`frostnotes.prefs.v1`: views, editing mode, inline title, readable line length, spellcheck, properties, line numbers, guides, folding, auto-pairs, smart lists, tabs, confirm delete, remote images, unsupported files, theme, font size, ribbon, zoom, last vault) | `$XDG_CONFIG_HOME/frostnotes/prefs.json`, versioned; the ribbon's preference goes, spellcheck stays unused (no checker) | 2 |
| The workspace per vault (`frostnotes.workspace.v1:<path>`, 500 ms after a change and on close; moves and removals applied to other vaults' workspaces), the event resume point, recent commands | `$XDG_STATE_HOME/frostnotes/workspace/<hash of the path>.json` and `state.json`; not migrated from WebKitGTK ([ADR-0038](../adr/0038-draw-the-desktop-app-natively-with-mygo-and-frostui.md)) | 3 |
| Vault settings (notesd's: `vault.settings`, `vault.configure`, `vault.configured`) | unchanged | 2 |

### Editor

| Today (CodeMirror 6) | Native | Phase |
| --- | --- | --- |
| One document per note shared by its tabs; saves 1 s after the last change and 10 s after the first, at once on Ctrl + S, blur, the last view closing, a move, a delete or the window closing; one write in flight; edits made meanwhile composed | one `editor.Buffer` per note shared by its tabs' editors (MyGo shares a buffer between editors); the save loop in Go | 4 |
| A merged answer or another writer's change applied as a remote change, outside the undo history; deleted notes re-created with unsaved text; refused bases kept for the merge view; retries at 2–60 s | the same loop; MyGo's buffer puts every edit in its undo history (an ask of MyGo, below) | 4 |
| Highlighting of Markdown in both modes; no per-language highlighting of code | a `Highlighter` over `internal/markdown`'s parse, run on a buffer snapshot off the UI thread | 4 |
| Live preview: the table of constructs in [design/app.md](../design/app.md#live-preview) (headings, emphasis, highlights, inline code, fences with a header and Copy, links, wikilinks with resolution, images and media, note embeds, callouts, quotes, tables, tasks, bullets, guides, rules, math, footnotes, tags, comments, block IDs, properties, inline title), revealed on the selection's lines | not possible on MyGo's editors today: Phase 5 | 5 |
| Completion of `[[`, `[[t#`, `[[t#^`, `#tag` (fuzzy, by count) | `OnCompletion` and `ShowCompletions`, sources from notesd and the shared parser | 4 |
| Folding of headings and by indentation (gutter only) | a `Folder` from the parser; the gutter's markers | 4 |
| Keys: Obsidian's map ([specs/ui.md](../specs/ui.md#keyboard-map)), CodeMirror's defaults without what Obsidian removes, Tab indents, smart lists, auto-pair brackets and Markdown | `Bindings` with `NoCommand` for the app's keys; smart lists and auto-pairs in `OnInput` | 4 |
| Find and replace (case, regexp, whole word) | the editor's find bar, drawn with frostui's look (`RenderFindBar`) | 4 |
| Follow a link: click in live preview, Alt + Enter, Ctrl + Enter, Ctrl + Alt + Enter; unresolved creates; web links open outside | `OnDefinition` (Ctrl + click) and the commands; `xdg-open` behind the allow-lists | 4 |
| Paste and drop files: `note.upload` (10 MB), the attachment folder, "Pasted image …", the link inserted | MyGo's clipboard and drag and drop | 4 |
| Inline title (Enter or blur renames); properties block (text, number, checkbox, date, date and time, lists of pills; rename a key, remove, add with Ctrl + ;, fold) | a title field and a properties panel above the editor: `ui.TextInput`, `ui.DateInput`, `ui.TimeInput`, `ui.TokenField`, `ui.Checkbox` | 4 |

### Reading view, history and merges

| Today | Native | Phase |
| --- | --- | --- |
| Reading view: notesd's HTML (`note.render`) sanitized twice, blob URLs, remote images by preference, KaTeX, task checkboxes toggle their line, callouts fold, links and tags follow, footnotes jump, audio and video play | MyGo's Markdown view over notesd's new Markdown format; `LoadImage` from blobs (and https when allowed); `Block` draws callouts and embeds; `OnLink` follows; tasks toggle by line; math as TeX; audio and video open outside | 4 |
| History: `note.versions` and the hub's `sync.history`; a version as read-only source; Show changes against the current text; Restore; Compare | `ui.List`; a read-only `editor.State`; `editor.DiffView` (unified) | 4 |
| Merge view: side by side, the left the note (editable, saving), the right a version, a revision or the held text; revert a chunk; Done resolves | `editor.DiffView` (split, `Editable`, `HunkActions`) | 4 |

### Shell and system (the Rust side)

| Today (`app/src-tauri`) | Native | Phase |
| --- | --- | --- |
| `notesd_connect` / `notesd_send`: lines over the socket (16 MiB), the multiplexing in TypeScript | the generated Go client (`api/`) over `lodge/kit/jsonrpc`, caller `local`; `rpc.hello`, then `events.subscribe` from the stored sequence | 2 |
| `paths.rs` (socket, data directory, log; the Flatpak's shared directory) | `internal/config`, which `paths.rs` mirrors | 2 |
| `notesblob://` (blobs by hash, ranges, content types) | read `<data dir>/blobs/<hh>/<hash>` directly; decode with `ui.DecodeBitmap` | 4 |
| `open_url` (http, https with a host, mailto) and `open_file` (inside the vault, a regular non-executable file, 35 extensions) with `xdg-open` | the same rules in Go, at the same risk level | 3 |
| `background_get` / `background_set` (the off-file, the generation, `frostnotes-request-background`) and `stop_on_exit` | the same files and helper, from Go | 2 |
| `notesd_log` (the last 8 KiB, the `PORTAL_FAILURE` line) | Go, with `internal/packagecheck`'s test moved to it | 2 |
| `problems.rs` (`frostyard-problems`, the panic hook) | `github.com/frostyard/std/problems`, `RecoverPanic` in each goroutine | 2 |
| Single instance (focus the window), the folder picker, the window's CSP and navigation limits | MyGo's single instance and dialogs; no web content, so no CSP | 2 |
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
  (Phase 4), and live preview comes in Phase 5, as decorations on MyGo's
  editor if its author adds them, or as Frostnotes' own editor on
  MyGo's text-input client, shaped text and the code editor's rope:
  laying out only the lines in view with cached heights (CodeMirror's
  height map, which [ADR-0025](../adr/0025-patch-what-costs-the-editor-its-frames.md)
  had to patch), hiding marks off the selection's lines, drawing widgets
  in place, with its own undo, IME and accessible text.
- **Large vaults.** The file tree is a `ui.Outline`, which builds only
  the rows in view (`ui.Tree` builds every item); search results and
  panes are `ui.List`s. The notes' source of the palette ranks names off
  the UI thread and is asked only while the palette is open, where
  frostui's palette takes a slice of every command each frame. The code
  editor lays out only the lines in view; the highlighter parses a
  snapshot off the UI thread, so a keystroke never waits for goldmark.
  Events from notesd wake the window through a coalescing waker, as
  ChairLift's console does. Images decode once and stay in a cache of
  bounded size. Tested on a `tools/vaultgen` vault of 10,000 notes.
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

### What MyGo has no equivalent for

- Hiding or replacing source in the code editor, widgets between its
  lines, and more than one font size in it (live preview).
- An edit outside the buffer's undo history (a merge applied as a remote
  change, [ADR-0022](../adr/0022-the-editor-saves-as-you-type-and-applies-merges-as-remote-changes.md)).
- Showing HTML outside a web view (notesd's rendered HTML).
- TeX (KaTeX), audio and video codecs, AVIF and SVG note images, and a
  spell checker.

Asks of MyGo's author, sent before Phase 4: hidden ranges, widgets and
per-line styles in `ui/editor` (as Zed's folds with placeholders and
blocks), and an edit outside the undo history that rebases it. Until the
second exists, the app keeps its own undo over the buffer's edits, so
that Ctrl + Z never takes back another writer's change.

## Generic pieces (uikit)

`internal/desktop/uikit` imports the standard library, MyGo, purego and
frostui only, and a test enforces it, as ChairLift's
`TestFrostUIImportsNoChairLift` does; moving it into frostui is a change
of import path. Frostnotes does not edit frostui. Its candidates:

1. **A chrome frame**: frostui's top bar, status bar, palette, dialogs
   and toasts around a body the app lays out, without the command
   center's rail, page area and console.
2. **Palette sources**: items asked for only while the palette is open
   and per query (notes, mail, feeds), recent items first, other Enter
   keys (a new tab, to the right, create), and the palette's key as a
   field.
3. **Tab strips and a split tree of tab groups**: closable, pinnable and
   draggable tabs, middle click, a tab list, drop on an edge to split.
4. **Side panels**: a left and a right panel with icon pane tabs,
   resizable within bounds, folding away right first as the window
   narrows.
5. **A tree over `ui.Outline`**: icons, badges, inline rename, drag and
   drop into folders, context menus, keyboard.
6. **A settings dialog**: sections in a list beside rows of switches,
   choices, numbers and buttons.
7. **A toast with an action** (frostui's `TopToasts` has a title and a
   close only).
8. **A daemon-connection screen**: connecting, reconnecting with a
   reason, a cause read from the daemon's log (frostmail has maild).
9. **A keymap**: commands' IDs bound to keys once, shown as key caps in
   the palette and the Keyboard shortcuts dialog, and taken from the
   editor with `NoCommand`.
10. **The coalescing waker**, now in ChairLift's `internal/window`.
11. **Opening links and files outside the app** behind allow-lists
    (http, https with a host, mailto; a file inside a root with an
    allowed extension), beside frostui's `LaunchDesktopApp`.

## Packages

- `cmd/frostnotes`: the program (app ID, one instance, problems, MyGo's
  run loop).
- `internal/desktop/uikit`: the generic pieces above.
- `internal/desktop/session`: the client, events, reconnecting and the
  stores (vaults, files, tags), with no MyGo import.
- `internal/desktop/workspace` and `internal/desktop/commands`: the
  workspace model and the command registry and keymap, pure Go, ported
  with their Vitest cases.
- `internal/desktop/editor`, `livepreview`, `reading`, `history`,
  `panes`, `settings`, `vaults`, `system` (paths, blobs, opening,
  background, notesd's log) and `window` (the frame and its wiring).

## Phase 1 — Inventory and plan (done 2026-11-10)

- The inventory and mapping above, from the code of `app/src`,
  `app/src-tauri`, `packaging/flatpak`, the workflows and the evidence,
  and from MyGo v0.8.3's documentation and source.
- [ADR-0038](../adr/0038-draw-the-desktop-app-natively-with-mygo-and-frostui.md),
  proposed.
- **Done when:** this plan and ADR-0038 are committed and the
  orchestrator has the chrome, the patterns and the generic pieces to
  check against the Frostmail port.

## Phase 2 — The frame (about 1 week)

- `cmd/frostnotes` and the packages above, on MyGo v0.8.3 and frostui
  through an uncommitted `go.work` until frostui is tagged; uikit and its
  import test.
- The session: connect to the socket, `rpc.hello`, events from the stored
  sequence, reconnect each second, the Connecting screen with notesd's
  cause ([design/app.md](../design/app.md#data)).
- The chrome: top bar, palette with commands, status bar, main menu,
  dialogs, toasts, About, Help, Keyboard shortcuts.
- The vault manager and joining a hub
  ([ADR-0024](../adr/0024-the-app-opens-and-creates-vaults-and-a-window-switches-between-them.md),
  [specs/ui.md](../specs/ui.md#vault-manager)); the settings dialog with
  every preference, vault setting, Sync, Hub and Background
  ([specs/ui.md](../specs/ui.md#settings-batch-2),
  [ADR-0033](../adr/0033-a-setting-keeps-notesd-running-without-a-window.md)).
- The system pieces: paths, notesd's log, the background setting,
  problems ([ADR-0036](../adr/0036-notesd-and-the-app-record-problems-in-the-journal.md)).
- `make desktop-build`, `desktop-run`; Tester tests in `make verify`.
- **Done when:** against a real notesd in a test, the app shows
  Connecting, lists, creates, opens and removes vaults and joins a
  loopback hub; the Tester writes each screen's PNG, light and dark; and
  `make check` passes.

## Phase 3 — Workspace and panes (about 1–2 weeks)

- The workspace model, its persistence and its operations
  ([ADR-0021](../adr/0021-keep-the-workspace-and-app-preferences-on-each-device.md),
  [design/app.md](../design/app.md#workspace)); tab strips, splits, view
  headers.
- Files, Search, Backlinks, Outgoing links, Tags, All properties, Outline
  and Tasks ([specs/ui.md](../specs/ui.md#sidebars-and-panes)); every
  menu; the quick switcher, the folder and template choosers; daily notes
  and templates; file tabs; opening outside the app.
- Every command and key of [specs/ui.md](../specs/ui.md#keyboard-map)
  ([design/app.md](../design/app.md#commands-and-keys)).
- **Done when:** every command ID of the keyboard map runs from the
  palette and from its keys in Tester tests, and a generated vault of
  10,000 notes shows its tree and ranks the switcher's notes with no frame
  over 16 ms.

## Phase 4 — Source mode, reading view, history and merges (about 2 weeks)

- The documents, saves and merges
  ([ADR-0022](../adr/0022-the-editor-saves-as-you-type-and-applies-merges-as-remote-changes.md),
  [design/app.md](../design/app.md#documents-saves-and-merges)) over
  MyGo's buffer, with the app's own undo until MyGo's arrives.
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
  ([design/app.md](../design/app.md#reading-view)); history and the merge
  view on `editor.DiffView`
  ([design/app.md](../design/app.md#history-and-merges),
  [specs/ui.md](../specs/ui.md#merge-view)); tab notices.
- **Done when:** M3's writers evidence passes against the native app in
  a test (vim, MCP and a hub writing while the app types lose no token,
  and the caret stays), typing in a 10,000-line note stays under 16 ms a
  keystroke in source mode, and the render fixtures pass for both
  formats.

## Phase 5 — Live preview (about 3 weeks)

- Decide between MyGo's decorations and Frostnotes' own editor by the
  answer to the asks, in an ADR.
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
- **Done when:** the Flatpak built from the branch installs on Snow,
  opens the user's vault and syncs it, with no Rust or Node in its
  build, and `make ci` and `flatpak.yml` pass.

## Phase 7 — The switch

- The parity list above, checked item by item in the PR's body.
- Delete `app/src-tauri`, the Tauri transport, the window buttons and
  `@tauri-apps/*`, the cargo entry of `dependabot.yml`, and the risk
  paths that named them; the React app builds only for lodge.
- ADR-0038 accepted, and ADR-0004, ADR-0007, ADR-0011, ADR-0030 and
  ADR-0031 marked superseded in part.
- **Done when:** the owner merges a PR in which the desktop app is the
  native one and the repository has no Tauri.

## Later / ideas

- A TeX renderer for `markdown.Options.InlineMath` and live preview, in
  place of TeX in the code font.
- Several windows (Open in new window), and the graph view of M5 drawn
  with MyGo's painter.
- Audio and video in the app, once MyGo plays them.

## Open questions

- **Ctrl + K:** frostui's palette key, or Obsidian's Insert Markdown
  link in the editor? Decided with the orchestrator's consistency notes,
  before Phase 2.
- **A trial before the switch,** as M3's 7 days? The owner decides by
  Phase 7.
- **MyGo's answer** on decorations and remote edits decides Phase 5's
  shape, by its start.
- **Where uikit's pieces land in frostui,** and when; the owner moves
  them.

## References

- Implements: [design/app.md](../design/app.md), [specs/ui.md](../specs/ui.md),
  [specs/render.md](../specs/render.md),
  [ADR-0038](../adr/0038-draw-the-desktop-app-natively-with-mygo-and-frostui.md)
- Replaces the desktop half of: [plan 0005](0005-m3-desktop-app.md) (M3);
  packaging follows [plan 0008](0008-m6-the-standalone-desktop-app.md)
- Roadmap: [plan 0002](0002-roadmap.md)
- ChairLift's [ADR-0017](https://github.com/frostyard/chairlift/blob/feat/mygo/docs/adr/0017-mygo-command-center-window.md)
  and [design/frostui.md](https://github.com/frostyard/chairlift/blob/feat/mygo/docs/design/frostui.md)
