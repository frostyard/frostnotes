---
id: "0067"
title: Find comments and count words as notesd does
milestone: M3
size: M
touch:
  - app/src/editor/syntax/comments.ts
  - app/src/editor/words.ts
given:
  - app/src/editor/syntax/comments.test.ts
  - app/src/editor/words.test.ts
acceptance: make ui-vitest F="src/editor/syntax/comments.test.ts src/editor/words.test.ts"
---
# T-0067: Find comments and count words as notesd does

## Goal

Obsidian's `%%comments%%` can cross blocks, so they are not syntax
nodes: find them as notesd does, as ranges of the text, and mark the
ones in view so the editor can dim them. Then count a note's words and
characters for the status bar, leaving comments out exactly as notesd's
word count does.

## Read first

- `app/src/editor/syntax/comments.ts` and `app/src/editor/words.ts`: the
  stubs. `commentStyle` is already part of `markdownSupport()`
  (`syntax/index.ts`).
- `docs/specs/markdown.md`, "Comments" and "Words".
- `internal/markdown/obsidian.go`, `findComments` and `literalRanges`,
  and `internal/markdown/structure.go`, `countWords`: notesd's rules.
- `app/src/editor/syntax/extract.ts`, `frontmatterEnd`.
- The given tests.

## Contract

Keep the exported names and types (`Span`, `findComments`,
`commentStyle`, `Counts`, `countWords`); replace each file's
"Task T-0067 …" sentence with a description.

**`findComments(state)`** returns the body's comments, sorted and
disjoint, each from its opening `%%` to just after its closing one, or
to the document's end when none closes it. The body starts at
`frontmatterEnd(text)`. Scan for `%%` from there: one inside a code,
math or HTML node of `syntaxTree(state)` opens nothing, and the scan
goes on after that node; any other opens a comment that ends with the
next `%%` wherever it is (inside code too); the scan goes on after it.
The nodes that hold a `%%` are those whose names are `FencedCode`,
`CodeBlock`, `InlineCode`, `HTMLBlock`, `CommentBlock`,
`ProcessingInstructionBlock`, `HTMLTag`, `Comment`,
`ProcessingInstruction`, `InlineMath` and `BlockMath` (T-0066's math
nodes) at or above `tree.resolveInner(pos, 1)`. The function runs on
every edit and every count: return the same result for the same tree
and document without scanning again (a `WeakMap` keyed by the tree).

**`commentStyle`**, an `Extension`: a view plugin whose decorations
mark each comment's part within the view's visible ranges with
`Decoration.mark({ class: "cm-fn-comment" })`, built again when the
document, the viewport or the syntax tree changes.

**`countWords(state, from = 0, to = doc.length)`** counts the text
between `from` and `to` that is in the body (from `frontmatterEnd`) and
outside every comment:

- `words`: the maximal runs of characters that are not white space,
  each holding a letter (`\p{L}`) or a decimal digit (`\p{Nd}`). White
  space is Go's `unicode.IsSpace`: `\t \n \v \f \r`, space, U+0085,
  U+00A0, U+1680, U+2000 to U+200A, U+2028, U+2029, U+202F, U+205F and
  U+3000 (not U+FEFF or U+200B). A comment splits a run, as white space
  does.
- `characters`: the code points of that text, except `\n` and `\r`.

## Tests (given, do not edit)

`app/src/editor/syntax/comments.test.ts`: inline, crossing lines and
blocks, open to the end, not opened in code or HTML, closed anywhere,
the body only; the marks in an editor's view and after an edit.
`app/src/editor/words.test.ts`: every fixture's `words` from the spec,
white space, letters and digits, markup, comments, frontmatter;
characters; ranges.

## Gotchas

- `findComments` must not depend on math nodes existing: T-0066 adds
  them, and until then no `%%` is in math.
- A view plugin without a class: `ViewPlugin.define((view) => ({
  decorations, update(u) { … } }), { decorations: (v) => v.decorations })`.
  Decorations go to a `RangeSetBuilder` in order.
- Write invisible characters in a regex as `\u` escapes, never as
  literal characters (U+2028 ends a regex literal).

## Out of scope

The status bar and when it counts (the planner's), the comment's color
(the theme's), and every file except the two under `touch`.

## Done when

`make accept T=0067`, `make check` and `make ui-check` pass, and only
the two files under `touch` changed.
