---
id: "0014"
title: Store a vault's files and keep their links resolved
milestone: M1
size: M # S: at most 150 non-test lines in 3 files; M: 400 lines in 6 files; L: 1,000 lines, one package or feature (ADR-0010).
touch:
  - internal/store/files.go
given:
  - internal/store/files_test.go # copied from 0014-store-files/_given/ when the task starts
acceptance: go test ./internal/store -run 'TestFile|TestPutFile|TestResolveLink|TestMoveFile|TestDeleteFile' -count=1
---
# T-0014: Store a vault's files and keep their links resolved

## Goal

The index records every file of a vault in the `files` table, and every
link a note makes in `links`, with the file it resolves to. When a file
appears, moves or goes, the links that could name it must resolve again,
or backlinks and the graph go stale. Implement the stubs in
`internal/store/files.go`: files by path, inserting, moving and deleting
them, and resolving links through `markdown.Resolve` inside the
transaction. The scan and the write path (plan 0003, Phase 4) call these.

## Read first

- docs/tasks/EXECUTOR.md
- internal/store/migrations/0001_init.sql: `files` (with `name`), `notes`,
  `links` (with `name` and `dest_id`) and `versions`, and their comments.
- internal/store/files.go: the stubs, their doc comments and the `File`
  type. The doc comments are the contract; keep them.
- internal/store/vault.go and store.go: the style (`scanVault`, `Tx`,
  `ErrNotFound`, `ErrConflict`, `IsUniqueViolation`, wrapped errors).
- internal/markdown/resolve.go (T-0004): `Resolve(source, target, files)`.
- docs/specs/markdown.md, "Link resolution".
- internal/store/files_test.go (given).

## Contract

Keep every signature and doc comment in `files.go`; replace the bodies.

- **`Kind`**: `FileNote` when `path.Base(f.Path)` ends in `.md`, exactly.
  **`FileName(p)`**: `strings.ToLower(path.Base(p))`.
- **Columns.** `files.name` is `FileName(path)`, `files.kind` is
  `string(f.Kind())`, and `files.mtime` is `ModTime.UnixNano()`; a `File`
  read back has `ModTime` = `time.Unix(0, mtime).UTC()`.
- **`PutFile`**: update size, mtime and hash of the row at
  `(f.VaultID, f.Path)` when there is one, and return it with its ID.
  Otherwise insert the row (`f.ID` is ignored), then re-resolve the links
  that could name the new file (below).
- **`MoveFile(id, to)`**: `ErrNotFound` (wrapped) when no file has that ID;
  update `path`, `name` and `kind`; a unique violation is `ErrConflict`
  (wrapped). When a note becomes an attachment, delete its `notes` row
  (the index rows cascade). Then re-resolve the links named after its old
  and its new name, and every link of the moved file itself (relative
  links depend on its folder). Return the moved file.
- **`DeleteFile(id)`**: delete the row (`ErrNotFound` when none) and
  re-resolve the links named after it. Its `versions` rows stay.
- **`FileByPath`** (on `DB` and on `Tx`): the row, or a wrapped
  `ErrNotFound`. **`Files`**: the vault's rows in byte order of path, a
  non-nil empty slice when none. **`FileCounts`**: notes and attachments.
- **`ResolveLink(vaultID, source, target)`**: run `markdown.Resolve`
  with a lookup that selects the paths of the vault's files whose `name`
  equals the name it asks for, through the transaction (so files put
  earlier in it count). Return the file at the path it picks and true,
  or false when it picks none. A lookup error is returned.
- **Re-resolving** the links "that could name" a file called `n`: the
  links whose source file is in the same vault and whose `links.name`
  equals `strings.TrimSuffix(n, ".md")`. For each, `ResolveLink(vault,
  <its source's path>, <its target>)`, and set `dest_id` to the file's ID
  or NULL. Read all the rows before updating any.

## Tests (given, do not edit)

`internal/store/files_test.go`: `TestFileKindAndName`, `TestPutFile`,
`TestPutFileResolvesWaitingLinks`, `TestResolveLink`, `TestMoveFile`,
`TestDeleteFile`. It writes `notes` and `links` rows with SQL, as
`IndexNote` (T-0015) will; T-0015 to T-0018's tests reuse its helpers
(`newFileTest`, `put`, `link`, `dest`).

## Gotchas

- `ON DELETE SET NULL` already clears `dest_id` when a file is deleted;
  re-resolving afterwards may find another file of the same name.
- SQLite's `lower()` folds ASCII only: compute `name` in Go.
- Close a `rows` before running other statements in the transaction.
- `markdown.Resolve`'s lookup has no error return: remember the first
  error in a variable and return it after `Resolve`.
- Byte order is SQLite's default `BINARY` collation: `ORDER BY path`.

## Out of scope

Indexing a note's contents (T-0015), versions (T-0018), the scan, and
every file other than `files.go`.

## Done when

`make accept T=0014` passes, `make check` passes, and only the files under
touch changed.
