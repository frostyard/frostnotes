# 0038 — Draw the desktop app natively with MyGo and frostui

- **Status:** Proposed
- **Date:** 2026-11-10

## Context

The desktop app is a Tauri v2 window that runs the React UI in the
system's WebKitGTK, and the same bundle is lodge's web app
([ADR-0004](0004-a-notes-daemon-with-thin-clients.md),
[ADR-0011](0011-look-like-frostmails-sibling-with-obsidians-layout.md),
[ADR-0030](0030-the-look-comes-from-lodges-ui-package.md)). It edits
notes in CodeMirror 6 with Obsidian's live preview
([ADR-0007](0007-edit-in-codemirror-6-with-live-preview.md)) and ships as
a Flatpak with its own notesd
([ADR-0031](0031-the-desktop-app-ships-as-a-flatpak-with-its-own-notesd.md)).

Frostyard is moving its desktop apps to MyGo
(`github.com/egoist/mygo`, a Go toolkit that draws its own widgets on
GTK 3 and OpenGL, loaded with purego, no CGO) and a shared kit, frostui.
ChairLift moved first
([ChairLift ADR-0017](https://github.com/frostyard/chairlift/blob/feat/mygo/docs/adr/0017-mygo-command-center-window.md)),
and frostui is leaving it for `github.com/frostyard/frostui`. On
2026-11-10 the owner decided, for Frostnotes:

- **Two UIs.** The desktop app becomes native, on MyGo and frostui. The
  React UI stays only as lodge's web app (phones, browsers, agents
  through lodge), and lodge stays.
- **Thin clients.** Both UIs are clients of the same JSON-RPC API, and
  logic stays in notesd.
- **One look with ChairLift** where it fits: the hidden title bar and
  frostui's top bar, the command palette (Ctrl K), the status bar,
  toasts, dialogs and the theme. Frostnotes keeps its own layout (files,
  tabs in splits, the editor, backlinks, search, history, merges);
  ChairLift's page rail, console and job queue are not forced on it.
- The desktop Flatpak keeps its own notesd.

What MyGo v0.8.3 offers, from its documentation and source:

- **A code editor** (`ui/editor`): a rope buffer with undo, tokenizers
  or a highlighter of the app's own, folding the app can define, several
  carets, find and replace, completions, hovers, inlay text,
  diagnostics, line and text backgrounds, Ctrl-click to a definition,
  a diff base, and layout of only the lines in view (a million lines
  edit as ten). It draws one monospaced font, it cannot hide a range of
  the text or draw a widget in place of one, and every edit of its
  buffer enters the undo history, where ADR-0022 applies another
  writer's change outside it.
- **A rich text editor** (`ui/richtext`): attributed text with headings,
  lists, tasks, quotes, code and images. It holds paragraphs and runs,
  not a file: its Markdown export writes its own spelling.
- **A Markdown view** (`ui/markdown`): CommonMark with GitHub's tables,
  tasks, footnotes and math as TeX, highlighted code, images from a
  loader, links to a handler, and blocks of the app's own drawing. Raw
  HTML shows as text, so a note can run nothing.
- **A diff viewer** (`editor.DiffView`): side by side or unified, words
  changed within lines, unchanged lines collapsed, an editable side.
- An outline that builds only the rows in view, splits, tabs, menus,
  context menus, drag and drop with other apps, folder pickers through
  the portal, single instance, deep links, start at login, a web view
  (WebKitGTK), shaped text with caret and hit-test geometry and a
  text-input client for widgets that edit text of their own.
- An AT-SPI tree for screen readers, and `ui.NewTester`, which renders a
  view headless (Pango and cairo, no display) with clicks, keys and a
  PNG of the frame.

notesd and its packages are Go, so a Go client can use the generated Go
client (`api/`) and the same Markdown parser as notesd
(`internal/markdown`, goldmark with the Obsidian extension), where the
React app has a TypeScript client and Lezer.

## Decision

- **The desktop app is a Go program, `frostnotes`, drawn with MyGo and
  frostui.** It replaces the Tauri shell (`app/src-tauri`) and the React
  UI on the desktop. The React app stays as lodge's web app, built by
  `make web-build` and `make lodge-app`; its desktop-only parts (the
  Tauri transport, the window buttons, `@tauri-apps/*`) go when the
  Tauri shell goes.
- **It is a thin client of the same API.** It speaks to notesd over the
  private socket, as the `local` caller `notesctl` is, with the generated
  Go client. It imports no package that owns state (`internal/engine`,
  `store`, `vault`, `syncer`, `hub`). What both UIs need and is not
  drawing (link resolution, rendering, embeds, templates, daily notes)
  is notesd's, added to `schema/rpc` first; the web app and the desktop
  app do not each write it.
- **The desktop reads Markdown with notesd's parser.** Its highlighting,
  folding, outline and live preview come from `internal/markdown`,
  which the app runs in process; the web app keeps Lezer, and the
  fixtures of [specs/markdown.md](../specs/markdown.md) still bind the
  two.
- **The note stays the file's bytes.** The editor's document is the
  note's exact text, saved with its base
  ([ADR-0006](0006-every-write-names-the-version-it-changes.md),
  [ADR-0022](0022-the-editor-saves-as-you-type-and-applies-merges-as-remote-changes.md));
  live preview is decoration, never transformation, as in ADR-0007. The
  rich text editor is not used for notes.
- **The editor comes in two steps.**
  1. Source mode on MyGo's code editor, with a Markdown highlighter over
     the shared parser (headings, emphasis, links, tags, callouts and
     code in their own colors and weights), folding by headings and
     lists, completions of links, tags and properties, hovers that
     preview a linked note, Ctrl-click to follow a link, and the find
     and replace bar; and the reading view on MyGo's Markdown view.
     Until MyGo's buffer takes an edit outside its undo history, the
     app keeps its own undo, so Ctrl + Z never takes back another
     writer's change.
  2. Live preview, a Frostnotes editor that hides the syntax away from
     the caret and draws widgets (checkboxes, images, embeds, callouts,
     tables, math), built on MyGo's text-input client and shaped text
     over the code editor's rope. Before it is built, MyGo's author is
     asked whether `ui/editor` will take hidden ranges, widgets and
     per-line styles (as Zed's folds with placeholders and blocks); if it
     will, live preview is decorations on MyGo's editor instead.
- **notesd renders the reading view for native views too.** Beside the
  sanitized HTML of [specs/render.md](../specs/render.md), `note.render`
  gains an additive format that MyGo's Markdown view reads: CommonMark
  with GitHub's extensions, with links resolved to vault paths, embeds
  expanded by the same rules of depth and cycles, attachments named by
  hash, highlights as `<mark>`, comments dropped and callouts as
  quotations that the view draws as callouts. The desktop shows no
  HTML and has no web view.
- **The look is frostui's.** No system title bar; frostui's top bar,
  command palette, status bar, dialogs and toasts; the theme follows the
  desktop's accent, light or dark, contrast and text size. Under the top
  bar, Obsidian's layout: a left sidebar (files, search), a workspace of
  tab groups in splits, each with its own tab strip, and a right
  sidebar (backlinks, outgoing links, tags, properties, outline, tasks).
  ChairLift's page rail, console and job queue are not used. Obsidian's
  default shortcuts stay
  ([plan 0005](../plans/0005-m3-desktop-app.md)'s exit criterion): the
  palette opens on Ctrl + P for commands and Ctrl + O for notes, and on
  Ctrl + K outside the editor, where Ctrl + K stays Insert Markdown
  link.
- **Generic pieces wait in one app-local package** that imports only the
  standard library, MyGo, purego and frostui, enforced by a test, so
  that they move into frostui by changing an import path. Frostnotes
  edits no frostui code.
- **The Flatpak keeps its own notesd.** The Go app replaces
  `/app/libexec/frostnotes-app`. The launcher `frostnotes.sh` (notesd's
  start, and its replacement on update,
  [ADR-0032](0032-an-update-replaces-the-running-notesd.md)) and the
  Background portal helper stay as they are. What the Rust shell did
  moves to Go with the same files and rules: the background setting
  ([ADR-0033](0033-a-setting-keeps-notesd-running-without-a-window.md)),
  notesd's log tail, opening links and files with `xdg-open` behind the
  same allow-lists, attachments read from notesd's blob store by hash
  ([ADR-0023](0023-the-app-shows-files-by-their-hash-and-uploads-them-through-notesd.md)),
  the folder picker (through the portal), one instance, and problems
  recorded with `github.com/frostyard/std/problems`, as notesd records
  them ([ADR-0036](0036-notesd-and-the-app-record-problems-in-the-journal.md)).
  The Flatpak build loses its Rust and Node modules. Release builds are
  MyGo production builds (no inspector, no automation plugin).
- **The workspace and preferences stay on the device**
  ([ADR-0021](0021-keep-the-workspace-and-app-preferences-on-each-device.md)),
  in a versioned JSON file per vault under the app's state directory
  instead of WebKitGTK's storage. The webview's copy is not migrated: the
  first native start opens the default layout.
- **Tauri stays until the native app reaches parity** with the list in
  [plan 0009](../plans/0009-mygo-desktop.md), then the shell, its Rust
  crates, its Flatpak modules and its CI go in one change.

## Consequences

- One language for the desktop app and notesd: the desktop builds and
  tests on the host with Go alone, with no Rust, Node or pnpm, and the
  nsl machine serves only the web app.
- One Markdown parser on the desktop, and notesd's types come from the
  generated Go client: no second client to keep in step there.
- Every view has headless tests with `ui.NewTester`; unit runs need
  Pango and cairo, and CI installs them.
- Two UIs to keep: a feature lands in the web app and the desktop app
  apart. Moving logic into notesd keeps what is written twice to drawing.
  [specs/ui.md](../specs/ui.md) splits into what both UIs do and how each
  draws it.
- Live preview is a widget of Frostnotes' own until MyGo offers hidden
  ranges and widgets: the largest piece of the move, and the reason
  Tauri stays until parity.
- MyGo has no TeX renderer, spell checker or audio and video decoders,
  and decodes PNG, JPEG, GIF (the first frame), WebP and BMP, where
  WebKitGTK also drew AVIF and SVG: math shows as TeX, audio and video
  embeds open in the desktop's player (as PDFs already do), and the
  spellcheck preference does nothing on the desktop until MyGo has one.
- MyGo is young, with one main author; Frostnotes pins it and keeps
  frostui between its views and MyGo, as ChairLift does. Its runtime
  needs GTK 3, libepoxy and Pango, which the GNOME runtime has.
- ADR-0011's stack and ADR-0030's look, ADR-0025's patches and
  ADR-0027's narrow layout apply to the web app only.

## Alternatives considered

- **Keep Tauri and React on the desktop:** one UI, but not frostyard's
  desktop look or toolkit, and it keeps Rust, pnpm and WebKitGTK's costs
  (ADR-0025) in the desktop app.
- **MyGo around a web view of the React editor:** live preview from the
  first day, but WebKitGTK, the bundle and a bridge stay in the desktop
  app, and the window has two toolkits and two accessibility trees.
- **MyGo's rich text editor:** it saves its own Markdown, which rewrites
  notes, as ADR-0007 found of TipTap.
- **Source and reading views only:** what ADR-0007 rejected; here it is
  only the first step.
- **Rendering the reading view from the parser in the app:** embeds,
  link resolution and cycles would be written a second time beside
  notesd's renderer.
- **frostui's command-center shell as it is:** its rail is a list of
  pages and its content a padded page of cards, where Frostnotes needs a
  file tree and edge-to-edge panes.

## References

- Shapes: [design/app.md](../design/app.md), [specs/ui.md](../specs/ui.md),
  [specs/render.md](../specs/render.md), [specs/rpc-api.md](../specs/rpc-api.md)
- Built in: [plan 0009](../plans/0009-mygo-desktop.md)
- Supersedes in part, once accepted: [ADR-0004](0004-a-notes-daemon-with-thin-clients.md)
  (the desktop client and one UI codebase),
  [ADR-0007](0007-edit-in-codemirror-6-with-live-preview.md) (CodeMirror on
  the desktop), [ADR-0011](0011-look-like-frostmails-sibling-with-obsidians-layout.md)
  and [ADR-0030](0030-the-look-comes-from-lodges-ui-package.md) (the
  desktop's stack and look),
  [ADR-0031](0031-the-desktop-app-ships-as-a-flatpak-with-its-own-notesd.md)
  (the Tauri module of the Flatpak)
- Builds on: [ADR-0003](0003-plain-markdown-files-are-the-source-of-truth.md),
  [ADR-0006](0006-every-write-names-the-version-it-changes.md),
  [ADR-0021](0021-keep-the-workspace-and-app-preferences-on-each-device.md),
  [ADR-0022](0022-the-editor-saves-as-you-type-and-applies-merges-as-remote-changes.md),
  [ADR-0023](0023-the-app-shows-files-by-their-hash-and-uploads-them-through-notesd.md),
  [ADR-0024](0024-the-app-opens-and-creates-vaults-and-a-window-switches-between-them.md),
  [ADR-0032](0032-an-update-replaces-the-running-notesd.md),
  [ADR-0033](0033-a-setting-keeps-notesd-running-without-a-window.md),
  [ADR-0036](0036-notesd-and-the-app-record-problems-in-the-journal.md);
  ChairLift's [ADR-0017](https://github.com/frostyard/chairlift/blob/feat/mygo/docs/adr/0017-mygo-command-center-window.md)
  and [design/frostui.md](https://github.com/frostyard/chairlift/blob/feat/mygo/docs/design/frostui.md)
