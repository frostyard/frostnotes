---
id: "0004"
title: Resolve link targets as Obsidian does
milestone: M1
size: S # S: at most 150 non-test lines in 3 files; M: 400 lines in 6 files; L: 1,000 lines, one package or feature (ADR-0010).
touch:
  - internal/markdown/resolve.go
given:
  - internal/markdown/resolve_test.go # copied from 0004-resolve-links/_given/ when the task starts
acceptance: go test ./internal/markdown -run 'TestResolve' -count=1
---
# T-0004: Resolve link targets as Obsidian does

## Goal

Add `markdown.Resolve`: given the note a link is in and the link's target,
find the vault file it names, by the rules in
`docs/specs/markdown.md#link-resolution`. The index uses it to store each
link's file, so backlinks, the graph and unresolved links match Obsidian.
A reference implementation of these rules agreed with all 179 links
Obsidian resolved in the user's vault (plan 0003).

## Read first

- docs/tasks/EXECUTOR.md
- docs/specs/markdown.md, section "Link resolution": rules 1–7, "Ties",
  the fixture vault and the fixture cases. The given test reads the vault
  and the cases from that file, so the table is the test.
- internal/markdown/resolve_test.go (given).
- internal/markdown/scan.go: the package's style (no package doc comment
  here; `frontmatter.go` has it).

## Contract

File `internal/markdown/resolve.go`, package `markdown`:

```go
// Resolve finds the file a link names, by Obsidian's rules
// (docs/specs/markdown.md#link-resolution). source is the linking note's
// vault path and target the link's Target. files returns every vault path
// whose last segment, lower-cased, equals name. Resolve returns the path
// as files spells it, or "" when the link is unresolved.
func Resolve(source, target string, files func(name string) []string) string
```

Vault paths are slash-separated, relative to the vault root, with no
leading `/`. Lower-casing is `strings.ToLower`; use `path`, never
`path/filepath`.

1. `target == ""`: return `source` without calling `files`.
2. A leading `/` marks the target rooted; remove all leading slashes. The
   names are `[t]` when `strings.ToLower(t)` ends in `.md`, else
   `[t + ".md", t]`, each passed through `path.Clean`. A name that is `..`
   or starts with `../` (after joining, in step 4) matches nothing.
3. Rooted: the first name, in order, with a file whose lower-cased path
   equals the lower-cased name wins; no match is "".
4. Relative: the same, with each name joined to `path.Dir(source)`
   (`path.Join`).
5. If the target as written starts with `./` or `../`, return "".
6. Suffix: for each name in order, the matches are the files whose
   lower-cased path equals the lower-cased name or ends with `"/"` plus it.
   The first name with any match decides, by the order under "Ties".
7. Otherwise "".

Ties: in steps 3 and 4, a file equal to the wanted path byte for byte
wins, then the smallest path in byte order (`strings.Compare`). In step 6,
compare, in order: in the source's folder (`path.Dir(f) == path.Dir(source)`)
first; then a file that equals the name or ends with `"/"` plus the name
byte for byte first; then fewer `/` in the path first; then byte order.

Ask `files` only for `strings.ToLower(path.Base(name))` of a cleaned name
(or of the joined path in step 4, which has the same last segment).

## Tests (given, do not edit)

`internal/markdown/resolve_test.go`: `TestResolveSpecFixtures` (the 35
cases from the spec), `TestResolveIgnoresLookupOrder`,
`TestResolveAsksForLowercaseNames`, `TestResolveEmptyTargetNeedsNoLookup`,
`TestResolveKeepsTheFilesSpelling`.

## Gotchas

- `path.Dir("Inbox.md")` is `"."`, and `path.Join(".", "Plan.md")` is
  `"Plan.md"`; `path.Join(".", "../Plan.md")` is `"../Plan.md"`, which
  leaves the vault.
- Step 5 checks the target as written (after removing a leading `/`), not
  the cleaned name: `path.Clean("./Overview.md")` has lost its `./`.
- A suffix is whole segments: `Block/Soil.md` must not match
  `North Block/Soil.md`. That is why the test is `"/"` plus the name.
- `files` may return its paths in any order; sort, never take the first.
- Never call `files` with a name that has folders in it or capitals.

## Out of scope

Parsing links, percent-decoding, subpaths, aliases, the index, and every
file other than `internal/markdown/resolve.go`. Do not edit the spec.

## Done when

`make accept T=0004` passes, `make check` passes, and only the files under
touch changed.
