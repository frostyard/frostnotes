---
id: "0073"
title: Draw math with KaTeX in live preview
milestone: M3
size: M
touch:
  - app/src/editor/preview/math.ts
given:
  - app/src/editor/preview/math.test.ts
acceptance: make ui-vitest F=src/editor/preview/math.test.ts
---
# T-0073: Draw math with KaTeX in live preview

## Goal

Math draws with KaTeX away from the selection's lines
(`docs/specs/ui.md`, Editor and reading view): `$…$` in its line, a `$$`
block as a block of its own; a click shows the source.

## Read first

- `app/src/editor/preview/math.ts`: the stub, already in `PROVIDERS`
  (`preview/index.ts`).
- `app/src/editor/preview/framework.ts`: inline and block providers
  (a block provider's replace decoration may cover whole lines),
  `blockDecorations`, `PreviewAction`.
- `app/src/editor/syntax/math.ts`: `InlineMath` and `BlockMath` with
  their `InlineMathMark` and `BlockMathMark` children.
- `app/src/reading/finish.ts`: how the reading view calls KaTeX.
- The given test (it runs in jsdom: KaTeX draws nothing in happy-dom).

## Contract

Keep the export `MATH: readonly Provider[]`; replace the file's
"Task T-0073 …" sentence with a description. Import KaTeX's styles
(`import "katex/dist/katex.min.css";`) as `finish.ts` does.

- **A math widget** draws `tex` with
  `katex.render(tex, el, { displayMode, throwOnError: false })` into a
  `span.cm-pv-math` (inline) or a `div.cm-pv-math.cm-pv-math-block`
  (display), with `data-pv="math"`; when KaTeX throws, the element's
  text is the TeX as written. `tex` is the text between the first and
  last marks, trimmed. Widgets compare equal by TeX and display; their
  `ignoreEvent` is false.
- **Inline math** (`kind: "inline"`, `nodes: ["InlineMath"]`): away from
  the selection (`!cx.revealed(node.from, node.to)`), replace the node
  with an inline widget. Return false.
- **Block math** (`kind: "block"`, `nodes: ["BlockMath"]`): away from
  the selection, replace from the start of the node's first line to the
  end of its last line with
  `Decoration.replace({ widget, block: true })`. Return false.
- **A click** on either answers a transaction that puts the cursor right
  after the math's opening mark, found from the widget's position
  (`view.posAtDOM`).

## Tests (given, do not edit)

`app/src/editor/preview/math.test.ts`: inline math drawn and its
source on the selection's lines, a TeX error, KaTeX refusing to draw,
clicks; block math as a block over its lines, its source, a one-line
block, clicks.

## Gotchas

- Run the test with its jsdom header as given; `make accept` does.
- KaTeX throws on a page it takes for one in quirks mode (it reads
  `document.compatMode`), as happy-dom's is: catch it.

## Out of scope

The reading view's math (done), the styles, and every file except
`app/src/editor/preview/math.ts`.

## Done when

`make accept T=0073`, `make check` and `make ui-check` pass, and only
`app/src/editor/preview/math.ts` changed.
