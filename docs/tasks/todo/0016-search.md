---
id: "0016"
title: Search notes with snippets
milestone: M1
size: S # S: at most 150 non-test lines in 3 files; M: 400 lines in 6 files; L: 1,000 lines, one package or feature (ADR-0010).
touch:
  - internal/store/search.go
given:
  - internal/store/search_test.go # copied from 0016-search/_given/ when the task starts
acceptance: go test ./internal/store -run 'TestSearch' -count=1
---
# T-0016: Search notes with snippets

## Goal

`note.search` and MCP's `search_notes` find notes by their words, best
match first, with a snippet around the match. Implement `SearchQuery`,
which turns what a person or an agent typed into an FTS5 query, and
`DB.Search`, which runs it over `notes_fts`. Runs after T-0015.

## Read first

- docs/tasks/EXECUTOR.md
- docs/plans/0003-m1-headless-vault.md, the "Search" decision.
- internal/store/migrations/0001_init.sql: `notes_fts` (its columns,
  tokenizer and prefix index).
- internal/store/search.go: the stubs and their doc comments.
- api/zz_generated.go: `SearchHit` and `SnippetPart`.
- SQLite's FTS5 documentation for `MATCH`, `bm25()` and `snippet()`.
- internal/store/search_test.go (given).

## Contract

**`SearchQuery(text)`** reads `text` left to right:

- spaces, tabs and line breaks separate terms;
- a `"` starts a phrase that runs to the next `"`, or to the end;
- anything else is a word, running to the next space, tab, line break or
  `"`.

A term with no letter or digit (`unicode.IsLetter`, `unicode.IsDigit`)
is dropped. Each other term is written as an FTS5 string, `"` + the term
with each `"` doubled + `"`, and a word gets `*` after it. Terms are
joined with one space. No terms: `""`. Operators and column names lose
their meaning inside quotes: `NEAR` and `title:x` are words.

**`Search(ctx, vaultID, query, limit)`**:

- `query == ""`: return an empty, non-nil slice without querying.
- `limit` below 1 is 1 and above 100 is 100.
- Select the vault's notes (`notes_fts` joined to `notes` and `files`)
  `WHERE notes_fts MATCH ?`, ordered by
  `bm25(notes_fts, 10.0, 5.0, 1.0, 2.0)` (title, aliases, body,
  frontmatter), then by path, `LIMIT ?`.
- Each hit's `Path` and `Title` (`notes.title`), and its snippet from
  `snippet(notes_fts, -1, char(2), char(3), '…', 16)`: split it at the
  `\x02` and `\x03` markers into `SnippetPart`s, text between `\x02` and
  `\x03` with `Match: true`, never an empty part.

## Tests (given, do not edit)

`internal/store/search_test.go`: `TestSearchQuery` (10 cases),
`TestSearch` (ranking, diacritics, frontmatter text, phrases, vaults,
limits), `TestSearchSnippets`.

## Gotchas

- In `bm25()` a lower score is a better match: order ascending.
- Column `-1` makes `snippet()` choose the column that matched best, so a
  title-only match's snippet is the title.
- `"plan the bed"` is a phrase, not a prefix: it does not match "beds".
- A user's text never reaches SQL except as the bound MATCH argument.

## Out of scope

Search syntax beyond words and phrases (no `-`, `OR` or `tag:`),
indexing (T-0015), and every file other than `search.go`.

## Done when

`make accept T=0016` passes, `make check` passes, and only the files under
touch changed.
