---
id: "0027"
title: Keep a device's sync state
milestone: M2
size: M # S: at most 150 non-test lines in 3 files; M: 400 lines in 6 files; L: 1,000 lines, one package or feature (ADR-0010).
touch:
  - internal/store/remotes.go
given:
  - internal/store/remotes_test.go # copied from 0027-device-sync-state/_given/ when the task starts
acceptance: go test ./internal/store -run 'TestSyncHubs|TestRemotes|TestAgreed|TestPending|TestLocalPushes' -count=1
---
# T-0027: Keep a device's sync state

## Goal

A device knows what to push by comparing its index with what it and the
hub last agreed each file is, not by keeping a queue (ADR-0017): a crash
or an outage then loses nothing to push. Implement the store methods in
`internal/store/remotes.go`: the hubs a device paired with, its synced
vaults and cursors, each file's agreed state, the pending changes, and the
record of the pushes it made. Runs after T-0026 (same package).

## Read first

- docs/tasks/EXECUTOR.md
- docs/adr/0017-revisions-number-the-hubs-log-and-hashes-name-content.md
- docs/specs/sync-protocol.md: "Pending changes", "Pulling and applying",
  "Joining".
- internal/store/migrations/0002_sync.sql: `sync_hubs`, `remotes`,
  `sync_files`, `sync_pushes`.
- internal/store/remotes.go: the stubs, the types and their doc comments
  (the contract).
- internal/store/files.go: `File`, `Files` (a vault's files by path).
- internal/store/store.go: `IsUniqueViolation`, `IsForeignKeyViolation`.
- api/zz_generated.go: `SyncHub`, `PushStatus`, `RefuseReason`.
- internal/store/remotes_test.go (given).

## Contract

Keep the types, signatures and doc comments in `remotes.go`.

- **Times** are stored with `FormatTime(t.Now())` and read back with
  `ParseTime`; `synced_at` NULL reads back as the zero time. Missing rows
  are wrapped `ErrNotFound`.
- **`PutSyncHub(h)`:** insert, or, when `hub_id` exists, update that row's
  `url`, `device_id` and `paired_at`; `paired_at` = now either way;
  `h.ID` and `h.PairedAt` are ignored. Return the row.
  **`SyncHubs`** by ID (non-nil when empty), **`SyncHub(id)`**,
  **`SyncHubByURL(url)`** (exact match). **`API()`:** `Hub` is `HubID`,
  `Device` is `DeviceID`.
- **`AddRemote(r)`:** insert with `joined_at` = now and `synced_at` NULL;
  a vault with a remote is wrapped `ErrConflict`, a missing vault or hub
  (a foreign key) wrapped `ErrNotFound`. Return the row.
- **`Remote`**, **`Remotes`** (by vault ID, non-nil), **`DeleteRemote`**
  (its `sync_files` and `sync_pushes` rows go too: the foreign keys
  cascade), **`SetCursor(vaultID, cursor, hubSeq)`** (`hub_seq =
  MAX(hub_seq, ?)`), **`MarkSynced`**: each a wrapped `ErrNotFound` when
  the vault has no remote.
- **`PutAgreed(a)`:** insert, or replace the row with `(vault_id,
  hub_file)`; a `file_id` another row has is wrapped `ErrConflict`.
  **`DeleteAgreed`** never fails for a missing row. **`AgreedByHubFile`**,
  **`AgreedByFile`**, **`AgreedRows`** (by path, non-nil).
- **`Pending(vaultID)`:** with the vault's `files` rows and agreed rows:
  - a file with no agreed row (by `file_id`): a create (`Agreed` nil);
  - a file whose path or hash differs from its agreed row: a put, with
    the file's path and hash;
  - an agreed row whose `file_id` has no `files` row: a delete, `Path` the
    agreed path, `Hash` `""`, `Deleted`;
  - a file with an equal agreed row: nothing.
  `Agreed` points at the change's agreed row. Leave out a change when the
  newest `sync_pushes` row of its `file_id` (highest `push_id`) has status
  `refused` and the same path and hash (`""` for a delete). Order:
  deletes, then puts, then creates, each by path. Non-nil when empty.
- **`AddPush(p)`:** insert with `push_id` = the vault's highest + 1 (1 for
  the first), `at` = now, `status` NULL, `reason` `""`; `hub_file` NULL
  when 0. Return the row. **`AnswerPush`** sets `status` and `reason`;
  wrapped `ErrNotFound` for a missing push. **`Unanswered`**: rows with
  `status` NULL; **`Pushes`**: rows with a status; both by push ID,
  non-nil.

## Tests (given, do not edit)

`internal/store/remotes_test.go`: `TestSyncHubs`, `TestRemotes`,
`TestAgreed`, `TestPending`, `TestLocalPushes`.

## Gotchas

- `sync_files.file_id` is not a foreign key on purpose: an agreed row
  whose file is gone is how a delete is noticed.
- A refused push is skipped only while the file is still in the refused
  state; a change to the file makes it pending again.
- The push ID comes from the database in the insert, so two pushes in one
  transaction get 1 and 2.

## Out of scope

The sync loop that calls these (`internal/sync`, Phase 4), the hub's
tables (T-0024 to T-0026), and every file other than `remotes.go`.

## Done when

`make accept T=0027` passes, `make check` passes, and only the files under
touch changed.
