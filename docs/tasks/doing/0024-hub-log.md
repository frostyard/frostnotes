---
id: "0024"
title: Keep the hub's log of revisions
milestone: M2
size: M # S: at most 150 non-test lines in 3 files; M: 400 lines in 6 files; L: 1,000 lines, one package or feature (ADR-0010).
touch:
  - internal/store/hub.go
given:
  - internal/store/hub_test.go # copied from 0024-hub-log/_given/ when the task starts
acceptance: go test ./internal/store -run 'TestServeVault|TestAppendRevision|TestPull|TestHasContent' -count=1
---
# T-0024: Keep the hub's log of revisions

## Goal

A hub keeps an append-only log of revisions for each vault it serves
(ADR-0017): a sequence number per vault without gaps, and each file's
place in the log, which is what devices pull. Implement the store methods
in `internal/store/hub.go`: the hub's identity, serving a vault,
appending revisions with their bookkeeping and event, and reading the log
as `log.pull` shows it.

## Read first

- docs/tasks/EXECUTOR.md
- docs/adr/0017-revisions-number-the-hubs-log-and-hashes-name-content.md
- docs/specs/sync-protocol.md: "Identifiers", "`log` methods" (`LogFile`,
  `log.pull`), "Blobs".
- internal/store/migrations/0002_sync.sql: `hub`, `hub_vaults`,
  `revisions`, `hub_files`, and their comments.
- internal/store/hub.go: the stubs, the types and their doc comments (the
  contract).
- internal/store/vault.go and store.go: `scanVault`, `Tx.Emit`,
  `FormatTime`, `ParseTime`, `IsUniqueViolation`, `ErrNotFound`,
  `ErrConflict`.
- api/zz_generated.go: `HubVault`, `LogVault`, `LogFile`, `LogPage`,
  `LogAppended`, `RevisionOp`.
- internal/store/hub_test.go (given).

## Contract

Keep the types, signatures and doc comments in `hub.go`; add unexported
helpers as you need.

- **IDs:** the hub ID and each sync ID are 16 bytes from `crypto/rand`
  as 32 lowercase hex digits (`encoding/hex`).
- **`HubID`:** the `hub` row's `hub_id`, inserting the row (`id = 1`,
  `created_at` = `FormatTime(t.Now())`) when there is none.
  **`HubIdentity`:** the same without inserting: `""` when there is none.
- **`ServeVault(vaultID)`:** `ErrNotFound` (wrapped) when the vault does
  not exist, `ErrConflict` when it is served; otherwise `HubID`, then
  insert `hub_vaults` with a new sync ID, `seq` 0 and `served_at` =
  `FormatTime(t.Now())`, and return it with the vault's name. Serving
  emits no event.
- **`HubVaults`, `HubVault`, `Tx.HubVault`, `HubVaultBySync`:** read
  `hub_vaults` joined with `vaults` for the name. `HubVaults` orders by
  name, then vault ID. A missing row is a wrapped `ErrNotFound`.
- **`API()`, `Log()`:** field for field (`Sync` is `SyncID`).
- **`AppendRevision(r)`**, in order:
  1. The vault must be served (`ErrNotFound` otherwise).
  2. Refuse (any error, nothing written): an op other than the six
     `RevisionOp` values; `FileID <= 0`; an empty `Path`; a delete with a
     hash; any other op without one; a create of a file that has a
     `hub_files` row; any other op of a file that has none.
  3. Insert the revision with `seq` = the vault's `seq` + 1 and `at` =
     `FormatTime(t.Now())`; `device_id`, `replica_id`, `push_id` and
     `lost_to` are NULL when the field is 0; `pruned` as given.
  4. Set `hub_vaults.seq` to the new seq.
  5. Insert or update the file's `hub_files` row: `seq` = the new seq;
     `revision` = the new seq for a create, put, move or delete (unchanged
     otherwise); `conflicted` = 1 for a conflict, 0 for a resolve,
     unchanged otherwise (0 for a new row).
  6. `t.Emit(ctx, api.LogAppended{Vault: syncID, Seq: seq})`.
  7. Return the revision as stored: `Seq` set, `At` parsed back (UTC, to
     the millisecond).
- **`HubFile`, `Tx.HubFile`, `Revision`, `Tx.Revision`:** read one row;
  wrapped `ErrNotFound` when missing.
- **`LogFile(vaultID, fileID)`:** the `hub_files` row joined with its head
  revision (`revisions.seq = hub_files.revision`): `File`, `Seq`,
  `Revision`, `Path` (the head's), `Hash` and `Size` (the head's; `""` and
  0 for a delete), `Deleted` (the head is a delete), `Conflicted`.
- **`Pull(vaultID, after, limit)`:** the vault's `hub_files` rows with
  `seq > after`, by `seq`, at most `limit` (500 when `limit <= 0`; at most
  1,000), each as `LogFile`. `Next` = the last file's `seq`, or `after`
  when there are none. `More` = whether a row with a larger `seq` remains
  (ask for one more than the limit). `Files` is never nil.
- **`HasContent(hash)`:** whether a create, put, move or conflict revision
  with that hash and `pruned = 0` exists; `""` is always false.

## Tests (given, do not edit)

`internal/store/hub_test.go`: `TestServeVault`, `TestAppendRevision`,
`TestAppendRevisionRefusesWhatTheLogCannotHold`, `TestPull`,
`TestPullCapsItsPages`, `TestHasContent`. Its `hubTest` helper is shared
with later cards' tests.

## Gotchas

- A move and a resolve carry the file's current hash, so `LogFile` takes
  the head's hash from the head revision, never from a conflict revision.
- A conflict revision is never a head: it changes `seq` and `conflicted`,
  not `revision`.
- `Tx.HubFile`, `Tx.Revision` and `Tx.HubVault` must read through the
  transaction (`t.QueryRowContext`), or they will not see what it wrote.
- Nothing in the store validates hashes as hex; the given tests use fake
  ones.

## Out of scope

Replicas, push answers, history and matching (T-0025), devices (T-0026),
retention (T-0028), the hub's push rules and the HTTP endpoints (Phase 4),
and every file other than `hub.go`.

## Done when

`make accept T=0024` passes, `make check` passes, and only the files under
touch changed.
