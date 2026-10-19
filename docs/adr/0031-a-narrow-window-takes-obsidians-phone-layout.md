# 0031 — A narrow window takes Obsidian's phone layout

- **Status:** Proposed
- **Date:** 2026-10-18

## Context

The window has Obsidian's desktop layout in frostmail's look
([ADR-0011](0011-look-like-frostmails-sibling-with-obsidians-layout.md),
[specs/ui.md](../specs/ui.md#layout)): a ribbon, two sidebars of panes,
tab groups in splits, and a status bar. Its workspace needs at least
320 CSS pixels beside open sidebars, and the window at least 640. A
phone's browser is 360 to 430 pixels wide, and is driven by touch and an
on-screen keyboard. The desktop UI relies on hover (tooltips, a tab's
close button, page previews), on right clicks, and on keys.

M4's exit asks for the same UI on the user's phone. Obsidian's own
phone app (version 1.14.4, read from its code):

- shows one note at a time;
- slides each sidebar in over it as a drawer;
- has a bottom bar with back, forward, the quick switcher, a new tab,
  the tab switcher with the count of open tabs, and a menu of the
  ribbon's actions;
- puts a toolbar of editing commands above the keyboard: undo, redo,
  links, embeds, tags, headings, bold, italics, lists, checkboxes,
  indentation and the like.

Its touch targets are 44 and 52 pixels.

## Decision

- **One UI, two layouts.** Below 600 CSS pixels wide, the window takes
  Obsidian's phone layout; at 600 and above, the desktop layout of
  `specs/ui.md`. The same panes, prompts, editor, reading view and
  commands serve both. Only the frame around them changes:
  - **One tab group.** The workspace shows the active group's active
    tab. Splits made on a wide screen stay in the stored workspace but
    are not drawn, and the split commands do not apply.
  - **The sidebars are drawers** that slide over the workspace from
    their edge, from a button in the view header or a swipe inward
    from the screen's edge. A drawer closes on a tap outside it, on
    Escape, or when it opens a note.
  - **The bottom bar** holds Obsidian's phone bar: back, forward, the
    quick switcher, a new tab, the tab switcher (the open tabs as a
    list) and the menu (the ribbon's actions, the command palette and
    settings). The status bar is not drawn; its counts move into the
    menu.
  - **The editing toolbar** sits above the on-screen keyboard while an
    editor has focus, with Obsidian's default phone commands, scrolling
    sideways.
  - **Prompts, menus and dialogs** fill the width. A context menu opens
    on a long press.
- **Touch sizes follow the pointer, not the width.** When the primary
  pointer is coarse, buttons and rows take touch sizes (at least
  44 pixels), and nothing depends on hover. A tablet keeps the desktop
  layout with touch sizes.
- **The keyboard map stays.** A hardware keyboard on a phone or tablet
  runs Obsidian's hotkeys as on the desktop.
- **The desktop app is unchanged.** Its window's minimum stays 640
  wide, so it never draws the phone layout.

[specs/ui.md](../specs/ui.md#narrow-windows) pins the layout,
refined from Obsidian 1.14.4's phone styles when its cards are written.

## Consequences

- Phone and desktop share every component, so a fix to a pane or the
  editor reaches both. Components gain touch sizes and long presses,
  which the presentational cards test.
- The workspace model gains a narrow view of one group. A stored
  workspace is per browser
  ([ADR-0021](0021-keep-the-workspace-and-app-preferences-on-each-device.md)),
  so a phone rarely holds desktop splits, and still opens one with
  splits.
- Swipes compete with the browser's own back gesture at the screen's
  edge. Buttons do everything a swipe does.
- The phone shows no splits and no side-by-side merge view. The merge
  view stacks its two sides.

## Alternatives considered

- **A separate mobile UI:** two UIs to build and keep in step, for one
  person's notes.
- **The desktop layout squeezed:** sidebars collapse and splits narrow,
  but hover, right clicks and 28-pixel buttons fail on touch, and no
  room remains for a note.
- **A layout of Frostnotes' own design:** Obsidian's phone layout is
  what an Obsidian user expects, as ADR-0011 chose for the desktop.

## References

- Shapes: [specs/ui.md](../specs/ui.md#narrow-windows), [design/app.md](../design/app.md#in-a-browser), [plan 0006](../plans/0006-m4-web-app.md)
- Builds on: [ADR-0011](0011-look-like-frostmails-sibling-with-obsidians-layout.md), [ADR-0021](0021-keep-the-workspace-and-app-preferences-on-each-device.md)
