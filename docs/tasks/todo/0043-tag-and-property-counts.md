---
id: "0043"
title: Count a vault's tags and properties
milestone: M3
size: S
touch:
  - internal/store/counts.go
given:
  - internal/store/counts_test.go
acceptance: go test ./internal/store -run 'TestTagCounts|TestPropertyCounts' -count=1
---
# T-0043: Count a vault's tags and properties

## Goal

The app's Tags and All properties panes list every tag and every
frontmatter key of a vault with how many notes have it
(`docs/specs/ui.md`, Right sidebar panes), through `note.tags` and
`note.properties` (`schema/rpc/note.yaml`). Implement the two store
queries behind them.

## Read first

- `internal/store/counts.go`: the two stubs and their doc comments.
- `internal/store/migrations/0001_init.sql`: the `files`, `notes`
  (`props` is the frontmatter as a JSON object) and `tags` tables.
- `internal/store/queries.go`: `ListNotes` and `noteConditions`, for how
  queries reach a vault's notes and its frontmatter keys
  (`json_each(notes.props)`).
- `schema/rpc/note.yaml`: `TagCount`, `PropertyCount`, `note.tags`,
  `note.properties`.
- The given test `internal/store/counts_test.go`.
- `docs/tasks/EXECUTOR.md`.

## Contract

Keep both signatures and their doc comments.

**`TagCounts(ctx, vaultID)`** returns one `api.TagCount` per tag of the
vault's notes, grouped by `tags.folded` (the tag lower-cased by Go when
it was indexed), ordered by `folded` in byte order:

- `Tag` is the spelling (`tags.name`) that sorts first in byte order
  among the notes that have it ("Garden" before "garden");
- `Count` is the number of notes that have it;
- nested tags count whole: a note tagged `garden/beds` counts for
  `garden/beds`, not for `garden`;
- only the vault's notes count; a vault without tags gives an empty,
  non-nil slice.

**`PropertyCounts(ctx, vaultID, types)`** returns one
`api.PropertyCount` per frontmatter key of the vault's notes, the keys of
each note's `notes.props` object, grouped ignoring case as Go's
`strings.ToLower` folds them ("Été" and "été" are one key), ordered by
that lower-cased key in byte order:

- `Key` is the spelling that sorts first in byte order;
- `Count` is the number of notes with the key, whatever its value: null,
  an empty list and a nested object all count;
- `Type` is `types[lowercased key]`, or `""` when `types` has no entry
  (or is nil);
- a vault without frontmatter gives an empty, non-nil slice.

SQLite's `lower()` folds only ASCII letters, so group property keys in
Go (or fold them with Go before comparing).

## Tests (given, do not edit)

`internal/store/counts_test.go`: `TestTagCounts`, `TestPropertyCounts`.

## Gotchas

- A note's tags are each one row per note (`PRIMARY KEY (file_id,
  folded)`), so counting rows counts notes; frontmatter keys are counted
  per note too, once however many spellings a note has.
- Attachments have no `notes` row; join through `notes` or `tags`, which
  only notes have.
- Another vault's notes must not count.

## Out of scope

The engine's `note.tags` and `note.properties`, reading property types
from the vault, and every file except `internal/store/counts.go`.

## Done when

`make accept T=0043` and `make check` pass, and only
`internal/store/counts.go` changed.
