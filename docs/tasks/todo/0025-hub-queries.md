---
id: "0025"
title: Answer devices' questions about the hub's log
milestone: M2
size: M # S: at most 150 non-test lines in 3 files; M: 400 lines in 6 files; L: 1,000 lines, one package or feature (ADR-0010).
touch:
  - internal/store/hublog.go
given:
  - internal/store/hublog_test.go # copied from 0025-hub-queries/_given/ when the task starts
acceptance: go test ./internal/store -run 'TestReplicasAndPushAnswers|TestPushRecords|TestMatch|TestHistory' -count=1
---
# T-0025: Answer devices' questions about the hub's log

## Goal

Devices push through replicas, retry pushes whose answers they lost,
match their files against the log when they join, and read a file's
history (docs/specs/sync-protocol.md). Implement the store methods in
`internal/store/hublog.go` behind `log.join`, a push's repeat answer,
`log.pushes`, `log.match` and `log.history`. Runs after T-0024.

## Read first

- docs/tasks/EXECUTOR.md
- docs/specs/sync-protocol.md: "`log` methods", "A push, on the hub" rule
  1 (repeats), "Joining".
- internal/store/migrations/0002_sync.sql: `hub_devices`, `hub_replicas`,
  `revisions`, `hub_pushes`.
- internal/store/hublog.go: the stubs, the types and their doc comments
  (the contract).
- internal/store/hub.go: `Revision`, and the helpers T-0024 left there
  for reading revisions.
- api/zz_generated.go: `Revision`, `PushRecord`, `LogMatch`,
  `MatchQuery`, `PushStatus`, `RefuseReason`.
- internal/store/hub_test.go (T-0024's helpers) and
  internal/store/hublog_test.go (given).

## Contract

Keep the types, signatures and doc comments in `hublog.go`; reuse
`hub.go`'s helpers rather than copying them, and add unexported ones in
`hublog.go` as you need.

- **`Revision.API(deviceName)`:** field for field (`File` is `FileID`),
  `DeviceName` as given; `Device` and `LostTo` are pointers to the values,
  nil when they are 0. `ReplicaID`, `PushID` and `VaultID` have no wire
  field.
- **`AddReplica(deviceID, vaultID)`:** wrapped `ErrNotFound` when the
  vault is not served (`Tx.HubVault`) or no `hub_devices` row has the ID;
  otherwise insert with `joined_at` = `FormatTime(t.Now())` and return the
  row (`JoinedAt` in UTC, to the millisecond). **`Replica(id)`:** the row,
  or wrapped `ErrNotFound`.
- **`RecordPush(a)`:** insert into `hub_pushes`, `file_id` NULL when 0,
  `reason` as given (`""` unless refused), `at` = `FormatTime(t.Now())`; a
  repeated `(replica_id, push_id)` is wrapped `ErrConflict`.
  **`Tx.PushAnswer`:** the row read through the transaction, `At` parsed;
  wrapped `ErrNotFound` when missing.
- **Device names:** a revision's device name is its `hub_devices.name`,
  or `"hub"` when `device_id` is NULL (a `LEFT JOIN`).
- **`PushRecords(vaultID, replicaID, pushes)`:** one `PushRecord` per
  push ID, in the order given; its revisions are those of the vault with
  that `replica_id` and `push_id`, by `seq`, as `API(name)`; a non-nil
  empty slice when none.
- **`Match(vaultID, qs)`:** one `LogMatch` per query, in order, with
  `Path` and `Hash` copied. `File` is the `file_id` of the revision with
  the highest `seq` whose `path` is `q.Path` and whose op is not
  `conflict` or `resolve`; nil when none. When there is a file,
  `Revision` is the highest `seq` of its revisions whose op is `create`,
  `put` or `move` and whose `hash` is `q.Hash`; nil when none.
- **`History(vaultID, fileID, before, limit)`:** wrapped `ErrNotFound`
  when the file has no `hub_files` row; otherwise its revisions, newest
  first, with `seq < before` when `before > 0`, at most `limit` (50 when
  `limit <= 0`, at most 200), as `API(name)`.

## Tests (given, do not edit)

`internal/store/hublog_test.go`: `TestReplicasAndPushAnswers`,
`TestPushRecords`, `TestMatch`, `TestHistory`.

## Gotchas

- A move carries the file's hash, so matching a hash finds moves too; a
  conflict revision's hash is content that lost, never a state a device's
  file was agreed at, so it never matches.
- A deleted file is still the file last at its path: a device joining with
  an old copy of it must learn the delete.
- `api.Revision` has pointer fields, so compare values, not pointers, when
  checking your output.

## Out of scope

Devices (T-0026), the device's own state (T-0027), retention (T-0028),
the push rules themselves (Phase 4), and every file other than
`hublog.go`.

## Done when

`make accept T=0025` passes, `make check` passes, and only the files under
touch changed.
