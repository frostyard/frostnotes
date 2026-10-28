# Design: sync

Living document. Rationale: [ADR-0005](../adr/0005-sync-through-a-hub-with-revisions-and-three-way-merges.md),
[ADR-0006](../adr/0006-every-write-names-the-version-it-changes.md),
[ADR-0016](../adr/0016-pair-devices-with-a-one-time-code-and-a-revocable-token.md) (pairing),
[ADR-0017](../adr/0017-revisions-number-the-hubs-log-and-hashes-name-content.md) (revisions),
[ADR-0018](../adr/0018-move-blobs-over-http-named-by-their-hash.md) (blobs),
[ADR-0019](../adr/0019-keep-every-revisions-record-and-thin-the-content-by-age.md) (retention),
[ADR-0020](../adr/0020-test-sync-with-a-seeded-simulation-of-a-hub-and-devices.md) (the simulation).
Contracts: [specs/sync-protocol.md](../specs/sync-protocol.md) and
[specs/merge.md](../specs/merge.md), written in
[plan 0004](../plans/0004-m2-sync.md)'s Phase 2.

**Status: built** through [plan 0004](../plans/0004-m2-sync.md)'s Phase 5:
`internal/hub` (pairing, the token check, the `/sync` WebSocket, pushes
as writes through the vault's actor), `internal/syncer` (the device's
loop), `internal/blobxfer`, `vault.Apply` and `vault.Push`, and the
seeded simulation, `internal/synctest` ([Testing](#testing)). Scenario
cards (Phase 6) and the exit evidence (Phase 7) are next. The specs are
the contract.

## Overview

Each vault syncs through one hub in a star. A hub is a notesd started with
`--hub`, serving some of its vaults. Devices push changed files with the
revision they last agreed on. The hub writes each change into its own copy
of the vault, merging three ways when the device was behind, appends a
revision to the vault's log, and announces it so other devices pull it. A
device applies what it pulls only where nothing changed since; it never
merges. The browser and remote agents (M4) write on the hub directly, as
local writers there.

```
 laptop notesd ──push(base)/pull(cursor)──┐  /sync WebSocket, device token
          └──── PUT/GET /sync/blobs ──────┤
                                          ▼
 desktop notesd ──────────────────▶ hub notesd ── revision log + blobs + merge
                                          ▲  └── vault directory (plain files: the heads)
 browser, remote agents (M4) ─────────────┘
```

## Design

### What syncs

Notes and attachments: the files a vault's index has
([vault.md](vault.md#index)). Folders are not entries, so an empty folder
does not sync, and a device removes a folder that applying a pulled delete
or move leaves empty. `.frostnotes/`, hidden paths and the paths a device
ignores stay on that device; a device skips pulled revisions at paths it
ignores. A file over 10 MB syncs but does not merge.

### State

- **Hub, per served vault:** a sync ID (random, 128 bits) and the log.
  `revisions`: sequence number (from 1, without gaps), file ID (the hub's
  `files.id`), path, content hash or deletion, base sequence number,
  device, source and author, time, for a conflict revision the revision it
  lost to, and the push ID it answers. Per file, the head (its latest
  revision that is not a conflict revision) and a conflict flag. The hub's
  vault directory holds the heads; contents are in its blob store.
- **Hub, devices:** each paired device's ID, its name, the SHA-256 of its
  token, and when it was paired and last seen; pending pairings, for 10
  minutes.
- **Device, per synced vault:** the hub's URL and ID, the hub vault's sync
  ID, the cursor (the highest sequence number whose heads it has applied),
  and per file its agreed state: the hub's file ID, the agreed revision,
  path and hash, and the conflict flag. The device token is in the Secret
  Service, keyed by the hub's ID.

A file whose path, hash or existence differs from its agreed state has a
change to push, and a file without an agreed state is a create. Nothing
else records pending changes, so a crash or an outage loses none
([ADR-0017](../adr/0017-revisions-number-the-hubs-log-and-hashes-name-content.md)).

### Device loop

1. **Detect.** Local writes and scanned outside edits change the index,
   and the loop compares it with the agreed state. It pushes a changed
   file after 2 seconds without another change to it, and at most 30
   seconds after its first unpushed change.
2. **Upload.** It asks the hub which of the changed contents it lacks,
   and uploads those ([blob transfer](#blob-transfer)).
3. **Push** `{file ID or none, base, path, hash or deleted, push ID}` for
   each changed file. The hub answers each with `accepted`, `merged`,
   `conflicted` or `refused`, and the head that results. A repeated push
   ID gets the first answer, so a push whose answer was lost is retried
   safely, and until it is answered no new change of its file is pushed:
   two creates of one file in flight would make two files on the hub. A
   writer's content that lost a merge on the device goes too, as a
   conflict item, so the hub's log keeps it.
4. **Apply the answer.** The device writes the head (the merged content,
   or the head that refused its delete), expecting the state it pushed,
   and records the head as agreed. If the file changed meanwhile, nothing
   is written and the agreed state stays, so the next push, from the same
   base, merges again on the hub; a create the hub merged whose file only
   moved meanwhile takes the merge where it now is.
5. **Pull** heads after the cursor, when the hub announces new revisions
   on the WebSocket and at every reconnect. For each head, if the file
   still has its agreed hash and path, apply it (create, overwrite, move
   or delete) and advance the agreed state and the cursor in one `Tx`;
   otherwise push the local change first and take the answer. A head
   whose path an unchanged copy of another file still holds here parks
   that copy beside it first, and the copy's own head, later in the pull,
   moves it on; so does a pushed file a later answer of the same push
   moves. A swap or a ring of moves never waits on itself, and the hub
   orders one push's moves the same way.

Applying (`vault.Apply`) compares instead of merging: the file must still
hold the expected hash, checked again just before the rename, or nothing
is written. It streams the content from `blobs/` into the temporary file
at any size, guards the path as every write does, so a hub cannot write
outside the vault, and records a `sync` version with the revision's
author.

### Hub

Writes to one vault run one at a time under its lock, so the merge order is
the log order. A push is a write into the hub's vault with the base
revision's hash as its base, through M1's write path
([vault.md](vault.md#writes)), and its revision is appended in the same
`Tx` as the hub's version. Local writes and outside edits on the hub
append revisions the same way.

- **Base is head:** apply the change.
- **Base is behind:** merge (base, head, pushed) and append the result. A
  hunk that still conflicts takes the head's side; the pushed version is
  appended as a conflict revision, and the file is flagged.
- **Base content pruned** ([retention](#retention)): merge with the head
  as two creates are merged.
- **Create on an existing path:** merge the two with their common lines as
  the base. Two daily notes made offline from one template merge cleanly.
- **Delete with a stale base:** refused. The head stands (edit beats
  delete), and the device restores it from the answer.
- **Put on a deleted file:** the file comes back at the pushed path, or at
  `name 1.md` if another file has taken it.
- **Move:** changes the file's path. The latest move wins, and a move onto
  a taken path gets ` 1`. A move and an edit pushed concurrently both
  apply.
- **Paths that differ only by case** from a live file's path are refused;
  the device keeps the file and lists it in `notesctl sync status` until
  it is renamed.
- **Files that are not text, or are over 10 MB,** do not merge: the later
  push wins, the other stays in the log, and the file is flagged.

### Merge

`internal/merge` (planner-owned; [specs/merge.md](../specs/merge.md) is
the contract, and its 44 fixtures, M0's 30 among them, are the golden
tests, with a seeded property test):

- **Text files** (`.md` and other UTF-8 text such as `.base`) merge. Other
  files do not: the later push wins and the other stays in the log.
- **Frontmatter** merges by top-level key. Each key's lines are one unit,
  so keys that only one side touched keep their bytes. A key both sides
  changed differently takes the head's value and makes the merge a
  conflict.
- **Body:** diff3 by line. Where both sides only inserted lines at the same
  place, both insertions are kept, the head's first; this is what lets two
  daily notes made offline from one template, or two tag lists each grown
  by one tag, merge cleanly. A flow list both sides changed
  (`tags: [a, b]`, as Obsidian writes tags) merges as a list: the head's
  items, without those the incoming side removed, then its additions. A
  hunk both sides changed differently merges
  again by word (words and the whitespace between them); insertions at the
  same place conflict at that level, since interleaved words read worse
  than a conflict revision. Whatever still conflicts takes the head's side.
- **Diffs** use Myers' linear-space algorithm: a 10,000-line note with
  scattered edits merges in about 2 ms; two 10,000-line files with nothing
  in common take about 0.3 s and 3 MB (measured in M0).
- The merge is deterministic: the same three inputs always give the same
  bytes.

### Pairing and sessions

`notesctl hub join <url> <vault>` asks the hub to pair (`/sync/pair`) and
shows a one-time code of 8 characters. The user approves it on the hub's
machine, over its local socket (`notesctl hub approve <code>`; the web
app's Hub settings list pending pairings and revoke devices, but never
approve), and the device collects its token: 32 random
bytes, of which the hub keeps only the SHA-256. The device keeps the token
in the Secret Service's default collection, which it makes, as libsecret
does, when the session has none (a fresh autologin session has only
GNOME Keyring's `session` collection). Before it asks the hub, the
device writes and deletes a probe secret, so a keyring that cannot keep
the token fails the join before the hub knows the device. When the device
cannot keep a token it has collected, it withdraws the pairing
(`DELETE /sync/device`), and the hub forgets the device, which has never
made a request. Every sync request carries the token: the `/sync`
WebSocket, which serves JSON-RPC as caller `device`, and each blob
request. `notesctl hub revoke <device>` closes the device's connections
and ends its sync. Devices refuse a hub URL that is not `https://`, except
on loopback
([ADR-0016](../adr/0016-pair-devices-with-a-one-time-code-and-a-revocable-token.md)).
During M2 TLS came from `tailscale serve` on another machine. From M4
the hub is lodge's notes app: devices reach it at lodge's
`/device/notes/` with the same tokens, and move there on their own once
the hub records that URL (`notesctl hub url`), or with
`notesctl hub relocate`. Browsers and agents reach notesd through lodge
([hub.md](hub.md),
[ADR-0026](../adr/0026-notesd-runs-as-a-lodge-app.md)).

A device whose token the hub revoked, or that lost it, pairs again with
the same command. `sync.join` on a paired hub whose token fails answers
`revoked` (not `unavailable`, which says the hub cannot be reached), and
`notesctl hub join` then says so, runs the pairing flow once and joins
again. The new pairing replaces the hub's row and the token, and wakes
the loops of every vault synced with that hub, which leave state
`revoked` without a restart. The hub knows the device as a new device
(the protocol's [URLs and tokens](../specs/sync-protocol.md#urls-and-tokens)).
A vault that stays joined keeps its sync state; one that left
(`notesctl sync leave`) joins again. The flow needs a vault that is not
joined: `hub join` for a vault still joined answers `conflict` before it
checks the token. To renew the token of a hub whose vaults all stay
joined, leave one of them and join it again, or join another vault on
that hub; either new pairing serves every vault on the hub.

From M6 the app runs the same flow with no terminal: the vault
manager's Join panel asks for the hub's URL and the vault's name, shows
the code to approve, and waits for the approval as `notesctl hub join`
does; the settings' Sync section joins the open vault, leaves a hub, and
pairs again with a new code for a vault in state `revoked`
([specs/ui.md](../specs/ui.md#vault-manager)). A join pairs the device
first where it has not paired with the hub, and a hub that refuses a
paired device's token pairs again once, as the command does.

A device that joins with files already in its folder matches them by
history: a file whose hash is a revision of the hub's file at the same
path takes that revision as its agreed base, so an unchanged older copy
takes the head; other files are pushed as creates, merged with the hub's
file where one has the path; files only the hub has are pulled.

### Blob transfer

Contents move outside JSON-RPC, over `PUT`, `GET` and `HEAD` of
`/sync/blobs/<sha256>` with the device token. Both ends stream: an upload
or download goes to a temporary file while it is hashed, and is kept only
if the hash is the one named. A device asks which hashes the hub lacks and
uploads them before it pushes; the hub refuses a push whose content it
lacks, removes uploads that no revision names after 24 hours, hands out
only content its log names, and refuses blobs over 1 GiB. Downloads resume
with `Range`; uploads restart. A device runs at most 4 transfers at once
([ADR-0018](../adr/0018-move-blobs-over-http-named-by-their-hash.md)).

### Retention

The hub keeps every revision's record forever, and thins content by age:
every revision's for 30 days, each file's last of each day for a year,
then its last of each month; heads always, and conflict revisions while
their file is flagged. A push whose base content was pruned merges as a
create. The pass runs daily with each notesd's prune of its own 30 days of
versions
([ADR-0019](../adr/0019-keep-every-revisions-record-and-thin-the-content-by-age.md)).

## Testing

`internal/synctest` runs a hub and several devices in one process, each a
whole notesd over its own temporary vault, on loopback through a transport
that partitions, delays and drops answers. A seeded scheduler drives edits
with current and stale bases, agents' appends, creates (two daily notes
from one template), deletes, moves, outside edits, attachments, offline
periods, and crashes between a file write and its `Tx`, between an upload
and its push, and between the hub's commit and its answer. Every edit
inserts a unique token. At each quiet point, the checks are:

- every device's vault equals the hub's heads, byte for byte;
- every token no writer removed on purpose is in a head or in a conflict
  revision, so no write was lost;
- no file in any vault is a conflict copy;
- the log's sequence numbers increase without gaps, every revision's base
  exists, and no two live files share a path, ignoring case;
- every device's agreed state is the hub's heads, and its index equals a
  rebuild.

Stepwise runs replay a seed exactly; concurrent runs find races.
Concurrent runs edit on disk only files they create: another program's
edit of a file a pull is writing loses whichever renames first, a
window the comparison before each rename narrows but cannot close. A
failing seed, reduced to the steps that still fail, is a plan in
`internal/synctest/testdata/regressions`, which `make test` replays with
a few hundred seeded steps; `make sync-sim` runs M2's 10,000 over a hub
and three devices. Scenario tests drive a world by script, naming paths
while the oracle keeps its books (`script.go`), and run profiles: op
weights with checks of their own, among them ops the default plans never
pick, such as swaps, case-only renames, attachments over 10 MB,
revocations and rejoins from an older copy (`profile.go`)
([ADR-0020](../adr/0020-test-sync-with-a-seeded-simulation-of-a-hub-and-devices.md)).

## Operational notes

- Clocks do not order anything; order comes from the hub's sequence
  numbers. Times date revisions and drive retention.
- With the hub down, devices keep working; their changes wait as
  differences from the agreed state.
- A revoked token stops sync; the device keeps its vault.
- `notesctl sync reset` drops a device's sync state and joins again:
  files match by history, and differing ones merge as creates.
- A hub that lost its database has new file IDs and a new vault sync ID;
  its devices stop and ask for a reset.
- Back up the hub by copying its vault directory and data directory; the
  vault directory alone is a complete copy of the heads.
- A hub serves one person's devices: every paired device may join every
  vault it serves, and `log.vaults` lists them all. Serving people who
  must not see each other's vaults needs per-vault grants, which M2 does
  not have.
- Each device needs its own copy of a vault: two devices syncing one
  folder, such as one network share mounted on two machines, would push
  each other's changes as their own.
- A device that moves from a host notesd to the Flatpak leaves the hub
  there, joins again as a new device and revokes the old one
  ([app.md](app.md#operational-notes)).

## References

- Rationale: [ADR-0005](../adr/0005-sync-through-a-hub-with-revisions-and-three-way-merges.md), [ADR-0006](../adr/0006-every-write-names-the-version-it-changes.md), [ADR-0016](../adr/0016-pair-devices-with-a-one-time-code-and-a-revocable-token.md), [ADR-0017](../adr/0017-revisions-number-the-hubs-log-and-hashes-name-content.md), [ADR-0018](../adr/0018-move-blobs-over-http-named-by-their-hash.md), [ADR-0019](../adr/0019-keep-every-revisions-record-and-thin-the-content-by-age.md), [ADR-0020](../adr/0020-test-sync-with-a-seeded-simulation-of-a-hub-and-devices.md), [ADR-0031](../adr/0031-the-desktop-app-ships-as-a-flatpak-with-its-own-notesd.md) (the default keyring, a withdrawn pairing)
- Related: [overview.md](overview.md), [vault.md](vault.md), [app.md](app.md#history-and-merges) (conflicts in the merge view), [hub.md](hub.md) (the hub behind lodge, moving devices)
- Contracts: [specs/sync-protocol.md](../specs/sync-protocol.md), [specs/merge.md](../specs/merge.md)
- Built in: [plan 0004](../plans/0004-m2-sync.md) ([roadmap — M2](../plans/0002-roadmap.md)); joining a hub from the desktop app is G1 of [plan 0008 — M6](../plans/0008-m6-the-standalone-desktop-app.md#phase-4--gaps-found-by-the-review)
