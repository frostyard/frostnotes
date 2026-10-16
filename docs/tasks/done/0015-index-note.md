---
id: "0015"
title: Index one parsed note
milestone: M1
size: M # S: at most 150 non-test lines in 3 files; M: 400 lines in 6 files; L: 1,000 lines, one package or feature (ADR-0010).
touch:
  - internal/store/index.go
given:
  - internal/store/index_test.go # copied from 0015-index-note/_given/ when the task starts
acceptance: go test ./internal/store -run 'TestLinkName|TestNoteRows|TestReindex|TestNoteLinks' -count=1
---
# T-0015: Index one parsed note

## Goal

Implement `Tx.IndexNote`, which stores what `markdown.Parse` found in a
note in the index tables, so listing, search, backlinks and the graph can
read it, and `LinkName`, which keys a link for re-resolution. The scan
and the write path call `IndexNote` for every changed note, in the same
transaction as the file's row. Runs after T-0014.

## Read first

- docs/tasks/EXECUTOR.md
- internal/store/migrations/0001_init.sql: `notes`, `props`, `tags`,
  `links`, `headings`, `blocks`, `tasks`, `notes_fts` and the trigger
  that removes a note's search row.
- internal/store/index.go: the stubs and their doc comments (the
  contract).
- internal/markdown/parse.go: `Doc` and its parts.
- internal/markdown/frontmatter.go: `SplitFrontmatter`.
- internal/store/files.go (T-0014): `ResolveLink`.
- internal/store/index_test.go (given) and files_test.go (its helpers).

## Contract

Keep the signatures and doc comments in `index.go`.

**`IndexNote(ctx, f, content, doc)`** first deletes `f`'s `notes` row
(the other rows cascade, and the trigger removes its search row), then
inserts:

- `notes`: `title` = the file name without `.md`; `props` = `doc.Props`
  as JSON from `encoding/json/v2` with `json.Deterministic(true)`, or
  `{}` when nil; `prop_error` = `doc.PropError.Error()` or `''`;
  `aliases` = `doc.Aliases` as a JSON array, `[]` when nil; `words`.
- `props`: for each key (any order), one row per scalar: the value itself
  (`ord` 0), or each item of a `[]any` value (`ord` = its index). Skip
  mappings and nested lists. `value`: a string as it is, a bool as
  `true`/`false`, an int64 with `strconv.FormatInt`, a float64 with
  `strconv.FormatFloat(v, 'g', -1, 64)`, nil as `''`. `folded` =
  `strings.ToLower(value)`. `num` = the number as a float64 for int64 and
  float64 values, NULL otherwise.
- `tags`: one row per `doc.Tags` entry: `ord` its index, `folded` the
  lower-cased name, `name`, `front`, `line`.
- `links`: one row per `doc.Links` entry, `ord` its index, every field as
  the `Link` has it (`stop` is `End`), `name` = `LinkName(target)`, and
  `dest_id` = the ID of the file `ResolveLink(f.VaultID, f.Path, target)`
  returns, or NULL.
- `headings`, `blocks`, `tasks`: one row each, `ord` its index.
- `notes_fts`: `rowid` = `f.ID`; `title`; `aliases` joined with `\n`;
  `body` = the content after its frontmatter (`SplitFrontmatter(content).Body`);
  `props` = the frontmatter text (`.Front`). Both pass through
  `strings.ToValidUTF8(s, "�")`.

Wrap errors with the file's path (`index <path>: %w`).

**`LinkName(target)`**: `""` for `""`; otherwise
`strings.TrimSuffix(strings.ToLower(path.Base(target)), ".md")`.

## Tests (given, do not edit)

`internal/store/index_test.go`: `TestLinkName`, `TestNoteRowsFromADoc`
(every table, column by column), `TestReindexReplacesANotesRows`,
`TestNoteRowsRecordAPropError`, `TestNoteLinksResolveLater`. The tests
build `Doc` values by hand, so they do not depend on the parser cards.

## Gotchas

- Insert into `notes` before its child tables: their foreign keys need it.
- Re-indexing must leave exactly one row set: delete before inserting,
  and rely on the cascade rather than deleting each table.
- `json.Marshal(nil map)` writes `null`; the column wants `{}`.
- A self-link (empty target) resolves to the note itself through
  `ResolveLink`; do not special-case it.
- Keep functions under 60 lines: one helper per group of tables reads well.

## Out of scope

Parsing, re-resolving other notes' links (T-0014 does it when files
change), queries (T-0016, T-0017), and every file other than `index.go`.

## Done when

`make accept T=0015` passes, `make check` passes, and only the files under
touch changed.
