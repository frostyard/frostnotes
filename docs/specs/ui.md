# Spec: the desktop UI (M3)

The look and behavior of Frostnotes' main window: Obsidian's layout
(ribbon, sidebars of panes, tabs in splits, status bar) drawn in
frostmail's look (its tokens, Inter, Lucide icons, client-side chrome).
It binds the app's CSS tokens (`app/src/styles/tokens.css`), the
presentational components built from task cards, the keymap, and the app
tests. Measurements are CSS pixels at 100% zoom. Keys are written for
Linux: Obsidian's Mod is Ctrl. Obsidian's behavior here is version 1.14.4's,
read from the app itself: its default hotkeys, settings, sort orders and
fuzzy matching. The user's Obsidian uses every default (its `app.json`,
`appearance.json` and hotkeys are empty), so its defaults are theirs.

Sections marked *(batch N)* are refined when their cards are written
([plan 0005](../plans/0005-m3-desktop-app.md)); everything else binds now.

## Tokens

CSS custom properties on `:root`, with dark values under
`@media (prefers-color-scheme: dark)` while the Theme preference is
System (the default); Light and Dark set `data-theme` on the root, whose
rules give the same light or dark values. Components MUST use
tokens through the Tailwind theme (`app/src/styles/app.css`), never
literal colors.

Frostmail's tokens, copied at M0 (frostmail's `docs/specs/ui.md`):

| Token | Light | Dark | Use here |
| --- | --- | --- | --- |
| `--bg-window` | `#ffffff` | `#1e1e1e` | the workspace, the editor, prompts, menus |
| `--bg-sidebar` | `#f2f2f4` | `#2a2a2c` | ribbon, sidebars |
| `--bg-toolbar` | `#f7f7f8` | `#2c2c2e` | title bar row, tab strips, status bar, notices |
| `--bg-banner` | `#f5f5f7` | `#2c2c2e` | notices in a tab, attachment chips |
| `--text-primary` | `#1d1d1f` | `#f5f5f7` | note text, titles |
| `--text-secondary` | `#6e6e73` | `#a1a1a6` | paths, counts, icons, inactive tabs |
| `--text-tertiary` | `#a1a1a6` | `#6e6e73` | placeholders, disabled, comments, syntax shown at the cursor |
| `--separator` | `#e3e3e6` | `#3a3a3c` | hairlines, indentation guides, horizontal rules |
| `--accent` | `#007aff` | `#0a84ff` | links, the caret, selection with focus, quote and embed bars |
| `--accent-contrast` | `#ffffff` | `#ffffff` | text on `--accent` |
| `--selection-inactive` | `#e4e4e7` | `#3a3a3c` | hover; selection without focus; editor selection |
| `--selection-sidebar` | `#dcdce0` | `#3a3a3c` | the active file's row, a selected pane tab |
| `--badge-bg`, `--badge-text` | `#e5e5ea`, `#6e6e73` | `#3a3a3c`, `#a1a1a6` | extension badges, key caps |
| `--focus-ring` | `rgb(0 122 255 / 0.5)` | `rgb(10 132 255 / 0.6)` | keyboard focus outline |
| `--flag-1` … `--flag-7` | red, orange, yellow, green, blue, purple, gray | the same, dark | callout colors (below) |

New for Frostnotes:

| Token | Light | Dark | Use |
| --- | --- | --- | --- |
| `--bg-code` | `#f5f5f7` | `#2a2a2c` | inline code, code blocks, a property row on hover |
| `--bg-highlight` | `rgb(255 204 0 / 0.4)` | `rgb(255 214 10 / 0.3)` | `==highlights==`, search matches |
| `--bg-tag` | `rgb(0 122 255 / 0.1)` | `rgb(10 132 255 / 0.2)` | tag pills (text `--accent`) |
| `--callout-cyan` | `#30b0c7` | `#40c8e0` | tip, important and summary callouts |
| `--popover-shadow` | `0 8px 24px rgb(0 0 0 / 0.16)` | `0 8px 24px rgb(0 0 0 / 0.5)` | menus, prompts, notices |
| `--backdrop` | `rgb(0 0 0 / 0.2)` | `rgb(0 0 0 / 0.4)` | the window behind a prompt or dialog (`bg-backdrop`) |

**Callout colors**, by the callout's type, ignoring case; an unknown type
is `note`:

