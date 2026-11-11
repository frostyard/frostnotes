# 0041 — Render native math as vector paths

- **Status:** Proposed
- **Date:** 2026-11-11

## Context

The native app's reading view and live preview retain TeX source because
MyGo has no TeX renderer. The web app renders math with KaTeX. Native views
must not execute note content as script and must work without a browser.

## Decision

Use the pure-Go `github.com/d2lang/mathjax-go` renderer to lay out TeX as SVG
glyph paths. MyGo draws those paths in the surrounding theme's text color.
Share a bounded per-view formula cache between inline and display rendering;
limit source length, nesting and generated output. Invalid or unsupported
formulas remain source. Live preview reveals source on selected lines, as
[ADR-0039](0039-live-preview-is-replacements-over-the-source.md) requires.

## Consequences

Native formulas need no JavaScript engine, external program or network.
The app carries the renderer's glyph data. Native and web math use different
engines, so supported commands and spacing may differ. Oversized expressions
remain source to bound rendering work.

## Alternatives considered

- **A browser running KaTeX:** it adds a second UI runtime to the native app.
- **External TeX executables:** they add installation and process management,
  and a document execution boundary.

## References

- Shapes: [app design](../design/app.md#live-preview), [UI spec](../specs/ui.md#in-the-native-desktop-app), [desktop plan](../plans/0009-mygo-desktop.md)
- Builds on: [ADR-0038](0038-draw-the-desktop-app-natively-with-mygo-and-frostui.md), [ADR-0039](0039-live-preview-is-replacements-over-the-source.md)
- Renderer: [mathjax-go](https://github.com/d2lang/mathjax-go)
