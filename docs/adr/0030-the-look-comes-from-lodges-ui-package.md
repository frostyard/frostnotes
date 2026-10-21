# 0030 — The look comes from lodge's UI package

- **Status:** Accepted (Brian, 2026-10-21)
- **Date:** 2026-10-21

## Context

[ADR-0011](0011-look-like-frostmails-sibling-with-obsidians-layout.md) gave
the app frostmail's look by copying frostmail's tokens and Tailwind theme,
and named when that should stop: "A shared package is worth it only when a
third app needs the same pieces."

The third app is here. frostread, a feed reader, is built beside frostmail
and frostnotes, and the suite's UI audit of 2026-10-21 found the two apps'
copies already drifting and four palettes in the suite. lodge now holds the
shared look as `@frostyard/lodge-ui`
([lodge ADR-0018](https://github.com/frostyard/lodge/blob/main/docs/adr/0018-a-ui-package-beside-the-client.md),
[lodge's UI package spec](https://github.com/frostyard/lodge/blob/main/docs/specs/ui-package.md)):
the colour tokens, the domain accents, the Tailwind theme over them and the
shared type roles, as ESM source that an app's bundler compiles.

Two of Frostnotes' constraints shape the move:

- **The app owns its preflight.** `app.css` imports Tailwind's layers one
  by one, with the app's patched copy of the preflight
  ([ADR-0025](0025-patch-what-costs-the-editor-its-frames.md)), so the
  shared stylesheet must not import `tailwindcss` whole and must carry no
  preflight.
- **Frostnotes has tokens and type roles of its own**: tags and callout
  cyan, and the status, tab and keycap roles. A package cannot take them
  without imposing them on the other apps.

## Decision

**Frostnotes takes its look from `@frostyard/lodge-ui`, and keeps only what
the package does not carry.**

- `app/src/styles/app.css` imports the package's `fonts.css`, `tokens.css`,
  `accents.css` and `theme.css`, after Tailwind's `theme.css`, the app's
  preflight and Tailwind's `utilities.css`. The package's theme therefore
  goes after Tailwind's theme, as it requires
  ([lodge's UI package spec](https://github.com/frostyard/lodge/blob/main/docs/specs/ui-package.md#how-an-app-consumes-it)).
- `app.css` adds `@source "../../node_modules/@frostyard/lodge-ui/src";`,
  because Tailwind v4 does not scan `node_modules`.
- `app/src/styles/tokens.css` keeps two tokens: `--bg-tag` and
  `--callout-cyan`, with the same light and dark values as before.
  `--bg-code`, `--bg-highlight`, `--popover-shadow` and `--backdrop` came
  from Frostnotes and are the package's now.
- `app.css` keeps the `@theme` entries the package does not have: the
  colour utilities of the two remaining tokens, and the `status`, `tab` and
  `keycap` type roles. The five shared roles and every colour token come
  from the package.
- `app.css` keeps the preflight, the base layer and `editor.css` as they
  were.
- The app pins the package to a lodge commit in `package.json`, and the
  lockfile pins it too. A lodge tag, `ui/vX.Y.Z`, replaces the commit once
  lodge releases one.
- **No visual change.** The values are frostmail's, which are Frostnotes',
  copied unchanged; the built stylesheet loses no custom property and
  changes none (see Consequences).

## Consequences

- The colour tokens, the accents and the five shared type roles have one
  source for frostmail, frostnotes and frostread. A colour fix lands once,
  in lodge, and each app takes it with a version bump.
- **The built stylesheet proves the move changed nothing that is drawn**:
  before and after, every custom property in the light root, the media dark
  block, `[data-theme="dark"]` and `[data-theme="light"]` has the same
  value (0 lost, 0 changed). The additions are the density tokens, the
  avatars and `--accent-contrast` written out in the dark, and utilities
  only the package's components use.
- **A token value change is now a package release**, not an app edit. The
  look changes when Frostnotes bumps `@frostyard/lodge-ui`, and the
  changelog of the package says what changed.
- Two tokens and three type roles stay duplicated in spirit: they are
  Frostnotes' and no other app wants them. If a second app wants them, they
  move into the package.
- The dependency is new. ADR-0009's counterpart in frostmail leaves a new
  dependency to the planner; here the same holds, and the planner approved
  `@frostyard/lodge-ui` on 2026-10-21.
- The preflight stays the app's copy, so a Tailwind upgrade still touches
  `preflight.css` here, not in the package ([ADR-0025](0025-patch-what-costs-the-editor-its-frames.md)).

## Alternatives considered

- **Keep the copy:** the audit already shows the two apps' copies drifting,
  and frostread would be the third. Refused for the reason ADR-0011 gave.
- **A package that also carries the preflight:** it would change the app's
  patched preflight, which ADR-0025 says must stay the app's. Refused.
- **A package that carries `--bg-tag` and `--callout-cyan` too:** they are
  Frostnotes' own; another app would carry tokens it does not use. They can
  move later, if a second app wants them.
- **The frostyard design system:** ADR-0011 refused it (marketing scale,
  dark only, remote fonts); it stays the brand source, not the app's
  tokens.
- **Putting the tokens in `@frostyard/lodge-client`:** that library has no
  runtime dependencies, and CSS and React would land in the shell and
  Porter (lodge's client spec). Refused.

## References

- Shapes: [specs/ui.md](../specs/ui.md#tokens), [design/app.md](../design/app.md)
- Builds on: [ADR-0011](0011-look-like-frostmails-sibling-with-obsidians-layout.md), [ADR-0025](0025-patch-what-costs-the-editor-its-frames.md)
- Lodge: [ADR-0018](https://github.com/frostyard/lodge/blob/main/docs/adr/0018-a-ui-package-beside-the-client.md), [the UI package spec](https://github.com/frostyard/lodge/blob/main/docs/specs/ui-package.md)
