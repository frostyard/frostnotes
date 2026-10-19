# Spec: notesd RPC protocol

The wire rules for every notesd client. Methods, types and events are in the
generated [rpc-api.md](rpc-api.md). Rationale:
[ADR-0008](../adr/0008-generate-the-rpc-contract-from-a-yaml-idl.md); where
it fits: [design/overview.md](../design/overview.md).

## Transports

- **The Unix socket** at `$FROSTNOTES_SOCKET`, default
  `$XDG_RUNTIME_DIR/frostnotes/notesd.sock`. notesd creates the parent
  directory 0700 and refuses one that is not 0700 and owned by the user. The
  socket is 0600 and removed on shutdown. A stale socket is replaced; a live
  one makes a second notesd exit. notesd closes connections whose peer UID
  (`SO_PEERCRED`) is not its own. Socket connections are caller `local`.
- **WebSockets.** Each WebSocket text frame carries one message followed by
  `\n`, and the server reads the frames as one stream of lines. A WebSocket
  listener authenticates the connection during the upgrade and fixes its
  caller kind for its lifetime:
  - the development gateway (`notesd -devgw 127.0.0.1:PORT`, M0): loopback
    only, a token from `FROSTNOTES_DEVGW_TOKEN` as `?token=`, and an
    `Origin` from the Vite dev server; caller `local`, standing in for the
    app's bridge;
  - the hub's `/sync` (M2): caller `device`, authenticated by a device
    token ([sync-protocol.md](sync-protocol.md)); from M4 on the lodge
    app socket at `/device/sync`, still with the device token;
  - `/api` on the lodge app socket (M4): caller `session`, a browser
    that lodge signed in, named by the `Lodge-*` headers lodge sets
    ([lodge-app.md](lodge-app.md#api),
    [ADR-0026](../adr/0026-notesd-runs-as-a-lodge-app.md)).
- **The lodge app socket** (M4) is HTTP/1.1 on a Unix socket that only
  lodge's user may open; its requests' caller kinds come from
  `Lodge-Caller` ([lodge-app.md](lodge-app.md#requests-on-the-app-socket)).
- **In-process connections.** `/mcp` on the lodge app socket (M4) gives
  each MCP session a connection inside notesd, a `net.Pipe` whose server
  end `rpcserver.ServeConn` serves as caller `agent`, carrying the
  token's scope ([lodge-app.md](lodge-app.md#mcp)).
- Each message is one line: a JSON object followed by `\n`, at most 16 MiB
  (`api.MaxMessageSize`). JSON escapes newlines in strings, so a message is
  never split. notesd answers a request whose reply would be longer with
  `internal` instead, and sends no longer notification; the app's bridge
  ends a connection that sends a longer line.
- **Files' bytes travel beside the RPC,** by content hash
  ([ADR-0023](../adr/0023-the-app-shows-files-by-their-hash-and-uploads-them-through-notesd.md)):
  the desktop app reads `notesblob://localhost/<sha256>.<ext>` from
  notesd's blob store itself (`app/src-tauri/src/protocol.rs`), the
  development gateway serves `GET /blobs/<sha256>.<ext>?token=…` on its
  listener (M3), and notesd serves `GET /content/blobs/<mac>/<sha256>.<ext>`
  on the lodge app socket to lodge-content, whose sign-in admits the
  browser, the `mac` being an HMAC of the hash under the content key
  that `rpc.hello` gives a session ([lodge-app.md](lodge-app.md#contentblobs), M4). Each answers only those paths (64 lowercase hex digits
  and an extension of 1 to 8 lowercase letters or digits), with the
  content type the extension names ([design/app.md](../design/app.md#attachments)),
  `X-Content-Type-Options: nosniff`, a `Content-Security-Policy` of
  `default-src 'none'; style-src 'unsafe-inline'; sandbox`, immutable
  caching, and one `Range` per request.

## Caller kinds

A connection's caller kind is fixed when it is accepted:

| Kind | Who | How it is authenticated |
| --- | --- | --- |
| `local` | the desktop app, `notesctl`, `notesctl mcp`, the development gateway | the private socket's peer UID; the gateway's token |
| `session` | a browser running the web app inside lodge (M4) | lodge's sign-in, passed as `Lodge-Caller: session` on the app socket, which only lodge's user may open ([lodge-app.md](lodge-app.md#requests-on-the-app-socket)) |
| `device` | another notesd syncing with the hub (M2) | a device token ([sync-protocol.md](sync-protocol.md)) |
| `agent` | an MCP session behind lodge's `/mcp/notes` (M4), in process | lodge's token, passed as `Lodge-Token` and `Lodge-Scope` ([mcp.md](mcp.md#through-lodge)) |

Every domain in `schema/rpc` lists the caller kinds it admits
([rpc-api.md](rpc-api.md) shows each domain's list), and a method may
list its own `callers`, which narrow its domain's: each must be one of
the domain's (M4,
[ADR-0026](../adr/0026-notesd-runs-as-a-lodge-app.md)). A method that
changes notes is marked `writes`, which an agent's read scope refuses.
A request for a method that does not admit the connection's caller
returns `methodNotFound` (-32601), exactly as for a method that does not
exist, so a caller cannot probe for methods it may not call. A request
dispatched with no caller kind is refused the same way.

From M4:

- `note` admits `local`, `session` and `agent`; `vault` admits `local`
  and `session`, with `vault.add` and `vault.remove` narrowed to
  `local`. `hub` admits `local` and `session`, with `hub.serve`,
  `hub.approve` and `hub.setURL` narrowed to `local`
  ([lodge-app.md](lodge-app.md#methods-and-callers)). `rpc` admits
  every kind, and `events` every kind but `agent`. `sync` stays
  `local`, and `log` `device`.
- **An agent's scope binds its connection.** An `agent` connection
  carries its token's scope (`vault=<id>`, with `;write` to write): a
  call that names another vault answers `notFound`, as for a vault that
  does not exist, and a method marked `writes` answers `methodNotFound`
  under a read scope.
- **A connection names its writes.** For `session` and `agent`
  connections, every version a call records carries the connection's
  source and author. A session's source is `editor` and its author
  `<Lodge-Node-Name> (web)`. An agent's source is `agent` and its author
  its token's name (`Lodge-Token`). The `source` and `author` parameters
  of `note` methods are ignored on these connections. `local`
  connections keep the parameters and their defaults.
- **`rpc.hello` says who is connected:** its result gains the
  connection's caller kind and, for a session, the signed-in login, its
  display name and the node's name, from lodge's headers. The web app
  shows them, and hides what its caller may not use.
- **`rpc.ping`** answers `null` to every caller after hello: the web
  app's heartbeat, which finds a connection a sleeping phone dropped.

## Messages

JSON-RPC 2.0, without batches:

- Request: `{"jsonrpc":"2.0","id":<number|string>,"method":"<domain>.<name>","params":{…}}`.
  `params` is an object or omitted; omitted and `null` mean `{}`.
- Response: `{"jsonrpc":"2.0","id":…,"result":…}` or
  `{"jsonrpc":"2.0","id":…,"error":{"code":…,"message":"…","data":…}}`. A
  method without a result returns `"result":null`. Responses may arrive in
  any order; match them by `id`.
- Server notification: `{"jsonrpc":"2.0","method":"event","params":<EventEnvelope>}`.
- Client notifications (requests without `id`) are ignored.

Params are decoded strictly: an unknown member or a missing required member
is `invalidParams` (-32602). Malformed JSON is `parseError` (-32700) with a
`null` id; a non-2.0 or method-less object is `invalidRequest` (-32600); an
unknown or inadmissible method is `methodNotFound` (-32601).

## Connection setup

1. The client sends `rpc.hello {protocol, client}`. Until hello succeeds,
   every other method returns `notReady` (1004). A different protocol major
   version returns `protocolMismatch` (1003). Requests are handled in order
   until hello succeeds, so a client may pipeline hello and its next call.
2. After hello, requests run concurrently, up to 32 per connection; reading
   pauses while 32 are in flight.

Within protocol 1, changes are additive: new methods, new optional params,
new result fields, new events, new enum values, new callers on a domain.
Clients ignore unknown fields and unknown events.

## Events

- `events.subscribe {sinceSeq?}` returns `{seq, resync}`. Events are sent
  only after that response.
- `EventEnvelope` is `{"seq":n,"event":"<domain>.<name>","data":{…}}`. Durable
  events have `seq` > 0 and are kept in notesd's changes log; transient
  events omit `seq`.
- Without `sinceSeq` the client receives live events only. With `sinceSeq`,
  notesd first replays the retained durable events with `seq > sinceSeq`, in
  order, then live events, delivering each `seq` once. If `sinceSeq` is older
  than the retained log, ahead of the latest `seq`, or negative, the result
  has `resync: true`, nothing is replayed, and the client must refetch its
  state.
- An event reaches a connection only when its domain admits the
  connection's caller (`api.EventAdmits`, generated from each domain's
  `callers`), as a method does: a device on a hub receives `log` events
  and none of the hub's `note` or `vault` events.
- A connection may subscribe once (`conflict` (1002) after that).
- A subscriber more than 1024 events behind is disconnected; it reconnects
  and resumes with its last `seq`.

## Clients

- Go: `api.Dial(ctx, socket, clientName)` connects and completes hello;
  `Client.<Domain>()` returns the typed methods; `Client.Notifications()`
  delivers events (the client fails with `ErrEventsOverflow` if 1024
  undelivered events pile up).
- TypeScript: `JsonRpcSession` over a line channel; the Tauri transport uses
  the bridge's `notesd_connect` and `notesd_send`, and the WebSocket
  transport sends one line per frame (M0 Phase 4). In the web app,
  inside lodge, it connects to `wss://<the page's host>/app/notes/api`,
  with no token, and its blob URLs are lodge-client's `contentURL` (M4,
  [design/app.md](../design/app.md#in-a-browser)).

## References

- Rationale: [ADR-0004](../adr/0004-a-notes-daemon-with-thin-clients.md), [ADR-0008](../adr/0008-generate-the-rpc-contract-from-a-yaml-idl.md), [ADR-0016](../adr/0016-pair-devices-with-a-one-time-code-and-a-revocable-token.md) (device tokens), [ADR-0026](../adr/0026-notesd-runs-as-a-lodge-app.md) (the lodge app socket, the session and agent callers, method-level callers)
- Design: [design/overview.md](../design/overview.md), [design/hub.md](../design/hub.md)
- Related: [lodge-app.md](lodge-app.md) (what notesd serves to lodge); lodge's [app contract](https://github.com/frostyard/lodge/blob/main/docs/specs/app-contract.md)
- Generated reference: [rpc-api.md](rpc-api.md)
