---
id: "0002"
title: Scan a note body for wikilinks and tags
milestone: M0
size: S # S: at most 150 non-test lines in 3 files; M: 400 lines in 6 files; L: 1,000 lines, one package or feature (ADR-0010).
touch:
  - internal/markdown/scan.go
given:
  - internal/markdown/scan_test.go # copied from 0002-scan-wikilinks-and-tags/_given/ when the task starts
acceptance: go test ./internal/markdown -run 'TestWikilinks|TestInlineTags' -count=1
---
# T-0002: Scan a note body for wikilinks and tags

## Goal

Add `Wikilinks` and `InlineTags` to `internal/markdown`: the links and tags
the index records for backlinks and the tag pane (M1), found the way
Obsidian finds them and never inside code. This is an M0 calibration card
for the executor (plan 0001, Phase 6). It runs after T-0001, whose
`frontmatter.go` holds the package's doc comment.

## Read first

- docs/tasks/EXECUTOR.md
- docs/adr/0003-plain-markdown-files-are-the-source-of-truth.md: the
  Obsidian dialect.
- docs/design/vault.md, section "Parse".
- internal/markdown/scan_test.go (given): every rule below is a case.

## Contract

File `internal/markdown/scan.go`, package `markdown` (no package doc
comment; `frontmatter.go` has it). Both functions take a note's body (the
text after frontmatter) and never modify it.

```go
// Link is a wikilink or wiki embed in a note's body.
type Link struct {
	Target  string // the note or file named: "Folder/Note" in [[Folder/Note#Goals|Plan]]; "" for a heading of the same note
	Subpath string // "#Goals" or "#^block-id", starting with '#'; "" when none
	Alias   string // the text after the first '|'; "" when none
	Embed   bool   // written ![[...]]
	Start   int    // byte offset of "[[", or of "!" for an embed
	End     int    // byte offset just after "]]"
}

// Wikilinks returns the body's wikilinks and embeds in order.
func Wikilinks(body []byte) []Link

// InlineTags returns the body's #tags without the '#', each once, in order.
func InlineTags(body []byte) []string
```

Code, where neither function looks:

- A fenced code block opens on a line whose first non-space character, after
  at most 3 spaces of indentation, starts a run of 3 or more backticks or
  tildes (an info string may follow). It closes on a later line that starts,
  after at most 3 spaces, with a run of the same character at least as long
  as the opening run, followed only by spaces. An unclosed fence runs to the
  end of the body.
- An inline code span, outside fences, is a run of N backticks closed by the
  next run of exactly N backticks on the same line. A run with no matching
  run on its line is plain text.

Wikilinks:

- `[[`, then one or more bytes that are none of `[`, `]` or `\n`, then
  `]]`. A `!` right before `[[` makes it an embed and moves `Start` to the
  `!`.
- The inner text splits at the first `|` into target part and `Alias`; the
  target part splits at the first `#` into `Target` and `Subpath` (which
  keeps its `#`). `Target`, `Alias` and the text after `#` are trimmed of
  surrounding spaces.
- A link whose `Target`, `Subpath` and `Alias` are all empty (`[[]]`) is not
  a link.
- Scanning continues after a link's `]]`.

Tags:

- A `#` at the start of the body or right after a Unicode space (space, tab,
  newline), followed by one or more Unicode letters, digits, `_`, `-` or `/`.
  The tag ends at the first other character.
- All digits (`#123`) is not a tag; `#2026-10` and `#v2` are.
- Not inside code, and not inside a wikilink's `Start:End`.
- Each tag is returned once, compared case-insensitively, with the spelling
  seen first.

## Tests (given, do not edit)

`internal/markdown/scan_test.go`: `TestWikilinks` (21 cases) and
`TestInlineTags` (14 cases).

## Gotchas

- Offsets are bytes, not runes: `[[Café ☕]]` ends at 13.
- `# Heading` is not a tag (a space follows the `#`), but `#Heading` with no
  space is.
- A fence closes only with the same character: a ``` line inside a `~~~`
  fence is content.
- Do not add a markdown library; the module has none and must not gain one.

## Out of scope

Markdown links (`[text](path)`), frontmatter `tags`, `%%comments%%`, HTML,
headings and block IDs. `frontmatter.go`.

## Done when

`make accept T=0002` passes, `make check` passes, and only the files under
touch changed.
