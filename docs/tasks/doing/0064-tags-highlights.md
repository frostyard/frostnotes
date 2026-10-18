---
id: "0064"
title: Parse tags and highlights in the editor
milestone: M3
size: S
touch:
  - app/src/editor/syntax/tags.ts
  - app/src/editor/syntax/highlights.ts
given:
  - app/src/editor/syntax/tags.test.ts
acceptance: make ui-vitest F=src/editor/syntax/tags.test.ts
---
# T-0064: Parse tags and highlights in the editor

## Goal

The editor's parser (`@lezer/markdown` with GFM) names Obsidian's syntax
so live preview can draw it and the app can find what notesd finds. Add
two extensions: inline `#tags`, which must agree with notesd's parser on
the spec's fixtures, and `==highlights==`.

## Read first

- `app/src/editor/syntax/tags.ts` and `highlights.ts`: the stubs, already
  registered in `OBSIDIAN` (`syntax/index.ts`).
- `app/src/editor/syntax/wikilink.ts`: an extension written the same way.
- `docs/specs/markdown.md`, "Tags" and "Callouts, highlights and
  footnotes".
- `internal/markdown/obsidian.go`, `tagParser`: notesd's rule, which the
  fixtures hold both parsers to.
- `app/src/editor/syntax/extract.ts`, `findTags`: reads your nodes.
- `@lezer/markdown`'s `Strikethrough` extension
  (`app/node_modules/@lezer/markdown/dist/index.js`): highlights are the
  same with `=`.
- The given test, and `app/src/editor/syntax/fixtures.test.ts`, which
  runs F05, F06, F08, F09 and F12 once a `Hashtag` node exists.

## Contract

Keep each file's exported name and type (`Hashtag` and `Highlight`, each
a `MarkdownConfig`); replace each file's "Task T-0064 …" sentence with a
description.

**`Hashtag`** defines one node, `Hashtag` (style `t.labelName` from
`@lezer/highlight`), spanning the `#` and the name, with no children. An
inline parser named `Hashtag` adds one at a `#` when:

- the `#` is at the start of the inline section (`pos === cx.offset`: a
  paragraph's, heading's, list item's, quote's or table cell's text) or
  right after a white space character (`/\s/`);
- one or more of these follow it, up to the first other character or
  the end of the line: Unicode letters (`\p{L}`), marks (`\p{M}`),
  decimal digits (`\p{Nd}`), `_`, `-` and `/`. Match code points, not
  UTF-16 units (`𝒜` is one letter);
- the name is not all decimal digits (`#123` is no tag; `#2026-10` is).

Otherwise the parser returns -1. Escapes, code, links and URLs need no
code of yours: the escape and code parsers take their text first, a
wikilink is parsed whole, and `findTags` skips tags inside a `Link` or
`Image`.

**`Highlight`** defines `Highlight` (style
`{ "Highlight/...": t.special(t.content) }`) and `HighlightMark` (style
`t.processingInstruction`), and an inline parser named `Highlight`,
`after: "Emphasis"`, that adds a delimiter for `==` (not `===`) exactly
as `Strikethrough` does for `~~`: it opens when not followed by white
space and closes when not preceded by it, with the same punctuation
rule (`/[\p{S}\p{P}]/u`). Lezer resolves the pairs into a `Highlight`
holding two `HighlightMark`s and the parsed text between.

## Tests (given, do not edit)

`app/src/editor/syntax/tags.test.ts`: where tags start, the blocks they
count in, their characters, where they end, all digits, a lone `#`,
what is not a tag; highlights' nodes, their content, lines, flanking
and code. `fixtures.test.ts` runs too: with your `Hashtag`, F05, F06,
F08, F09 and F12 must pass.

## Gotchas

- `cx.slice(from, to)` and `cx.char(pos)` take document positions;
  `cx.end` is the section's end, and a section can span lines, so stop
  a name at a line break.
- A regex with `\p{…}` needs the `u` flag; anchoring it at the start of
  a slice is the simplest way to take a name.
- A heading's `#` marks are not inline text, so `# Title` holds no tag
  and `## Heading with #tag` holds one.

## Out of scope

Drawing tags or highlights (live preview, batch 3's next PR), the
editor's other extensions, `extract.ts`, and every file except the two
under `touch`.

## Done when

`make accept T=0064`, `make check` and `make ui-check` pass, and only
the two files under `touch` changed.
