# 0042 — The desktop app takes frostui's Frostyard look

- **Status:** Proposed
- **Date:** 2026-11-11

## Context

[ADR-0038](0038-draw-the-desktop-app-natively-with-mygo-and-frostui.md)
made the desktop app's look frostui's, with a theme that followed the
desktop's accent, light or dark, contrast and text size. frostui's
[ADR-0011](https://github.com/frostyard/frostui/blob/main/docs/adr/0011-draw-the-frostyard-brand-in-a-dark-and-a-light-theme.md)
then gave the kit frostyard's brand: two themes of its own, Frostyard
(dark) and Frostyard light, Inter as the text font, text sizes that follow
the desktop's text size, nearly square corners, hairlines instead of
shadows, and a code color for fields and code. Its `Adapt` no longer takes
the desktop's accent. frostui v0.5.0 ships it, and its plan
([the Frostyard look](https://github.com/frostyard/frostui/blob/main/docs/plans/frostyard-look.md),
phase 5) moves Frostnotes onto it.

Frostnotes has its own Theme setting (System, Light, Dark), which set
MyGo's light or dark theme. Its live preview drew headings in the editor's
monospaced font and fences on the editor's ground, and its own pieces (the
brand tile, tab strips, pane tabs, callouts, note embeds, the vault manager)
set font sizes and corners of their own.

## Decision

- **The theme is Frostyard or Frostyard light.** Each frame the window sets
  `frostui.FrostyardTheme(dark)` with the desktop's preferences but not its
  accent, before anything reads the theme: Light and Dark pick one, System
  takes the one of the desktop's light or dark. The desktop's high contrast
  and text size apply; its accent never does. The zoom stays a zoom of the
  whole window.
- **Text and corners are the kit's.** The app's own text uses frostui's
  styles (`Title`, `Body`, `Muted`, `Caption`) or their sizes through
  `TextSize`; corners are the theme's `Radius` for buttons and the brand
  tile, `RadiusChip` for rows, callouts and code, `RadiusCard` for note
  embeds. No literal font size or corner remains in `internal/desktop`.
- **Notes in the brand.** Live preview draws headings in Inter, bold, in the
  text color, at their level's multiple of the editor's text size (the
  Font size setting), and the lines of code blocks, fences included, on the
  code color. The reading view draws code blocks on the code color inside a
  hairline, at a chip's corners, and callouts as frostui's banners are drawn:
  the tone's fill and line, with the tone's bar.
- **Selected rows are the kit's.** The vault list, the history and the
  panes' rows take frostui's selected row (the selected fill, a 1px accent
  rule on the leading edge, the selected text); hovered rows and tool
  buttons change their text, not their face.

## Consequences

The desktop app looks like ChairLift and Frostmail on the same kit, in both
themes, and a change of the brand reaches it with a frostui release. A user
who chose an accent on the desktop no longer sees it in Frostnotes. Live
preview mixes two families: Inter on heading lines, through MyGo's per-line
styles, and the monospaced font on the rest, at the Font size setting, which
the desktop's text size does not change. The Flatpak's metainfo screenshots
(`make screenshots`) show the former look until they are taken again.

## Alternatives considered

- **Keep MyGo's light and dark themes for the Theme setting:** `Adapt`
  turns them into Frostyard anyway, but only when the shell draws, after
  what read the theme earlier in the frame; setting Frostyard first gives
  every view the same theme.
- **The kit's own sizes for headings** (`Title` for level 1, `Heading`
  below): they are sized for the window's text, not the editor's, and would
  put levels 2 to 6 below the editor's 16 DIP body text.
- **A code block's header on the code color in live preview:** MyGo puts a
  block within the text's width, narrower than a line's background, so the
  header and the lines would not line up; the header stays on the editor's
  ground above them.

## References

- Shapes: [app design](../design/app.md#the-native-look), [UI spec](../specs/ui.md#in-the-native-desktop-app), [desktop plan](../plans/0009-mygo-desktop.md)
- Builds on: [ADR-0038](0038-draw-the-desktop-app-natively-with-mygo-and-frostui.md), [ADR-0039](0039-live-preview-is-replacements-over-the-source.md); frostui's [ADR-0011](https://github.com/frostyard/frostui/blob/main/docs/adr/0011-draw-the-frostyard-brand-in-a-dark-and-a-light-theme.md) and [theme spec](https://github.com/frostyard/frostui/blob/main/docs/specs/frostyard-theme.md)
- Supersedes in part: [ADR-0038](0038-draw-the-desktop-app-natively-with-mygo-and-frostui.md) (the theme follows the desktop's accent)
