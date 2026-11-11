# 0040 — Remove the inactive native Spellcheck setting

- **Status:** Proposed
- **Date:** 2026-11-11

## Context

The native desktop app introduced by [ADR-0038](0038-draw-the-desktop-app-natively-with-mygo-and-frostui.md)
keeps a Spellcheck preference, but MyGo's editor has no spelling checker.
Changing the preference has no visible effect. The web app uses its browser's
spelling support independently.

## Decision

Remove Spellcheck from the native app's Settings and preferences. Older native
preference files may still contain `spellcheck`; loading ignores that field.
The web app keeps its existing preference. A future native spelling checker
can introduce a setting when it has an implementation.

## Consequences

The native app no longer offers an ineffective control. Native editing has no
spelling underlines, and the old preference value is not retained when saving.

## Alternatives considered

- **Keep a disabled setting:** it occupies Settings without offering an action.
- **Add a spelling engine in this change:** it also requires dictionaries,
  language selection, licensing and editor diagnostics; none is present today.

## References

- Shapes: [app design](../design/app.md#live-preview), [UI spec](../specs/ui.md#in-the-native-desktop-app), [desktop plan](../plans/0009-mygo-desktop.md)
- Builds on: [ADR-0038](0038-draw-the-desktop-app-natively-with-mygo-and-frostui.md)
- Resolves: [issue 150](https://github.com/frostyard/frostnotes/issues/150)
