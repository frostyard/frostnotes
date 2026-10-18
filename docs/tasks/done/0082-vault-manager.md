---
id: "0082"
title: Build the vault manager
milestone: M3
size: M
touch:
  - app/src/features/vaults/VaultManager.tsx
given:
  - app/src/features/vaults/VaultManager.test.tsx
acceptance: make ui-vitest F=src/features/vaults/VaultManager.test.tsx
---
# T-0082: Build the vault manager

## Goal

The app opens and creates vaults from Obsidian's vault chooser
(`docs/specs/ui.md`, Vault manager; ADR-0024): the vaults listed
at the left, Create new vault, Open folder as vault and how to join a
hub at the right. Without a vault it fills the window; otherwise it is
a dialog over it. Build the component; the container does what it asks.

## Read first

- `app/src/features/vaults/VaultManager.tsx`: the types and the stub.
- `docs/specs/ui.md`, "Vault manager".
- `app/src/features/settings/SettingsDialog.tsx`: a dialog frame with a
  backdrop, a side list and a Close button, in the app's look.
- `app/src/features/menu/Menu.tsx`: the menu a row's options open.
- The given test `app/src/features/vaults/VaultManager.test.tsx`.
- `docs/tasks/EXECUTOR.md`, especially the TypeScript conventions.

## Contract

Keep every exported name and signature; replace the stub and the file's
"Task T-0082 …" sentence with a description.

- **The frame:** an element with `role="dialog"` and
  `aria-label="Manage vaults"`, holding the vault list at the left
  (280 wide, `bg-sidebar`) and, at the right, `panel` when given, else
  the actions.
  - `mode` "dialog": `aria-modal="true"`, the spec's 720 × 480 frame
    inside a `fixed inset-0 z-50` backdrop (`bg-backdrop`); a Close
    button (`aria-label` and `title` "Close", lucide `X`) at the top
    right. `onClose()` on Close, on Escape (a `keydown` on the frame;
    stop its propagation), and on a `pointerdown` on the backdrop itself,
    not inside the frame.
  - `mode` "window": the frame fills its parent (`flex-1`), not fixed,
    with no backdrop and no Close, and Escape does nothing.
- **The vault list:** a `ul` named "Vaults" (`aria-label`), one `li`
  per vault in the order given. In each, a `button` showing the name
  (13/18 semibold) over the path (12/16 `text-secondary`), both
  truncated; its text is the name then the path. The window's vault
  (`current`) has `aria-current="true"` and `bg-selection-sidebar`.
  A click calls `onOpen(id)`. Beside it, an options button named
  `Options for <name>` (`aria-label` and `title`, lucide `Ellipsis`,
  shown on the row's hover or its own focus) opens a `Menu` labelled
  `Options for <name>` with one item, "Remove from list", which calls
  `onRemove(vault)`. A `contextmenu` on the row opens the same menu at
  the pointer (prevent its default). With no vaults, the list is a
  centered "No vaults yet" instead of the `ul`.
- **The actions:** an `h2` "Frostnotes", then three `section`s, each
  named by its title (`aria-label`): an `h3` title over a description,
  and the control at the right.
  - "Create new vault", "Make a new vault in a folder you choose.",
    a Create button (accent) calling `onCreate()`.
  - "Open folder as vault", "Choose a folder of Markdown files.", an
    Open button. With `canPick`, Open calls `onOpenFolder()` with no
    argument. Without, a text input named "Folder path" comes before
    Open; Open, or Enter in the input, calls `onOpenFolder(path)` with
    the trimmed path, and nothing while it is blank.
  - "Join a vault on a hub": the text "Join it with", a `code` element
    holding exactly `notesctl hub join URL NAME`, and ", then open its
    folder here."; no control.

## Tests (given, do not edit)

`app/src/features/vaults/VaultManager.test.tsx`: the rows and the
current one; opening; Remove from list from the options and from the
context menu; no vaults; the three actions; a typed folder; the panel;
closing the dialog three ways; the window mode.

## Gotchas

- Presentational: no stores, no client, no picker
  (`docs/tasks/EXECUTOR.md`). The container sorts the vaults and
  confirms Remove.
- The options button sits beside the row's button, not inside it (a
  button cannot hold a button).
- The given test renders the real `Menu`.

## Out of scope

The create form (T-0083, given as `panel`), what opening, creating and
removing do, and every file except the one under `touch`.

## Done when

`make accept T=0082`, `make check` and `make ui-check` pass, and only
`app/src/features/vaults/VaultManager.tsx` changed.
