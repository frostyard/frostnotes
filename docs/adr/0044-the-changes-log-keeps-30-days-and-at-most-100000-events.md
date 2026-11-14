# 0044 — The changes log keeps 30 days, and at most 100,000 events

- **Status:** Proposed
- **Date:** 2026-11-13

## Context

notesd appends every durable event (`note.changed`, `note.moved`,
`note.removed`, `vault.added`, `vault.configured`, `vault.removed`) to the
`changes` table in the transaction that makes the change. Nothing deletes
a row, so the table grows with every write, scan and sync apply for the
life of the database
([#219](https://github.com/frostyard/frostnotes/issues/219)). A row is
about 200 bytes: the event's name, its JSON (a vault ID, a path, a
64-character hash, a source) and a time.

The log has one reader: `events.subscribe` with `sinceSeq`
(`internal/events/broker.go`), which replays the retained events after
`sinceSeq`, or answers `resync: true` when `sinceSeq` is older than the
retained log ([specs/rpc-protocol.md](../specs/rpc-protocol.md#events)).
Sync does not read it: a device finds what to push by comparing `files`
with `sync_files` (`store.Pending`), and the hub's log is the `revisions`
table. The sync loop subscribes without `sinceSeq`. The table's
`AUTOINCREMENT` keeps sequence numbers unique after a prune.

The apps subscribe from the last sequence number they saw, so moves made
while they were closed reach their tabs
([ADR-0021](0021-keep-the-workspace-and-app-preferences-on-each-device.md),
whose consequences ask notesd to keep the log "long enough to make that
useful"). Both apps also reload the vault list and the open vault on
every connection, so a `resync` costs them no state: only a tab whose file
moved while the app was away shows the file as gone instead of following
it. Local versions are kept for 30 days
([ADR-0006](0006-every-write-names-the-version-it-changes.md)), and the
hub thins its log's content after 30 days
([ADR-0019](0019-keep-every-revisions-record-and-thin-the-content-by-age.md)).

## Decision

- **The daily prune pass trims the changes log** (`Manager.Prune`, with
  M1's prune of versions): it deletes the durable events recorded more than
  30 days ago (`Config.Keep`, the versions' window), and all but the
  newest 100,000 (`Config.KeepEvents`).
- **A client away longer gets `resync`,** as the protocol already says.
  The newest event can go too after 30 quiet days; a client that saw it
  still resumes, because the latest sequence number survives the prune.
- **`events.subscribe` checks the oldest retained event again after it
  reads the replay,** so a prune between its two reads gives `resync`, not
  a replay with a gap.

## Consequences

- The table holds at most 100,000 rows, about 20 MB, whatever the rate of
  changes; at an ordinary rate it holds 30 days.
- An app closed for more than 30 days, or over 100,000 events (a first
  scan of a very large vault, a long sync catch-up), reloads with
  `resync`, and its tabs do not follow the moves of that time.
- No API, schema or migration change: `resync` and the retained log are in
  protocol 1 already.

## Alternatives considered

- **Count only:** bounds the table, but a quiet notesd would keep months
  of events no client needs, and a busy one keeps fewer than a day.
- **Age only:** a burst (a first scan, a bulk move) is kept in full for 30
  days, so the table is not bounded.
- **Keep everything (today):** the table grows for the life of the
  database, and the `resync` branch of the protocol never runs.

## References

- Shapes: [design/vault.md](../design/vault.md#changes-log),
  [specs/rpc-protocol.md](../specs/rpc-protocol.md#events)
- Builds on: [ADR-0021](0021-keep-the-workspace-and-app-preferences-on-each-device.md),
  [ADR-0006](0006-every-write-names-the-version-it-changes.md),
  [ADR-0019](0019-keep-every-revisions-record-and-thin-the-content-by-age.md)
