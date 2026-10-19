# 0011 — Look like frostmail's sibling, with Obsidian's layout

- **Status:** Accepted
- **Date:** 2026-10-16

## Context

The user chose a look shared with frostmail, the Frostyard mail client,
which looks like neither GNOME nor KDE (frostmail ADR-0009). Obsidian is
the inspiration for how the app works: its layout, panes and shortcuts.
One React UI serves the desktop app and the web app
([ADR-0004](0004-a-notes-daemon-with-thin-clients.md)), and the web app is
v1's answer on phones.

## Decision

- **Frostmail's stack:** React 19 and strict TypeScript; small zustand
  stores; Tailwind v4 over CSS-variable tokens for light and dark; the
  bundled Inter Variable font; Lucide icons; client-side window chrome on
  the desktop, where the tab bar is the title bar. The tokens, chrome,
  transport and test harness are copied from frostmail at M0.
- **Obsidian's layout:** a left sidebar (files, search, tags, bookmarks), a
  workspace of tabs in splits, a right sidebar (backlinks, outgoing links,
  outline, properties, local graph) and a status bar (sync state, word
  count). A quick switcher, a command palette, and Obsidian's default
  shortcuts.
- **Responsive to phone width in the browser:** one pane at a time, with
  the sidebars as drawers.
- **Presentational components take props and call callbacks;** containers
  connect them to stores and the client. Cards build presentational
  components, and the planner owns the data layer, the editor core and the
  workspace model.
- **Tests:** Vitest with happy-dom and Testing Library against the mock
  transport, Biome for lint and format, and app tests in the built app
  under WebKitGTK.

## Consequences

- Frostmail and frostnotes look like one family, and frostmail's
  components are a starting point rather than a library.
- Obsidian users find things where they expect them.
- No theme or plugin system in v1. CSS snippets may come later.
- Copied code drifts from frostmail's. A shared package is worth it only
  when a third app needs the same pieces.

## Alternatives considered

- **The frostyard design system:** built for the marketing site (display
  type, square cards, deep-blue gradients), not a dense working app.
- **A close copy of Obsidian's look:** it would be Obsidian's look, not
  Frostyard's.
- **A UI package shared with frostmail now:** couples two pre-release apps
  that change daily.

## References

- Icon extension: [ADR-0013](0013-use-a-sibling-frostyard-notebook-icon.md),
  [overview — application icon](../design/overview.md#application-icon)
- Shapes: [design/app.md](../design/app.md), [specs/ui.md](../specs/ui.md)
- Extended by: [ADR-0021](0021-keep-the-workspace-and-app-preferences-on-each-device.md) (where the workspace and preferences live), [ADR-0025](0025-patch-what-costs-the-editor-its-frames.md) (the app's copy of Tailwind's preflight)
- Builds on: frostmail ADR-0009; [ADR-0004](0004-a-notes-daemon-with-thin-clients.md), [ADR-0007](0007-edit-in-codemirror-6-with-live-preview.md)
- Built in: [roadmap — M3, M4](../plans/0002-roadmap.md), [plan 0005](../plans/0005-m3-desktop-app.md)
