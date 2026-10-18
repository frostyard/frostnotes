---
id: "0048"
title: Draw the window's notices
milestone: M3
size: S
touch:
  - app/src/features/notice/Notices.tsx
given:
  - app/src/features/notice/Notices.test.tsx
acceptance: make ui-vitest F=src/features/notice/Notices.test.tsx
---
# T-0048: Draw the window's notices

## Goal

The window tells the user what just happened, "Moved Garden plan to
Projects.", "Copied the path.", in short notices at its top right that
leave on their own (`docs/specs/ui.md`, Notices). Draw the stack as a
presentational component with its timers; a container keeps the list
and removes a notice when asked.

## Read first

- `app/src/features/notice/Notices.tsx`: `NoticeItem`, `NoticesProps`,
  the two durations and the stub.
- `docs/specs/ui.md`, "Notices".
- `app/src/styles/app.css`: the colors and `shadow-popover`.
- The given test `app/src/features/notice/Notices.test.tsx`.
- `docs/tasks/EXECUTOR.md`, especially the TypeScript conventions.

## Contract

Keep every exported name, value and signature; replace the stub and the
file's "Task T-0048 …" sentence with a description of the stack.

**The stack:** a `div`, `fixed`, 52px from the top (8 below the 44-high
title bar row: `top-[52px]`), 8px from the right (`right-2`), 320 wide
at most (and at most the window less 16px), a column 8px apart, `z-40`;
it takes no pointer events itself (`pointer-events-none`), its notices
do (`pointer-events-auto`). The notices in `notices`' order, the first
at the top.

**A notice:** a `div` with `role="status"`, `bg-toolbar`, 1px
`border-separator`, rounded 8 (`rounded-lg`), `shadow-popover`, 12/16
text, padding 10 12 (`px-3 py-2.5`), as a row 12px apart: its `text`
(selectable, taking the remaining width), then its action, if any, as a
`button` labeled `action.label`, 12/16 `text-accent`, no background.

**Leaving:**

- Each notice is dismissed (`onDismiss(id)`) `NOTICE_MS` after it first
  shows, or `NOTICE_ACTION_MS` with an action. Time each notice from the
  render in which its `id` first appears; later renders, and other
  notices coming and going, do not restart it. A notice that leaves
  `notices` before its time has its timer cleared; if the same `id`
  shows again later, its time starts again.
- A click on a notice dismisses it.
- A click on its action runs `action.run()`, then dismisses the notice
  once (the click must not also count as a click on the notice).
- When the stack unmounts, every timer is cleared.

`onDismiss` may be a new function on every render: use the latest one
when a timer fires.

## Tests (given, do not edit)

`app/src/features/notice/Notices.test.tsx`: order and roles, the two
durations, timing from first show, a notice that leaves early, clicks,
actions, and unmounting. They use Vitest's fake timers.

## Gotchas

- A `div` with an `onClick` is a static element to Biome
  (`noStaticElementInteractions`, `useKeyWithClickEvents`). A notice
  needs no keyboard of its own: a click only hastens what its timer
  does, and its action is a real button. A `biome-ignore` comment
  naming those two rules with that reason is acceptable on the notice's
  element only.
- Keep timers in a ref keyed by `id`, not in state: re-rendering must
  not restart them.
- The stack is presentational: no stores, no client (`docs/tasks/EXECUTOR.md`).

## Out of scope

Which notices the app raises and the store that holds them (the
planner's), and every file except `app/src/features/notice/Notices.tsx`.

## Done when

`make accept T=0048`, `make check` and `make ui-check` pass, and only
`app/src/features/notice/Notices.tsx` changed.
