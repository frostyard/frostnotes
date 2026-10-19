# 0028 — An agent's scope can limit writes to folders

- **Status:** Proposed
- **Date:** 2026-10-19

## Context

An agent reaches notes through lodge's `/mcp/notes` with a token whose
scope notes defines ([ADR-0026](0026-notesd-runs-as-a-lodge-app.md)).
Today a scope is `vault=<id>`, which reads one vault, or
`vault=<id>;write`, which also writes all of it. The engine enforces it,
not the tools' code (the M4 plan review's finding 9):
- a call naming another vault is `notFound`;
- a method marked `writes` is `methodNotFound` under a read scope.

Porter, frostyard's assistant, is a lodge app that owns none of the
user's data. It reaches notes as an agent, over lodge's local agent
socket on the same host (lodge's ADR-0014), with its own token. The user
decided what Porter may do in the vault (Porter's
[ADR-0005](https://github.com/frostyard/porter/blob/main/docs/adr/0005-memory-and-routines-live-in-the-vault.md)):
- it **reads the whole vault**;
- it **writes only under `Porter/`:** `Memory.md`, `Briefs/` and
  `Reviews/`;
- it **never writes `Porter/Routines/`,** whose notes are scheduled
  prompts the user writes. If Porter could write one, a hostile mail it
  read could plant one;
- it **appends to today's daily note** for capture, interactively and
  after the user confirms, and never edits text already there.

`vault=<id>;write` would let Porter, or an instruction smuggled in
through what it reads, rewrite any note. Porter's own policy is not a
boundary: the review that made the engine enforce the scope applies here
too.

The vault may be on a network share whose file system ignores case
(the user's is). There, `porter/routines/x.md` is the file
`Porter/Routines/x.md`, and a path spelled otherwise than the file on
disk is the same file ([design/vault.md](../design/vault.md#writes)).
The path guard refuses absolute paths, `.` and `..`, hidden segments and
any symlink on the way, but it does not normalize case or Unicode.

## Decision

**The scope grammar grows, additively, in protocol 1:**

```
vault=<id>[;write[=<prefix>[,<prefix>…]]][;except=<prefix>[,<prefix>…]][;daily]
```

- **A bare `;write`** still writes the whole vault, as today.
- **`write=`** limits writes to paths under its prefixes.
- **`except=`** takes paths under its prefixes out of every write,
  including `write`, `write=` and `daily`. **`except` always wins.**
- **`daily`** allows appending to today's daily note, and nothing more.
- **Reads** stay the whole vault.

Porter's token is `vault=<id>;write=Porter/;except=Porter/Routines/;daily`.

**How a scope is written:**
- The parts appear at most once each, in the order above. Anything else
  is an error, and lodge answers 403 at `initialize`, as it does for any
  scope notes cannot parse. So a scope reads one way, and its string
  form is canonical.
- A prefix names a folder: a vault-relative path that ends in `/`. It
  matches whole segments, so `Porter/` never matches `Porterhouse.md`.
  Without its final `/`, a prefix must pass the path guard. An empty
  prefix is an error: the whole vault is a bare `;write`.
- Prefixes are percent-encoded as URL path segments are, with `,`, `;`
  and `%` always encoded, so a scope stays ASCII in `Lodge-Scope`.
  `Porter/` is written as it is.
- `except=` needs a write right (`write`, `write=` or `daily`) to take
  from. Without one it is an error.

**How a path is judged:**
- The path is judged as the path guard accepts it, after it passes the
  guard.
- It is compared with each prefix segment by segment, after Unicode
  NFC normalization and case folding.
- Folding is what keeps `except` sound on a file system that ignores
  case. On one that keeps case, it makes `write=` admit other spellings
  of the same folder's name, which `except` folds the same way.

**What each write method needs:**
- **`note.create`, `write`, `upload`, `delete`, `mkdir` and `revert`:**
  the path must be writable. For `revert` of a deleted file, that is its
  last path.
- **`note.move`:** both the path and `to` must be writable.
- **Link updates:** a move's `updateLinks` rewrites other notes, and
  each rewrite is a write. Under a scope with `write=` or `except=`, a
  move whose link updates would write a note outside the scope is refused
  whole. The agent can move without `updateLinks`.
- **Folders (`note.moveFolder` and `note.deleteFolder`):** the folder,
  and for a move its destination, must be writable whole. Both must be
  under a write prefix, and no `except` prefix may lie at or under either.
  So `Porter/` cannot be moved or deleted while `Porter/Routines/` is
  excepted.
- **Attachments** are files like notes: `note.upload` under a write
  prefix is allowed. `write=` is about paths, not kinds of files.

**`daily`:**
- It admits `note.daily` and a new `note.appendDaily` for **today**
  only, in notesd's time zone, and for no other day.
- `note.daily` creates a missing daily note from the vault's daily-note
  settings and template, as it does for any caller.
- `note.appendDaily` appends text at the end of the daily note and
  creates it first when it is missing. It writes with the note's current
  hash as its base, under the vault's lock, so it never edits text that
  is there.
- `except` still applies: a daily note under an `except` prefix cannot
  be appended to.
- That Porter appends only after the user confirms is Porter's rule.
  Notes cannot know it and does not try.

**What a refused write answers:**
- A write to a path outside the scope answers **`invalidParams`.** Its
  message names the scope's writable folders and `daily`, so an agent can
  correct itself.
- The path is readable, so saying it does not exist would be untrue. The
  method works for other paths, so saying it does not exist would be
  untrue too.
- A scope with no write right refuses every writing method with
  `methodNotFound`, as today. The router admits `note.daily` and
  `note.appendDaily` under `daily`.
- A call that names another vault is still `notFound`.

**Enforcement:**
- The engine enforces all of this on the agent connection, as it does
  today's scope.
- The MCP tools offer only what the scope can do, as a convenience,
  never as the check.
- A notesd that predates this grammar cannot parse the new scopes, and
  fails closed with 403 at `initialize`.

## Consequences

- Porter can keep its memory and its output in the vault, where the user
  reads and edits them on every device, without being able to change
  anything else.
- A planted instruction can make Porter write only inside `Porter/`
  outside its routines, or append to today's note.
- The append is visible. Its version names Porter's token, and
  `note.revert` undoes it.
- Every write method gains a path check, and the folder and link-update
  rules are subtle. Each needs tests at its boundaries: a prefix and its
  neighbours, `except` inside `write=`, case and Unicode spellings, moves
  across the boundary in both directions, and a link update that would
  reach outside.
- Case folding admits more than a case-keeping file system would hold
  apart. Since `except` folds the same way, this widens nothing that is
  excepted.
- The MCP tools must cope with notes an agent may read but not write.
  Under a limited scope, `read_note` and `list_notes` say whether each
  note is writable, and the write tools say which folders they may
  write, so an agent rarely tries a write the engine refuses.
- `note.appendDaily` is a new method, additive in protocol 1. The MCP
  tool `append_to_daily_note` moves onto it for every scope, so an
  append never rewrites earlier text, even under a bare `;write`.
- Reads stay whole-vault. A token that reads only some folders is later
  work, and this grammar leaves room for a `read=` part.

## Alternatives considered

- **`vault=<id>;write;path=Porter/`** (the first sketch): it cannot take
  `Porter/Routines/` out, and the routines are the point.
- **A separate folder or vault for Porter:** a second vault would split
  the user's notes, and memory and briefs belong beside the notes they
  are about.
- **Enforcing the folders in Porter, or in the MCP tools:** that rests
  the boundary on the code an injected instruction is steering. The
  review that made the engine enforce scope rules it out.
- **`notFound` for an out-of-scope write:** consistent with another
  vault, but untrue for a note the agent can read. An agent told that a
  note it just read does not exist tends to try to create it.
- **`methodNotFound` for an out-of-scope write:** consistent with a read
  scope, but untrue for a method that works on other paths. An agent
  would conclude that it cannot write at all.
- **A new error code:** clearer, but `invalidParams` with a precise
  message says the same thing without a code that clients would have to
  learn.
- **Case-sensitive matching:** on a share that ignores case,
  `porter/routines/x.md` would escape `except=Porter/Routines/`.
- **Append-only `note.write` under `daily`,** checking that the new text
  extends the base: the merge rules make "extends" hard to state when the
  note changed meanwhile. An append done inside notesd needs no such
  check.

## References

- Shapes: [specs/lodge-app.md](../specs/lodge-app.md#mcp) (the scope),
  [specs/mcp.md](../specs/mcp.md#through-lodge) (the tools under a
  scope), [specs/rpc-protocol.md](../specs/rpc-protocol.md#caller-kinds)
  (an agent's scope binding its connection),
  [design/vault.md](../design/vault.md#daily-notes) (the daily note an
  append writes), [plan 0007](../plans/0007-folder-scopes-for-agents.md)
- Builds on: [ADR-0026](0026-notesd-runs-as-a-lodge-app.md) (notes
  defines the scope, and the engine enforces it),
  [ADR-0006](0006-every-write-names-the-version-it-changes.md) (an append
  is a write with a base), [ADR-0012](0012-agents-reach-notes-through-mcp.md)
- Elsewhere: lodge's
  [ADR-0006](https://github.com/frostyard/lodge/blob/main/docs/adr/0006-agents-reach-each-app-through-its-own-mcp-endpoint.md)
  (agents reach each app through its own MCP endpoint) and
  [ADR-0014](https://github.com/frostyard/lodge/blob/main/docs/adr/0014-agents-on-lodges-host-use-a-local-agent-socket.md)
  (the local agent socket); Porter's
  [ADR-0005](https://github.com/frostyard/porter/blob/main/docs/adr/0005-memory-and-routines-live-in-the-vault.md)
  (memory and routines in the vault)
