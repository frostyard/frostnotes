---
id: "0071"
title: Draw fenced code blocks in live preview
milestone: M3
size: S
touch:
  - app/src/editor/preview/code.ts
given:
  - app/src/editor/preview/code.test.ts
acceptance: make ui-vitest F=src/editor/preview/code.test.ts
---
# T-0071: Draw fenced code blocks in live preview

## Goal

A fenced code block draws as a block of code (`docs/specs/ui.md`,
Editor and reading view): its lines on the code color with the first and
last rounded, and, while the selection is elsewhere, its fences hidden,
its language at the top right and a button that copies its code.

## Read first

- `app/src/editor/preview/code.ts`: today's provider, which only colors
  the lines; it is already in `PROVIDERS` (`preview/index.ts`).
- `app/src/editor/preview/framework.ts`: `Provider`, `DrawContext`
  (`line`, `revealed`, `hide`, `add`), `PreviewCommand`'s `copy`.
- `app/src/editor/preview/basic.ts`: a widget (`CheckboxWidget`).
- @lezer/markdown's `FencedCode`: `CodeMark` (each fence), `CodeInfo`
  (the language), `CodeText` (the code).
- The given test.

## Contract

Keep the exports `code` and `CODE: readonly Provider[]` (`[code]`);
replace the file's sentence about T-0071 with a description. `code` is
`id: "code"`, `kind: "inline"`, `nodes: ["FencedCode"]`, and its draw
returns false.

- **Lines, always:** each line of the block gets one line decoration:
  class `cm-pv-code`, plus `cm-pv-code-first` on the first line and
  `cm-pv-code-last` on the last (both on a one-line block).
- **Away from the selection** (`!cx.revealed(node.from, node.to)`):
  replace the opening fence, from its first `CodeMark` to the end of its
  line, with a header widget: a `span.cm-pv-code-header` holding a
  `span.cm-pv-code-lang` (the `CodeInfo` text, or empty) and a
  `button.cm-pv-code-copy` (`type="button"`,
  `aria-label="Copy code"`, text `Copy`, `data-pv="code"`); its
  `ignoreEvent` is false. When a closing fence is on a later line than
  the opening one, hide it from its `CodeMark` to the end of its line.
- **The copy button's click** answers
  `{ kind: "command", command: { kind: "copy", text } }` with the
  block's `CodeText` (empty when it has none), the block found from the
  button's position (`view.posAtDOM`).

## Tests (given, do not edit)

`app/src/editor/preview/code.test.ts`: line classes, hidden fences and
the language, the source while the selection is in the block, a block
without a language, copying, an empty block.

## Out of scope

The styles (the planner's theme), copying itself (the app runs the
command), and every file except `app/src/editor/preview/code.ts`.

## Done when

`make accept T=0071`, `make check` and `make ui-check` pass, and only
`app/src/editor/preview/code.ts` changed.
