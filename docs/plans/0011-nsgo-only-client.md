# Plan 0011: NSGo as the only client

This plan makes the NSGo client (`cmd/frostnotes-nsgo`) Frostnotes' only
client on Linux and in Lodge, and removes the MyGo client
([ADR-0049](../adr/0049-make-the-nsgo-client-the-only-client.md)). It is
Frostnotes' part of
[frostyard/frostmail's plan 0017](https://github.com/frostyard/frostmail/blob/main/docs/plans/0017-nsgo-as-the-only-client.md),
its Phase 5, and it supersedes the fallback of
[plan 0009](0009-mygo-desktop.md) and
[ADR-0047](../adr/0047-use-the-shared-nsgo-editor-on-linux-and-lodge.md).

Status (2026-11-25): the owner answered the plan's questions
([below](#the-owners-answers)) and approved ADR-0049 (#287). Phase 1 is
built (#288). Phase 2 is in progress.

## What the NSGo client lacks

A side-by-side read of both clients' code on 2026-11-25 (at `0aab290`)
found 122 things the MyGo client does. Of them the NSGo client has 7, has
32 in part, and lacks 82; 1 was not determined.

| Group | Things | Has | In part | Lacks |
| --- | --- | --- | --- | --- |
| A. History, compare and revert, the merge view, sync conflicts | 12 | 0 | 2 | 10 |
| B. Attachments, images, files that are not Markdown | 11 | 0 | 0 | 11 |
| C. Live preview's marks | 23 | 4 | 7 | 12 |
| D. Side panes, the properties editor, tabs, splits, back and forward | 20 | 0 | 3 | 17 |
| Outside the 4 groups | 56 | 3 | 20 | 32 |

**How the NSGo client edits.** Source mode is a multiline `ui.Input`
(`main.go`). Live preview (`inline.go`) is a list of rows, 1 for each
line of the source, where a press makes 1 line a single-line input; it
calls the scanner with no options and drops a piece that spans lines.
NSGo's code editor (`github.com/nsgo-dev/nsgo/base/editor`, v0.3.0) has
an app's highlighter, styled ranges (`Decoration`: a range's `Style` and
its lines' `LineStyle`), folds, completions, find and replace and multiple
selections. It has no hidden or replaced range, no widget in a line or
between lines, and 1 line height. So it cannot draw live preview as
[ADR-0039](../adr/0039-live-preview-is-replacements-over-the-source.md)
draws it. And a new value is recorded as an undo step, with no edit kept
out of the history and no way to clear it (`state.replaceText`), which
[ADR-0022](../adr/0022-the-editor-saves-as-you-type-and-applies-merges-as-remote-changes.md)
rules out for merges and other writers' changes.

**Other gaps that shape the work.**
- The NSGo client lists notes with `note.list`, so attachments and empty
  folders never show; the MyGo client uses `note.files`. The MyGo client
  reads a file's bytes from notesd's data directory, which the web build
  cannot; Lodge serves them at `/content/blobs/`
  ([specs/lodge-app.md](../specs/lodge-app.md)).
- NSGo's diff view (`nsgo/ui/diff`) takes a chunk back but has no
  editable side.
- The kit's `Shell` has 1 inspector and 1 auxiliary panel on the right,
  and no tab strips or status bar. NSGo's `ui.DockArea` has tab groups,
  splits and a layout kept as JSON.
- The MyGo client calls 25 methods the NSGo client never calls: `note.`
  backlinks, daily, deleteFolder, files, links, mkdir, moveFolder,
  properties, readVersion, renderMarkdown, tags, tasks, template, upload;
  `sync.` conflicts, history, leave, readRevision, resolve; `hub.` devices,
  pending, revoke, status; `vault.` configure, settings.
- Logic the NSGo client needs is in packages that import MyGo or frostui's
  root module: `editor/format.go`, `links.go` and `properties.go`, the
  attachment rules of `editor/attach.go`, `panes/tree.go`, `catalog` (for
  `frostui.Match` only) and `system/open.go`.

**Tests.** The MyGo client's packages have 192 tests and the typing
benchmark; the packages ADR-0049 keeps have 100 and the scanner's
benchmark; the NSGo client has 29.

**What the NSGo client has that the MyGo client lacks:** the Lodge build,
a narrow layout, the notes tree's filter, Restore deleted note by path,
the window kept on the device, an Update links choice on each move, a
read-only recovered draft on a refused base, and the kit's look with its
pictures. Each stays.

## Phase 1 — Logic out of MyGo's packages (S, built: #288)

- The formatting commands, smart lists, links under the caret, frontmatter
  and its properties, and the attachment rules (`FreePath`, `EmbedFor`,
  `CleanName`) move with their tests to a package that imports no UI.
  `panes/tree.go` moves too. `catalog` matches without frostui.
  `system/open.go`'s allow-lists (`WebURL`, `VaultFile`,
  `OpenableExtensions`, the mailto cleaning) stay in `system` without
  frostui; the MyGo client keeps `frostui.Opener` in `cmd/frostnotes`.
- The MyGo client uses the moved code and behaves as before.
- **Done when:** these packages import neither MyGo nor frostui's root
  module, their tests pass in `make verify`, and the MyGo client's tests
  pass unchanged.

## Phase 2 — Source mode on NSGo's code editor (M)

Implements [design/app.md, Editor](../design/app.md#editor).

- Source mode is NSGo's code editor, bound to the document model
  (`internal/desktop/notes`): Obsidian's highlighting as an app highlighter
  and decorations; folds by headings and indentation; line numbers,
  indent guides, tab size, indenting with tabs; auto-paired brackets and
  marks; smart lists; `[[`, `#` and heading and block completions; find
  and replace in the note; the formatting commands; following the link
  under the caret; multiple selections; the inline title; the readable
  line length.
- **The undo rule until NSGo has an edit kept out of the history:** a merge
  or another writer's change replaces the editor's text without adding a
  step, by starting the editor again with the new text and the caret and
  scroll mapped through the change. The user's undo history ends there,
  as live preview's does today. Ctrl+Z never undoes another writer's
  change.
- An NSGo issue ([nsgo-dev/nsgo#2409](https://github.com/nsgo-dev/nsgo/issues/2409),
  filed 2026-11-25) asks for hidden and replaced ranges, widgets in and
  between lines, lines of their own height, and an edit kept out of the
  history (as MyGo's `EditRemote`).
- The typing gate covers NSGo's source mode and live preview: a keystroke
  in a 10,000-line note at most 16 ms on a developer machine and 25 ms on
  CI ([ADR-0043](../adr/0043-the-typing-gate-is-looser-on-ci.md)).
- **Done when:** the client's suite holds each of these in source mode,
  including a merge that Ctrl+Z does not undo, and the typing gate passes
  on NSGo in both modes.

## Phase 3 — Tabs, splits and the side panes (L)

Group D. Implements [design/app.md, Workspace](../design/app.md#workspace)
and [Properties](../design/app.md#properties).

- Tabs and splits on NSGo's `ui.DockArea`, from the workspace model
  (`internal/desktop/workspace`): new, close, close others and to the
  right, undo close, pin, reorder, the tab keys, split right and down, the
  active group; back and forward for each tab; opening in a new tab or to
  the right from the tree, panes, editor and switcher; the workspace kept
  for each vault, with moves and deletes applied; caret and scroll
  restored; Reset workspace layout.
- The right sidebar's 6 panes (backlinks with unlinked mentions, outgoing
  links, tags, all properties, outline, tasks), the search pane, the view
  header with its breadcrumb and More options, and a status bar (backlinks,
  conflicts, sync, the mode, counts).
- The properties editor above the note, its typed fields, and Add file
  property.
- **Done when:** the suite holds each, with its pictures.

## Phase 4 — History, the merge view and conflicts (M)

Group A. Implements [design/app.md, History and merges](../design/app.md#history-and-merges).

- Version history (notesd's versions, then the hub's revisions), a version
  shown read-only, Show changes, Restore this version.
- **The merge view is a diff, then edit** (the owner's answer 2): the other
  text beside the note, each chunk taken back as an edit of the note that
  saves with its base, and Done, which calls `sync.resolve` when the file
  has a conflict on the hub. Other edits are made in the note. This
  changes ADR-0022's editable merge view and is recorded in ADR-0049.
- Compare on the conflict and refused-base notices; the list of sync
  conflicts.
- **Done when:** the suite holds each, on a conflict made by a real notesd.

## Phase 5 — Files, attachments and folders (L)

Group B, and the files and folders of the owner's answer 4.

- The tree from `note.files`: attachments, empty folders, extension
  badges, Show unsupported files; sort orders, collapse and expand all,
  expansion kept, the active file revealed, context menus, drag to move,
  copy path.
- Image, audio and video tabs; other files open in the default app behind
  `system`'s allow-list. In Lodge, files load from `/content/blobs/`.
- Paste, drop and Attach file, through `note.upload`, placed by the
  vault's attachment folder.
- New folder; rename, move and delete a folder; delete or move any file;
  Untitled N notes made at once.
- **Done when:** the suite holds each, on desktop and in the web build's
  run in Lodge's fixture (`make nsgo-lodge-evidence`).

## Phase 6 — Live preview's marks and the reading view (L)

Group C and the reading view (the owner's answers 1 and 3).

- Live preview stays rows of lines (the owner's answer 1), carries pieces
  across lines, and draws every mark the MyGo client draws: emphasis,
  strike, highlight, inline code, comments, rules, tags, footnotes, math,
  tables, callouts, code blocks with Copy, Markdown links, wikilinks with
  subpaths and creation, embeds, hidden frontmatter, page preview.
- The reading view on NSGo's `TextView`.
- **Done when:** the suite holds each mark and the reading view, with
  their pictures. When NSGo's editor has what the Phase 2 issue asks for,
  live preview moving onto it is a change of its own.

## Phase 7 — Commands, daily notes and settings (M)

The owner's answers 5, 6 and 7.

- Every command of the MyGo client, Obsidian's keys, the switcher's
  aliases, recents and Create, the Keyboard shortcuts dialog and About.
- Today's daily note and Insert template, with their settings.
- The vault's Files and links settings, and the preferences that apply on
  NSGo, kept where the client keeps its appearance. Zoom waits on NSGo,
  which has none.
- **Done when:** the suite holds each.

## Phase 8 — The hub, notices and the app (M)

The owner's answers 8, 9 and 10.

- A hub's devices with Revoke, pending approvals, Stop syncing, and the
  full sync states.
- A deleted note's Restore, delete with Undo, Retry now, Close anyway,
  confirm delete as a preference, info toasts.
- The problem record, why notesd is not running, the Background portal's
  answer, the window's title, and web links behind the allow-list.
- **Done when:** the suite holds each, and a crash of the client makes a
  problem record in a test journal.

## Phase 9 — MyGo is removed (M)

- `cmd/frostnotes` and the packages of `internal/desktop/` that import
  MyGo or frostui go; the Flatpak's `frostnotes-mygo`, `--mygo` and its
  smoke test, the metainfo's text, README and `docs/development.md`, the
  Makefile's MyGo targets and its frostui test run; `go.mod`'s MyGo,
  frostui and the fork's `replace`; `make flatpak-sources`.
- `app/` is reduced to the loader of the NSGo web build (`app/nsgo.html`,
  `app/src/nsgo-lodge.ts`, Vite and `@frostyard/lodge-client`), with the
  React UI, `legacy-lodge-app` and its checks gone.
- **Done when:** the Flatpak holds 1 client, the root `go.mod` names
  neither MyGo nor the fork's `replace`, `make ci` passes, and the
  owner releases the version (`make bump`).

## Later / ideas

- Live preview on NSGo's code editor, once it has hidden ranges and
  widgets ([nsgo-dev/nsgo#2409](https://github.com/nsgo-dev/nsgo/issues/2409)).
- Zoom, once NSGo has it.

## Open questions

- None open. The owner's answers are below.

## The owner's answers

Brian answered these on 2026-11-25, each as a choice among options the
agent set out, after the side-by-side read above.

1. **The editor:** source mode moves to NSGo's code editor; live preview
   stays rows of lines, grown to draw every mark, with editing 1 line at a
   time; an NSGo issue asks for hidden ranges and widgets.
2. **The merge view:** a diff that takes chunks back, then edits in the
   note.
3. **The reading view** is kept.
4. **Files and folders:** all kept.
5. **Commands and keys:** all kept.
6. **Daily notes and templates:** kept.
7. **The vault's settings and the preferences:** all kept.
8. **The hub's devices, approvals, Stop syncing and sync states:** kept.
9. **The notices:** MyGo's kept.
10. **The app** (problem record, why notesd is not running, the portal's
    answer, the window's title, web links): all kept.
11. **The typing gate:** the same limits on NSGo, before MyGo's goes.

Earlier, on 2026-11-24 (frostyard/frostmail's plan 0017, its answer 1),
he decided that the 4 groups are kept before MyGo goes.

## References

- Implements: [ADR-0049](../adr/0049-make-the-nsgo-client-the-only-client.md),
  [design/app.md](../design/app.md#shared-nsgo-editor-preview)
- Builds on: [plan 0009](0009-mygo-desktop.md),
  [ADR-0022](../adr/0022-the-editor-saves-as-you-type-and-applies-merges-as-remote-changes.md),
  [ADR-0039](../adr/0039-live-preview-is-replacements-over-the-source.md),
  [ADR-0043](../adr/0043-the-typing-gate-is-looser-on-ci.md)
- The same work in FrostMail and PocketGo:
  [frostyard/frostmail's plan 0017](https://github.com/frostyard/frostmail/blob/main/docs/plans/0017-nsgo-as-the-only-client.md)
