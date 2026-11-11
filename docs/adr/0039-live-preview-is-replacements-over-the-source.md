# 0039 — Draw live preview as replacements over the source

- **Status:** Proposed
- **Date:** 2026-11-10

## Context

The native desktop app ([ADR-0038](0038-draw-the-desktop-app-natively-with-mygo-and-frostui.md))
edits notes in MyGo's code editor, which today shows source only. Live preview
is decoration over the source, never a transformation
([ADR-0007](0007-edit-in-codemirror-6-with-live-preview.md)). MyGo's author
accepted editor support in three pieces
([egoist/mygo#1651](https://github.com/egoist/mygo/issues/1651)): ranges shown
as something else, then elements between lines, then per-line text styles. The
first is a pull request
([egoist/mygo#1655](https://github.com/egoist/mygo/pull/1655)), not yet in a
release; the app builds on a fork that has it and the second piece
([egoist/mygo#1662](https://github.com/egoist/mygo/pull/1662)) until one does
([plan 0009](../plans/0009-mygo-desktop.md)).

The web app computes decorations from CodeMirror's syntax tree. The native
editor has no syntax tree, and a keystroke in a 10,000-line note has 16 ms.

## Decision

- `internal/desktop/livepreview` works out what shows, in plain Go, with no
  MyGo import: from a note's text and the lines selections touch, a sorted
  list of pieces, each a byte range that hides or shows as a bullet, a quote
  bar, a check box or a rule.
- It reads a line at a time, with the blocks that span lines (frontmatter,
  fences, math, comments) followed in one pass, and keeps each line's pieces
  by the line's text: a keystroke walks every line to follow the blocks, but
  parses again only the line it changed.
- The reveal rule stays: a line a selection touches shows its source, and a
  block that spans lines shows its source when a selection enters it.
- The editor turns pieces into MyGo's `Replacement`s and blocks into its
  `Block`s.
- Callouts, tables, note embeds and images are blocks that notesd renders
  with `note.renderMarkdown` and the reading view's code draws, so they look
  as they do there. Only those on screen are asked for.
- Links, tags and footnotes are elements in the line, so a click reaches them
  without first moving the caret, which would show their source. A link's
  label too long for one element keeps to hidden marks.
- What needs per-line styles (headings' sizes, a done task's strike-through)
  waits for that piece of MyGo.

## Consequences

- The rules are tested without a window, and on any MyGo version.
- A hand-written line scanner is a second reader of Markdown beside
  `internal/markdown` and the highlighter. It may disagree with them on odd
  input; the worst outcome is a mark shown or hidden wrongly, since the text
  itself never changes.
- Live preview stays incomplete until MyGo has per-line styles, and the app
  cannot replace Tauri before then.

## Alternatives considered

- **Compute from `internal/markdown`'s parse:** it parses whole documents, and
  a keystroke would pay for that in a long note.
- **Frostnotes' own editor widget:** the fallback in
  [plan 0009](../plans/0009-mygo-desktop.md) should MyGo's pull requests stall.
  They have not.

## References

- Shapes: [design/app.md](../design/app.md#live-preview), [specs/ui.md](../specs/ui.md#in-the-native-desktop-app), [plan 0009](../plans/0009-mygo-desktop.md#phase-5--live-preview-about-3-weeks)
- Builds on: [ADR-0007](0007-edit-in-codemirror-6-with-live-preview.md), [ADR-0038](0038-draw-the-desktop-app-natively-with-mygo-and-frostui.md)
