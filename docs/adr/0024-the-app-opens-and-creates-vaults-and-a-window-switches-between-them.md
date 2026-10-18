# 0024 — The app opens and creates vaults, and a window switches between them

- **Status:** Accepted
- **Date:** 2026-10-18

## Context

M3 ends with the user writing in Frostnotes instead of Obsidian
([plan 0005](../plans/0005-m3-desktop-app.md), evidence 4). Today only
`notesctl vault add` registers a vault. With none, the window says to
run it, and the app can open no folder of its own. Obsidian starts at a
vault chooser: the vaults it knows, Create new vault and Open folder as
vault. Its vault menu's Manage vaults leads back there.

notesd owns every vault ([overview](../design/overview.md)). `vault.add`
and `vault.remove` belong to the vault domain, whose callers are local
ones, the app among them. The page already reaches both over the bridge.

- `vault.add` registers an existing directory that neither lies inside
  a vault nor holds one.
- `vault.remove` forgets a vault. Its index, version history and sync
  state go with it, by the store's cascades. Its files stay.

The shell has no file dialog, and a page cannot learn a folder's path
by itself: `<input type=file webkitdirectory>` hands it files, not a
path. Frostmail's shell uses `tauri-plugin-dialog` to pick files.

Obsidian opens each vault in a window of its own. The app has one
window; more windows are later work in plan 0005. Its design already
says a window shows one vault, which the vault row switches in place,
and that the app reopens the last one. Each vault keeps its own
workspace on the device
([ADR-0021](0021-keep-the-workspace-and-app-preferences-on-each-device.md)).
No ADR has recorded that.

## Decision

- **The app registers, creates and forgets vaults through notesd's
  vault domain,** as notesctl does:
  - Open folder as vault is `vault.add`.
  - Create new vault is `vault.add` with `create`, an additive field.
    notesd makes the folder first; its parent must exist and the folder
    must not.
  - Remove from list is `vault.remove`.

  notesd stays the only writer of what a vault holds; the shell makes
  no folder.
- **A folder comes from the system's folder picker:** the open dialog
  of `tauri-plugin-dialog`, choosing folders. The window's capability
  grants that alone (`dialog:allow-open`). The picker is how a person
  names a folder, not a security boundary: a page that could run script
  already reaches `vault.add` with any path. In a browser, which has no
  picker, the path is typed.
- **A window shows one vault and switches in place.** Choosing a vault,
  opening a folder or creating one makes it the window's vault, and the
  app reopens the last one on start. When the app has more windows,
  Open in new window joins the vault list. Until then, the vault row
  and the window's title (`<note> - <vault> - Frostnotes`) name the
  vault the window shows.
- **Without a vault, the window is the vault manager,** Obsidian's
  start screen. With vaults, the vault menu's Manage vaults opens the
  same manager over the window.
- **Remove from list asks first, and says what goes:**
  - The folder and its files stay.
  - notesd's index and version history for it go, and for a synced
    vault, its place on the hub, as `vault.remove` already does.

  The window then shows another vault, or the manager.
- **`vault.add`'s errors show as notices in the person's words:** a
  folder inside a vault, a vault inside the folder, not a folder, a
  folder that already exists.

## Consequences

- The user can begin with the app alone. notesctl is still needed to
  join a hub, which the vault manager says how to do.
- A new dependency, `tauri-plugin-dialog` (crate and package), pinned
  like the others. It brings `tauri-plugin-fs`'s crate, which the app
  does not register. The window's capability gains one permission.
- `vault.add` gains a field, additive within protocol 1, and
  `notesctl vault add` gains `--create`.
- Two vaults are never side by side, which Obsidian allows. Until more
  windows come, someone who keeps two vaults open switches between them.
- Removing a synced vault loses its history on this device, and adding
  it again means joining the hub again. The confirmation says so.

## Alternatives considered

- **The shell makes the new folder** with a command of its own: a
  second writer outside notesd, whose checks would have to keep step
  with notesd's.
- **A path typed into a field, with no dialog plugin:** no dependency,
  but not how people pick folders on a desktop, and Obsidian's buttons
  open the picker.
- **A window per vault, as Obsidian does:** needs the more-windows
  work (per-window sessions, the single-instance hand-over, per-window
  workspaces). Switching in place now does not rule it out later.
- **Leave registering to notesctl:** the trial would start in a
  terminal, and a new user could not start at all.

## References

- Shapes: [design/app.md](../design/app.md#data) (one vault per
  window), [specs/ui.md](../specs/ui.md#sidebars-and-panes) (the vault
  row; the vault manager), [specs/rpc-api.md](../specs/rpc-api.md)
  (`vault.add`), [plan 0005, Phase 8](../plans/0005-m3-desktop-app.md#phase-8--vaults-from-the-app)
- Builds on: [ADR-0003](0003-plain-markdown-files-are-the-source-of-truth.md),
  [ADR-0011](0011-look-like-frostmails-sibling-with-obsidians-layout.md),
  [ADR-0021](0021-keep-the-workspace-and-app-preferences-on-each-device.md)
