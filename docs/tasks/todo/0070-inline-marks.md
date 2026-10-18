---
id: "0070"
title: Draw highlights, tags, inline code and footnote references in live preview
milestone: M3
size: M
touch:
  - app/src/editor/preview/inline.ts
given:
  - app/src/editor/preview/inline.test.ts
acceptance: make ui-vitest F=src/editor/preview/inline.test.ts
---
# T-0070: Draw highlights, tags, inline code and footnote references in live preview

## Goal

Live preview draws Obsidian's inline syntax as it will read, away from
the lines the selection touches, and shows it as typed on them
(`docs/specs/ui.md`, Editor and reading view): a highlight without its
`==`, a tag as a pill that searches for it, inline code without its
backticks, a footnote reference as a raised label that goes to its
definition.

## Read first

- `app/src/editor/preview/inline.ts`: the stub, already in `PROVIDERS`
  (`preview/index.ts`).
- `app/src/editor/preview/framework.ts`: `Provider`, `DrawContext`
  (`revealed`, `hide`, `add`), `PreviewAction`, `PreviewCommand`.
- `app/src/editor/preview/basic.ts`: providers written the same way
  (`marks`, `wikilinks`, the task checkbox widget).
- The syntax nodes: `Highlight` with `HighlightMark`s
  (`syntax/highlights.ts`), `Hashtag` (`syntax/tags.ts`), `InlineCode`
  with `CodeMark`s (@lezer/markdown), `FootnoteReference` with its
  `FootnoteLabel` and `FootnoteDefinition` (`syntax/footnotes.ts`).
- The given test.

## Contract

Keep the export `INLINE: readonly Provider[]`; replace the file's
"Task T-0070 …" sentence with a description. Every provider is
`kind: "inline"`. Away from the selection means
`!cx.revealed(node.from, node.to)` (a single-line node may pass just
`node.from`).

- **Highlights** (`nodes: ["Highlight"]`): away from the selection, hide
  each `HighlightMark`. (The theme colors the text.)
- **Tags** (`nodes: ["Hashtag"]`, `id: "tag"`): away from the selection,
  and not inside a `Link` or `Image` node, add
  `Decoration.mark({ class: "cm-pv-tag", attributes: { "data-pv": "tag", "data-tag": <name without "#"> } })`
  over the whole tag. A click answers
  `{ kind: "command", command: { kind: "search", query: "tag:#<name>" } }`.
- **Inline code** (`nodes: ["InlineCode"]`): away from the selection,
  hide the first and the last `CodeMark` and mark the text between with
  `Decoration.mark({ class: "cm-pv-inline-code" })`. Return false.
- **Footnote references** (`nodes: ["FootnoteReference"]`,
  `id: "footnote"`): away from the selection, replace the reference with
  a widget, a `sup` of class `cm-pv-footnote` with `data-pv="footnote"`
  holding the label's text, whose `ignoreEvent` is false. A click
  answers a transaction that puts the cursor at the start of the
  `FootnoteDefinition` with the same label, or, when there is none, just
  after the reference's `[^`, with `scrollIntoView: true`.

## Tests (given, do not edit)

`app/src/editor/preview/inline.test.ts` draws each kind in an editor's
view and reads its DOM: hidden marks, pills and their search, code,
footnotes and their clicks, and the source on the selection's lines.

## Gotchas

- Widgets are classes over CodeMirror's `WidgetType` (as `basic.ts`
  has them): give `eq` the fields that change what it draws.
- `view.posAtDOM(el)` gives a widget's position; the syntax tree
  (`syntaxTree(view.state)`) finds a definition by its label.
- A provider's `draw` returning false skips the node's children.

## Out of scope

The pills' and code's styles (the planner's theme), other syntax, and
every file except `app/src/editor/preview/inline.ts`.

## Done when

`make accept T=0070`, `make check` and `make ui-check` pass, and only
`app/src/editor/preview/inline.ts` changed.
