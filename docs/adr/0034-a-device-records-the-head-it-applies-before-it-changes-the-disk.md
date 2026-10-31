# 0034 — A device records the head it applies before it changes the disk

- **Status:** Accepted
- **Date:** 2026-10-30

## Context

A device applies a head it pulled, or the hub's answer to its push,
with `vault.Apply`
([sync-protocol.md](../specs/sync-protocol.md#pulling-and-applying)).
Apply changes the disk first (a rename, or a removal), then records the
change and the file's agreed state in one `Tx`. A crash between the two
loses the `Tx`. After the restart, the scan finds the disk changed and
records an outside version, as it does for any program's edit
([vault.md](../design/vault.md#writes)). The agreed row
([ADR-0017](0017-revisions-number-the-hubs-log-and-hashes-name-content.md))
still holds the old revision, so the device pushes the change back from
that old base, as a change of its own. It made none.

The hub judges that push by the old base
([ADR-0005](0005-sync-through-a-hub-with-revisions-and-three-way-merges.md)),
and the hub may have gone on meanwhile. On 2026-10-30, tests through
real notesd processes (`TestACrashWhileApplyingAHeadPushesNothingBack`)
showed each of these:

- **A pulled delete** pushed back deletes the file again after another
  device brought it back with the same content (rule 4: the head's
  content is the base's).
- **A pulled move,** or a move and an edit, pushed back moves the file
  back after a later move (the latest move wins).
- **A pulled put** pushed back brings back a file deleted later (a put on
  a deleted file).
- **A pulled create** pushed back makes the file again after a later
  delete.
- **An answer** the hub merged, pushed back, conflicts with a later edit
  of the merged lines, and flags the file though no writer lost.

The seeded simulation
([ADR-0020](0020-test-sync-with-a-seeded-simulation-of-a-hub-and-devices.md))
crashes at this point, but it did not find these orderings.

## Decision

- **Before it calls `vault.Apply` for a head,** a device records the head
  in its own `Tx`, in `sync_applying`: the hub file, the local file and
  the path and hash it replaces, the agreed row's sequence number at that
  time, the head (sequence number, revision, path, hash, deleted,
  conflicted), the path the change puts the file at, and, for an answer,
  the push and the answer's status and reason. This covers pulled heads,
  answers, and a copy dropped for another local file.
- **The `Tx` of the change deletes the record,** in the same `Tx` that
  sets or drops the agreed row and answers the push. When nothing reached
  the disk (the file changed, or the vault cannot hold the path), the
  record is deleted at once.
- **The records are settled before each sync session starts,** so before
  it pushes. This waits for the vault's first scan and needs a scan that
  succeeded, which indexed what the crash left on disk. A scan fails as a
  whole and changes nothing (a folder it cannot read, or an empty folder
  of an indexed vault, such as an unmounted share): the index can still
  hold a file the crash took away, which would go out as a create. While
  no scan has succeeded since the vault opened, the loop does not start
  the session: it reports the error and tries again after its backoff,
  without dialing the hub, until a rescan succeeds.
- **A delete record settles without a scan that succeeded.** It looks at
  one path, which the settle brings up to date in the index, and at its
  local file's row; a stale row at another path cannot mislead it. It
  must not wait: the interrupted delete of a vault's last file leaves an
  empty folder over an index that still holds the file, which every scan
  refuses, so the vault would never sync again. A move, put or create
  record still waits: a stale row at the path a moved file left would go
  out as a create.
- **Settling** works so. A record whose file is now the head (at its path
  with its hash, or, for a delete, nothing at the old path and the local file gone from
  the index) is agreed, and its push answered, as the lost `Tx` would have
  done. A head that moves and rewrites its file and stopped between the
  two is finished: the file at the head's path with its old content takes
  the head's content. Any other record is discarded: the agreed row
  stays, so the file's own change is pushed, or the cursor, still below
  the head, makes the next pull apply it again. A record whose agreed row
  changed since it was made is discarded too.

## Consequences

- A device never pushes back as its own a change it only applied: what
  the hub did meanwhile stands.
- Each head a device applies costs one more small `Tx`.
- After a crash that left records of a move, put or create, a vault whose
  scans keep failing does not sync at all, pulls included, until a scan
  succeeds: `sync.status` shows the vault offline with the error.
- A delete settled after a crash is indexed as an outside deletion (the
  scan or the settle found it), not as a `sync` version: its attribution is lost, as it
  is for a local write that crashes in the same place.
- The moves that park a file aside for another head, and move it back
  ([sync-protocol.md](../specs/sync-protocol.md#pulling-and-applying),
  rule 5, and the answers' parking), have no record. A crash after such a
  rename and before its `Tx` leaves the file at its parked name, and the
  device pushes that name as a move: a name to fix by hand, nothing lost,
  as before. A crash after a move back changes nothing on the hub: the
  path is the agreed revision's.
- The record lives in the device's database, not in the vault
  ([ADR-0003](0003-plain-markdown-files-are-the-source-of-truth.md)).
  `sync.leave` and `sync.reset` drop it with the remote.

## Alternatives considered

- **Record first, then change the disk, with a journal the scan
  finishes:** the vault would commit the agreed row before the rename and
  a journal would tell the next scan to finish or undo it. It changes the
  vault's write path, which every writer shares, for a problem only sync
  has, and a scan that finds the change undone must also undo the agreed
  row. The syncer's record keeps the change inside sync.
- **Let the hub refuse a stale base:** the hub would need a revision
  check, not a content check, on deletes and moves, which refuses
  legitimate pushes from a device that was offline. The hub cannot tell a
  stale push from a real one; only the device knows it changed nothing.

## References

- Shapes: [specs/sync-protocol.md](../specs/sync-protocol.md#pulling-and-applying), [design/sync.md](../design/sync.md#state), [design/vault.md](../design/vault.md#writes)
- Builds on: [ADR-0005](0005-sync-through-a-hub-with-revisions-and-three-way-merges.md), [ADR-0017](0017-revisions-number-the-hubs-log-and-hashes-name-content.md), [ADR-0020](0020-test-sync-with-a-seeded-simulation-of-a-hub-and-devices.md)
