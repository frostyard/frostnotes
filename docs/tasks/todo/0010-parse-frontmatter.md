---
id: "0010"
title: Parse a note's frontmatter
milestone: M1
size: M # S: at most 150 non-test lines in 3 files; M: 400 lines in 6 files; L: 1,000 lines, one package or feature (ADR-0010).
touch:
  - internal/markdown/front.go
given:
  - internal/markdown/front_test.go # copied from 0010-parse-frontmatter/_given/ when the task starts
acceptance: go test ./internal/markdown -run 'TestParseFront|TestParseProperty' -count=1
---
# T-0010: Parse a note's frontmatter

## Goal

`markdown.Parse` reads a whole note into a `Doc` for the index
(docs/specs/markdown.md, "Parse output"). The planner wrote `Parse`, the
`Doc` types and an empty `parseFront`. This card fills it in: the
properties, a property error, aliases, frontmatter tags and the wikilinks
in property values. It is the first of three parser cards: T-0011 (the
Obsidian syntax in the body) and T-0012 (links, headings, block IDs,
tasks and words) follow.

## Read first

- docs/tasks/EXECUTOR.md
- docs/specs/markdown.md: "Notes and files" (positions), "Frontmatter",
  "Parse output", and fixtures F06, F07 and F08.
- internal/markdown/parse.go: `Doc`, `Link`, `Tag`, `Parse`, `source`
  (`s.note`, `s.line`), `wikilinkPattern` and `splitWikilink`. `Parse`
  calls `parseFront` first, then sorts the links by `Start` and keeps each
  tag once (`finish`), so you do neither.
- internal/markdown/frontmatter.go: `SplitFrontmatter` and `Split`.
- internal/markdown/fixtures_test.go: how the given test reads the spec's
  fixtures and compares parts of a `Doc`.
- internal/markdown/front_test.go (given).

## Contract

Replace the stub in `internal/markdown/front.go`:

```go
func parseFront(d *Doc, s *source, split Split)
```

Without frontmatter (`!split.HasFront`), it does nothing. Otherwise,
decode `split.Front` with `go.yaml.in/yaml/v3` into a `yaml.Node`:

- **Errors.** YAML that does not parse, a document that is not a mapping,
  or a key that appears twice is a property error: set `d.PropError` to an
  error whose message says why (`frontmatter: …`; for a repeated key it
  names the key), and set nothing else: `Props`, `Aliases`, tags and links
  stay nil. An empty or comment-only frontmatter is an empty mapping:
  `d.Props = map[string]any{}`, no error.
- **Props** is the whole mapping as JSON-compatible values: a mapping is
  `map[string]any` (keys as written, whatever their YAML type), a
  sequence `[]any`, an alias the value it names, and a scalar by its
  resolved tag: `!!null` is nil, `!!bool` a bool, `!!int` an int64,
  `!!float` a float64 except infinities and NaN, which stay strings as
  written; every other scalar (`!!str`, `!!timestamp`, …) is its text as
  written (`n.Value`). So `d: 2026-10-16` is the string `"2026-10-16"`.
- **Line numbers.** A key's line in the note is the line of the
  frontmatter's first byte, plus the node's `Line` minus 1. The
  frontmatter starts after `---\n`, or `---\r\n`, after an optional BOM.
- **`tags`**: each item of a sequence, or the whole value of a scalar
  split at commas and white space (`strings.FieldsFunc`); only scalar
  items that are not null count, by their text as written (`2026` is
  `"2026"`); a leading `#` is dropped; empty names are skipped. Append each
  as `Tag{Name, Front: true, Line: <the tags key's line>}`.
- **`aliases`**: each non-null scalar item of a sequence, or the value of a
  scalar as one alias, as written; empty strings are skipped.
- **Property links**: for every other key, its value if it is a `!!str`
  scalar, or each `!!str` item of a sequence (nested mappings are not
  searched). Each match of `wikilinkPattern` in the decoded string whose
  inner text `splitWikilink` accepts is a `Link{Kind: LinkProperty,
  Target, Subpath, Alias, Embed: <the match starts with "!">, Key, Line:
  <the key's line>}`. `Start` and `End` cover the k-th match of
  `wikilinkPattern` in the note bytes at or after the value's position (its
  node's `Line` and `Column`, a column counting runes), where k is the
  match's index in the decoded string. Keys appear in YAML order.

## Tests (given, do not edit)

`internal/markdown/front_test.go`: `TestParseFrontFixtures` (the props,
frontmatter tags and property links of every spec fixture),
`TestParseFrontmatterProps`, `TestParseFrontmatterErrorKeepsNothing`,
`TestParseFrontmatterTags`, `TestParseFrontmatterAliases`,
`TestParsePropertyLinks`, `TestParsePropertyLinkOffsets`.

## Gotchas

- In YAML, `#` after white space starts a comment: `tags: a #b` has one
  tag. Only quoted strings can hold `#tag`.
- `yaml.Unmarshal` into a `yaml.Node` does not reject repeated keys;
  check them yourself while walking the mapping.
- `yaml.v3`'s `Column` counts characters: `café: "[[x]]"` puts the value
  at column 7 though `é` is two bytes.
- Decode a scalar's value with `n.Decode(&x)` into the Go type its tag
  names; never `fmt.Sprint` a decoded `any` (that turns `1.50` into
  `1.5`, and tags keep their text as written).
- The note may start with a BOM; `split.Front` excludes it and the
  delimiter line.

## Out of scope

The body (T-0011 and T-0012), sorting links and de-duplicating tags
(`finish` does it), property types from `types.json`, and every file
other than `front.go`.

## Done when

`make accept T=0010` passes, `make check` passes, and only the files under
touch changed.
