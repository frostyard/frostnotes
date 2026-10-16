---
id: "0005"
title: Store file contents by SHA-256
milestone: M1
size: M # S: at most 150 non-test lines in 3 files; M: 400 lines in 6 files; L: 1,000 lines, one package or feature (ADR-0010).
touch:
  - internal/blobs/blobs.go
given:
  - internal/blobs/blobs_test.go # copied from 0005-blob-store/_given/ when the task starts
acceptance: go test ./internal/blobs -race -count=1
---
# T-0005: Store file contents by SHA-256

## Goal

Add `internal/blobs`, the content-addressed store under
`$FROSTNOTES_DATA_DIR/blobs`. Every version of a note that notesd writes or
scans is kept here (ADR-0006), the write path reads a writer's base from
it to merge, and M2's sync moves blobs by ID. A blob appears whole or not
at all, so `kill -9` at any moment never leaves a partial one. The design
is frostmail's `internal/blob`, plus a directory fsync, checked reads, a
listing for pruning and a sweep of temporary files.

## Read first

- docs/tasks/EXECUTOR.md
- docs/design/vault.md, sections "On disk" and "Versions".
- internal/blobs/blobs_test.go (given).
- internal/config/paths.go: `Paths.Blobs` is the directory notesd passes to
  `New` (no change there).

## Contract

New package `internal/blobs`, one file `blobs.go`, with a package doc
comment. An **ID** is the lowercase hex SHA-256 of a blob's bytes: exactly
64 characters, each `0-9` or `a-f`. A blob lives at
`<dir>/<id[:2]>/<id>`.

```go
var ErrInvalidID = errors.New("blobs: invalid id")
var ErrCorrupt = errors.New("blobs: content does not match its id")

type Store struct{ dir string } // safe for concurrent use
func New(dir string) *Store     // creates nothing
func ID(data []byte) string     // the ID data has as a blob

func (s *Store) Put(ctx context.Context, r io.Reader) (string, error)
func (s *Store) Open(id string) (io.ReadCloser, error)
func (s *Store) Get(id string) ([]byte, error)
func (s *Store) Has(id string) (bool, error)
func (s *Store) Remove(id string) error
func (s *Store) IDs(ctx context.Context) ([]string, error)
func (s *Store) RemoveTemp(before time.Time) (int, error)
```

Every method that takes an ID checks it first and returns `ErrInvalidID`
(unwrapped) for anything else; that check is what keeps an ID from naming
a path outside the store.

- **Put:** `os.MkdirAll(dir, 0o700)`; `os.CreateTemp(dir, ".put-*")`; copy
  `r` with `io.Copy(io.MultiWriter(tmp, h), r)`, `h := sha256.New()`; then
  check `ctx.Err()` (return it unwrapped); `Chmod(0o600)`, `Sync`, `Close`.
  Create `<dir>/<id[:2]>` with 0700. If the blob already exists, keep it
  and drop the temporary file. Otherwise `os.Rename` it into place, then
  fsync the shard directory (open it, `Sync`, close). Every failure removes
  the temporary file. Wrap errors as `blobs: <what>: %w`, so a reader's
  error stays in the message.
- **Open:** `os.Open` of the blob; the `*os.File` is the reader. Wrap
  errors with `%w` so a missing blob matches `fs.ErrNotExist`.
- **Get:** `os.ReadFile`; a missing blob matches `fs.ErrNotExist`; if
  `ID(data) != id`, return nil and an error wrapping `ErrCorrupt`.
- **Has:** `os.Stat`; `fs.ErrNotExist` is `false, nil`.
- **Remove:** `os.Remove`; a missing blob is not an error.
- **IDs:** every blob ID, sorted ascending, as a non-nil slice (empty when
  `dir` is empty or missing). Count only regular files whose name is a
  valid ID inside a two-character shard directory equal to the ID's first
  two characters. Check `ctx.Err()` once per shard.
- **RemoveTemp:** delete the regular files directly in `dir` whose names
  start with `.put-` and whose modification time is before `before`, which
  a crash between `CreateTemp` and `Rename` leaves behind; return how many.
  A missing `dir` is `0, nil`.

## Tests (given, do not edit)

`internal/blobs/blobs_test.go`: `TestPutGetRoundTrip` (layout and modes),
`TestEmptyBlob`, `TestPutIsIdempotent` (1 MiB), `TestConcurrentPuts`,
`TestHasAndRemove`, `TestGetDetectsCorruption`, `TestRejectsInvalidIDs` (seven
IDs, four methods), `TestPutFailuresLeaveNothing` (a failing reader, a
canceled context), `TestListIDs` (strays skipped), `TestRemoveTemp`.

## Gotchas

- The ID is `hex.EncodeToString(h.Sum(nil))`: lowercase. An uppercase ID
  is invalid.
- Check `ctx.Err()` after copying and before renaming: the canceled-context
  test expects `context.Canceled` and an empty directory.
- Concurrent Puts of the same bytes race to the same name: renaming over an
  existing blob of equal content is fine, but never truncate or rewrite it.
- `IDs` must not list `.put-*` files, `notes.txt`, an ID in the wrong
  shard, or a directory named like an ID.

## Out of scope

Compression, reference counting and pruning policy (the planner's versions
work), streaming from `Get`, and every file except
`internal/blobs/blobs.go`.

## Done when

`make accept T=0005` passes, `make check` passes, and only the files under
touch changed.
