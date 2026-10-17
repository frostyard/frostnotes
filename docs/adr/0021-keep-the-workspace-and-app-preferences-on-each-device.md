# 0021 — Keep the workspace and app preferences on each device

- **Status:** Proposed
- **Date:** 2026-10-17

## Context

M3's desktop app has three kinds of state besides the notes:

- **The workspace:** splits and tab groups, each tab's note, mode, scroll
  position and back and forward history, the sidebars' panes and widths,
  and the recently opened files the quick switcher lists first. It
  changes with every click.
- **App preferences:** theme, font size, readable line length,
  spellcheck, the default view and editing mode, how properties show.
  Only the app reads them; nothing notesd does depends on them.
- **Vault settings:** the daily-note folder, format and template, the
  template folder, where new notes and attachments go, the link format,
  whether moves update links, ignore patterns. notesd uses them, and M1
  reads them from `.frostnotes/vault.yaml`, else from `.obsidian/`
  ([design/vault.md](../design/vault.md#on-disk)), but cannot yet write
  them.

Obsidian keeps all three in the vault (`.obsidian/workspace.json`,
`app.json`, `appearance.json`), so the workspace file churns on every tab
switch and two machines opening one vault overwrite each other's layout,
as the user's two machines do over SMB today. notesd's own state never
lives in the vault
([ADR-0003](0003-plain-markdown-files-are-the-source-of-truth.md)).
Frostmail keeps view state in the app and its preferences in maild,
because some of them change what maild does with the app closed
(frostmail ADR-0026). The web app (M4) runs the same UI in browsers on
other devices, a phone among them, where a desktop's layout makes no
sense. The user's `app.json` and `appearance.json` are empty: they use
Obsidian's defaults.

## Decision

- **The workspace belongs to the device and the vault, and the app keeps
  it** in its local storage (WebKitGTK's on the desktop, the browser's in
  M4), one versioned JSON document per vault, keyed by the vault's path
  (an ID can be reused when notesd's database is made anew; a browser's
  storage is already per origin). The app writes it 500 ms after the last
  change and when the window closes. A document that fails validation, or
  has an unknown version, is replaced by the default layout, which is
  Obsidian's: Files and Search on the left; Backlinks, Outgoing links,
  Tags, All properties and Outline on the right, collapsed; one empty tab.
- **The workspace is a tree** of splits (a row or a column, with sizes)
  whose leaves are tab groups. A tab holds a view (a note in editing or
  reading mode, an empty tab, a file's history, a merge, an attachment)
  and its own back and forward history. Each sidebar is a tab group of
  panes with a width and an open flag. One tab is active. The last 50
  opened files are kept for the quick switcher.
- **Tabs follow the vault.** The app applies `note.moved` and
  `note.removed` to every saved workspace as they arrive, and keeps the
  last durable event's sequence number; at start it subscribes from that
  number, so moves made while it was closed (by sync, `notesctl` or an
  agent) reach its tabs. When notesd answers `resync`, paths stay as they
  were, and a tab whose file is gone says so.
- **App preferences live beside the workspace,** per device, one
  versioned record whose defaults are Obsidian's.
- **Vault settings stay in `.frostnotes/vault.yaml`,** now written:
  `vault.settings` returns the effective settings and where each came
  from (`.frostnotes/`, `.obsidian/`, or the default), and
  `vault.configure` changes them, writing `vault.yaml` atomically. The
  settings M3 adds (new note location, attachment folder, link format,
  wikilinks or markdown links, updating links on move) are read from
  `.obsidian/app.json` as defaults, as M1 reads the daily-note and
  template settings; excluded files are M1's ignore patterns.

## Consequences

- No API, schema or migration for layout or preferences, and none of
  them churns a file in the vault or a revision in the hub's log.
- A phone and a desktop keep layouts of their own.
- Clearing the app's site data resets its layout and preferences; no note
  or setting notesd uses is lost.
- Preferences do not follow the user to the second machine, nor do vault
  settings until `.frostnotes/` syncs (plan 0004's later work); both
  machines start from Obsidian's defaults, which are the user's.
- The app must replay events since its last sequence number at start, and
  notesd must keep the changes log long enough to make that useful (it
  keeps all of it today).
- The workspace's validator is the planner's and must drop what it does
  not understand rather than fail, so an older or newer app never wedges
  the window.

## Alternatives considered

- **In the vault, as Obsidian does (`.frostnotes/workspace.json`):** a
  write on every tab switch, two machines fighting over one file, and
  notesd's state in the vault.
- **In notesd, as frostmail's preferences:** an API change and a
  migration per preference for state no daemon behavior needs, and a hub
  would serve one layout to every browser.
- **Syncing preferences through the hub now:** waits for `.frostnotes/`
  to sync, which M2 left for later.

## References

- Shapes: [design/app.md](../design/app.md#workspace), [specs/ui.md](../specs/ui.md), [design/vault.md](../design/vault.md#on-disk) (vault settings written)
- Builds on: [ADR-0003](0003-plain-markdown-files-are-the-source-of-truth.md), [ADR-0011](0011-look-like-frostmails-sibling-with-obsidians-layout.md)
- Built in: [plan 0005 — M3](../plans/0005-m3-desktop-app.md)
