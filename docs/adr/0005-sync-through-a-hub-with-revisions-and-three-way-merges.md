# 0005 — Sync through a hub with revisions and three-way merges

- **Status:** Accepted
- **Date:** 2026-10-16

## Context

The user wants sync built in from the start, replacing Obsidian Sync.
Devices edit offline. Other tools edit the files on any device. The web app
and remote agents edit through the hub. An edit must never be lost, and
Syncthing-style conflict copies are the failure mode to avoid. Notes are
plain files with no IDs inside
([ADR-0003](0003-plain-markdown-files-are-the-source-of-truth.md)), and one
person owns all of them.

## Decision

- **Star topology.** Every device's notesd syncs with one hub, a notesd in
  the hub role ([ADR-0004](0004-a-notes-daemon-with-thin-clients.md)).
  Devices never sync with each other.
- **The hub keeps the history.** Each vault has an append-only revision
  log: sequence number, file ID, path, operation (create, put, move,
  delete), content hash, base revision, author (a device, a web session or
  an MCP client) and time. Content lives in a content-addressed blob store.
  A file's head is its latest revision. The log gives every note a version
  list and a restore.
- **File identity is sync state.** The hub gives each file an ID, and a move
  is a revision of the same file. Devices keep path-to-ID in their index and
  send moves explicitly (a rename in the app or `notesctl`) or as detected (a
  delete and a create of the same content in one scan).
- **Devices push with a base, and only the hub merges.** A device sends each
  changed file with the revision it last agreed on. If that base is the
  head, the hub appends the change. If not, the hub merges three ways (the
  base, the head, the pushed content), appends the result and returns it,
  and the device writes it to disk. Because merges happen in one place, in
  log order, every device sees the same outcome.
- **Merge rules.** Text files merge; other files do not. Frontmatter merges
  key by key. The body merges by line (diff3), then by word inside a
  conflicting hunk. Two creates of the same path merge with their common
  lines as the base, so two daily notes made offline from one template
  merge. An edit beats a delete. A move and an edit both apply.
- **No conflict files and no conflict markers.** When a hunk still
  conflicts, the head takes the hub's side of that hunk, every other hunk
  merges, and the device's whole version stays in the log as a conflict
  revision. The note is flagged until the user resolves it in the app's
  merge view or with `notesctl`. For a non-text file, the later push wins,
  the earlier one stays in the log, and the file is flagged.
- **Pull by sequence.** A device keeps a cursor into the log and a
  WebSocket on which the hub announces new revisions. It applies a revision
  only if the file on disk still matches the last agreed revision;
  otherwise it pushes its edit first and takes the merged result.
- **Pairing.** A device joins a hub with a one-time code approved on the
  hub, and keeps its device token in the Secret Service. Tokens are
  revocable per device.
- **Not end-to-end encrypted.** The hub reads notes: it merges, indexes,
  serves the web app and answers MCP. It is self-hosted, and TLS protects
  the wire.
- **One user.** A hub serves one person's vaults, any number of them. Shared
  vaults are out of scope.

## Consequences

- Sync is our code end to end, so it is testable in one process: a hub and
  several devices, with a randomized simulation of offline edits, moves and
  deletes that must converge with every write reachable in the log.
- The merge library decides whether edits survive. The planner owns it,
  with golden tests per rule.
- The log grows with every save. The hub needs a retention policy (M2), and
  saves from the editor are coalesced before they are pushed.
- Whoever controls the hub's machine can read the notes.
- Two paths that differ only by case cannot coexist on Windows or macOS; the
  hub refuses a create that would collide that way.
- Live co-editing of one note from two places works only as fast merges,
  with no shared cursors.

## Alternatives considered

- **Bring your own sync (Syncthing, git):** what the user had: conflict
  copies, no merges, and nothing for the web app to talk to.
- **CRDTs per note (Automerge, Yjs):** conflict-free and good for live
  co-editing, but the CRDT's state is not the file, every outside edit must
  be diffed into it, history grows per keystroke, and Automerge needs cgo.
  Live co-editing is not a goal for one person.
- **Peer to peer:** no single history or merge authority, harder to test,
  and the web app needs a server anyway.
- **Merging on devices:** every device would have to reach the same result
  from the same inputs; one merger is simpler and deterministic.
- **Conflict copies or markers in the file:** litter the vault or break
  rendering for every other tool.
- **End-to-end encryption:** the hub could no longer merge, index, serve
  the web app or answer MCP. Revisit only with a design where clients do all
  four.

## References

- Shapes: [design/sync.md](../design/sync.md), [design/overview.md](../design/overview.md), [design/vault.md](../design/vault.md) (local deletes and conflict versions), [specs/sync-protocol.md](../specs/sync-protocol.md), [specs/merge.md](../specs/merge.md)
- Builds on: [ADR-0003](0003-plain-markdown-files-are-the-source-of-truth.md), [ADR-0004](0004-a-notes-daemon-with-thin-clients.md)
- Related: [ADR-0006](0006-every-write-names-the-version-it-changes.md)
- Built in: [roadmap — M2](../plans/0002-roadmap.md)
