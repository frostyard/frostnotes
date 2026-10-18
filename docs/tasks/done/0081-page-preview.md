---
id: "0081"
title: Draw a link's page preview
milestone: M3
size: S
touch:
  - app/src/features/preview/PagePreview.tsx
given:
  - app/src/features/preview/PagePreview.test.tsx
acceptance: make ui-vitest F=src/features/preview/PagePreview.test.tsx
---
# T-0081: Draw a link's page preview

## Goal

Holding Ctrl while hovering a link shows the note it leads to in a small
box (`docs/specs/ui.md`, Keyboard map, Mouse), as Obsidian's page
preview does. Draw the box; the container decides when it shows and
fills it with the rendered note.

## Read first

- `app/src/features/preview/PagePreview.tsx`: the types and stubs.
- `app/src/features/menu/Menu.tsx`: a popover with the app's look
  (`bg-window`, `border-separator`, `shadow-popover`, rounded 8).
- The given test.

## Contract

Keep every exported name and signature; replace the file's "Task
T-0081 …" sentence with a description.

- **`previewPosition(anchor, width, height)`**: `left` is the link's
  left, kept at least 8px inside the window's left and right edges for a
  box `PREVIEW_WIDTH` wide; `top` is 4px under the link when a box
  `PREVIEW_HEIGHT` high fits there with 8px to the window's bottom, else
  4px above the link's top, never above 8.
- **`PagePreview`**: a `div` with `role="dialog"` and
  `aria-label="Preview of <title>"`, fixed at `previewPosition(anchor,
  window.innerWidth, window.innerHeight)`, `width` `PREVIEW_WIDTH` and
  `maxHeight` `PREVIEW_HEIGHT` (inline styles, in px), above other
  content (`z-50`), with the popover look. First a `button` holding the
  title (13/16 semibold, truncated, a bottom border) whose click calls
  `onOpen()`; then a body `div` of class `rendered` that scrolls, which
  is given to `fill(el)` once laid out (`useLayoutEffect`), and again if
  `fill` changes.
- **Closing:** `onClose()` on Escape (a `keydown` on `window`), on a
  `mousedown` anywhere outside the box (on `document`), and when the
  pointer leaves the box. Listeners go when the box does.

## Tests (given, do not edit)

`app/src/features/preview/PagePreview.test.tsx`: positions, the title
and body, the inline place and size, and closing.

## Out of scope

When the preview shows and what fills it (the planner's), and every file
except `app/src/features/preview/PagePreview.tsx`.

## Done when

`make accept T=0081`, `make check` and `make ui-check` pass, and only
`app/src/features/preview/PagePreview.tsx` changed.
