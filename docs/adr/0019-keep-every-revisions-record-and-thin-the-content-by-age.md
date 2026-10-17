# 0019 — Keep every revision's record, and thin the content by age

- **Status:** Accepted
- **Date:** 2026-10-16

## Context

[ADR-0005](0005-sync-through-a-hub-with-revisions-and-three-way-merges.md)'s
log grows with every push, and its consequences ask for a retention policy
in M2. Each notesd keeps its own versions for 30 days
([ADR-0006](0006-every-write-names-the-version-it-changes.md)); the hub's
log is meant as the long-term history. Devices push after a quiet period,
not on every keystroke
([ADR-0017](0017-revisions-number-the-hubs-log-and-hashes-name-content.md)).
The user's vault holds 666 notes in about 2.2 MiB, and 8 attachments. A
device can be away for weeks, and a push whose base content is gone
cannot merge three ways. The roadmap's starting point was every revision
for 30 days, then daily for a year, and conflicts until resolved.

## Decision

- **Records are kept forever.** A revision's row (sequence number, file,
  path, hash, base, author, time) is never deleted, so sequence numbers
  have no gaps, every base names a row, and history lists every change.
- **Content thins by age, per file:** every revision's content for 30
  days; then each file's last state of each day (its last create, put or
  move), in the hub's time zone, for a year; then its last of each month.
  A file's head keeps its content however old, and so does a conflict
  revision while its file is flagged. A conflict revision is content that
  lost, never a state of the file, so once its file is resolved it is kept
  only for its 30 days.
- **A pruned revision says so:** history shows it without content, and it
  cannot be restored.
- **A push whose base content is pruned merges as a create**
  (`merge.Create` of the head and the pushed content, with the lines they
  share as the base). Nothing is lost, though lines both sides changed
  may appear twice, and the answer says that the base was gone.
- **The pass runs daily with M1's prune,** under every vault's lock, and
  removes a blob only when no head, no revision that keeps content and no
  local version names it.
- **Devices keep their own 30 days,** as before.

## Consequences

- The hub's store grows with the days and months on which a file changes,
  not with its saves: little for notes; an attachment changed every day
  costs a copy a day for a year.
- The hub's base check must find content in the log, not only in its
  local versions, which are pruned at 30 days.
- A device away for more than 30 days that edited a note offline may see
  repeated lines after its push; the merge keeps both sides.
- Retention is fixed in M2; settings for it wait for a need.

## Alternatives considered

- **All content forever:** simplest, and fine for notes, but attachments
  that change cost unbounded space.
- **The last N revisions of each file:** a burst of saves would push out
  last month's state.
- **Pinning each known device's agreed bases:** a lost or abandoned
  device would pin content forever, where merging as a create loses
  nothing.
- **Dropping records with their content:** gaps in the log, and bases
  that name nothing.

## References

- Shapes: [design/sync.md](../design/sync.md#retention), [specs/sync-protocol.md](../specs/sync-protocol.md), [specs/merge.md](../specs/merge.md)
- Builds on: [ADR-0005](0005-sync-through-a-hub-with-revisions-and-three-way-merges.md), [ADR-0006](0006-every-write-names-the-version-it-changes.md), [ADR-0017](0017-revisions-number-the-hubs-log-and-hashes-name-content.md)
- Answers: the roadmap's question on hub retention ([plan 0002](../plans/0002-roadmap.md#open-questions))
- Built in: [plan 0004](../plans/0004-m2-sync.md)
