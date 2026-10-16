---
id: "0011"
title: Parse wikilinks, tags, math and comments with goldmark
milestone: M1
size: M # S: at most 150 non-test lines in 3 files; M: 400 lines in 6 files; L: 1,000 lines, one package or feature (ADR-0010).
touch:
  - internal/markdown/obsidian.go
  - internal/markdown/scan.go
  - internal/markdown/scan_test.go
given:
  - internal/markdown/obsidian_test.go # copied from 0011-obsidian-extension/_given/ when the task starts
acceptance: go test ./internal/markdown -run 'TestParseObsidian|TestParseWikilink|TestParseInlineTags|TestParseTagLines|TestParseHidden|TestFindComments' -count=1
---
# T-0011: Parse wikilinks, tags, math and comments with goldmark

## Goal

`markdown.Parse` parses a note's body with goldmark, so CommonMark decides
what is code: the regex scanner of T-0002 counted 93 links inside code
that Obsidian does not. This card writes the goldmark extension for the
Obsidian syntax (wikilinks and embeds, inline tags, math) and finds
comments, replacing `scan.go`. T-0002's 35 cases now run over `Parse` in
the given test. Runs after T-0010.

## Read first

- docs/tasks/EXECUTOR.md
- docs/specs/markdown.md: "Body" (where Obsidian syntax is not
  recognized, "Wikilinks and embeds", "Tags") and fixtures F01, F02, F03,
  F05, F12, F13 and F14.
- internal/markdown/parse.go: `Parse`, `source` (`base`, `line`,
  `inComment`, `comments`), `newParser` (which registers `obsidian{}`),
  `wikilinkPattern`, `splitWikilink`, `finish`.
- internal/markdown/obsidian.go: the three stubs you replace.
- internal/markdown/scan.go: T-0002's wikilink and tag rules, which you
  move into inline parsers, then delete with its test.
- goldmark v1.8.6 in the module cache (`go env GOMODCACHE`):
  `parser/code_span.go` and `parser/fcode_block.go` show an inline and a
  block parser; `parser/parser.go` the interfaces `InlineParser` and
  `BlockParser`; `ast/ast.go` `BaseInline`, `BaseBlock` and `NewNodeKind`;
  `text/reader.go` `PeekLine`, `Advance` and `PrecendingCharacter`.
- internal/markdown/obsidian_test.go (given).

## Contract

In `internal/markdown/obsidian.go`:

**`obsidian.Extend(m goldmark.Markdown)`** adds to `m.Parser()` inline
parsers for wikilinks (triggers `[` and `!`), tags (`#`) and inline math
(`$`), each at priority 199 (before goldmark's link parser at 200), and a
block parser for math blocks (`$`) at priority 690. Each makes a node
kind of its own (`ast.NewNodeKind("Frostnotes…")` in package-level
variables, which are constant after init).

- **Wikilink:** at `[[` or `![[`, match `^` + `wikilinkPattern` against
  `block.PeekLine()`; when it matches and `splitWikilink` accepts its inner
  text, advance past it and return a node holding a `Link{Kind: LinkWiki,
  Target, Subpath, Alias, Embed}` and its start and end in body offsets
  (the segment's `Start`). Otherwise return nil.
- **Tag:** only when `block.PrecendingCharacter()` is white space (goldmark
  reports the start of a block's text as `'\n'`). Match
  `^#[\p{L}\p{M}\p{Nd}_/-]+`; a name of digits only is not a tag. Advance
  and return a node with the name.
- **Inline math:** `$$…$$` closes at the next `$$` on the line; `$…$` opens
  only when the next byte is not a space, tab or line break, and closes at
  the next `$` on the line that is not escaped with `\` and not after a
  space or tab. No closer: return nil. The node covers both `$`s.
- **Math block:** a line whose text starts with `$$` (after at most three
  spaces: `pc.BlockOffset()`) opens a block; it is one line when the rest
  of that line holds `$$`, otherwise it runs through the next line that
  holds `$$`, or to the end of its container. It can interrupt a paragraph
  and never accepts an indented line. Its node's `IsRaw()` is true, so
  goldmark parses nothing inside, and its `Lines()` include its first line.

**`findComments(root ast.Node, s *source) [][2]int`** returns the
comments as sorted note-offset ranges `[start, end)`, `%%` included.
First collect the *literal ranges* where `%%` opens nothing: every
`FencedCodeBlock`, `CodeBlock`, `HTMLBlock` and math block (from its
`Pos()`, or its first line's start if earlier, to its last line's `Stop`),
every `CodeSpan` (from its `Pos()` to its last text child's `Stop`), every
`RawHTML` (its segments) and inline math node. Then scan the body bytes
for `%%`: outside a literal range one opens a comment; the next `%%`
anywhere closes it; an unclosed comment runs to the end of the note.

**`collectObsidian(d *Doc, root ast.Node, s *source)`** walks the tree and
appends, in tree order:

- each wikilink node not in a comment (`s.inComment(start)`), with
  `Start` and `End` in note offsets (plus `s.base`) and `Line` from
  `s.line(Start)`;
- each tag node not in a comment and not inside an `*ast.Link` or
  `*ast.Image`, as `Tag{Name, Line}`.

Delete `internal/markdown/scan.go` and `internal/markdown/scan_test.go`:
`Wikilinks`, `InlineTags` and their regex scanner go; their cases live in
the given test.

## Tests (given, do not edit)

`internal/markdown/obsidian_test.go`: `TestParseObsidianFixtures` (wiki
links and inline tags of every spec fixture), `TestParseWikilinks`
(T-0002's 21 cases and 10 more, with offsets),
`TestParseWikilinkOffsetsInTheNote`, `TestParseInlineTags` (T-0002's 14
and 11 more), `TestParseTagLines`, `TestParseHiddenSyntax` (math,
comments, callouts, highlights, footnotes), `TestFindComments`.

## Gotchas

- `block.PeekLine()` returns the rest of the current line from the
  trigger; never read past it for a wikilink or a tag.
- goldmark skips a backslash-escaped punctuation character before calling
  any parser, which is why `\[[x]]` and `\#x` need no code of yours.
- Node offsets from goldmark are body offsets; the note's are `+ s.base`.
- A tag inside `[link text](x.md)` is a tag node under an `*ast.Link`:
  skip it when collecting, not when parsing.
- In a table, `\|` reaches the wikilink parser as written: `splitWikilink`
  already drops the backslash.
- Comments are found after parsing, so a comment's contents are parsed
  like the rest; `collectObsidian` (and T-0012) drop what lies inside one.
- Keep each function under 60 lines; the block parser's `Open`,
  `Continue` and `Close` are three methods.

## Out of scope

Markdown links, headings, block IDs, tasks and words (T-0012), the
frontmatter (T-0010), rendering, and every file not under touch.

## Done when

`make accept T=0011` passes, `make check` passes, and only the files under
touch changed.
