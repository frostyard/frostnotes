# Plan 0011: no desktop window

This plan takes Frostnotes to no desktop window
([ADR-0050](../adr/0050-frostnotes-has-no-desktop-window.md)): Obsidian
edits a desktop's vault folder, notesd in the Flatpak syncs it with the
hub, and Lodge serves the React app. It removes the MyGo client
([plan 0009](0009-mygo-desktop.md)) and the NSGo client
([ADR-0047](../adr/0047-use-the-shared-nsgo-editor-on-linux-and-lodge.md)).

Status (2026-11-25): the owner decided on ADR-0050 that day. No phase has
started.

## How the plan changed

Until 2026-11-25 this plan made the NSGo client the only client
([ADR-0049](../adr/0049-make-the-nsgo-client-the-only-client.md), now
superseded), as Frostnotes' part of
[frostyard/frostmail's plan 0017](https://github.com/frostyard/frostmail/blob/main/docs/plans/0017-nsgo-as-the-only-client.md).
A side-by-side read of both clients' code (at `0aab290`) found 122 things
the MyGo client did: the NSGo client had 7, had 32 in part, and lacked 82.
The owner answered 11 questions on what to keep (all of it), and 4 changes
were merged:

- #287 and #289: ADR-0049 and this plan's first form.
- #288: the editor's plain logic moved out of MyGo's packages, into
  `internal/desktop/markup` and `catalog`.
- #290 and #291: the NSGo client's source mode on NSGo's code editor,
  with completions, find, formatting commands and links.
- An NSGo issue,
  [nsgo-dev/nsgo#2409](https://github.com/nsgo-dev/nsgo/issues/2409),
  asks for what live preview needs from NSGo's editor.

A third change, tabs and splits, was stopped before it was pushed. The
same day the owner decided that Frostnotes needs no desktop window, and
that Lodge serves the React app again: it still has what the NSGo client
lacked, and its checks still run on main.

## Phase 1 — Lodge serves the React app (S)

Implements [design/app.md, In a browser](../design/app.md#in-a-browser)
and [specs/lodge-app.md](../specs/lodge-app.md).

- `make lodge-app` and `make web-build` stage the React app (today's
  `legacy-lodge-app`) as `/app/notes/`, and the `.deb` holds it.
- Lodge's evidence suite of the React app (`make m4-evidence`) runs on
  what the `.deb` installs, in place of the NSGo build's
  (`make nsgo-lodge-evidence`).
- **Done when:** the `.deb` of a build holds the React app and no NSGo
  build, and `make m4-evidence` passes on it.

## Phase 2 — The Flatpak carries notesd (M)

Amends [ADR-0031](../adr/0031-the-desktop-app-ships-as-a-flatpak-with-its-own-notesd.md);
implements [design/app.md, The Flatpak](../design/app.md#the-flatpak).

- The Flatpak builds notesd and notesctl only. Its launcher starts notesd
  on the same data directory and socket as before, and asks the
  Background portal to start it at login, the setting of
  [ADR-0033](../adr/0033-a-setting-keeps-notesd-running-without-a-window.md)
  being on by default and changed with a command, not a window.
- What a window did is a `notesctl` command: add a vault, pair the device,
  join the hub's vault, sync's state, why notesd is not running. Each
  missing one is added.
- The metainfo, its screenshot and the README describe notesd for a
  desktop's editor, Obsidian among them.
- **Done when:** a clean install of the Flatpak on a desktop, with
  `notesctl` alone, registers a folder, pairs with a hub, syncs a change
  made in another editor, and runs notesd again after a logout and a
  login; and an existing install keeps its vaults and pairing after the
  update.

## Phase 3 — The desktop clients are removed (M)

- `cmd/frostnotes`, `cmd/frostnotes-nsgo` and the packages of
  `internal/desktop/` that only the clients use go, with the Makefile's
  desktop and NSGo targets (`desktop-*`, `nsgo-*`), the typing gates, CI's
  jobs for them, and the docs that describe them.
- `go.mod` requires neither MyGo, its fork's `replace`, nor frostui's root
  module; the repository has no NSGo module; `make flatpak-sources`.
- **Done when:** `make ci` passes with neither client, and `go.mod` names
  none of these modules.

## Phase 4 — Release (S)

- **Done when:** the owner releases the version (`make bump`), and the
  Flatpak and the `.deb` of that tag are published.

## Later / ideas

- None.

## Open questions

- None.

## The owner's answers

Brian decided these on 2026-11-25, after the side-by-side read:

1. Frostnotes needs no desktop window: Obsidian edits, notesd syncs, and
   Lodge reads the hub.
2. A desktop's notesd stays in the Flatpak, with no window.
3. The web app stays as it is, without plan 0011's earlier groups.
4. Lodge serves the React app again, and both desktop clients go.

## References

- Implements: [ADR-0050](../adr/0050-frostnotes-has-no-desktop-window.md),
  [design/app.md](../design/app.md), [specs/lodge-app.md](../specs/lodge-app.md)
- Builds on: [plan 0006](0006-m4-notes-inside-lodge.md),
  [plan 0008](0008-m6-the-standalone-desktop-app.md),
  [plan 0009](0009-mygo-desktop.md)
