---
id: "0044"
title: Filter searches by path, file name and tag
milestone: M3
size: M
touch:
  - internal/store/search.go
given:
  - internal/store/filters_test.go
acceptance: go test ./internal/store -run 'TestParseQuery|TestSearchNotes' -count=1
---
# T-0044: Filter searches by path, file name and tag

## Goal

The app's search pane narrows a search the way Obsidian's does, with
`path:`, `file:` and `tag:` (`schema/rpc/note.yaml`, `note.search`).
Implement `ParseQuery`, which splits what the user typed into words to
match and filters, and `SearchNotes`, which runs the full-text search
with the filters, or lists the notes the filters admit when there are
no words.

## Read first

- `internal/store/search.go`: `SearchQuery` and `Search` (task card
  T-0016's, kept as they are), the `Query` type, and the two stubs.
- `internal/store/queries.go`: `noteConditions`, for the tag condition
  (a tag or one nested under it, on `tags.folded`).
- `internal/store/migrations/0001_init.sql`: `files`, `notes`, `tags`,
  `notes_fts`.
- `schema/rpc/note.yaml`: `note.search`'s doc.
- The given test `internal/store/filters_test.go`.
- `docs/tasks/EXECUTOR.md`.

## Contract

Keep `SearchQuery`, `Search`, the `Query` type and the two signatures
and doc comments as they are.

**`ParseQuery(text)`** walks `text` as `SearchQuery` does: whitespace
separates words, and a `"` starts a phrase that runs to the next `"` (or
to the end). A phrase is never a filter. A word is a filter when, before
its first `:`, it reads `path`, `file` or `tag`, ignoring case; the rest
is its value, which may start a phrase (`path:"Daily notes"` has the
value `Daily notes`, running to the next `"` even past spaces, or to the
end). Then:

- `path:` values go to `Paths`, `file:` values to `Files`, each as typed;
- `tag:` values go to `Tags` without one leading `#` and lower-cased
  with `strings.ToLower`;
- a filter whose value is empty (`path:`, `file:""`, `tag:#`) is dropped;
- every other word and phrase, in order, joined with spaces, goes
  through `SearchQuery` to make `Match` (`"path:x"` in quotes is a
  phrase; `foo:bar` and `paths:x` are words);
- slices stay nil when there is no filter of their kind, and `Match` is
  `""` when nothing is left to match.

**`SearchNotes(ctx, vaultID, q, limit)`**, `limit` clamped to 1…100 as
`Search` clamps it:

- with no `Match` and no filter, an empty, non-nil slice;
- with `Match`: the vault's notes that match it, ranked and with
  snippets exactly as `Search` gives them, keeping only those the filters
  admit, at most `limit` of them (the limit counts admitted notes);
- with filters alone: every note of the vault they admit, by path, at
  most `limit`, each with its title and an empty, non-nil `Snippet`;
- **filters admit** a note when its path contains every `Paths` value,
  its file name (the last path segment) contains every `Files` value,
  both ignoring case as `strings.ToLower` folds it, and it has every
  `Tags` value or a tag nested under it (`tags.folded`); attachments
  never match;
- only the vault's notes; every result is a non-nil slice.

## Tests (given, do not edit)

`internal/store/filters_test.go`: `TestParseQuery`, `TestSearchNotes`.

## Gotchas

- Case: SQLite's `LIKE` and `lower()` fold only ASCII, and `file:CAFÉ`
  must find "Café plan.md". Compare paths in Go after `strings.ToLower`,
  or fold both sides with Go.
- The limit applies after filtering: don't let SQL's `LIMIT` cut ranked
  matches the path and file filters would then drop.
- `file:` is the last segment only: `file:garden` does not match
  `Garden/Beds.md`.

## Out of scope

Switching the engine's `note.search` to these (the planner's, Phase 4),
other Obsidian operators (`line:`, `section:`, `-` negation, `OR`), and
every file except `internal/store/search.go`.

## Done when

`make accept T=0044` and `make check` pass, and only
`internal/store/search.go` changed.
