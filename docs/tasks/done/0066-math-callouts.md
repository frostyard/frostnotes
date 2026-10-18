---
id: "0066"
title: Parse math in the editor and read callouts
milestone: M3
size: M
touch:
  - app/src/editor/syntax/math.ts
  - app/src/editor/syntax/callouts.ts
given:
  - app/src/editor/syntax/math.test.ts
acceptance: make ui-vitest F=src/editor/syntax/math.test.ts
---
# T-0066: Parse math in the editor and read callouts

## Goal

Math hides links and tags from both parsers, so the editor's parser
(`@lezer/markdown` with GFM) must find `$…$` and `$$` blocks exactly
where notesd does. Callouts need no parsing (a callout is a blockquote,
its content parsed as usual), but live preview needs what a callout's
first line says: read it from the tree.

## Read first

- `app/src/editor/syntax/math.ts` and `callouts.ts`: the stubs; `math.ts`
  is already registered in `OBSIDIAN` (`syntax/index.ts`).
- `docs/specs/markdown.md`, "Body" (math) and "Callouts, highlights and
  footnotes".
- `internal/markdown/obsidian.go`: `inlineMathParser`, `inlineMathEnd`
  and `mathBlockParser`, notesd's rules, which yours follow.
- `internal/render/syntax.go`, `calloutMarker`: the renderer's pattern
  for a callout's first line.
- `@lezer/markdown`'s `FencedCode` block parser
  (`app/node_modules/@lezer/markdown/dist/index.js`): a block parser
  that moves across lines, keeping quote marks.
- The given test, and `app/src/editor/syntax/fixtures.test.ts`, which
  runs F02 once `InlineMath`, `BlockMath` and `Hashtag` (T-0064) exist.

## Contract

Keep each file's exported names and types (`MathSyntax`, a
`MarkdownConfig`; `Callout` and `readCallout`); replace each file's
"Task T-0066 …" sentence with a description.

**`MathSyntax`** defines `InlineMath` (style `t.special(t.string)`),
`InlineMathMark` (`t.processingInstruction`), `BlockMath`
(`block: true`, style `t.special(t.string)`) and `BlockMathMark`
(`t.processingInstruction`). Neither node's content is parsed.

- An inline parser named `InlineMath` at a `$`, over the rest of the
  line (stop at a line break): `$$` opens math that closes at the next
  `$$` on the line; a single `$` opens only before a character that is
  not a space, tab or line break, skips any character after a backslash,
  and closes at the next `$` not preceded by a space or tab. No close on
  the line: -1. The node holds an `InlineMathMark` over each delimiter
  (`$` or `$$`).
- A block parser named `BlockMath`, `before: "FencedCode"`: a line
  whose text, indented at most 3 columns past its container
  (`line.indent - line.baseIndent <= 3`), starts `$$` (at `line.pos`)
  opens a block. When the rest of that line holds `$$`, the block is
  that line; else it takes each following line while the line is still
  in the block's containers, and ends with the first that holds `$$`
  anywhere, or with the document or its container. The node runs from
  the first `$$` to the end of its last line (`cx.prevLineEnd()`),
  holding a `BlockMathMark` over the opening `$$` and one over the
  closing `$$`, and any quote marks of its lines (`line.markers`), in
  order. It interrupts a paragraph (`endLeaf`).

**`readCallout(state, quote)`**: null unless `quote` is a `Blockquote`.
Take the text of the quote's first line after its first `QuoteMark` and
any spaces or tabs, and match it against the renderer's pattern,
`^\[!([A-Za-z0-9_-]+)\]([+-]?)(?:[ \t]+(.*?))?[ \t]*$`. No match: null.
Else `type` is the type lowercased, `fold` the sign or `""`, `title`
the title trimmed or `""`, `markFrom` where `[` is and `markTo` just
after `]` and its sign.

## Tests (given, do not edit)

`app/src/editor/syntax/math.test.ts`: inline math's delimiters, flanking,
escapes, one line, what it hides, code; block math's extent, one-line
blocks, interrupting a paragraph, unclosed, in a quote, ending with its
quote, indented code; callouts' type, fold and title, what is not one,
lists and nested quotes, other nodes.

## Gotchas

- Whether a line is still in the block's containers is `Line.depth`
  (`line.depth >= cx.depth`), there at runtime though not in
  `@lezer/markdown`'s types: read it through a cast with a comment
  saying so, as `frontmatter.ts` reads the context's `input`.
- `cx.nextLine()` moves the parser to the next line and returns false
  at the document's end; after a one-line block, move past it once.
- In `readCallout`, `quote.getChild("QuoteMark")` is the first line's
  mark; `state.doc.lineAt(pos)` gives its line.

## Out of scope

Drawing math (KaTeX) or callouts (live preview), comments (T-0067),
and every file except the two under `touch`.

## Done when

`make accept T=0066`, `make check` and `make ui-check` pass, and only
the two files under `touch` changed.
