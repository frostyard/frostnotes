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
  - the hub's endpoints (M2 and M4): caller `device` or `session`, specified
    with the hub's authentication.
- Each message is one line: a JSON object followed by `\n`, at most 16 MiB
  (`api.MaxMessageSize`). JSON escapes newlines in strings, so a message is
  never split.

## Caller kinds

Every domain in `schema/rpc` lists the caller kinds it admits (`local`,
`session`, `device`; [rpc-api.md](rpc-api.md) shows each domain's list).
A request for a method whose domain does not admit the connection's caller
returns `methodNotFound` (-32601), exactly as for a method that does not
exist, so a caller cannot probe for methods it may not call. A request
dispatched with no caller kind is refused the same way.

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
  transport sends one line per frame (M0 Phase 4).

## References

- Rationale: [ADR-0004](../adr/0004-a-notes-daemon-with-thin-clients.md), [ADR-0008](../adr/0008-generate-the-rpc-contract-from-a-yaml-idl.md)
- Design: [design/overview.md](../design/overview.md)
- Generated reference: [rpc-api.md](rpc-api.md)
