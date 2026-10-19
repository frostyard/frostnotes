# 0028 — Remote MCP serves local agents with scoped tokens

- **Status:** Proposed
- **Date:** 2026-10-18

## Context

[ADR-0012](0012-agents-reach-notes-through-mcp.md) put MCP on the hub at
`/mcp`, over Streamable HTTP, with per-client tokens scoped to vaults
and to read or read-write. It noted that claude.ai's connectors expect
OAuth, and left that to M4. The roadmap's M4 row asked for Claude
Desktop or claude.ai to use the hub's MCP.

claude.ai, the Claude apps and Claude Desktop's connectors reach a
remote MCP server from Anthropic's side. That needs a public HTTPS
endpoint and MCP's authorization: OAuth 2.1 with discovery, client
registration and a consent page. The hub is on the tailnet only
([ADR-0026](0026-the-hub-stays-on-the-tailnet-and-joins-it-itself.md)).

Claude Code and other local MCP hosts run on the user's own machines.
They connect to a Streamable HTTP server directly and can send a fixed
`Authorization` header (`claude mcp add --transport http … --header`).
Today they use `notesctl mcp` over stdio, which needs a local notesd
with the vault.

On 2026-10-18 the user decided that remote MCP in M4 is for Claude Code
and local agents only, over the tailnet, each with a scoped bearer token
issued and revoked through `notesctl`. claude.ai and the Claude apps
move to later work.

The MCP tools are a client of notesd's API
([specs/mcp.md](../specs/mcp.md)). Each write names its base, records
source `agent` and an author, and can be reverted. A note's text
reaches the agent marked as data.

## Decision

- **`/mcp` serves agents on the tailnet,** with the MCP SDK's Streamable
  HTTP handler and the tools `notesctl mcp` serves. claude.ai, the
  Claude apps and Claude Desktop's remote connectors are later work,
  with a public endpoint and OAuth 2.1. Claude Desktop keeps
  `notesctl mcp` over stdio on a device.
- **Each agent has its own token,** a bearer secret:
  - `fnm_` and 32 bytes from `crypto/rand` in base64url;
  - the hub keeps its SHA-256, never the token;
  - it has a name, unique on the hub, which is the user's label for the
    agent (`claude-code@laptop`), one vault, and one access, `read` or
    `write`;
  - it does not expire.

  One vault per token matches `notesctl mcp --vault` and keeps vault
  names out of the tools.
- **Tokens are issued only on the hub's socket.** `notesctl hub token
  issue NAME --vault V [--write]` prints the token once. Without
  `--write`, a token reads. `notesctl hub tokens` lists them, with when
  each was last used, and `notesctl hub token revoke NAME` revokes one.
  The web app's hub settings list and revoke tokens, and never issue
  one ([ADR-0027](0027-browsers-sign-in-with-their-tailnet-identity.md)).
- **Every request carries its token** as `Authorization: Bearer`.
  - A missing, unknown or revoked token is 401 with `WWW-Authenticate:
    Bearer`.
  - An MCP session belongs to the token that opened it; a request for
    it with another token is refused.
  - Revoking a token ends its sessions at once.
  - A request with an `Origin` header is 403: agents send none, and no
    browser page reaches `/mcp`.
- **The token's scope is what the hub builds for it.** Each MCP session
  gets the tool server for the token's vault, read-only for `read`
  (only the four read tools are listed, as with `--read-only`).
  - The tool server reaches the engine over an in-process connection
    of a new caller kind, `agent`, which `rpc` and `note` admit and no
    other domain does.
  - So a tool cannot reach the hub's devices, tokens or vault list,
    whatever it is asked.
- **The hub names an agent's writes:** source `agent`, author the
  token's name. The name the client gives in `initialize` is
  self-chosen, so it goes to the log only. As for sessions, the
  connection's attribution replaces the write's parameters.
- **Sessions are bounded:** at most 8 live MCP sessions per token, and a
  session idle for an hour ends. Each request is bounded by the RPC
  limits.
- **The tailnet is the fence, and the token the key.** `/mcp` does not
  ask that the peer be an allowed login, so an agent on a tagged node
  (a container, a build machine) can use a token. The hub logs each
  session's node.

## Consequences

- Claude Code on any of the user's machines reaches the vault without a
  local notesd or a synced copy. Its writes merge on the hub like any
  other writer's.
- A token sits in an agent's configuration file (`~/.claude.json`), in
  plain text. Whoever reads it can use the token from the tailnet until
  it is revoked. Its scope bounds the damage: one vault, read-only by
  default, and every write in the history under the token's name.
- Agents that insist on OAuth discovery cannot connect. claude.ai and
  the phone's Claude app cannot reach the notes in M4.
- `notesctl hub token` joins the hub's commands, and the hub's settings
  pane lists tokens.
- A new caller kind is internal to notesd. It changes nothing on the
  wire, and domains admit it explicitly.

## Alternatives considered

- **OAuth 2.1 now:** an authorization server, client registration, PKCE
  and a consent page are a large security surface. They serve clients
  that, without a public endpoint, cannot reach the hub anyway.
- **The tailnet identity for agents too:** an agent on the user's
  laptop would get the user's whole access, with no scope, no read-only
  mode and no attribution per agent.
- **Tokens for several vaults,** named in each tool's input: every tool
  would grow a vault argument, and a prompt could steer an agent to a
  vault the user meant to keep out.
- **A public endpoint behind Funnel for claude.ai:** ruled out by the
  user's decision and ADR-0026.

## References

- Shapes: [specs/mcp.md](../specs/mcp.md#the-hubs-endpoint), [specs/hub-http.md](../specs/hub-http.md), [design/hub.md](../design/hub.md#agents), [specs/rpc-protocol.md](../specs/rpc-protocol.md#caller-kinds), [plan 0006](../plans/0006-m4-web-app.md)
- Builds on: [ADR-0012](0012-agents-reach-notes-through-mcp.md), [ADR-0006](0006-every-write-names-the-version-it-changes.md), [ADR-0026](0026-the-hub-stays-on-the-tailnet-and-joins-it-itself.md), [ADR-0027](0027-browsers-sign-in-with-their-tailnet-identity.md)
- Moves to later: [roadmap](../plans/0002-roadmap.md#later) (claude.ai and the Claude apps)
