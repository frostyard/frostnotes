---
id: "0001"
title: Split a note at its frontmatter, byte for byte
milestone: M0
size: S # S: at most 150 non-test lines in 3 files; M: 400 lines in 6 files; L: 1,000 lines, one package or feature (ADR-0010).
touch:
  - internal/markdown/frontmatter.go
given:
  - internal/markdown/frontmatter_test.go # copied from 0001-split-frontmatter/_given/ when the task starts
acceptance: go test ./internal/markdown -run 'TestSplitFrontmatter' -count=1
---
# T-0001: Split a note at its frontmatter, byte for byte

## Goal

Create package `internal/markdown` with `SplitFrontmatter`, which finds a
note's YAML frontmatter and the body after it without copying or changing a
byte. The index (M1) reads properties from the front and links from the
body, and writes (ADR-0006) rewrite one part while the other keeps its exact
bytes. This is an M0 calibration card for the executor (plan 0001, Phase 6).

## Read first

- docs/tasks/EXECUTOR.md
- docs/adr/0003-plain-markdown-files-are-the-source-of-truth.md: notes are
  written byte for byte.
- docs/design/vault.md, section "Parse".
- internal/markdown/frontmatter_test.go (given): every rule below is a case.

## Contract

File `internal/markdown/frontmatter.go`, package `markdown`, with the
package doc comment `// Package markdown parses notes (docs/design/vault.md#parse).`

```go
// Split is a note divided at its frontmatter.
type Split struct {
	HasFront  bool   // the note has frontmatter
	Front     []byte // the YAML between the delimiter lines, with its line endings; empty without frontmatter
	Body      []byte // everything after the closing delimiter line; the whole note without frontmatter
	BodyStart int    // the byte offset of Body in the note
}

// SplitFrontmatter finds note's frontmatter.
func SplitFrontmatter(note []byte) Split
```

Rules:

- Frontmatter opens only on the note's first line, which must be exactly
  `---` followed by `\n` or `\r\n`. A UTF-8 byte order mark (`EF BB BF`)
  before it is allowed and is not part of `Front`.
- It closes at the first later line that is exactly `---` or `...`
  (before `\n`, `\r\n` or the end of the note). Lines with leading spaces or
  other text do not close it.
- `Front` is the bytes between the opening line's ending and the closing
  line's start. `Body` starts after the closing line's ending (or at the end
  of the note when the closing line has none), and `BodyStart` is its
  offset.
- Without an opening line, or when frontmatter is never closed:
  `HasFront` is false, `Front` is empty, `Body` is the whole note and
  `BodyStart` is 0.
- `Front` and `Body` are subslices of `note` (no copies).

## Tests (given, do not edit)

`internal/markdown/frontmatter_test.go`: `TestSplitFrontmatter` (17 cases)
and `TestSplitFrontmatterAliasesTheNote`.

## Gotchas

- Do not parse YAML. This card only finds the boundaries.
- A value containing `---` on an indented line, or a line like `---x`, is
  not a closing delimiter.
- `\r\n` endings: the delimiter test ignores the `\r`, but `Front` keeps
  every byte, `\r` included.
- A note that is only `---\n` has no frontmatter.

## Out of scope

Parsing the YAML, tags, links, or anything in the body. Other files in
`internal/markdown` (a later card adds them).

## Done when

`make accept T=0001` passes, `make check` passes, and only the files under
touch changed.
