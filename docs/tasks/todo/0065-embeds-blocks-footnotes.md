---
id: "0065"
title: Parse embeds, block IDs and footnotes in the editor
milestone: M3
size: M
touch:
  - app/src/editor/syntax/embeds.ts
  - app/src/editor/syntax/blockIds.ts
  - app/src/editor/syntax/footnotes.ts
given:
  - app/src/editor/syntax/embeds.test.ts
acceptance: make ui-vitest F=src/editor/syntax/embeds.test.ts
---
# T-0065: Parse embeds, block IDs and footnotes in the editor

## Goal

Three more of Obsidian's syntaxes for the editor's parser
(`@lezer/markdown` with GFM): `![[embeds]]`, `^block-ids` and GFM
footnotes. Embeds and block IDs must agree with notesd's parser on the
spec's fixtures; footnotes are for live preview.

## Read first

- `app/src/editor/syntax/embeds.ts`, `blockIds.ts`, `footnotes.ts`: the
  stubs, already registered in `OBSIDIAN` (`syntax/index.ts`).
- `app/src/editor/syntax/wikilink.ts`: `wikiLinkElement(cx, pos)` builds
  the `WikiLink` element whose `[[` is at `pos`, or null.
- `app/src/editor/syntax/extract.ts`: `findLinks` reads an `Embed` around
  a `WikiLink`; `findBlocks` reads `BlockId` nodes and applies the rule
  for a block ID that stands alone.
- `docs/specs/markdown.md`, "Wikilinks and embeds", "Block IDs" and
  "Callouts, highlights and footnotes".
- `@lezer/markdown`'s types (`app/node_modules/@lezer/markdown/dist/index.d.ts`):
  `BlockParser.leaf`, `LeafBlockParser`, `LeafBlock`,
  `BlockContext.addLeafElement`, `MarkdownParser.parseInline`; and in
  `index.js`, `LinkReferenceParser` and `SetextHeadingParser`, leaf
  parsers like the one you write.
- The given test, and `app/src/editor/syntax/fixtures.test.ts`, which
  runs F01 once `Embed` exists and F10 once `BlockId` does.

## Contract

Keep each file's exported name and type (`Embed`, `BlockId` and
`Footnote`, each a `MarkdownConfig`); replace each file's "Task T-0065 …"
sentence with a description.

**`Embed`** defines `Embed` (no style) and `EmbedMark` (style
`t.processingInstruction`). An inline parser named `Embed`,
`before: "Image"`, at a `!` whose next character starts a wikilink
(`wikiLinkElement(cx, pos + 1)` is not null) adds an `Embed` from the
`!` to the link's end with two children: an `EmbedMark` over the `!`,
then the `WikiLink` element. Otherwise -1, so `![alt](x.png)` stays an
`Image`.

**`BlockId`** defines `BlockId` (style `t.meta`), over the `^` and the
ID, no children. An inline parser named `BlockId` adds one at a `^`
when the `^` is at the start of the inline section or right after white
space (`/\s/`), one or more ASCII letters, digits or `-` follow, and
only white space follows them to the section's end (`cx.end`).
`findBlocks` keeps those in a `Paragraph`, and one that is its
paragraph's whole text only right after a list, quote or table.

**`Footnote`** defines `FootnoteReference` (style `t.labelName`),
`FootnoteDefinition` (`block: true`), `FootnoteMark` (style
`t.processingInstruction`) and `FootnoteLabel` (style `t.labelName`). A
label is one or more characters other than white space, `[` and `]`.

- An inline parser named `FootnoteReference`, `before: "Link"`, adds at
  `[^label]` a `FootnoteReference` holding a `FootnoteMark` over `[^`, a
  `FootnoteLabel`, and a `FootnoteMark` over `]`.
- A block parser named `FootnoteDefinition`,
  `before: "LinkReference"`, with only a `leaf` hook: a paragraph whose
  text starts `[^label]:` becomes a `FootnoteDefinition` over its first
  line only: a `FootnoteMark` over `[^`, the `FootnoteLabel`, a
  `FootnoteMark` over `]:`, then the rest of the line parsed as inline
  content (`cx.parser.parseInline(text, offset)`). Its leaf parser
  finishes the block at the paragraph's second line (`nextLine` adds the
  element for the first line and returns true, which leaves the second
  line to start a new block) or at its end (`finish`). It is not a link
  reference.

## Tests (given, do not edit)

`app/src/editor/syntax/embeds.test.ts`: embed nodes and links, in a
table cell, what is not an embed; block IDs at a paragraph's or item's
end, alone after a list, quote or table, and not elsewhere; footnote
references, labels, definitions and their inline content, and link
references left working. `fixtures.test.ts` runs too: F01 and F10 must
pass.

## Gotchas

- `cx.elt(type, from, to, children)` builds an element;
  `cx.addElement(elt)` adds it and returns its end, the parser's return
  value.
- A `LeafBlockParser` can be a plain object; `leaf.start` is the
  paragraph's position and `leaf.content` its text so far.
- `BlockContext.addLeafElement(leaf, elt)` adds a leaf's element with
  any quote marks inside it.

## Out of scope

Drawing embeds, block IDs or footnotes (live preview), `extract.ts`
and `wikilink.ts` (the planner's), and every file except the three
under `touch`.

## Done when

`make accept T=0065`, `make check` and `make ui-check` pass, and only
the three files under `touch` changed.