| Types | Color | Icon (Lucide) |
| --- | --- | --- |
| note | `--flag-5` | `pencil` |
| abstract, summary, tldr | `--callout-cyan` | `clipboard-list` |
| info | `--flag-5` | `info` |
| todo | `--flag-5` | `circle-check` |
| tip, hint, important | `--callout-cyan` | `flame` |
| success, check, done | `--flag-4` | `check` |
| question, help, faq | `--flag-2` | `circle-help` |
| warning, caution, attention | `--flag-2` | `triangle-alert` |
| failure, fail, missing | `--flag-1` | `x` |
| danger, error | `--flag-1` | `zap` |
| bug | `--flag-1` | `bug` |
| example | `--flag-6` | `list` |
| quote, cite | `--flag-7` | `quote` |

### Type

`font-family: "Inter Variable", system-ui, sans-serif`; the UI's base is
13px. Counts use `font-variant-numeric: tabular-nums`. Monospace is
`ui-monospace, "DejaVu Sans Mono", monospace`.

| Role | Size / line height | Weight | Theme utility |
| --- | --- | --- | --- |
| sidebar row, tree row | 13 / 18 | 400 | `text-sidebar-row` |
| sidebar section | 11 / 14 | 600 | `text-sidebar-section` |
| title bar title | 13 / 16 | 600 | `text-toolbar-title` |
| tab title | 13 / 16 | 400 (500 for the focused group's active tab) | `text-tab` |
| view header | 13 / 16 | 400 | `text-tab` |
| status bar | 11 / 14 | 400 | `text-status` |
| empty state, prompt input | 15 / 20 | 400 | `text-empty` |
| prompt row | 13 / 18, its note 12 / 16 | 400 | `text-sidebar-row` |
| key cap, extension badge | 10 / 12 | 600 | `text-keycap` |

The editor and the reading view (Obsidian's defaults):

| Element | Size | Weight | Line height |
| --- | --- | --- | --- |
| body | 16px | 400 | 1.5 |
| inline title, H1 | 1.618em | 700 | 1.2 |
| H2 | 1.462em | 600 | 1.2 |
| H3 | 1.318em | 600 | 1.3 |
| H4 | 1.188em | 600 | 1.4 |
| H5 | 1.076em | 600 | 1.5 |
| H6 | 1em | 600 | 1.5 |
| code | 0.875em monospace | 400 | 1.5 |

The Font size preference (10 to 30, default 16) scales the body and with
it every `em` above.

## Layout

```
┌────┬──────────────────┬──────────────────────────────────────────┬───────────────┬───────┐
│ ▤  │ [▭][⌕]           │ Garden plan ✕ │ Reading list ✕ │ +    ⌄ │ [↙][↗][#][▣][≡]│ ▥ –□✕ │ title bar row 44
├────┼──────────────────┼──────────────────────────────────────────┼───────────────┴───────┤
│ ⌕  │ ✎ ▭+ ⇅ ⇕         │ ← →        Projects / Garden plan   📖 ⋯ │ Backlinks             │ view header 36
│ ▦  │ ▾ Projects       │                                          │  Linked mentions   2  │
│ ⎘  │     Garden plan  │      Garden plan                         │  ▾ Weekly review      │
│ >_ │     Seeds   PNG  │      ┌ Properties ──────────────┐        │     …[[Garden plan]]… │
│    │ ▸ Journal        │      │ tags   #garden            │        │                       │
│    │   Inbox          │      Text of the note …                  │                       │
│    │ Notes ⇕       ⚙  │                                          │                       │
├────┴──────────────────┴──────────────────────────────────────────┴───────────────────────┤
│                                                      ☁  2 backlinks  312 words  1,804 characters │ status bar 24
└───────────────────────────────────────────────────────────────────────────────────────────┘
 ribbon 44   left sidebar 260          workspace (rest)                 right sidebar 300
```

- **Window:** default 1200 × 780, minimum 640 × 400 (as
  `app/src-tauri/src/main.rs` makes it), no system title bar.
- **Panes:** the ribbon is 44 wide. The left sidebar defaults to 260
  wide and the right to 300, each between 200 and 600; the workspace
  takes the rest, at least 320. Sidebars, splits and the workspace are
  separated by a 1px `--separator` line with a 6px invisible drag
  handle, which turns `--accent` while dragged. Widths persist with the
  workspace
  ([ADR-0021](../adr/0021-keep-the-workspace-and-app-preferences-on-each-device.md)).
  A collapsed sidebar takes no width; the ribbon stays. Open sidebars
  give way to the workspace's 320, the right first, down to their
  minimum; a window too narrow for them at that collapses the right
  sidebar, then the left. Their stored widths and open flags stay, for a
  wider window.
- **Buttons** in the chrome are 28 × 28 with 16px Lucide icons in
  `--text-secondary`, rounded 6; hover `--selection-inactive`; pressed or
  selected `--selection-sidebar`; disabled at 40% opacity. Every
  icon-only button has an accessible name and a tooltip naming it and its
  keys ("Open quick switcher (Ctrl + O)").

### Title bar row

44 high, `--bg-toolbar`, 1px `--separator` bottom: the window's drag
region (Tauri's `data-tauri-drag-region` on it and on every empty part;
a double click on empty space toggles maximize). It is split at the
pane boundaries:

1. Over the ribbon: Toggle left sidebar (`panel-left`, pressed while
   open).
2. Over the left sidebar, when open: its pane tabs (below).
3. Over the workspace: the tab strips of the tab groups whose top edge is
   the workspace's (one per column of a row split). Tab groups lower
   down draw their own strips, 44 high, at their top.
4. Over the right sidebar, when open: its pane tabs.
5. Toggle right sidebar (`panel-right`, pressed while open), then the
   window controls (minimize, maximize or restore, close) on the
   desktop; a browser draws its own.

### Ribbon

44 wide, `--bg-sidebar`, 1px `--separator` right edge. Buttons centered,
from 8 below the title bar row, 4 apart: Open quick switcher
(`file-search`), Open today's daily note (`calendar-days`), Insert
template (`files`), Open command palette (`terminal`). Turning the Show
ribbon preference off (it is on) hides it.

### Sidebars and panes

- **Pane tabs:** one 28 × 28 button per pane, 4 apart from 8 at the
  left, in the title bar row over the sidebar: `role="tab"`,
  `aria-selected`, its pane's name as accessible name and tooltip. The
  selected pane's button is `--selection-sidebar`. The left sidebar's
  panes: Files (`folder-closed`), Search (`search`). The right's:
  Backlinks (`arrow-down-left`), Outgoing links (`arrow-up-right`), Tags
  (`tags`), All properties (`archive`), Outline (`list`).
- **A pane** fills its sidebar under the title bar row, `--bg-sidebar`.
  Most panes start with a 32-high toolbar of 24 × 24 buttons with 16px
  icons, centered, 4 apart.
- **The vault row** closes the left sidebar: 36 high, 1px `--separator`
  top, 8px side padding. The vault's name (13/18, 600) and
  `chevrons-up-down` (12px) form one button that opens the vault menu
  (the vaults by name, the current one checked); Settings (`settings`,
  "Settings (Ctrl + ,)") sits at the right.

### Files pane *(batch 2)*

- **Toolbar:** New note (`square-pen`), New folder (`folder-plus`),
  Change sort order (`arrow-up-narrow-wide`, a menu: File name (A to Z),
  File name (Z to A), Modified time (new to old), Modified time (old to
  new), the current one checked), Collapse all or Expand all
  (`chevrons-down-up`, `chevrons-up-down`).
- **The tree** (`role="tree"`, named "Files") draws the rows
  `lib/fileTree.ts` builds (T-0040), in order:
  - **Rows** are 28 high, rounded 6, inset 8 at each side; left padding
    4 + 16 × depth, then a 16-wide column holding a folder's chevron
    (`chevron-right`, 14px, `--text-secondary`, turned 90° while open)
    and nothing for a file, then the name, 13/18, truncated. A file
    other than a note shows its extension after the name, uppercased, in
    a badge: 10/12 600 `--badge-text` on `--badge-bg`, 4 radius, 4px
    side padding, 6px from the name. Files carry no icon, as in
    Obsidian.
  - **Indentation guides:** a 1px `--separator` line per open folder,
    under its chevron, along its children's rows (the Show indentation
    guides preference).
  - **The active file** (the active tab's) is `--selection-sidebar`;
    the row with keyboard focus shows the focus ring. ↑ and ↓ move
    focus, → opens a folder or moves into it, ← closes it or moves to
    its parent, Enter opens a file (Ctrl+Enter in a new tab) or toggles
    a folder, F2 renames.
  - **Rename** replaces the name with a 13/18 input; Enter moves the
    file (`note.move`, links updated by the vault's setting), Escape
    cancels; a name with `/` is refused there.
  - **Drag and drop:** files and folders drop onto folders (and the
    vault's root, the pane's empty space) to move; a file dropped on an
    editor inserts a link to it.
  - **Context menu:** on a note or file, Open in new tab, Open to the
    right, —, Rename… (F2), Move file to…, —, Copy path, Open version
    history, Open in default app, —, Delete; on a folder, New note, New
    folder, —, Rename…, Move folder to…, —, Delete. Delete asks first
    (title `Delete "<name>"?`, text "You can restore it from its version
    history.", OK labeled Delete) while the Confirm file deletion
    preference is on (the default).
  - **Empty vault:** "No notes yet", 15/20 `--text-secondary`, centered,
    and a New note button under it.
- The Show unsupported files preference (off, as in Obsidian) shows
  files of other types.

### Search pane *(batch 2)*

A 28-high input at the top (8px margin, `--bg-window`, 1px
`--separator`, rounded 6, `search` icon inside at the left, a clear
button at the right while it has text), searching 250 ms after typing
stops (`note.search`, at most 100). Under it, "N results" (11/14
`--text-secondary`), then one group per note: a 28-high header (chevron,
the note's title 13/18 600, its folder 12/16 `--text-secondary`) and its
snippet, 12/16, with matched parts on `--bg-highlight`. A click opens
the note and the current file's search with the query. Ctrl+Shift+F
opens the pane with the editor's selection as the query, quoted when it
holds a space.

### Right sidebar panes *(batch 2)*

- **Backlinks:** for the active note, "Linked mentions" with its count
  (`note.backlinks`), one group per linking note (its title and the
  number of links), each link's line under it (12/16, the link on
  `--bg-highlight`); "Unlinked mentions", collapsed, with its count: notes
  that mention the title or an alias as text without linking. A click
  opens the source at the line.
- **Outgoing links:** the active note's links (`note.links`), each target
  once: resolved ones by title, then unresolved ones in
  `--text-tertiary`. A click opens it; an unresolved one creates it.
- **Tags:** every tag in the vault with its count, nested at `/` (a
  parent counts its own notes and its children's), sorted by name (A to
  Z) or count (a toolbar menu). A click searches `tag:#name`.
- **All properties:** every frontmatter key in the vault with its type's
  icon and count, sorted as Tags are. A click lists the notes with that
  key in the search pane (`note.list` with the key).
- **Outline:** the active note's headings, live from the editor's syntax
  tree, nested as `lib/outline.ts` builds them (T-0041): rows 28 high,
  left padding 8 + 16 × depth, a chevron where a heading has children.
  A click scrolls the editor to the heading and focuses it. "No
  headings" when there are none.

### Workspace

#### Tab strip

Each tab group has one strip, 44 high, `--bg-toolbar`, 8px side
padding, its tabs 2 apart (`app/src/features/workspace/TabStrip.tsx`,
T-0042):

- **The strip** is `role="tablist"`, named by its caller. In the title
  bar row, it and its empty space are drag region.
- **A tab** is `role="tab"`, `aria-selected` when it is its group's
  active tab, focusable only then (`tabIndex` 0, else -1), and
  draggable. It is 30 high, rounded 6, with 10px side padding and a 6px
  gap, between 80 and 220 wide: tabs share the strip and shrink
  together, and the title truncates. Its tooltip is its whole title.
  - Inactive: `--text-secondary`; hover `--selection-inactive`.
  - Active: `--bg-window` and `--text-primary`; in the focused group,
    weight 500.
  - **Close:** a 16 × 16 button named "Close <title>" with a 12px `x`,
    rounded 4, at the tab's right; shown on the active tab and on hover,
    present but transparent otherwise. A pinned tab has a 12px `pin`
    image named "Pinned" there instead.
- **New tab:** after the last tab, a 28 × 28 button named "New tab"
  (`plus`, tooltip "New tab (Ctrl + T)").
- **Tab list:** at the strip's right end, a 28 × 28 button named "Tab
  list" (`chevron-down`, `aria-haspopup="menu"`); its menu lists the
  group's tabs, the active one checked.
- **Pointer:** a click on a tab activates it; a middle click closes it
  unless pinned; a right click opens its menu. A click on Close closes
  without activating.
- **Keys** on a tab: ← and → activate the previous and next tab (not past
  the ends), Home and End the first and last; the Menu key and
  Shift+F10 open its menu; Delete closes it unless pinned.
- **The tab menu:** Close, Close others, Close tabs to the right, Close
  all, —, Pin or Unpin, —, Split right, Split down.
- **Dragging** a tab over a tab group shows where it would land: on the
  strip, between tabs; on the group's middle, at its end; near an edge
  (the outer quarter), a new split there, the target area tinted
  `--accent` at 15%.

#### View header

36 high, `--bg-window`, under the strip:

- At the left, 8px in: Navigate back (`arrow-left`) and Navigate forward
  (`arrow-right`), 24 × 24, disabled with no history.
- Centered: the breadcrumb, 13/16: the note's folders in
  `--text-secondary`, each a button that reveals it in the Files pane,
  separated by " / " in `--text-tertiary`, then the title in
  `--text-primary`.
- At the right, 8px in: Reading view (`book-open`) while editing or
  Editing view (`pencil`) while reading, then More options (`ellipsis`):
  Open version history, —, Split right, Split down, —, Rename… (F2), Move
  file to…, Copy path, Reveal in Files, Open in default app, —, Delete
  file.
- Turning the Show view header preference off (it is on) hides it.

#### Empty tab

"New tab" content, centered: four buttons as links, 15/20 `--accent`, 8
apart: Create new note (Ctrl + N), Go to file (Ctrl + O), See recent
files (Ctrl + O), Close (Ctrl + W).

#### Notices in a tab

A 32-high bar under the view header, `--bg-banner`, 12/16, 12px side
padding, with buttons at its right (12/16 `--accent`, no background),
`role="status"`: a conflict ("2 changes conflicted with
<author or source>; your text is kept as a version." Compare, Dismiss), a
refused base ("Not saved: notesd no longer has the version this text
started from. Your text is kept." Compare), a deleted note ("This note
was deleted." Restore, Close), a failed save ("Not saved: <reason>.
Retrying." Retry now).

### Editor and reading view

- **The page:** content at most 700 wide (the Readable line length
  preference, on; off, the view's width), centered, 32px from the top
  and the sides, with half the view's height below the last line.
- **Inline title** (the Show inline title preference, on): the file's
  name in the H1 style, editable; Enter or leaving it renames the file,
  Escape restores it; 0.5em below it.
- **Properties block** (the Properties in document preference: Visible,
  the default; Hidden; Source): under the inline title. "Properties"
  (13/16, `--text-secondary`, a chevron that folds it), then a row per
  key, 30 high: the type's icon (14px) and the key (13/18,
  `--text-secondary`) in a 9em column, then the value's editor (text;
  list as pills; number; checkbox; date; date and time; tags as tag
  pills); hover `--bg-code`, rounded 6. "Add property" (`plus`) last.
  Ctrl+; adds a property.
- **Live preview** draws, away from the selection's lines (on them the
  syntax shows as typed, its marks in `--text-tertiary`):
  - headings in the type table, their `#`s hidden; emphasis, bold and
    strikethrough with their marks hidden;
  - links: `--accent`, underlined; a wikilink shows its alias or target,
    a markdown link its text; an unresolved link at 70% opacity; a click
    follows a link (Ctrl+click in a new tab, Ctrl+Alt+click to the
    right);
  - tags: pills, `--bg-tag`, `--accent`, radius 1em, padding 0.1em
    0.5em, 0.875em;
  - highlights on `--bg-highlight`; comments (`%%…%%`) in
    `--text-tertiary`;
  - inline code on `--bg-code`, rounded 4, padding 0.1em 0.25em; code
    blocks on `--bg-code`, rounded 6, padding 12 16, the language at the
    top right (11/14 `--text-tertiary`) and a copy button on hover;
  - blockquotes with a 2px `--accent` bar and 16px padding; callouts
    on their color at 10%, rounded 6, padding 12 12 12 24, the title in
    the color, 600, after its icon, foldable with `+` and `-`;
  - task checkboxes 16px, rounded 4, a 1px `--text-tertiary` border,
    `--accent` with a white check when done, the done item's text in
    `--text-secondary` and struck through; a click toggles the one
    character;
  - list bullets as 6px dots in `--text-secondary`, with indentation
    guides; horizontal rules 2px `--separator`;
  - embeds with a 2px `--accent` bar and 24px padding; images at most the
    page's width, `|300` or `|300x200` sizing them; math with KaTeX;
    footnote references raised.
- **The reading view** draws the same elements with the same sizes, so
  switching modes moves nothing but the hidden syntax. Checkboxes still
  toggle.

### Prompts: quick switcher and command palette *(batch 2)*

- **The frame:** a dialog, 700 wide (at most 80% of the window),
  centered, its top 60 from the window's, at most 70% of the window's
  height; `--bg-window`, 1px `--separator`, rounded 12,
  `--popover-shadow`; the window behind dimmed (`--backdrop`). A
  44-high input (15/20, 16px side padding, 1px `--separator` bottom),
  then the results, then a 28-high instructions footer (11/14
  `--text-tertiary`, items 16 apart: "↑↓ to navigate", "↵ to open", …).
  ↑ and ↓ move the selected row (`--selection-inactive`, rounded 6, inset
  8); Escape closes; a click chooses.
- **Quick switcher** (Ctrl+O): rows 32 high; the name 13/18 with matched
  characters in `--accent` and 600, then its folder 12/16
  `--text-secondary` 8 after; an alias row shows the alias, a 12px
  `forward` icon and the note's name. With an empty query, the recently
  opened files, newest first. Matching is Obsidian's fuzzy search
  (`lib/fuzzy.ts`, T-0039) against each note's path without `.md` (its
  name first, else the whole path) and against each alias, best first,
  at most 50. Enter opens the note in the current tab, Ctrl+Enter in a
  new tab, Ctrl+Alt+Enter to the right; Shift+Enter creates a note named
  as typed, and so does Enter when nothing matches.
- **Command palette** (Ctrl+P): rows 32 high; the command's name with
  its matches, and at the right its keys as caps (10/12 600
  `--badge-text` on `--badge-bg`, rounded 4, 2px 6px padding, 4 apart).
  Only commands that apply now are listed. With an empty query, the last
  10 commands run, then the rest by name. Enter runs it.

### Status bar

24 high, `--bg-toolbar`, 1px `--separator` top, 11/14 `--text-secondary`,
12px side padding; its items at the right, 16 apart, each a button where
it acts:

1. **Sync** (only for a synced vault): `cloud` while idle (tooltip
   "Synced <time>"), `refresh-cw` turning while syncing, `cloud-off`
   while offline ("Offline: <error>"), `circle-alert` when revoked or
   reset (the state's text); then "N conflicts" when there are any,
   which opens the conflicts list.
2. **Backlinks:** "N backlinks" for the active note; opens the Backlinks
   pane.
3. **Words and characters** of the active note, or of the selection
   while there is one: "N words", "N characters" (grouped thousands).
   Both count the body outside comments: words as notesd counts them
   ([specs/markdown.md](markdown.md#words)), characters as every
   character but line breaks.
4. **Editing mode:** an icon for live preview (`pencil`), source mode
   (`code-xml`) or reading (`book-open`); a menu switches it.

While the connection is not ready, its state shows at the left, as M0's
status bar does.

### Menus

Context menus and button menus: `--bg-window`, 1px `--separator`, rounded
8, `--popover-shadow`, 4px padding, at least 180 wide; `role="menu"`.
Items 28 high, 13/18, 8px side padding, rounded 4, an optional 16px icon
in `--text-secondary` and keys at the right (12/16 `--text-tertiary`);
the item under the pointer or keyboard is `--accent` with
`--accent-contrast`. Separators are 1px `--separator` with 4px margins.
↑ and ↓ move, Enter chooses, → and ← open and close submenus, Escape
and a click outside close. The webview's own context menu never shows.

### Notices

At the top right, 8 below the title bar row and 8 from the edge, at most
320 wide, stacking downward 8 apart: `--bg-toolbar`, 1px `--separator`,
rounded 8, `--popover-shadow`, 12/16, padding 10 12; an optional action
(12/16 `--accent`) at the right. A notice leaves after 4 s, or 10 s with
an action, or when clicked; `role="status"`.

### Settings *(batch 2)*

A dialog, 900 × 640 (at most 90% × 85% of the window): a 200-wide
`--bg-sidebar` list of sections at the left, the section at the right
with 24px padding. A setting is a row: its name (13/18) and description
(12/16 `--text-secondary`) at the left, its control at the right (a
36 × 20 switch, a select, a 200-wide input), 1px `--separator` between
rows. Changes apply at once. Defaults are Obsidian 1.14.4's.

| Section | Setting | Default | Kept |
| --- | --- | --- | --- |
| Editor | Default view for new tabs: Editing view, Reading view | Editing view | app |
| | Default editing mode: Live Preview, Source mode | Live Preview | app |
| | Show inline title | on | app |
| | Show view header | on | app |
| | Readable line length | on | app |
| | Spellcheck | on | app |
| | Properties in document: Visible, Hidden, Source | Visible | app |
| | Show line numbers | off | app |
| | Show indentation guides | on | app |
| | Fold headings; Fold indent | on; on | app |
| | Auto-pair brackets; Auto-pair Markdown syntax | on; on | app |
| | Smart lists | on | app |
| | Indent using tabs; Tab size | on; 4 | app |
| | Confirm file deletion | on | app |
| | Load remote images ([ADR-0007](../adr/0007-edit-in-codemirror-6-with-live-preview.md)) | on | app |
| Files and links | Default location for new notes: Vault folder, Same folder as current file, In the folder specified below | Vault folder | vault |
| | Folder to create new notes in | (none) | vault |
| | New link format: Shortest path when possible, Relative path to file, Absolute path in vault | Shortest path when possible | vault |
| | Use [[Wikilinks]] | on | vault |
| | Automatically update internal links (off: ask on each move) | off | vault |
| | Default location for new attachments | Vault folder | vault |
| | Show unsupported files | off | app |
| | Excluded files | none | vault |
| Appearance | Theme: System, Light, Dark | System | app |
| | Font size | 16 | app |
| | Show ribbon | on | app |
| Hotkeys | the keyboard map below, read-only in M3 | | |
| Daily notes | Date format; New file location; Template file location | `YYYY-MM-DD`; vault root; none | vault |
| Templates | Template folder location; Date format; Time format | none; `YYYY-MM-DD`; `HH:mm` | vault |
| Sync | the vault's hub, state, last sync, conflicts (read-only in M3) | | |

"App" settings are the app's preferences, "vault" settings are the
vault's `.frostnotes/vault.yaml`, read from `.obsidian/` while unset
([ADR-0021](../adr/0021-keep-the-workspace-and-app-preferences-on-each-device.md)).

### History *(batch 2)*

A `history` tab, titled "History of <name>": at the left a 280-wide list
of the file's versions, newest first, each 44 high: the time (13/18,
"Today at 9:41", "Yesterday at …", else the date), and under it its
source's icon (`pencil` editor, `bot` agent, `hard-drive` outside,
`cloud` sync) with the author and the size (12/16 `--text-secondary`);
a conflict version has a "Conflict" badge, a deletion reads "Deleted".
For a synced vault, a second list, "On the hub", of `sync.history`. At
the right, the selected version read-only in the editor's look, or with
Show changes, its difference from the current text. Restore this version
(`note.revert` with the current base) and Compare (the merge view) at
the top right.

### Merge view

A `merge` tab, titled "Merge <name>": a 36-high bar ("Comparing with
<the other side>" 13/16 at the left; Done at the right), then
`@codemirror/merge`'s side-by-side view: the note's document at the
left, editable, saving as the editor does; the other version at the
right, read-only. Changed chunks are tinted (`--flag-4` at 15% on the
left, `--flag-1` at 15% on the right), each with an arrow to take the
other side's chunk. Done closes the tab and, when the file is flagged on
its hub, resolves it (`sync.resolve`).

## Keyboard map

Obsidian 1.14.4's default hotkeys on Linux, with Obsidian's command IDs,
which the command registry uses too. `lib/keymap.ts` (T-0038) holds this
table. A binding matches a key press when its key matches and Ctrl, Alt
and Shift are exactly as listed; a press with Meta matches nothing.
Letters match in either case.

| Keys | Command ID | Command |
| --- | --- | --- |
| Ctrl + O | `switcher:open` | Open quick switcher |
| Ctrl + P | `command-palette:open` | Open command palette |
| Ctrl + N | `file-explorer:new-file` | Create new note |
| Ctrl + Shift + N | `file-explorer:new-file-in-new-pane` | Create new note to the right |
| Ctrl + T | `workspace:new-tab` | New tab |
| Ctrl + W | `workspace:close` | Close current tab |
| Ctrl + Shift + W | `workspace:close-window` | Close window |
| Ctrl + Shift + T | `workspace:undo-close-pane` | Undo close tab |
| Ctrl + Tab, Ctrl + Page Down | `workspace:next-tab` | Go to next tab |
| Ctrl + Shift + Tab, Ctrl + Page Up | `workspace:previous-tab` | Go to previous tab |
| Ctrl + 1 … Ctrl + 8 | `workspace:goto-tab-1` … `workspace:goto-tab-8` | Go to tab 1 … 8 |
| Ctrl + 9 | `workspace:goto-last-tab` | Go to last tab |
| Ctrl + Alt + ← | `app:go-back` | Navigate back |
| Ctrl + Alt + → | `app:go-forward` | Navigate forward |
| Ctrl + , | `app:open-settings` | Open settings |
| F1 | `app:open-help` | Open help (Settings, at Hotkeys) |
| F2 | `workspace:edit-file-title` | Rename file |
| Ctrl + E | `markdown:toggle-preview` | Toggle reading view |
| Ctrl + ; | `markdown:add-metadata-property` | Add file property |
| Ctrl + Shift + F | `global-search:open` | Search in all files |
| Ctrl + F | `editor:open-search` | Search current file |
| Ctrl + H | `editor:open-search-replace` | Search & replace in current file |
| Ctrl + S | `editor:save-file` | Save current file |
| Ctrl + B | `editor:toggle-bold` | Toggle bold |
| Ctrl + I | `editor:toggle-italics` | Toggle italics |
| Ctrl + K | `editor:insert-link` | Insert Markdown link |
| Ctrl + L | `editor:toggle-checklist-status` | Toggle checkbox status |
| Ctrl + / | `editor:toggle-comments` | Toggle comment |
| Ctrl + D | `editor:delete-paragraph` | Delete paragraph |
| Alt + Enter | `editor:follow-link` | Follow link under cursor |
| Ctrl + Enter | `editor:open-link-in-new-leaf` | Open link under cursor in new tab |
| Ctrl + Alt + Enter | `editor:open-link-in-new-split` | Open link under cursor to the right |
| Ctrl + Alt + Shift + Enter | `editor:open-link-in-new-window` | Open link under cursor in new window (a new tab until windows come) |
| Ctrl + G | `graph:open` | Open graph view (M5; nothing before) |
| Ctrl + = | `window:zoom-in` | Zoom in |
| Ctrl + - | `window:zoom-out` | Zoom out |
| Ctrl + 0 | `window:reset-zoom` | Reset zoom |

The last three are Obsidian's View menu (Electron's), not commands
there. Keys are shown as Obsidian shows them: modifiers in the order
Ctrl, Alt, Shift, joined with " + "; ← → ↑ ↓ for the arrows, "Space" for
the space bar, other names split into words ("Page Down"), a letter
uppercased.

**In the editor,** CodeMirror's default keymap applies without what
Obsidian removes from it (Alt + ↑ and Alt + ↓ with and without Shift,
Alt + L, Alt + V, Ctrl + M) and without any key the table binds (so
Ctrl + D, Ctrl + I, Ctrl + Enter and Ctrl + / are the commands above),
with its history keys (Ctrl + Z, Ctrl + Shift + Z, Ctrl + Y), Tab and
Shift + Tab indenting, and in its search panel F3 and Shift + F3 for the
next and previous match.

**Mouse:** Ctrl + click on a link opens it in a new tab, Ctrl + Alt +
click to the right; middle click on a tab closes it, on a file opens it
in a new tab. Ctrl while hovering a link in the editor shows its page
preview; hovering one in the reading view does too.

**Where keys apply:** the window's listener sees a key press only after
the editor and fields have had it. In a text field outside the editor,
only presses with Ctrl or Alt, and F1 and F2, reach the table; Escape
belongs to the field, a prompt or a menu.

## Rules

- No component draws literal colors or fonts; tokens only.
- Rendered note HTML reaches the page only through the reading view,
  live preview's rendered blocks and page previews, each sanitized by
  notesd and again by DOMPurify and finished in a template before the
  page has it, and never through `dangerouslySetInnerHTML` elsewhere; no
  element of it carries an event handler, a class the renderer does not
  draw, or `style`, except the markup KaTeX draws for math afterwards,
  from the `data-tex` text alone. Its box (`.rendered`) contains its
  layout, so nothing in it is placed over the window.
- The editor's document is the file's text: only CodeMirror transactions
  change it, and only the document registry writes it, with its base
  ([ADR-0022](../adr/0022-the-editor-saves-as-you-type-and-applies-merges-as-remote-changes.md)).
- Vault files load from their hash's URL only
  ([ADR-0023](../adr/0023-the-app-shows-files-by-their-hash-and-uploads-them-through-notesd.md));
  web links open in the system browser (`http`, `https` and `mailto`
  only), never in the webview.
- Every interactive element is reachable by keyboard and has an
  accessible name.

## References

- Rationale: [ADR-0011](../adr/0011-look-like-frostmails-sibling-with-obsidians-layout.md), [ADR-0007](../adr/0007-edit-in-codemirror-6-with-live-preview.md), [ADR-0021](../adr/0021-keep-the-workspace-and-app-preferences-on-each-device.md), [ADR-0022](../adr/0022-the-editor-saves-as-you-type-and-applies-merges-as-remote-changes.md), [ADR-0023](../adr/0023-the-app-shows-files-by-their-hash-and-uploads-them-through-notesd.md); frostmail's `docs/specs/ui.md` for the tokens and the chrome
- Context: [design/app.md](../design/app.md)
- Built in: [plan 0005 — M3](../plans/0005-m3-desktop-app.md)
