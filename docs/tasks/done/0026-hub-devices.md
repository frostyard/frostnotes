---
id: "0026"
title: Record the hub's paired devices
milestone: M2
size: S # S: at most 150 non-test lines in 3 files; M: 400 lines in 6 files; L: 1,000 lines, one package or feature (ADR-0010).
touch:
  - internal/store/devices.go
given:
  - internal/store/devices_test.go # copied from 0026-hub-devices/_given/ when the task starts
acceptance: go test ./internal/store -run 'TestDevices' -count=1
---
# T-0026: Record the hub's paired devices

## Goal

A hub knows each paired device by the SHA-256 of its token, never the
token itself, notes when it was last seen, and revokes it for good
(ADR-0016). Implement the store methods in `internal/store/devices.go`.
Runs after T-0025 (same package).

## Read first

- docs/tasks/EXECUTOR.md
- docs/adr/0016-pair-devices-with-a-one-time-code-and-a-revocable-token.md
- docs/specs/sync-protocol.md: "Pairing", "URLs and tokens", `hub.devices`
  and `hub.revoke`.
- internal/store/migrations/0002_sync.sql: `hub_devices`.
- internal/store/devices.go: the stubs, `HubDevice` and their doc comments
  (the contract).
- api/zz_generated.go: `HubDevice`.
- internal/store/devices_test.go (given).

## Contract

Keep the type, signatures and doc comments in `devices.go`.

- **Times** are stored with `FormatTime` and read with `ParseTime`;
  `seen_at` and `revoked_at` are NULL when unset, and read back as the
  zero `time.Time`. `token_hash` NULL reads back as `""`.
- **`AddDevice(name, tokenHash)`:** insert with `paired_at` =
  `FormatTime(t.Now())` and return the row as stored; a token hash another
  row has is wrapped `ErrConflict`.
- **`DeviceByToken(tokenHash)`:** the row with that `token_hash`, or
  wrapped `ErrNotFound` (a revoked device has none, so it is not found).
- **`Device(id)`:** the row, revoked or not, or wrapped `ErrNotFound`.
  **`Devices()`:** every row by ID; a non-nil empty slice when none.
- **`RevokeDevice(id)`:** set `token_hash` to NULL and `revoked_at` to
  `FormatTime(t.Now())` unless it is already set; revoking again changes
  nothing and is not an error. A missing device is wrapped `ErrNotFound`.
- **`TouchDevice(id, at)`:** set `seen_at` to `FormatTime(at)` when it is
  NULL or at least a minute before `at`; report whether it changed.
- **`API(connected)`:** `ID`, `Name`, `PairedAt`; `SeenAt` a pointer to
  the time, nil when zero; `Connected` as given; `Revoked` when
  `RevokedAt` is set.

## Tests (given, do not edit)

`internal/store/devices_test.go`: `TestDevices`.

## Gotchas

- Revoked devices keep their rows: revisions name them by ID, and
  `hub.devices` lists them.
- Times compare as text in `TimeFormat`, so `seen_at <= ?` works in SQL.

## Out of scope

Making tokens, hashing them, pairing (all in `internal/hub`, Phase 4),
replicas (T-0025), and every file other than `devices.go`.

## Done when

`make accept T=0026` passes, `make check` passes, and only the files under
touch changed.
