---
id: "0042"
title: Draw a tab group's strip
milestone: M3
size: M
touch:
  - app/src/features/workspace/TabStrip.tsx
given:
  - app/src/features/workspace/TabStrip.test.tsx
acceptance: make ui-vitest F=src/features/workspace/TabStrip.test.tsx
---
# T-0042: Draw a tab group's strip

## Goal

Every tab group in the workspace has a strip of tabs, and the strips
along the window's top are its title bar (`docs/specs/ui.md`, Tab
strip). Draw the strip as a presentational component: its tabs, their
close buttons or pins, the New tab and Tab list buttons, the pointer and
the keys, and the commands it sends. The workspace's containers decide
what each command does, open the menus and handle drops.

## Read first

- `app/src/features/workspace/TabStrip.tsx`: `TabItem`, `TabCommand`,
  `TabStripProps` and the stub.
- `docs/specs/ui.md`, "Layout" (buttons) and "Tab strip".
- `app/src/features/chrome/TitleBar.tsx`: how the title bar draws its
  buttons and drag region; match its look.
- `app/src/styles/app.css`: the colors and the `text-tab` type role.
- The given test `app/src/features/workspace/TabStrip.test.tsx`.
- `docs/tasks/EXECUTOR.md`, especially the TypeScript conventions.

## Contract

Keep every exported name and signature; replace the stub and the file's
"Task T-0042 …" sentence with a description of the strip.

**The strip:** a `div` with `role="tablist"` and `aria-label={label}`,
44 high (`h-11`), `bg-toolbar`, 8px side padding, the tabs 2px apart, as
`flex items-center`. In order: the tabs, the New tab button, an empty
`div` that takes the remaining width, the Tab list button. When
`dragRegion` is set, the tablist and that empty `div` carry
`data-tauri-drag-region`; no tab or button ever does, and without
`dragRegion` nothing does.

**A tab** (one per `tabs` entry, in order): a `div` with `role="tab"`,
`aria-label={title}`, `aria-selected={active}`, `tabIndex` 0 when active
and -1 otherwise, `title={title}`, `data-tab-id={id}`, `draggable`, and
the class `group`. It is 30 high, rounded 6 (`rounded-md`), 10px side
padding, 6px gap, at least 80 and at most 220 wide, shrinking with its
siblings (`flex: 0 1 220px`); `text-tab`. Inside, the title in a `span`
that truncates, then:

- unpinned: a `button` named `Close <title>` (`aria-label`),
  `tabIndex={-1}`, 16 × 16, rounded 4, holding a 12px Lucide `X`;
  `opacity-100` on the active tab, else `opacity-0` and
  `group-hover:opacity-100`;
- pinned: a 12px Lucide `Pin` with `role="img"` and `aria-label="Pinned"`
  instead.

Colors: an active tab has `bg-window text-primary`, plus `font-medium`
while `focused`; an inactive one `text-secondary` and
`hover:bg-selection-inactive`.

**Buttons**, 28 × 28, rounded 6, `text-secondary`, hover
`bg-selection-inactive`, with 16px Lucide icons:

- New tab: `aria-label="New tab"`, `title="New tab (Ctrl + T)"`, `Plus`;
  a click sends `{ kind: "newTab" }`;
- Tab list: `aria-label="Tab list"`, `aria-haspopup="menu"`,
  `title="Tab list"`, `ChevronDown`; a click sends `{ kind: "list", x, y
  }` with the button's `getBoundingClientRect()` `left` and `bottom`.

**Pointer:**

- a click on a tab sends `{ kind: "activate", id }`, even when it is
  already active;
- a click on its Close button sends `{ kind: "close", id }` and nothing
  else (stop the click from reaching the tab);
- a middle click (`onAuxClick` with `button` 1) sends `close`, unless the
  tab is pinned; other buttons do nothing;
- a right click (`onContextMenu`) prevents the default and sends `{
  kind: "menu", id, x: clientX, y: clientY }`;
- dragging a tab calls `onDragStart(id, event)`.

**Keys** on a tab (`onKeyDown`):

- ArrowLeft, ArrowRight, Home and End: prevent the default and stop the
  event's propagation (the window's keymap must not see them); then send
  `activate` for the previous, next, first or last tab, but only when
  that is another tab (nothing past either end);
- after a key sends `activate`, move focus to that tab once the strip is
  rendered with it active (keep the id in a ref; focus it in an effect
  when `tabs` shows it active);
- Delete sends `close`, unless the tab is pinned;
- the Menu key (`"ContextMenu"`) and Shift+F10 prevent the default and
  send `menu` with the tab's `getBoundingClientRect()` `left` and
  `bottom`; F10 without Shift does nothing.

## Tests (given, do not edit)

`app/src/features/workspace/TabStrip.test.tsx`: roles, names and
attributes, styling of the active tab, close buttons and pins, clicks,
middle clicks, the context menu, New tab and Tab list, the keys and
their focus, dragging, and the drag region.

## Gotchas

- A tab's accessible name comes from `aria-label`, so its Close button's
  name does not join it.
- happy-dom lays nothing out: `getBoundingClientRect()` gives zeros
  there, so the tests check only that `x` and `y` are numbers for the
  keyboard menu and the tab list.
- The strip is presentational: no stores, no client, no workspace
  imports (`docs/tasks/EXECUTOR.md`).

## Out of scope

The menus themselves, what the commands do, drop targets and reordering
(the planner's workspace containers), placing strips in the title bar,
and every file except `app/src/features/workspace/TabStrip.tsx`.

## Done when

`make accept T=0042`, `make check` and `make ui-check` pass, and only
`app/src/features/workspace/TabStrip.tsx` changed.
