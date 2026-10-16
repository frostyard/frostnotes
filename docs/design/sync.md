# Design: sync

Living document. Rationale: [ADR-0005](../adr/0005-sync-through-a-hub-with-revisions-and-three-way-merges.md),
[ADR-0006](../adr/0006-every-write-names-the-version-it-changes.md).
Contracts: `specs/sync-protocol.md` and `specs/merge.md` (M2).

**Status: designed, not built** (M2). The protocol below is a sketch; the
M2 specs are the contract.

## Overview

Each vault syncs through one hub in a star. Devices push changed files with
the revision they last agreed on. The hub appends each change to the
vault's revision log, merging three ways when the device was behind, and
announces new revisions so devices can pull them. The browser and remote
agents write on the hub directly, as local writers there.

```
 laptop notesd ──push(base)/pull(cursor)──┐
                                          ▼
 desktop notesd ──────────────────▶ hub notesd ── revision log + blobs + merge
                                          ▲  └── vault directory (plain files)
 browser, remote agents ──────────────────┘
```

## Design

### State

- **Hub, per vault:** `files` (file ID, current path, head revision,
  deleted flag, conflict flag) and `revisions` (sequence number, file ID,
  operation create, put, move or delete, path, content hash, base sequence,
  author, time, and for a conflict revision the revision it lost to).
  Content is in the hub's blob store. The hub's vault directory always
  holds the heads.
- **Device, per vault:** the hub's URL; the device token, in the Secret
  Service; the pull cursor (the last applied sequence number); and per file,
  its file ID, agreed revision and agreed hash. A file whose current hash
  differs from its agreed hash has a local change to push.

### Device loop

1. **Detect.** Local writes and scanned outside edits mark files changed.
   Editor saves are coalesced for 2 seconds of quiet before pushing.
2. **Upload.** Content goes to the hub's blob endpoint first
   (`PUT /blobs/<sha256>`, idempotent), so pushes carry only hashes.
3. **Push** `{fileID or none, path, op, base, hash}` for each changed file.
   The hub answers each with `accepted(seq)`, `merged(seq, hash)` or
   `conflicted(seq, hash, conflictSeq)`.
4. **Apply the answer.** For `merged` and `conflicted`, write the returned
   content with the pushed hash as the base (ADR-0006). If the user typed
   meanwhile, that is a new local change based on the returned revision.
5. **Pull** revisions after the cursor, when the hub announces new ones on
   the WebSocket and at every reconnect. For each, if the file still has
   its agreed hash, write the revision (create, overwrite, move or delete)
   and advance the agreed revision and the cursor in one `Tx`; otherwise
   push the local change first and take the merged result.

### Hub

Pushes for one vault are applied one at a time, each in a `Tx`, so the
merge order is the log order.

- **Base is head:** append the change.
- **Base is behind:** merge (base, head, pushed) and append the result. A
  hunk that still conflicts takes the head's side; the pushed version is
  appended as a conflict revision, and the file is flagged.
- **Create on an existing path:** merge the two with their common lines as
  the base. Two daily notes made offline from one template merge cleanly.
- **Delete with a stale base:** refused. The head stands (edit beats
  delete), and the device restores it on its next pull.
- **Put on a deleted file:** the file comes back at its last path, or at
  `name 1.md` if another file has taken the path.
- **Move:** changes the file's path. The latest move wins, and a move onto
  a taken path gets ` 1`. A move and an edit pushed concurrently both apply.
- **Paths that differ only by case** from an existing path are refused.

### Merge

`internal/merge` (planner-owned; 30 golden cases from the M0 spike in
`merge_test.go`, which become M2's golden tests, and a seeded property test):


- **Text files** (`.md` and other UTF-8 text such as `.base` and
  `.frostnotes/*.yaml`) merge. Other files do not: the later push wins and
  the other stays in the log.
- **Frontmatter** merges by top-level key. Each key's lines are one unit,
  so keys that only one side touched keep their bytes. A key both sides
  changed differently takes the head's value and makes the merge a
  conflict.
- **Body:** diff3 by line. Where both sides only inserted lines at the same
  place, both insertions are kept, the head's first; this is what lets two
  daily notes made offline from one template, or two tag lists each grown
  by one tag, merge cleanly. A hunk both sides changed differently merges
  again by word (words and the whitespace between them); insertions at the
  same place conflict at that level, since interleaved words read worse
  than a conflict revision. Whatever still conflicts takes the head's side.
- **Diffs** use Myers' linear-space algorithm: a 10,000-line note with
  scattered edits merges in about 2 ms; two 10,000-line files with nothing
  in common take about 0.3 s and 3 MB (measured in M0).

- The merge is deterministic: the same three inputs always give the same
  bytes.

### Pairing and sessions

`notesctl hub join <url> <vault>` asks the hub to pair and shows a one-time
code. The user approves it on the hub (the web app's devices page or
`notesctl hub approve <code>`), and the device receives a token for that
hub, kept in the Secret Service and revocable per device. Browser sign-in
and remote MCP tokens are decided in the hub's authentication ADR (before
M4).

## Testing

`internal/synctest` runs a hub and several devices in one process, each a
notesd over its own temporary vault, on loopback. A seeded random scheduler
drives edits at random positions, creates, deletes, moves, outside edits,
offline periods, and crashes between a file write and its `Tx`. Every edit
inserts a unique token. When the run goes quiet, the checks are:

- every device's vault equals the hub's heads, byte for byte;
- every inserted token is in a head or in a conflict revision, so no write
  was lost;
- no file in any vault is a conflict copy;
- the log's sequence numbers increase without gaps, and every revision's
  base exists.

A failing seed is a regression test.

## Operational notes

- Clocks do not matter; order comes from the hub's sequence numbers.
- With the hub down, devices keep working and queue their changes.
- A revoked token stops sync; the device keeps its vault.
- `notesctl sync reset` drops a device's sync state and re-syncs: identical
  files match by hash, and differing ones merge as creates.
- Back up the hub by copying its vault directory and data directory; the
  vault directory alone is a complete copy of the heads.

## References

- Rationale: [ADR-0005](../adr/0005-sync-through-a-hub-with-revisions-and-three-way-merges.md), [ADR-0006](../adr/0006-every-write-names-the-version-it-changes.md)
- Related: [overview.md](overview.md), [vault.md](vault.md)
- Built in: [roadmap — M2](../plans/0002-roadmap.md)
