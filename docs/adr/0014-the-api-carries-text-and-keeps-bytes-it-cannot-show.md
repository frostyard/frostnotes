# 0014 — The API carries text, and keeps the bytes it cannot show

- **Status:** Accepted
- **Date:** 2026-10-16

## Context

A note is read as bytes, and invalid UTF-8 is kept on disk exactly
([ADR-0003](0003-plain-markdown-files-are-the-source-of-truth.md),
[specs/markdown.md](../specs/markdown.md#notes-and-files)). The RPC API
carries a note's content as a JSON string, which must be valid UTF-8:
`note.read` shows each invalid sequence as U+FFFD, and a client cannot send
the original bytes back. Reviewing M1's batch 2 found the consequence: a
writer that reads such a note, edits one line and writes it back would
replace every stray byte in the note with U+FFFD, bytes it never saw and
never meant to change, although every write names its base
([ADR-0006](0006-every-write-names-the-version-it-changes.md)). Notes like
this are rare (the user's vault has none), but any program can make one,
and an outside edit can bring one into a note at any time. Binary
attachments are the same problem in full: a PNG is not text at all.

## Decision

- **The API carries text.** `content` stays a string in `note.read`,
  `note.write`, `note.create` and `note.daily`; no base64 field is added.
  `note.read` shows each sequence of bytes that is not UTF-8 as U+FFFD, and
  its `hash` is the hash of the bytes, as before.
- **A write keeps the bytes the writer could not see.** Before it merges,
  `note.write` puts back the base's invalid bytes wherever the writer's
  text leaves the words around them as they were: a three-way merge by
  line and then by word, whose base is the base as `note.read` showed it,
  whose winning side is the writer's text, and whose other side is the
  base's bytes, which differ from the shown text only where it reads
  U+FFFD. A writer that changes a word holding a stray byte replaces it; a
  writer that leaves it alone keeps it. The result is then written as any
  write is, merged with the file when the base is behind
  ([vault.md](../design/vault.md#writes)).
- **Bytes that are not text do not cross the API.** A client cannot put
  invalid UTF-8 in a JSON string; `notesctl write` refuses such input
  before sending it. Binary attachments are added to a vault by copying
  them into it; the app's upload of attachments (M3) gets a method of its
  own.

## Consequences

- No write through the API changes bytes the writer did not change, the
  rule ADR-0006 exists for, and no client has to know about encodings: the
  editor and agents edit text.
- A note with invalid bytes stays editable through the API, also after an
  outside edit introduces one.
- The returned `hash` of such a write is not the hash of the `content` the
  writer sent; writers already take the next base from the result.
- Binary content is out of the API until M3 designs the attachment
  upload. A `note.write` of an attachment whose bytes are not text, on a
  stale base, is refused rather than merged.

## Alternatives considered

- **An error:** refusing writes to notes that are not valid UTF-8 is safe
  but leaves such a note uneditable through the app and agents, from the
  moment any program writes one stray byte into it.
- **Base64 content:** a `contentBase64` beside `content` is exact for every
  file, but doubles every text field and makes each client choose between
  them, while the clients that edit notes edit text. Binary attachments
  deserve an upload path of their own rather than a variant of
  `note.write`.
- **Replacing the bytes:** writing U+FFFD over them, as Obsidian does when
  it saves such a note, loses bytes no writer chose to change.

## References

- Shapes: [design/vault.md](../design/vault.md#writes), [specs/rpc-api.md](../specs/rpc-api.md) (`note.read`, `note.write`), [specs/markdown.md](../specs/markdown.md#notes-and-files)
- Builds on: [ADR-0003](0003-plain-markdown-files-are-the-source-of-truth.md), [ADR-0006](0006-every-write-names-the-version-it-changes.md), [ADR-0008](0008-generate-the-rpc-contract-from-a-yaml-idl.md)
- Built in: [plan 0003, Phase 4](../plans/0003-m1-headless-vault.md#phase-4--engine-planner-alongside-phase-5)
