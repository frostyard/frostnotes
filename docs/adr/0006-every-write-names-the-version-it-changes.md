# 0006 — Every write names the version it changes

- **Status:** Accepted
- **Date:** 2026-10-16

## Context

Five kinds of writer touch the same note: the editor, other tools on disk,
sync, the web app and agents. Any of them writing blind can erase another's
edit: a stale editor buffer saved over a pulled revision, an agent's append
landing on a note changed a second earlier, vim saving over a merge.

## Decision

- **Every write carries its base:** the revision (for files notesd has
  seen) or content hash the writer started from. This applies to
  `note.write` from the app, the web app and MCP, to a device's push
  ([ADR-0005](0005-sync-through-a-hub-with-revisions-and-three-way-merges.md)),
  and to notesd applying a pulled revision to disk.
- **notesd compares, then writes or merges.** If the base is current, the
  write goes through. If not, notesd merges three ways with the same merge
  library the hub uses, writes the result, and returns the merged content
  and its new revision to the writer. A conflicting hunk follows the
  conflict rule of ADR-0005.
- **The disk is compared too.** Just before the rename, notesd re-reads the
  file. If it changed since notesd last read it (an outside edit not yet
  scanned), notesd merges that change in rather than overwriting it.
- **The editor saves as you type** (debounced) with its base. When notesd
  returns merged content, the editor applies only the difference as one
  transaction, keeping the cursor, selection and undo history.
- **Only a create has no base,** and it fails if the path exists.
- **Every version notesd sees is kept.** Each notesd keeps the content of
  every version it writes or scans in its content-addressed blob store,
  with a local version list per file, for 30 days by default (Obsidian's
  File recovery). Any write, an agent's included, can be reverted on the
  device that made it, with or without a hub. The hub's revision log
  ([ADR-0005](0005-sync-through-a-hub-with-revisions-and-three-way-merges.md))
  is the long-term history.

## Consequences

- No writer can lose another's edit. Lost edits become merge bugs, which
  golden tests can catch.
- The editor needs a diff-apply path from M3 on.
- Each pair of writers gets a test: editor and outside edit, editor and
  pull, agent and editor, push and push.
- Writes cost a read and a hash. Notes are small, so this is cheap.

## Alternatives considered

- **Last writer wins by mtime:** what most apps do; silently loses edits.
- **Locks:** outside tools and offline devices never take them.
- **Editor-only protection:** leaves agents, the web app and sync blind.

## References

- Shapes: [design/vault.md](../design/vault.md), [design/sync.md](../design/sync.md), [specs/sync-protocol.md](../specs/sync-protocol.md), [specs/merge.md](../specs/merge.md)
- Builds on: [ADR-0003](0003-plain-markdown-files-are-the-source-of-truth.md), [ADR-0005](0005-sync-through-a-hub-with-revisions-and-three-way-merges.md)
- Built in: [roadmap — M1](../plans/0002-roadmap.md)
