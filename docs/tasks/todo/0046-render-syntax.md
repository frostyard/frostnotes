---
id: "0046"
title: Render tags, highlights, math, callouts and lines
milestone: M3
size: M
touch:
  - internal/render/syntax.go
given:
  - internal/render/syntax_test.go
acceptance: go test ./internal/render -run 'TestSyntaxFixtures' -count=1
---
# T-0046: Render tags, highlights, math, callouts and lines

## Goal

The reading view draws the rest of Obsidian's syntax as notesd renders
it (`docs/specs/render.md`, "Obsidian's syntax"): tags, `==highlights==`,
math for KaTeX, callouts, task checkboxes and headings that carry their
note line (so the app can toggle a task and scroll to a heading), and
block IDs that leave the text for a `data-block` attribute.

## Read first

- `docs/specs/render.md`: "Interface", "Obsidian's syntax", and every
  fixture (R01 to R18; yours are tagged `[syntax]`, and T-0045's
  `[links]` must keep passing).
- `internal/render/render.go`: the `obsidianHTML` extension, `lineOf`
  (a source offset's note line), `lineless` (true in an embedded note),
  the fallback renderer.
- `internal/render/links.go` (T-0045's): `attr` and `writeText`, which
  you may call.
- `internal/render/syntax.go`: the stub `extendSyntax`.
- `internal/markdown/render.go`: `TagKind`, `TagOf`, `InlineMathKind`,
  `MathBlockKind`, `MathOf`; `docs/specs/markdown.md#block-ids`.
- goldmark's `extension/strikethrough.go` (a delimiter parser to copy for
  `==`), `extension/tasklist.go` (`ast.TaskCheckBox`) and
  `renderer/html/html.go` (`RenderAttributes` writes `data-` attributes
  set on a node).
- The given test `internal/render/syntax_test.go`.
- `docs/tasks/EXECUTOR.md`.

## Contract

Keep `extendSyntax`'s signature and doc comment; everything else in
`syntax.go` is yours. Every fixture in the spec must render exactly as
shown, byte for byte.

- **Register** in `extendSyntax`: an inline parser for highlights, an AST
  transformer, and node renderers at priority 100 (the fallback's is 900)
  for `markdown.TagKind()`, `markdown.InlineMathKind()`,
  `markdown.MathBlockKind()`, your highlight and callout kinds, and
  goldmark's `ast.KindTaskCheckBox` (from `extension/ast`).
- **Tags:** `<a class="tag" data-tag="NAME">#NAME</a>`.
- **Highlights:** a delimiter parser like goldmark's strikethrough, for
  `=`, accepting a run of exactly two not preceded by `=`, its node drawn
  as `<mark>` around its children.
- **Math:** `<span class="math" data-tex="TEX"></span>` inline; `<div
  class="math" data-tex="TEX"></div>` and a newline for a block; `TEX`
  from `markdown.MathOf`.
- **Callouts** (in the transformer): a blockquote whose first child is a
  paragraph whose first line matches
  `^\[!([A-Za-z0-9_-]+)\]([+-]?)(?:[ \t]+(.*?))?[ \t]*$` becomes a callout
  node: its type the first group lower-cased, its fold the second, its
  title the third as plain text, or the type with its first letter
  upper-cased. Drop the paragraph's inline children up to and including
  the text node that ends the first line (a soft or hard line break); drop
  the paragraph if nothing is left; the blockquote's other children
  become the callout's. Draw it as `<div class="callout"
  data-callout="TYPE"`, then ` data-fold="+"` or `"-"` when there is one,
  then ` data-block="ID"` when the blockquote had one, `>`, a newline,
  `<div class="callout-title">TITLE</div>`, a newline, `<div
  class="callout-content">`, a newline, the children, then
  `</div>\n</div>\n`. A blockquote without the marker stays a blockquote.
- **Task checkboxes:** `<input type="checkbox" disabled=""`, then `
  checked=""` when checked, then ` data-line="N"` (the note line of the
  checkbox's paragraph's first line, `e.lineOf`), `> ` (a space after).
  In an embedded note (`e.lineless()`), no `data-line`.
- **Headings** (in the transformer): set the attribute `data-line` to
  the note line of the heading's first line, except in an embedded note;
  goldmark's heading renderer writes it.
- **Block IDs** (in the transformer, before callouts): for each paragraph
  and list item's text block whose last child is a text node,
  - when that text ends with white space, `^`, then ASCII letters, digits
    or `-` (and trailing white space), cut that from the text and set
    `data-block` to the ID, on the paragraph, or for a list item's text
    block on its list item;
  - when that text node is a lone `^ID` after a line break in the same
    paragraph, remove it, clear the break before it, and set `data-block`
    as above;
  - when the whole paragraph is a lone `^ID` right after a list, a
    blockquote or a table, remove the paragraph and set `data-block` on
    that block.
- No mutable package-level variables beyond node kinds and compiled
  regexps.

## Tests (given, do not edit)

`internal/render/syntax_test.go`: `TestSyntaxFixtures`, which runs every
fixture of the spec through `HTML`.

## Gotchas

- Transform block IDs before callouts, so `^q1` after a quote reaches
  the callout's `data-block` when the quote becomes a callout.
- A callout's `[!type]` is plain text to goldmark (no link starts with
  `[!`); read the marker from the paragraph's first line segment, not
  from the inline nodes.
- goldmark's blockquote renderer writes no newline after `<blockquote
  data-block="…">`: the fixtures show that, and you need not fix it.
- Embedded notes render through the same extension, so headings and
  checkboxes there must leave `data-line` out.

## Out of scope

Links and embeds (T-0045), sanitizing (Phase 4), Obsidian's custom task
statuses (`[/]`, `[-]`: GFM's checkboxes only), and every file except
`internal/render/syntax.go`.

## Done when

`make accept T=0046` and `make check` pass, and only
`internal/render/syntax.go` changed.
