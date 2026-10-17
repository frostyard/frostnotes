# 0017 — Revisions number the hub's log, and hashes name content

- **Status:** Accepted
- **Date:** 2026-10-16

## Context

[ADR-0005](0005-sync-through-a-hub-with-revisions-and-three-way-merges.md)
gives each hub vault an append-only log of revisions, each with a base
revision, and [ADR-0006](0006-every-write-names-the-version-it-changes.md)
lets a write name "the revision (for files notesd has seen) or content
hash" it started from. M1 built the local half: a write's base is the
SHA-256 of the content the writer read, every version's content is in a
content-addressed store, and a write that is behind merges before it
renames ([vault.md](../design/vault.md#writes)). File IDs are local
integers, kept across moves and never reused. Content can return to an
earlier hash (an undo, a revert), so a hash says what a file held, not
when. A pull needs an order that survives a hub restart and a lost
answer. The hub's vault directory holds the heads and has local writers
of its own: other programs on its disk now, the web app and remote agents
from M4.

## Decision

- **A revision is an entry in a hub vault's log:** a sequence number
  (from 1 in each hub vault, without gaps), the file's ID on the hub, the
  file's path, its content hash or its deletion, its base revision, the
  device (or the hub) and the source and author that made it, the time,
  and for a conflict revision the revision it lost to. A file's head is
  its latest revision that is not a conflict revision. The hub's file ID
  is the hub's own `files.id`.
- **The hash names content; the sequence number names history.** Local
  writers on any notesd keep naming the hash they read, as in M1. A device
  names to the hub the revision it last agreed on, with that revision's
  hash, which the hub checks.
- **Every change to a served vault is a revision,** appended in the same
  `Tx` as the hub's local version: a device's push, a local write on the
  hub, an outside edit its scan finds. The log and the hub's files agree
  after any crash: a crash between a rename and its `Tx` is found by the
  next scan, as in M1, and becomes a revision.
- **A device keeps an agreed state per file:** the hub's file ID and the
  agreed revision, path and hash. What differs between that state and its
  index is what it pushes; nothing else records pending changes.
- **A push names the file's whole new state** (its path, and its content
  hash or its deletion) **and its base revision.** The hub compares each
  part with the head, then appends, merges or refuses as ADR-0005 says,
  and every answer names the head that results.
- **A device never merges another device's content.** It applies a pulled
  head, or an answer's, only if the file still has its agreed hash and
  path, compared again just before the rename; otherwise it pushes its
  change first. Its local writers still merge among themselves by hash
  (ADR-0006), within the one device.
- **A pull brings heads.** For each file whose head is newer than the
  device's cursor, a pull returns the head, in sequence order. Earlier
  revisions are history, read on request.
- **Revisions are about files** the index has: notes and attachments.
  Folders are not entries, so an empty folder does not sync, and a device
  removes a folder that applying a pulled delete or move leaves empty.
  `.frostnotes/`, hidden paths, and paths a device ignores stay on that
  device.

## Consequences

- One counter per hub vault orders everything, and a device's place in
  sync is one cursor and its agreed rows.
- A device needs no base content for sync: the hub merges, with the base
  revision's content from its own store.
- The hub's merge is M1's write path with a revision added, so local and
  hub merges cannot drift apart.
- `internal/vault` gains an apply path that compares instead of merging,
  and a hook through which the hub appends revisions; both are planner
  code (ADR-0010).
- Losing the hub's database loses its file IDs and its log: devices see a
  new hub vault and must reset, which matches files by hash.
- `.frostnotes/` settings differ per device until a later milestone syncs
  them, and a file one device ignores still syncs between the others.

## Alternatives considered

- **The content hash as the revision:** a hash cannot order a log, and a
  file returning to earlier content would have one "revision" twice.
- **Per-device counters or vector clocks:** they serve many mergers; with
  one merger, the hub's counter is the order.
- **A revision for every local version:** every save of every device in
  the log; devices push their current state after a quiet period instead.
- **Devices merging what they pull:** two mergers could reach different
  results, which ADR-0005 rejected; M1's merging write path must not take
  pulls.
- **Folders as entries:** a log of directories for the rare empty folder,
  and a second kind of move.

## References

- Shapes: [design/sync.md](../design/sync.md#state), [design/vault.md](../design/vault.md#writes), [specs/sync-protocol.md](../specs/sync-protocol.md)
- Builds on: [ADR-0003](0003-plain-markdown-files-are-the-source-of-truth.md), [ADR-0005](0005-sync-through-a-hub-with-revisions-and-three-way-merges.md), [ADR-0006](0006-every-write-names-the-version-it-changes.md)
- Related: [ADR-0018](0018-move-blobs-over-http-named-by-their-hash.md), [ADR-0019](0019-keep-every-revisions-record-and-thin-the-content-by-age.md)
- Answers: the roadmap's question on empty folders ([plan 0002](../plans/0002-roadmap.md#open-questions))
- Built in: [plan 0004](../plans/0004-m2-sync.md)
