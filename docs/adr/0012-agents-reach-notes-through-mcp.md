# 0012 — Agents reach notes through MCP

- **Status:** Accepted
- **Date:** 2026-10-16

## Context

The user wants an MCP server in v1, so that agents (Claude Code, Claude
Desktop, claude.ai) can search, read and write notes. A local agent could
read the vault folder directly, but without the index, without safe writes
and without remote access. Agent writes are the least trusted writes, and
note content an agent reads can carry instructions.

## Decision

- **MCP tools are adapters over notesd's engine:** `search_notes`,
  `read_note`, `list_notes` (by folder, tag or property), `get_backlinks`,
  `query_notes` (property queries, once M5 has them), `create_note`,
  `edit_note` (replace a unique passage, append, prepend, set properties)
  and `append_to_daily_note`. They use the official Go SDK
  (`github.com/modelcontextprotocol/go-sdk`).
- **Locally, `notesctl mcp` is a stdio MCP server** that proxies to the
  local notesd, so an agent's config launches it like any MCP command. It
  can write unless started with `--read-only`.
- **On the hub, notesd serves MCP over Streamable HTTP at `/mcp`** with
  per-client tokens, each scoped to vaults and to read or read-write when
  it is created.
- **Every agent write is an ordinary write** with a base
  ([ADR-0006](0006-every-write-names-the-version-it-changes.md)),
  attributed to its MCP client in the history, and revertable in one step.
- **Tool results mark note content as data,** and tool descriptions say so,
  to blunt instructions planted in notes.

## Consequences

- Agents are useful from M1, before the app exists: the user's vault can be
  searched and edited through Claude Code from the first milestone.
- Every agent edit is visible and reversible in the history.
- claude.ai's remote connectors expect OAuth; the hub's token scheme may
  need an OAuth front (decided with the hub's auth ADR before M4).
- The MCP surface is hand-written, not generated, so it is tested against
  the engine like any other client.

## Alternatives considered

- **Agents read the files directly:** no search index, no backlinks, no
  safe writes, nothing remote.
- **A REST API for agents:** agents speak MCP.
- **Read-only MCP:** safer, but the user wants agents to write (daily-note
  appends, captures), and the history makes writes reversible.

## References

- Shapes: [design/overview.md](../design/overview.md)
- Builds on: [ADR-0004](0004-a-notes-daemon-with-thin-clients.md), [ADR-0006](0006-every-write-names-the-version-it-changes.md)
- Built in: [roadmap — M1 (local), M4 (hub)](../plans/0002-roadmap.md)
