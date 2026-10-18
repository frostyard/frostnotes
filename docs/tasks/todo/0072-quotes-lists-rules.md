---
id: "0072"
title: Draw quotes, lists, rules and done tasks in live preview
milestone: M3
size: M
touch:
  - app/src/editor/preview/lists.ts
given:
  - app/src/editor/preview/lists.test.ts
acceptance: make ui-vitest F=src/editor/preview/lists.test.ts
---
# T-0072: Draw quotes, lists, rules and done tasks in live preview

## Goal

Blockquotes, bullet lists, horizontal rules and done tasks draw as they
read (`docs/specs/ui.md`, Editor and reading view): a quote's lines with
a bar and without their `>` marks, bullets as dots, a rule as a line,
away from the selection's lines; a done task's text muted.

## Read first

- `app/src/editor/preview/lists.ts`: the stub, already in `PROVIDERS`
  (`preview/index.ts`).
- `app/src/editor/preview/framework.ts` and `basic.ts` (the task
  checkbox provider draws a task's mark; leave it to it).
- @lezer/markdown's nodes: `Blockquote`, `QuoteMark`, `BulletList`,
  `OrderedList`, `ListItem`, `ListMark`, `Task`, `TaskMarker`,
  `HorizontalRule`.
- The given test.

## Contract

Keep the export `LISTS: readonly Provider[]`; replace the file's
"Task T-0072 …" sentence with a description. Every provider is
`kind: "inline"`.

- **Quotes** (`nodes: ["Blockquote"]`): each line of the quote gets the
  line decoration `cm-pv-quote`, always.
- **Quote marks** (`nodes: ["QuoteMark"]`): on a line the selection does
  not touch, hide the mark and the space right after it, if any.
- **Bullets** (`nodes: ["ListMark"]`): a mark of a `ListItem` in a
  `BulletList` that holds no `Task`, on a line the selection does not
  touch, is replaced by a widget, an empty `span.cm-pv-bullet`. Ordered
  lists keep their numbers.
- **Rules** (`nodes: ["HorizontalRule"]`): on a line the selection does
  not touch, the line gets the line decoration `cm-pv-hr-line` and the
  rule is replaced by a widget, an empty `span.cm-pv-hr`.
- **Done tasks** (`nodes: ["Task"]`): when the character between the
  `TaskMarker`'s brackets is `x` or `X`, mark the task's text, from after
  the marker and one space to the end of its first line, with
  `Decoration.mark({ class: "cm-pv-task-done" })`, whatever the
  selection.

## Tests (given, do not edit)

`app/src/editor/preview/lists.test.ts`: quote bars and hidden marks,
nested quotes, bullets, the cursor's line, tasks' marks left alone, done
tasks, rules, a setext heading's underline.

## Gotchas

- `cx.line(pos, deco)` decorates the line at pos once per class, so a
  nested quote's line is marked once.
- An inline provider's decorations may not cross a line break: stop a
  done task's mark at its line's end.

## Out of scope

Indentation guides, the styles (the planner's theme), and every file
except `app/src/editor/preview/lists.ts`.

## Done when

`make accept T=0072`, `make check` and `make ui-check` pass, and only
`app/src/editor/preview/lists.ts` changed.
