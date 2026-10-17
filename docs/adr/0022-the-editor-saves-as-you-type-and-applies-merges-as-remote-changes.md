# 0022 — The editor saves as you type and applies merges as remote changes

- **Status:** Accepted
- **Date:** 2026-10-17

## Context

[ADR-0006](0006-every-write-names-the-version-it-changes.md) makes every
write name its base: notesd merges a stale write three ways and returns
the merged content, and "the editor applies only the difference as one
transaction, keeping the cursor, selection and undo history". M3 builds
that editor and must settle how:

- when it saves, and what it does with edits typed while a save is in
  flight;
- how merged content reaches a buffer that kept changing meanwhile;
- what undo does after another writer's change lands;
- how changes made by other writers (sync, an agent, vim, `notesctl`)
  reach an open note, announced by `note.changed`;
- what the user sees when a merge conflicts, which keeps the file's newer
  side of a hunk and the writer's content as a conflict version
  ([design/vault.md](../design/vault.md#writes));
- what happens when one note is open in two tabs.

The editor is CodeMirror 6 over the file's exact text
([ADR-0007](0007-edit-in-codemirror-6-with-live-preview.md)). Its history
maps the undo stack through changes that are not added to it, which is how
its collaborative-editing examples keep undo local. `note.write` takes
the whole content. Typing in a 10,000-line note must stay under 16 ms a
keystroke (the roadmap's M3 target; M0's spike measured 9 ms at p95 for
the decorations alone).

## Decision

- **One document per open note.** The app keeps one document per vault
  and path: its text, its base (the hash notesd last confirmed, from
  `note.read` or a write's answer) and the text that base names. Tabs
  showing the note are views of it; a change in one view is forwarded to
  the others, and the document saves once.
- **Saves are debounced:** 1 s after the last change and at most 10 s
  after the first unsaved one, and at once on Ctrl+S, when the editor
  loses focus, when its tab closes or shows another note, before a move
  or a delete, and when the window closes.
- **One write in flight per document.** A save sends `note.write` with
  the base and the whole text; edits typed meanwhile wait for its answer,
  and the next save carries the new base.
- **Merged answers apply as one remote transaction.** When the answer is
  `merged`, the app diffs the text it sent against the answer's content,
  maps that change over the edits typed since the send, and dispatches it
  as one transaction that is not added to the history. The cursor and
  selections map through it, and Ctrl+Z still undoes only the user's own
  edits. The base becomes the answer's hash; if edits were typed since
  the send, the next save follows.
- **Other writers' changes apply the same way.** On `note.changed` with a
  hash that is neither the base nor an answer the document is waiting
  for: with no unsaved edits, the app reads the note and applies the
  difference as a remote transaction; with unsaved edits, it saves at
  once, so notesd merges them into the new content and answers with the
  result. A write in flight is answered first. `note.moved` moves the
  document and its tabs. On `note.removed`, a document with unsaved edits
  is created again with them, since an edit beats a delete
  ([ADR-0005](0005-sync-through-a-hub-with-revisions-and-three-way-merges.md));
  otherwise its tabs say the note was deleted and offer to restore it.
- **Conflicts are shown, not hidden.** An answer with `conflicts` above
  zero applies like any merge, and the note's tabs show a notice naming
  the other writer, with Compare, which opens the merge view against the
  conflict version notesd kept. A base notesd refuses (its version is
  gone) reloads the note and opens the merge view with the editor's text
  as the other side, so nothing typed is lost and nothing is overwritten.
- **The merge view is `@codemirror/merge`** over two texts: the note's
  document, editable and saving as above, and another version
  (read-only): a conflict version, a hub revision, or any version from
  the history. Taking a chunk from the other side is an ordinary edit of
  the document, so it saves with its base and Ctrl+Z undoes it. Done
  clears the file's conflict flag on the hub (`sync.resolve`) when it has
  one.
- **The reading view renders the document's text,** not the file, so
  switching modes shows what was typed even before it is saved.

## Consequences

- No writer, the editor included, overwrites another: every merge is
  notesd's, every conflicting hunk the editor loses is kept as a version
  and offered back, and the editor never writes without a base.
- Undo stays the user's own: after a merge, Ctrl+Z undoes their typing
  within the merged text and never another writer's change.
- A save of a large note sends the whole text; saves run when typing
  pauses, so keystrokes do not pay for them. The exit evidence measures
  both.
- Applying a merge needs a diff of the sent text and the answer: the merge
  package's, with a time budget; past the budget, the change replaces the
  whole text and the cursor keeps its line.
- `@codemirror/merge` joins the app's dependencies (the planner adds it,
  pinned).
- Every writer pair gets a test with the editor in it: the editor and an
  outside edit, a pull, an agent's append, and a second tab.

## Alternatives considered

- **Replace the whole buffer with the merged content:** loses the cursor,
  the selection, the undo history and edits typed during the write.
- **Merge in the app:** a second merger in TypeScript beside
  `internal/merge`; ADR-0005 and ADR-0006 keep merges in one place.
- **Send changes instead of whole text:** notesd would need an
  operational write API; whole text with a hash base is what every other
  writer uses. Later, if large notes prove slow.
- **Save only on Ctrl+S or blur:** a crash loses everything since, and the
  user's Obsidian saves on its own.
- **Lock the file while it is open:** other tools and offline devices
  never take the lock (ADR-0006).

## References

- Shapes: [design/app.md](../design/app.md#editor), [specs/ui.md](../specs/ui.md#merge-view)
- Builds on: [ADR-0005](0005-sync-through-a-hub-with-revisions-and-three-way-merges.md), [ADR-0006](0006-every-write-names-the-version-it-changes.md), [ADR-0007](0007-edit-in-codemirror-6-with-live-preview.md)
- Built in: [plan 0005 — M3](../plans/0005-m3-desktop-app.md)
