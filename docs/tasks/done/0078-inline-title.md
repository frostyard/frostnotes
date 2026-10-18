---
id: "0078"
title: Draw the inline title
milestone: M3
size: S
touch:
  - app/src/features/workspace/InlineTitle.tsx
given:
  - app/src/features/workspace/InlineTitle.test.tsx
acceptance: make ui-vitest F=src/features/workspace/InlineTitle.test.tsx
---
# T-0078: Draw the inline title

## Goal

The Show inline title preference shows a note's file name above it in
the H1 style, editable (`docs/specs/ui.md`, Editor and reading view):
Enter or leaving it renames the file, Escape restores it. Draw it; the
container renames.

## Read first

- `app/src/features/workspace/InlineTitle.tsx`: the types and stub.
- `docs/specs/ui.md`, "Editor and reading view", Inline title, and the
  heading sizes in `app/src/editor/theme.ts` (`.cm-pv-h1`).
- The given test.

## Contract

Keep every exported name and signature; replace the file's "Task
T-0078 …" sentence with a description.

- A text field (`aria-label="Title"`, no spellcheck) holding `title`,
  in the H1 style (1.618em, bold, line height 1.2, `text-primary`, no
  border or background), full width, 0.5em above what follows. A new
  `title` prop replaces its text.
- **Enter** leaves the field (which commits) and calls `onDone()`.
  **Escape** restores the text, leaves without committing and calls
  `onDone()`.
- **Committing** (leaving the field): the text trimmed; blank or equal
  to `title` restores it and renames nothing; holding any of
  `\ / : * ? " < > | # ^ [ ]` (export `FORBIDDEN`) restores it, renames
  nothing, and shows below the field a `p` with `role="alert"` saying a
  file name cannot contain these characters; anything else calls
  `onRename(name)`. The next edit takes the alert away.

## Tests (given, do not edit)

`app/src/features/workspace/InlineTitle.test.tsx`: renaming on Enter
and blur, Escape, blank and unchanged names, forbidden characters, a new
title.

## Out of scope

Renaming the file and placing the title (the planner's), and every file
except `app/src/features/workspace/InlineTitle.tsx`.

## Done when

`make accept T=0078`, `make check` and `make ui-check` pass, and only
`app/src/features/workspace/InlineTitle.tsx` changed.
