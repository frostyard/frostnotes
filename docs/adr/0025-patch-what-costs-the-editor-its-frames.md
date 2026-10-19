# 0025 — Patch what costs the editor its frames

- **Status:** Proposed
- **Date:** 2026-10-18

## Context

M3's exit evidence 1 asks the built app to scroll a generated 10,000-line
note at 60 fps, frame p95 under 20 ms, with every live-preview provider on
([plan 0005, Phase 7](../plans/0005-m3-desktop-app.md#phase-7--review-and-exit-evidence)).
It ran at 52 to 54 fps. Profiling it in WebKitGTK (a per-frame trace of
CodeMirror's measuring, JavaScript sampling through the remote inspector,
and the editor's lines laid out alone under variants) found the frames'
cost in the live-preview code, which the app owns and changed, and in two
dependencies, which it does not:

- **Tailwind's preflight** resets the box of every element's `::before`
  and `::after` with one universal rule. A rule that can match every
  element's pseudo-elements makes WebKit resolve both pseudo-elements'
  styles for every element. Live preview's lines are dense with small
  elements (marks, KaTeX's spans), and that resolution was over a quarter
  of the style and layout of a scroll's new lines. No rule added after it
  can stop it matching.
- **CodeMirror's height map** keeps a node for every line the view has
  measured, and `HeightMapBranch.updateHeight` visits every node on every
  measure, measured or not: 0.4 ms a scroll frame at the note's top,
  1.5 ms by its end, and on every keystroke too. CodeMirror's main branch
  does the same as 6.43.13 (2026-10-18). The nodes keep an `outdated`
  flag that a walk could follow, but the app cannot change the walk from
  outside.

With both changed, and live preview's own drawing made cheaper, the note
scrolls at 59.9 to 60 fps. Without the height map's change it scrolls at
58.6 to 59.4 fps, short of the target.

## Decision

- **The app keeps its own copy of Tailwind's preflight**
  (`app/src/styles/preflight.css`), equal to Tailwind 4.3.3's but for one
  selector: the reset's `::after` and `::before` leave out `.cm-line`,
  `.cm-gutterElement` and their descendants. `app/src/styles/app.css`
  imports Tailwind's theme and utilities and this copy, as `tailwindcss`
  imports its own three. Nothing in a line or the gutter draws a
  pseudo-element the reset would change. A test keeps the copy equal to
  Tailwind's but for that selector.
- **`@codemirror/view` is patched with pnpm**
  (`app/patches/@codemirror__view@6.43.13.patch`, listed in
  `app/pnpm-workspace.yaml`'s `patchedDependencies`): a branch of the
  height map visits a child that is not measured only when it is forced or
  outdated. pnpm applies the patch on install. The patch goes when
  CodeMirror walks only what changed.
- **A dependency is patched only for a cost measured in the app** that its
  own code cannot avoid, as small a change as the cost needs, with the
  measurement in the plan that needed it.

## Consequences

- Evidence 1 is met (plan 0005, Phase 7), and a keystroke's measure no
  longer walks every line the view has measured.
- Upgrading Tailwind or `@codemirror/view` has a step more. A Tailwind
  release that changes its preflight fails the test until the copy is
  taken again; a `@codemirror/view` release fails `pnpm install` until the
  patch is made again for it or dropped.
- A pseudo-element drawn inside an editor line gets the browser's default
  box, not the reset's: whatever draws one sets its own `box-sizing`,
  margins and borders.
- The app carries a small fix CodeMirror could carry for everyone;
  offering it upstream is a follow-up.

## Alternatives considered

- **Override the reset for the editor:** no rule removes another's match;
  WebKit resolves the pseudo-elements whatever wins the cascade.
- **Drop the preflight:** changes the whole app's look and frostmail's
  shared base (ADR-0011).
- **The editor in a shadow root with its own styles:** cuts it off from
  the app's tokens and the reading view's styles, which rendered blocks
  share; a large change for one selector.
- **Patch CodeMirror at run time from the app:** reaches into private
  fields and prototypes and stops working silently on an upgrade, where a
  pnpm patch fails loudly.
- **Wait for CodeMirror:** leaves evidence 1 unmet until it ships.

## References

- Shapes: [design/app.md — Live preview](../design/app.md#live-preview), [plan 0005 — Phase 7](../plans/0005-m3-desktop-app.md#phase-7--review-and-exit-evidence)
- Builds on: [ADR-0007](0007-edit-in-codemirror-6-with-live-preview.md) (CodeMirror 6 and live preview), [ADR-0011](0011-look-like-frostmails-sibling-with-obsidians-layout.md) (Tailwind over the tokens), [ADR-0009](0009-development-environment-and-gates.md) (pnpm in nsl)
