---
id: "0069"
title: Complete links and tags in the editor
milestone: M3
size: M
touch:
  - app/src/editor/complete.ts
given:
  - app/src/editor/complete.test.ts
acceptance: make ui-vitest F=src/editor/complete.test.ts
---
# T-0069: Complete links and tags in the editor

## Goal

Typing `[[` lists the vault's files and notes' aliases, `[[note#` its
headings, `[[note#^` its block IDs, and a tag's `#` the vault's tags
(`docs/design/app.md`, Extensions). Write the two completion sources
over a catalog of what the vault holds; the planner fills the catalog
from the app's stores and notesd and adds the sources to the editor.

## Read first

- `app/src/editor/complete.ts`: the types and stubs.
- `app/src/lib/switcher.ts` (`switcherResults`, `stripMd`) and
  `app/src/lib/fuzzy.ts` (`prepareFuzzySearch`, `sortByScore`): the
  quick switcher's ranking, which links reuse.
- `@codemirror/autocomplete`'s `CompletionSource`, `CompletionContext`,
  `CompletionResult`, `Completion` and `pickedCompletion`.
- The given test: its texts show the cursor as `@`.

## Contract

Keep every exported name and signature; replace the file's
"Task T-0069 …" sentence with a description. `COMPLETION_LIMIT` (50) is
the most options either source lists. Results set `filter: false`: the
sources rank, CodeMirror does not filter again.

**Where nothing completes:** inside a node named `InlineCode`,
`FencedCode`, `CodeBlock`, `InlineMath`, `BlockMath`, `HTMLBlock` or
`CommentBlock` at or above
`syntaxTree(context.state).resolveInner(context.pos, -1)`: return null.

**`linkCompletion(catalog)`** reads the cursor's line up to the cursor;
it completes when that text ends with `[[` and a query holding no `[`,
`]` or `|`, else returns null. `from` is where the query starts, except
as below.

- A query with a `#`: the text before it is the target. After `#^`,
  list `catalog.blocks(target)` (awaited), `from` after the `^`, each
  option labelled with its ID, `detail` its line's text, and written as
  the ID. Else list `catalog.headings(target)`, `from` after the `#`,
  labelled and written as the heading's text. The text after the `#`
  (or `#^`) ranks them: blank lists them in their order, else the
  fuzzy matches of the text (or the ID) best first, both at most
  `COMPLETION_LIMIT`.
- Else files: a blank query lists the files in the catalog's order, a
  query ranks files and aliases with `switcherResults(files, query, [])`
  (at most 50). A file's option is labelled with its path without `.md`
  (`stripMd`) and written as `catalog.linkText(path)`; an alias's is
  labelled with the alias, `detail` its note's path without `.md`, and
  written as `linkText(path) + "|" + alias`.
- Choosing an option (its `apply` function) replaces `from` to the
  cursor with what it writes, adds `]]` unless the text after the
  cursor starts with `]]`, and puts the cursor after the `]]`, with the
  `pickedCompletion` annotation and `userEvent: "input.complete"`.

**`tagCompletion(catalog)`** completes when the cursor's line up to the
cursor ends with a `#` at the line's start or after white space, then
one or more tag characters (T-0064's: `\p{L}`, `\p{M}`, `\p{Nd}`, `_`,
`-`, `/`), else null. `from` is after the `#`. Options are the
catalog's tags ranked by the fuzzy match of the query, best first,
equal scores by count (most first) then name; labelled and written
(`apply`, a string) as the tag's name, `detail` its count.

## Tests (given, do not edit)

`app/src/editor/complete.test.ts`: files in order and ranked, aliases,
what is written and the closing brackets, headings of a note and of
the edited note, block IDs, an unknown note, the limit, where links do
not complete; tags ranked, written, and where they do not complete.

## Gotchas

- `switcherResults` with a blank query lists only the `recent` paths
  you pass, in that order.
- The catalog's `headings` and `blocks` are promises: the link source
  is async.
- `context.state.doc.lineAt(context.pos)` gives the line; slice its
  text up to `context.pos - line.from`.

## Out of scope

The catalog's data and adding the sources to the editor (the
planner's), and every file except `app/src/editor/complete.ts`.

## Done when

`make accept T=0069`, `make check` and `make ui-check` pass, and only
`app/src/editor/complete.ts` changed.
