---
id: "0028"
title: Thin the hub's log content by age
milestone: M2
size: M # S: at most 150 non-test lines in 3 files; M: 400 lines in 6 files; L: 1,000 lines, one package or feature (ADR-0010).
touch:
  - internal/store/retention.go
given:
  - internal/store/retention_test.go # copied from 0028-hub-retention/_given/ when the task starts
acceptance: go test ./internal/store -run 'TestPruneRevisions' -count=1
---
# T-0028: Thin the hub's log content by age

## Goal

The hub keeps every revision's record forever but thins its content by age
(ADR-0019), so its store grows with the days and months a file changes,
not with every save. Implement `PruneRevisions` in
`internal/store/retention.go`: decide which revisions keep their content,
mark the others pruned, and report the blobs nothing needs any more. Runs
after T-0027 (same package).

## Read first

- docs/tasks/EXECUTOR.md
- docs/adr/0019-keep-every-revisions-record-and-thin-the-content-by-age.md
- internal/store/migrations/0002_sync.sql: `revisions`, `hub_files`.
- internal/store/retention.go: the stub and its doc comment (the
  contract).
- internal/store/versions.go: `PruneVersions`, which does the same for a
  notesd's own versions, and `UsedHashes`.
- internal/store/hub.go: `Revision`, `ParseTime` use (T-0024).
- internal/store/hub_test.go (T-0024's helpers) and
  internal/store/retention_test.go (given).

## Contract

Keep the signature and doc comment. For every revision whose op is
`create`, `put`, `move` or `conflict` and that is not yet pruned, it is
**kept** when any of these holds, and **pruned** (`pruned = 1`)
otherwise:

1. `at` is after `now` minus 30 days (30 × 24 hours);
2. it is its file's head (`hub_files.revision`);
3. it is a `conflict`, its file is flagged (`hub_files.conflicted`), and
   its `seq` is after the file's last `resolve` revision (0 when none);
4. it is a create, put or move, `at` is after `now` minus 365 days, and it
   has the highest `seq` among its file's creates, puts and moves on its
   day, the date of `at` in `loc`;
5. it is a create, put or move with the highest `seq` among its file's
   creates, puts and moves in its month (year and month of `at` in `loc`).

Rules 4 and 5 compare against all of the file's creates, puts and moves,
pruned or not. `delete` and `resolve` revisions are never pruned and never
count as content. Return the distinct hashes of the revisions this call
pruned that no `create`, `put`, `move` or `conflict` revision with
`pruned = 0`, no `files` row and no `versions` row has, sorted; a non-nil
empty slice when none.

## Tests (given, do not edit)

`internal/store/retention_test.go`: `TestPruneRevisions`,
`TestPruneRevisionsKeepsHashesOthersName`,
`TestPruneRevisionsCountsDaysInTheHubsZone`,
`TestPruneRevisionsCountsMonthsInTheHubsZone`.

## Gotchas

- Days and months are those of `loc`, not UTC: parse `at` (stored in UTC)
  and convert it with `In(loc)` before taking its date. SQLite's date
  functions do not know `loc`, so group in Go.
- A conflict revision is content that lost, never a state of the file:
  it is never a day's or month's last.
- A move carries the same hash as the state before it, so a pruned put
  whose content a kept move still names frees nothing.
- Read every row before you update: iterating a result set while updating
  the same table is a trap.

## Out of scope

Removing blobs (the daily pass does it with what this returns), deciding
when the pass runs (Phase 4), and every file other than `retention.go`.

## Done when

`make accept T=0028` passes, `make check` passes, and only the files under
touch changed.
