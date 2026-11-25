# 0050 — Frostnotes has no desktop window

- **Status:** Accepted (Brian, 2026-11-25)
- **Date:** 2026-11-25

## Context

A desktop vault is a folder of Markdown files
([ADR-0003](0003-plain-markdown-files-are-the-source-of-truth.md)). notesd
on that machine syncs the folder with the hub, and the hub merges
([ADR-0005](0005-sync-through-a-hub-with-revisions-and-three-way-merges.md)).
Lodge serves a web app over the hub's notesd
([ADR-0026](0026-notesd-runs-as-a-lodge-app.md)). Any editor that writes the
folder takes part in sync: notesd's watcher sees its writes and merges
them like any other writer's.

Frostnotes had 2 desktop windows on 2 toolkits: the MyGo client
([ADR-0038](0038-draw-the-desktop-app-natively-with-mygo-and-frostui.md))
and the NSGo client, whose web build is Lodge's app since
[ADR-0047](0047-use-the-shared-nsgo-editor-on-linux-and-lodge.md).
[ADR-0049](0049-make-the-nsgo-client-the-only-client.md) planned to give
the NSGo client everything the MyGo client has: a side-by-side read found
82 things it lacked and 32 it had in part.

The owner edits his vaults in Obsidian on his desktops. On 2026-11-25 he
decided that Frostnotes needs no desktop window: Obsidian edits a local
folder, notesd on the machine syncs it with the hub, and Lodge's web app
reads the hub.

The React app (`app/`, CodeMirror 6 live preview, tabs and splits,
history and the merge view, sync conflicts, the side panes, the phone
layout) was Lodge's app until 2026-11-23. It is still built and tested on
main (`make ui-check` in CI), and `make legacy-lodge-app` still builds it
for Lodge.

## Decision

1. **Frostnotes has no desktop window.** A desktop edits its vaults with
   any editor of Markdown files, Obsidian among them.
2. **Lodge serves the React app again**, in the NSGo web build's place.
3. **The Flatpak `org.frostyard.Frostnotes` carries notesd and notesctl
   only.** It runs notesd at login through the Background portal, on the
   same data directory and socket as before, so an installed vault, its
   device token and its pairing stay. Vaults are added, paired and
   joined with `notesctl`.
4. **The MyGo client and the NSGo client are removed,** the NSGo web build
   with them. The root `go.mod` then requires neither MyGo, its fork's
   `replace`, nor frostui's root module, and the repository has no NSGo
   module.

This supersedes ADR-0049, ADR-0047, and ADR-0038's choice of a native
desktop app, and amends
[ADR-0031](0031-the-desktop-app-ships-as-a-flatpak-with-its-own-notesd.md):
the Flatpak holds no window.

## Consequences

- One UI, the React app, for browsers and phones through Lodge. It keeps
  Lodge's look (`@frostyard/lodge-ui`,
  [ADR-0030](0030-the-look-comes-from-lodges-ui-package.md)), not the
  Frostyard look that FrostMail and PocketGo share through frostui's
  `workspaceui`.
- Obsidian's own features (its plugins, its graph, its settings) are the
  desktop's. notesd ignores `.obsidian/` as it ignores every other
  dot-directory ([design/vault.md](../design/vault.md)), so each desktop
  keeps its own Obsidian settings and plugins, unsynced. notesd still
  reads the daily notes', templates', links' and property types' settings
  from it where `.frostnotes/` sets none.
- A conflict between two devices is settled in the React app (its merge
  view) or by editing the file: notesd keeps the conflict versions either
  way.
- Setting up a desktop is a command line: `flatpak run
  --command=notesctl org.frostyard.Frostnotes` adds a vault, pairs the
  device and joins the hub's vault. No form explains an error, so
  notesctl's messages carry that.
- The work merged for ADR-0049 goes with the NSGo client: source mode on
  NSGo's code editor, its completions, find, formatting commands and links
  (#290, #291). The UI-free packages it built or moved
  (`internal/desktop/markup`, `catalog`; #288) go with the clients unless
  notesd or notesctl uses them.
- The desktop's typing gate (`make desktop-bench`, `make nsgo-bench`,
  [ADR-0043](0043-the-typing-gate-is-looser-on-ci.md)) goes with the
  desktop editors.
- Frostnotes departs from frostyard/frostmail's
  [plan 0017](https://github.com/frostyard/frostmail/blob/main/docs/plans/0017-nsgo-as-the-only-client.md),
  in which every app's NSGo client is its only client.

## Alternatives considered

- **ADR-0049, the NSGo client with all of the MyGo client's features:**
  months of work for a window the owner does not use.
- **Keep the NSGo web build in Lodge:** it has a fraction of the React
  app's features, and the React app is still tested.
- **A notesd package with a systemd user unit instead of the Flatpak:**
  existing installs would need moving, and the Flatpak already runs
  notesd at login.

## References

- Shapes: [plan 0011: no desktop window](../plans/0011-nsgo-only-client.md),
  [design/app.md](../design/app.md)
- Builds on: [ADR-0026](0026-notesd-runs-as-a-lodge-app.md),
  [ADR-0031](0031-the-desktop-app-ships-as-a-flatpak-with-its-own-notesd.md),
  [ADR-0033](0033-a-setting-keeps-notesd-running-without-a-window.md)
- Supersedes: [ADR-0049](0049-make-the-nsgo-client-the-only-client.md),
  [ADR-0047](0047-use-the-shared-nsgo-editor-on-linux-and-lodge.md),
  [ADR-0038](0038-draw-the-desktop-app-natively-with-mygo-and-frostui.md)'s
  native desktop app
