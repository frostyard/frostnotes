---
id: "0012"
title: Parse markdown links, headings, block IDs, tasks, words
milestone: M1
size: M # S: at most 150 non-test lines in 3 files; M: 400 lines in 6 files; L: 1,000 lines, one package or feature (ADR-0010).
touch:
  - internal/markdown/structure.go
given:
  - internal/markdown/structure_test.go # copied from 0012-parse-structure/_given/ when the task starts
acceptance: go test ./internal/markdown -count=1
---
# T-0012: Parse markdown links, headings, block IDs, tasks and words

## Goal

The last parser card: from goldmark's tree, collect the markdown links to
vault files, the headings, block IDs and tasks, and count the words. With
T-0010 and T-0011 merged, `Parse` then returns everything
docs/specs/markdown.md's "Parse output" lists, and the given test runs
every spec fixture against the whole `Doc`. Runs after T-0011.

## Read first

- docs/tasks/EXECUTOR.md
- docs/specs/markdown.md: "Markdown links to vault files", "Headings",
  "Block IDs", "Tasks", "Words", and all the parse fixtures.
- internal/markdown/parse.go: `source` (`body`, `base`, `line`,
  `inComment`, `linkEnds`) and `newParser`, whose link parser records
  where each link ends in `s.linkEnds`.
- internal/markdown/obsidian.go (T-0011): how a collector walks the tree
  and skips comments.
- goldmark v1.8.6: `ast/inline.go` (`Link`, `Image`, `Text`
  `SoftLineBreak`), `ast/block.go` (`Heading`, `ListItem`, `Paragraph`,
  `TextBlock`), `extension/ast/table.go` (`Table`), `util/util.go`
  (`UnescapePunctuations`, `ResolveNumericReferences`,
  `ResolveEntityNames`).
- internal/markdown/structure_test.go (given).

## Contract

Replace the stub in `internal/markdown/structure.go`:

```go
func collectStructure(d *Doc, root ast.Node, s *source)
```

Walk the tree once and append, in tree order, only what does not start in
a comment (`s.inComment` of the note offset):

- **Markdown links.** Each `*ast.Link` and `*ast.Image` whose
  `Destination` is not empty, has no URL scheme
  (`^[A-Za-z][A-Za-z0-9+.-]*:`) and does not start with `//`. Resolve the
  destination as goldmark's renderer does (`UnescapePunctuations`, then
  `ResolveNumericReferences`, then `ResolveEntityNames`), split it at the
  first `#` into `Target` and `Subpath` (with its `#`), and percent-decode
  each with `url.PathUnescape`, keeping the text as it was when that
  fails. `Alias` is the label's plain text: the bytes of its `*ast.Text`
  descendants, a soft line break as a space, trimmed. `Embed` is true for
  an image. `Start` is the node's `Pos()` plus `s.base` (the `[`, or an
  image's `!`), `End` is `s.linkEnds[node]` plus `s.base`, and `Kind` is
  `LinkMarkdown`. Autolinks (`*ast.AutoLink`) are never links.
- **Headings.** Each `*ast.Heading`: `Level`; `Text` is its lines' source
  (`seg.Value(s.body)`), each trimmed of white space, joined with one
  space; `Line` is its first line's, or its `Pos()`'s for an empty
  heading.
- **Tasks.** Each `*ast.ListItem` whose first child is a `*ast.Paragraph`
  or `*ast.TextBlock` whose first line's source matches
  `^\[([^\n\r])\][ \t]`: `Status` is the captured character, `Text` the
  rest of that line trimmed, `Line` the line's. goldmark's task-list
  extension does not change the source, so `[/]` and `[ ]` read alike.
- **Block IDs.** For each `*ast.Paragraph` and `*ast.TextBlock`, take the
  source from its first line's start to its last line's stop, trailing
  white space trimmed. Its ID is a match of `\s\^([A-Za-z0-9-]+)$`, or,
  when its whole text matches `^\^([A-Za-z0-9-]+)$` and its previous
  sibling is an `*ast.List`, `*ast.Blockquote` or table
  (`*extension/ast.Table`), that. `Line` is the line of the `^`.
- **Words.** `d.Words` counts, over the note bytes from `s.base` on, the
  maximal runs of runes that are not white space (`unicode.IsSpace`) and
  not in a comment, holding at least one letter or digit
  (`unicode.IsLetter`, `unicode.IsDigit`). Decode with `utf8.DecodeRune`;
  an invalid byte is a rune that is neither space nor letter.

## Tests (given, do not edit)

`internal/markdown/structure_test.go`: `TestParseSpecFixtures` (every
fixture, every field), `TestParseMarkdownLinks`, `TestParseLinksInOrder`,
`TestParseHeadings`, `TestParseTasks`, `TestParseBlockIDs`,
`TestParseWords`. The acceptance command runs the whole package, so
T-0010's and T-0011's tests must keep passing.

## Gotchas

- goldmark keeps a destination's backslash escapes and entities as
  written (`a\_b` stays `a\_b`); resolve them before splitting at `#`.
- A link's label may hold emphasis and code: walk its descendants for
  `*ast.Text`, not just its children.
- A setext heading's `Lines()` are its paragraph's lines; an ATX
  heading's one segment excludes the `#`s and a closing sequence.
- Line segments may include `\r\n` or `\n`: trim before matching ends.
- A lone `^id` after a paragraph is not a block ID; after a list, quote or
  table it is. Blank lines between them do not matter: they are not
  nodes.
- Count words over bytes, not over the tree: code and markup count as
  written; only comments do not.

## Out of scope

Wikilinks, tags, math and comments (T-0011), frontmatter (T-0010),
checking that a subpath's heading or block exists, and every file other
than `structure.go`.

## Done when

`make accept T=0012` passes, `make check` passes, and only the files under
touch changed.
