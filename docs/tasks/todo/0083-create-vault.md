---
id: "0083"
title: Build the vault manager's create form
milestone: M3
size: S
touch:
  - app/src/lib/vaultName.ts
  - app/src/features/vaults/CreateVault.tsx
given:
  - app/src/features/vaults/CreateVault.test.tsx
acceptance: make ui-vitest F=src/features/vaults/CreateVault.test.tsx
---
# T-0083: Build the vault manager's create form

## Goal

Create new vault asks for a name and a folder to make the vault in, as
Obsidian's does (`docs/specs/ui.md`, Vault manager;
ADR-0024). Build the name's check as pure functions and the form
that the vault manager shows in its actions' place.

## Read first

- `app/src/lib/vaultName.ts` and `app/src/features/vaults/CreateVault.tsx`:
  the types and the stubs.
- `docs/specs/ui.md`, "Vault manager", the create form.
- `app/src/features/dialog/ConfirmDialog.tsx`: the app's buttons
  (`h-7 rounded-md`, the accent one `bg-accent text-accent-contrast`).
- The given test `app/src/features/vaults/CreateVault.test.tsx`.
- `docs/tasks/EXECUTOR.md`, especially the TypeScript conventions.

## Contract

Keep every exported name and signature; replace the stubs and each
file's "Task T-0083 …" sentence with a description.

- **`vaultNameError(name)`**, on the trimmed name: `"A vault needs a
  name."` when empty; `"A vault's name cannot start with a dot."` when it
  starts with `.`; `'A vault\'s name cannot hold any of these: \ / : * ?
  " < > | # ^ [ ]'` when `VAULT_NAME_FORBIDDEN` matches; else `null`.
- **`vaultFolder(location, name)`**: `location` without trailing `/`,
  then `/`, then the trimmed name (`"/"` and `"Garden"` give
  `"/Garden"`).
- **`CreateVault`** keeps the typed name in state and draws a `section`
  named "Create new vault" (`aria-label`):
  - a Back button (`aria-label` and `title` "Back", lucide
    `ArrowLeft`) calling `onBack()`, and an `h2` "Create new vault";
  - a "Vault name" text input (a `label` for it), focused when the
    form opens. Once the name has been edited, its error, if any,
    shows under it (12/16 `text-flag-1`) and the input has
    `aria-invalid="true"`; before any edit, no error shows. Enter in
    it creates, as Create does;
  - "Location": with `canPick`, the location's path (or `"No folder
    chosen"` while it is null; truncated, 12/16 `text-secondary`) and a
    Browse button calling `onBrowse()`; without, a text input named
    "Location" (`aria-label`) showing `location ?? ""` whose changes
    call `onLocation(value)`;
  - a Create button (accent), disabled unless `vaultNameError(name)` is
    null and `location` is a non-blank string; it calls
    `onCreate(name.trim())`.

## Tests (given, do not edit)

`app/src/features/vaults/CreateVault.test.tsx`: names taken and refused
with each message, `vaultFolder`; the form, its focus, creating once
both are good, Enter, the error after an edit, Browse, a typed folder,
Back.

## Gotchas

- The form is presentational: the container opens the picker and holds
  the location (`docs/tasks/EXECUTOR.md`).
- No error before the name is edited, but Create stays disabled.
- `lib/` imports nothing from `features/`.

## Out of scope

Making the vault, the vault manager (T-0082), and every file except
the two under `touch`.

## Done when

`make accept T=0083`, `make check` and `make ui-check` pass, and only
the two files under `touch` changed.
