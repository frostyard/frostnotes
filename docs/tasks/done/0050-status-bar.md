---
id: "0050"
title: Draw the status bar's items
milestone: M3
size: S
touch:
  - app/src/features/status/StatusBar.tsx
given:
  - app/src/features/status/StatusBar.test.tsx
acceptance: make ui-vitest F=src/features/status/StatusBar.test.tsx
---
# T-0050: Draw the status bar's items

## Goal

The status bar says, at a glance, whether the vault is synced and has
conflicts, how many notes link to the active one, how long it is, and
which mode it is in (`docs/specs/ui.md`, Status bar). Draw those items;
the connection's state stays at the left while it is not ready. The
container gathers the numbers and opens the menus and panes.

## Read first

- `app/src/features/status/StatusBar.tsx`: `StatusSync`,
  `StatusCounts`, `EditingMode`, `StatusBarProps` and the stub, which
  draws the connection only.
- `docs/specs/ui.md`, "Status bar".
- `app/src/rpc/gen/api.ts`: `SyncState`.
- `app/src/features/chrome/IconButton.tsx`: the chrome's buttons.
- The given test `app/src/features/status/StatusBar.test.tsx`.
- `docs/tasks/EXECUTOR.md`, especially the TypeScript conventions.

## Contract

Keep every exported name and signature; replace the stub and the file's
"Task T-0050 …" sentence with a description of the bar.

**The bar:** the stub's `footer` with `role="status"` and its classes
(24 high, `bg-toolbar`, 1px `border-separator` top, `text-status`,
`text-secondary`, 12px side padding, items 16px apart). First a `span`
taking the remaining width, holding the connection's text: "Connecting
to notesd…" while connecting, "Reconnecting to notesd… (<reason>)" when
lost, nothing when ready. Then, in this order, each only when its prop
is not null:

1. **Sync** (`sync`): a `span` with `role="img"`, `aria-label` and
   `title` both `sync.text`, and `data-sync-state` the state, holding a
   14px Lucide icon: `Cloud` for idle, `RefreshCw` with the class
   `animate-spin` for syncing, `CloudOff` for offline, `CircleAlert` for
   revoked and reset. Then, when `sync.conflicts` is above 0, a
   `button` "N conflicts" ("1 conflict") calling `onConflicts`.
2. **Backlinks** (`backlinks`): a `button` "N backlinks" ("1
   backlink") calling `onBacklinks`.
3. **Counts** (`counts`): two `span`s, "N words" and "N characters"
   ("1 word", "1 character").
4. **Editing mode** (`mode`): a `button` with `aria-haspopup="menu"`,
   `aria-label` "Editing mode: Live Preview", "Editing mode: Source
   mode" or "Editing mode: Reading view", `title` the mode's name
   without the prefix, holding a 14px Lucide icon: `Pencil` for live,
   `CodeXml` for source, `BookOpen` for read. A click calls
   `onMode(left, top)` with the button's `getBoundingClientRect()`.

Numbers are grouped in thousands as en-US writes them (`1,804`). The
buttons are text-sized with 4px side padding, rounded 4, hover
`bg-selection-inactive` and `text-primary`. No other text: the bar's
whole `textContent` is the items' texts in order (the icons have none).

## Tests (given, do not edit)

`app/src/features/status/StatusBar.test.tsx`: the connection's text,
the sync icon and its states, conflicts, backlinks, singulars, grouped
counts, the items' order, and the editing mode's names and menu.

## Gotchas

- `StatusBarProps` imports `Connection` from `data/stores` as a type;
  keep it a type-only import.
- A Lucide icon passes `className` to its `svg`, which happy-dom reports
  through `getAttribute("class")`.
- The bar is presentational: no stores, no client (`docs/tasks/EXECUTOR.md`).

## Out of scope

Computing the counts and the backlinks, the menus and panes the buttons
open (the planner's), and every file except
`app/src/features/status/StatusBar.tsx`.

## Done when

`make accept T=0050`, `make check` and `make ui-check` pass, and only
`app/src/features/status/StatusBar.tsx` changed.
