---
id: "0018"
title: Record, list and prune versions
milestone: M1
size: S # S: at most 150 non-test lines in 3 files; M: 400 lines in 6 files; L: 1,000 lines, one package or feature (ADR-0010).
touch:
  - internal/store/versions.go
given:
  - internal/store/versions_test.go # copied from 0018-versions/_given/ when the task starts
acceptance: go test ./internal/store -run 'TestAddVersion|TestVersions|TestPruneVersions' -count=1
---
# T-0018: Record, list and prune versions

## Goal

Every version of every file notesd writes or scans is kept for 30 days
(ADR-0006), and a deleted file's versions outlive it, so `note.revert`
can bring it back (plan 0003, "Deleting a note"). Implement the `versions`
table's store methods: append a version, list a file's versions, find a
deleted file's versions by its path, and prune old versions, reporting
the blobs no one needs any more. Runs after T-0014.

## Read first

- docs/tasks/EXECUTOR.md
- docs/design/vault.md, sections "Versions" and "Writes".
- internal/store/migrations/0001_init.sql: `versions` and its comments.
- internal/store/versions.go: the stubs, `Version` and their doc comments
  (the contract).
- internal/store/vault.go: `scanVault`, `FormatTime`, `ParseTime`.
- api/zz_generated.go: `Version` and `VersionSource`.
- internal/store/versions_test.go (given).

## Contract

Keep the type, signatures and doc comments in `versions.go`.

- **`API()`**: `api.Version` with every field of the same name, `Deleted`
  = `Hash == ""`, and `Conflict`.
- **`AddVersion(v)`**: insert `v` (its `ID` and `At` ignored) with `at` =
  `FormatTime(t.Now())`, and return the row as stored: its new ID, and
  `At` parsed back (UTC, to the millisecond).
- **`Versions(fileID, limit)`**: newest first (`ORDER BY id DESC`), at
  most `limit`, 50 when `limit <= 0`; a non-nil empty slice when none.
- **`GetVersion(id)`**: the row, or a wrapped `ErrNotFound`.
- **`LastFileAt(vaultID, path)`**: the `file_id` of the newest version
  (highest ID) recorded with that vault and path, or a wrapped
  `ErrNotFound`.
- **`PruneVersions(before)`**: delete the versions with `at` before
  `FormatTime(before)`, except, for each file that still has a `files`
  row, its newest version. Of the hashes the deleted rows held (ignoring
  `""`), return, sorted, those that no remaining `versions` row and no
  `files` row holds. Return a non-nil empty slice when none.

## Tests (given, do not edit)

`internal/store/versions_test.go`: `TestAddVersion`,
`TestVersionsOutliveTheirFile`, `TestPruneVersions`.

## Gotchas

- `versions.file_id` is not a foreign key: a deleted file's versions keep
  its ID, and `DeleteFile` leaves them.
- Times sort as text in `TimeFormat`, so `at < ?` compares correctly.
- The newest version of a live file is kept however old: it is the base
  of the next merge.
- `DELETE … RETURNING hash` gives the deleted hashes; read them all before
  checking which are still used.

## Out of scope

Removing blobs (the engine does, with what `PruneVersions` returns), the
daily pruning pass, and every file other than `versions.go`.

## Done when

`make accept T=0018` passes, `make check` passes, and only the files under
touch changed.
