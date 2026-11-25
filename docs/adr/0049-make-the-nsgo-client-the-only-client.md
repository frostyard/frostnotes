# 0049 — Make the NSGo client the only client

- **Status:** Proposed
- **Date:** 2026-11-25

## Context

Frostnotes has 2 desktop clients of notesd's API:

- **The MyGo client** (`cmd/frostnotes`, `internal/desktop/`), on MyGo and
  frostui ([ADR-0038](0038-draw-the-desktop-app-natively-with-mygo-and-frostui.md)).
  It has version history with compare and revert, the merge view, the list
  of sync conflicts, attachments and images, the side panes, tabs and
  splits, and most of live preview's marks. `go.mod` requires MyGo through
  the owner's fork (a `replace` of `github.com/egoist/mygo` by
  `github.com/bketelsen/mygo`) and frostui's root module.
- **The NSGo client** (`cmd/frostnotes-nsgo`, a module of its own), on NSGo
  and frostui's `workspaceui` kit, which FrostMail and PocketGo share.
  [ADR-0047](0047-use-the-shared-nsgo-editor-on-linux-and-lodge.md) made it
  the default on Linux and in Lodge, and kept the MyGo client in the
  Flatpak behind `--mygo` for what the NSGo client lacks.

Each interaction is written twice, on 2 toolkits. On 2026-11-24 the owner
decided, for FrostMail, Frostnotes and PocketGo, that the NSGo client
becomes each app's only client
([frostyard/frostmail's plan 0017](https://github.com/frostyard/frostmail/blob/main/docs/plans/0017-nsgo-as-the-only-client.md),
its answer 1): Frostnotes keeps all 4 groups of the MyGo client's features
before MyGo goes. What the MyGo client has outside those 4 groups had no
answer then.

## Decision

The NSGo client becomes Frostnotes' only client, on Linux and in Lodge.

1. **It first gets the MyGo client's 4 groups of features:** version
   history with compare and revert, the merge view and the sync
   conflicts; attachments, images and files that are not Markdown; live
   preview's missing marks; the side panes, tabs and splits. Each group is
   a change of its own, with its tests in the client's suite and its
   pictures.
2. **What the MyGo client has outside the 4 groups is kept too** (the
   owner's answers of 2026-11-25, in
   [the plan](../plans/0011-nsgo-only-client.md#the-owners-answers)): the
   reading view, files and folders, the commands and keys, daily notes
   and templates, the vault's settings and the preferences, the hub's
   devices, the notices, and the app's problem record, notesd's cause,
   the portal's answer, the window's title and web links.
3. **Source mode is NSGo's code editor; live preview stays rows of
   lines.** NSGo's code editor cannot hide or replace text or put a widget
   in a line, so live preview is drawn as the NSGo client draws it today,
   as replacements over the source
   ([ADR-0039](0039-live-preview-is-replacements-over-the-source.md)) on 1
   line at a time, grown to every mark. An NSGo issue asks for what the
   editor lacks. Until NSGo can apply an edit outside the undo history, a
   merge or another writer's change ends the user's undo history instead
   of entering it, which keeps
   [ADR-0022](0022-the-editor-saves-as-you-type-and-applies-merges-as-remote-changes.md)'s
   rule that Ctrl+Z undoes only the user's own edits.
4. **The merge view is a diff, then an edit.** NSGo's diff view takes a
   chunk back as an edit of the note, which saves with its base; it has no
   editable side, so other edits are made in the note. This replaces
   ADR-0022's editable merge view.
5. **The typing gate keeps its limits** on NSGo's source mode and live
   preview before MyGo's gate goes.
6. **MyGo is removed** once the 4 groups and the kept items are built:
   `cmd/frostnotes`, the packages of `internal/desktop/` that import MyGo
   or frostui's root module, the Flatpak's `--mygo` and its MyGo build, and
   what names them. The packages that import neither (`notes`,
   `livepreview`, `session`, `vaults`, `workspace`, `prefs`) stay where
   the NSGo client uses them, and `system`, which the NSGo client imports,
   first loses its 1 file that imports frostui (`open.go`). The root
   `go.mod` then requires neither MyGo, its fork's `replace`, nor
   frostui's root module.
7. **The React app** is reduced to the loader of the NSGo client's web
   build in Lodge, or the loader is made without it.

This supersedes ADR-0047's MyGo fallback, ADR-0038's choice of MyGo,
and ADR-0022's editable merge view.

## Consequences

- One client to change, test and draw. The kit's standard for an app
  (frostui's `docs/specs/workspaceui.md`) and the NSGo client's live suite
  (`make nsgo-live`) are the product's.
- The Flatpak holds 1 client and is smaller. A user who opens history or
  a side pane with `--mygo` today has no fallback after the removal, so
  the removal waits for those features in the NSGo client.
- MyGo's own tools go with it: `make desktop-build`, `desktop-run`,
  `desktop-smoke`, `desktop-evidence`, and the typing gate
  (`make desktop-bench`, [ADR-0043](0043-the-typing-gate-is-looser-on-ci.md)),
  which measures MyGo's editor. The NSGo editor needs a typing gate of its
  own before MyGo's goes.
- Live preview's long tail (tables, embeds, math) moves to NSGo's editor,
  which draws replacements over the source as the MyGo client did
  ([ADR-0039](0039-live-preview-is-replacements-over-the-source.md)).
  What NSGo's editor cannot yet draw waits on NSGo, which frostyard does
  not own.
- The owner's MyGo fork is no longer needed by Frostnotes.

## Alternatives considered

- **Keep MyGo as the fallback** (ADR-0047): every feature stays written
  twice, and the Flatpak carries 2 toolkits.
- **Remove MyGo now:** history, conflicts, attachments and the side panes
  would be gone from the desktop until rebuilt.
- **Drop the 4 groups:** the owner chose to keep them.

## References

- Shapes: [plan 0011: NSGo as the only client](../plans/0011-nsgo-only-client.md),
  [design/app.md](../design/app.md#shared-nsgo-editor-preview)
- Builds on: [ADR-0043](0043-the-typing-gate-is-looser-on-ci.md),
  [ADR-0046](0046-test-a-shared-nsgo-source-editor.md),
  [ADR-0047](0047-use-the-shared-nsgo-editor-on-linux-and-lodge.md),
  [ADR-0039](0039-live-preview-is-replacements-over-the-source.md)
- Supersedes: [ADR-0047](0047-use-the-shared-nsgo-editor-on-linux-and-lodge.md)'s
  MyGo fallback, [ADR-0038](0038-draw-the-desktop-app-natively-with-mygo-and-frostui.md)'s
  choice of MyGo, [ADR-0022](0022-the-editor-saves-as-you-type-and-applies-merges-as-remote-changes.md)'s
  editable merge view
- The same decision in FrostMail:
  [frostyard/frostmail's ADR-0063](https://github.com/frostyard/frostmail/blob/main/docs/adr/0063-make-the-nsgo-client-the-only-client.md);
  in PocketGo:
  [frostyard/pocketgo's ADR-0071](https://github.com/frostyard/pocketgo/blob/main/docs/adr/0071-make-the-nsgo-client-the-only-client.md)
