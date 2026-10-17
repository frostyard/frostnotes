# Spec: sync protocol

How a device's notesd and a hub sync a vault: the hub's HTTP endpoints,
pairing, the `log` methods a device calls, what the hub does with a push,
and what a device does with what it pulls. `internal/hub` implements the
hub's side and `internal/syncer` the device's; `notesctl hub` and `notesctl
sync` drive them through the local `hub` and `sync` domains
([rpc-api.md](rpc-api.md)). Rationale:
[ADR-0005](../adr/0005-sync-through-a-hub-with-revisions-and-three-way-merges.md),
[ADR-0016](../adr/0016-pair-devices-with-a-one-time-code-and-a-revocable-token.md),
[ADR-0017](../adr/0017-revisions-number-the-hubs-log-and-hashes-name-content.md),
[ADR-0018](../adr/0018-move-blobs-over-http-named-by-their-hash.md),
[ADR-0019](../adr/0019-keep-every-revisions-record-and-thin-the-content-by-age.md);
where it fits: [design/sync.md](../design/sync.md). Merges follow
[merge.md](merge.md). Built in [plan 0004](../plans/0004-m2-sync.md).

## Interface

### Identifiers

| Name | Form | Assigned by |
| --- | --- | --- |
| hub ID | 32 lowercase hex digits (128 random bits) | the hub, the first time it serves |
| sync ID (`vault` in `log` calls) | 32 lowercase hex digits | the hub, when it serves a vault |
| device ID | integer > 0 | the hub, when a device collects its token |
| replica ID | integer > 0, one per join of a vault by a device | the hub, at `log.join` |
| file ID | integer > 0: the hub's `files.id`, kept across moves and deletes | the hub |
| sequence number (`seq`) | integer > 0, per hub vault, from 1 without gaps | the hub |
| push ID | integer > 0, increasing, per replica | the device |
| hash | 64 lowercase hex digits: SHA-256 of the content | either side |

### Endpoints

`notesd --hub ADDR` serves these on `ADDR` (plain HTTP; TLS is in front,
ADR-0016). Every other path answers 404.

| Path | Methods | Credential | Purpose |
| --- | --- | --- | --- |
| `/sync/pair` | `POST` | none | start a pairing |
| `/sync/pair/{pairing}` | `GET` | `Bearer` pairing secret | wait for approval and collect the token |
| `/sync` | `GET` (WebSocket upgrade) | `Bearer` device token | JSON-RPC as caller `device` |
| `/sync/blobs/{hash}` | `HEAD`, `GET`, `PUT` | `Bearer` device token | contents |

- A request whose `Origin` header is present and is not exactly
  `notesd://device` is refused with 403: a device sends that origin, or
  none, and a browser always sends its page's own, which can never be it.
- A missing, unknown or revoked device token is 401, with
  `WWW-Authenticate: Bearer` and no detail.
- Error responses are `text/plain` with one line saying why.

### Pairing

`POST /sync/pair`, body `{"name": "laptop"}` (JSON, at most 1 KiB; the
name 1 to 64 bytes of UTF-8 without control characters):

```json
{"pairing": "9f2c…(32 hex)", "secret": "(43 base64url characters)", "code": "7K4M-Q2XD",
 "hub": "(hub ID)", "expiresAt": "2026-10-16T21:10:00.000Z"}
```

- 201 with the body above; 400 for a bad body, 413 over 1 KiB, 429 while
  8 pairings are pending.
- The code is 8 characters of Crockford's base32 alphabet
  (`0123456789ABCDEFGHJKMNPQRSTVWXYZ`), shown as two groups of four. A
  pairing lasts 10 minutes. Pending pairings live in the hub's memory: a
  restart forgets them.
- `hub.approve {code}` (local socket only) approves the pending pairing
  with that code. Codes compare without case, spaces or `-`, reading `O`
  as `0` and `I` and `L` as `1`.

`GET /sync/pair/{pairing}` with `Authorization: Bearer <secret>` waits up
to 30 s for the pairing to change:

| Status | Body | Meaning |
| --- | --- | --- |
| 200 | `{"state": "pending"}` | not yet approved; ask again |
| 200 | `{"state": "approved", "token": "fnd_…", "device": 7, "hub": "(hub ID)"}` | approved; the pairing is gone |
| 404 | | no such pairing, a wrong secret, or already collected |
| 410 | | expired |

The hub creates the device token when it is collected: `fnd_` followed by
32 bytes from `crypto/rand` in unpadded base64url. It stores the
SHA-256 (hex) of the whole token, never the token. A device that loses the
approved answer pairs again; the unused device shows in `hub.devices` as
never seen and can be revoked.

### The sync connection

`GET /sync` upgrades to a WebSocket with the device token. The hub serves
JSON-RPC on it as specified in [rpc-protocol.md](rpc-protocol.md), caller
`device`, for the token's device. The device calls `rpc.hello`, then
`events.subscribe` without `sinceSeq`, then `log` methods. Revoking the
device closes its connections. Events reach a connection only when their
domain admits its caller, so a device receives `log.appended` and none of
the hub's `note` or `vault` events.

### `log` methods (caller `device`)

Every method but `log.vaults` names a hub vault by its sync ID (`vault`);
an unknown one, or one the hub no longer serves, is `notFound`. A replica
must be one of the connection's device (`notFound` otherwise).

| Method | Params | Result |
| --- | --- | --- |
| `log.vaults` | | `[]LogVault` `{sync, name, seq}`: every served vault, `seq` its latest |
| `log.join` | `vault` | `Replica` `{id, vault, seq}`: a new replica for this device |
| `log.missing` | `vault`, `hashes` (at most 1,000) | `[]string`: the hashes the hub has no content for |
| `log.push` | `vault`, `replica`, `items []PushItem` (at most 100) | `[]PushAnswer`, one per item, in order |
| `log.pull` | `vault`, `after`, `limit` (default 500, at most 1,000) | `LogPage` `{files []LogFile, next, more}` |
| `log.match` | `vault`, `files []MatchQuery` `{path, hash}` (at most 1,000) | `[]LogMatch` `{path, hash, file?, revision?}` |
| `log.history` | `vault`, `file`, `before?`, `limit` (default 50, at most 200) | `[]Revision`, newest first, below `before` |
| `log.pushes` | `vault`, `replica`, `pushes []int` (at most 1,000) | `[]PushRecord` `{push, revisions []Revision}` |
| `log.resolve` | `vault`, `file`, `seq` | `LogFile`: the conflict flag cleared |

Event `log.appended {vault, seq}` (transient): the vault's log grew to
`seq`.

`PushItem`:

| Field | Type | Required | Meaning |
| --- | --- | --- | --- |
| `push` | int | yes | the push ID |
| `file` | int | no | the hub's file ID; absent for a create |
| `base` | int | yes | the agreed revision; 0 for a create |
| `baseHash` | string | yes | that revision's hash; empty for a create |
| `path` | string | yes | the file's path on the device |
| `hash` | string | no | its content's hash; absent when `deleted` |
| `deleted` | bool | no | the device deleted the file |
| `source` | `VersionSource` | yes | who changed it on the device (editor, agent, outside) |
| `author` | string | yes | the writer's name there; empty for an outside edit |
| `at` | time | yes | when the device recorded the change |

`PushAnswer`: `push`; `status` (`accepted`, `merged`, `conflicted` or
`refused`); `reason` when refused (`RefuseReason`, below); `file`, the
`LogFile` after the push (absent only for a refused create); `conflicts`,
the hunks the head won; and `basePruned` when the base's content was gone
and the push merged as a create.

`LogFile`: `file`; `seq`, its latest revision of any kind; `revision`,
its head (the latest create, put, move or delete); `path`; `hash` (empty
when deleted); `deleted`; `size`; `conflicted`; and the head's `author`
and `deviceName` (`hub` for the hub's own writers).

`Revision`: `seq`, `file`, `op` (`create`, `put`, `move`, `delete`,
`conflict`, `resolve`), `path`, `hash` (empty for a delete), `size`,
`base`, `device` (absent for the hub's own writers) and `deviceName`,
`source`, `author`, `at`, `lostTo` (a conflict revision's winner), and
`pruned` (its content is gone, ADR-0019).

### Blobs

`/sync/blobs/{hash}`, with the device token; a `hash` that is not 64
lowercase hex digits is 400.

- `PUT` streams the body into the hub's blob store. 201 when stored, 204
  when the hub already had it; 400 when the body's SHA-256 is not `hash`
  (nothing is stored); 411 without `Content-Length`; 413 over the hub's
  limit (1 GiB unless `notesd --hub-max-blob` says otherwise), checked
  before reading.
- `GET` answers 200 with `Content-Type: application/octet-stream`,
  `Content-Length` and `ETag: "<hash>"`, and honors `Range` (206). `HEAD`
  answers the same without a body. Both are 404 unless a revision of a
  served vault names `hash` and its content is kept.
- A blob that no version, file or revision names is removed by the daily
  prune once it is 24 hours old, so an upload waits a day for its push.

### Local domains

On a device, `sync` (caller `local`):

| Method | Does |
| --- | --- |
| `sync.pair {url, name?}` | starts a pairing with the hub at `url` (default name: the host name) and returns `{pairing, code, hub, url, expiresAt}` |
| `sync.paired {pairing}` | waits up to 30 s; `{state: pending, approved or expired, hub}`; on approval keeps the token and records the hub |
| `sync.hubs` | the paired hubs: `{id, hub, url, device, pairedAt}` |
| `sync.join {vaultId, url, name}` | syncs the local vault with the hub vault called `name` on the paired hub at `url` |
| `sync.leave {vaultId}` | stops syncing it and drops its sync state; the files stay |
| `sync.status {vaultId?}` | `[]SyncStatus`: state (`idle`, `syncing`, `offline`, `revoked`, `reset`), cursor, hub sequence, pending and conflicted counts, refused files, last sync, error |
| `sync.conflicts {vaultId}` | flagged files, each with its conflict revisions since its last resolve |
| `sync.resolve {vaultId, path}` | clears a file's flag on the hub (`log.resolve`) |
| `sync.reset {vaultId}` | leave, then join again |
| `sync.verify {vaultId}` | `VerifyReport`: files that differ from the hub's heads, pushes the hub has no record of, flagged files |
| `sync.history {vaultId, path, limit?}` | the hub's revisions of a file (`log.history`) |

Event `sync.changed {status}` (transient) when a vault's sync state
changes.

On a hub, `hub` (caller `local`):

| Method | Does |
| --- | --- |
| `hub.status` | `{hub, listen?, vaults, devices, connected}` |
| `hub.serve {vaultId}` | serves a registered vault: gives it a sync ID and appends a create revision for each of its files, by path; `conflict` if it is served |
| `hub.vaults` | the served vaults: `{vaultId, sync, name, seq, servedAt}` |
| `hub.pending` | pending pairings: `{code, name, addr, createdAt, expiresAt, approved}` |
| `hub.approve {code}` | approves one; `notFound` for an unknown or expired code |
| `hub.devices` | `{id, name, pairedAt, seenAt?, connected, revoked}` |
| `hub.revoke {device}` | clears the device's token hash and closes its connections; its revisions keep its name |

## Rules

### URLs and tokens

- A device accepts a hub URL whose scheme is `https`, or `http` with a
  loopback host (`localhost`, `127.0.0.0/8`, `::1`); anything else is
  `invalidParams`. Endpoint paths are appended to the URL's path.
- The device keeps its token in its secret store under
  `hub/<hub ID>/token` and nowhere else. A 401 on any request marks every
  vault synced with that hub `revoked`, and sync with it stops until the
  device pairs again.
- The hub records a device's last request time, at most once a minute.

### Pending changes

A device has a change to push for a file when, comparing its index
(`files`) with its agreed state (`sync_files`):

| Index | Agreed state | Change |
| --- | --- | --- |
| a file | none | create |
| a file | the same path and hash | none |
| a file | another path or hash | put (with the new path and hash) |
| none | a row | delete |

- A changed file waits until it has gone 2 s without another change, or
  30 s since its first unpushed change, whichever comes first.
- A push the hub refused is not sent again until the file changes from the
  state that was refused; `sync.status` lists it.
- Items go out deletes first, then puts, then creates, so a path a file
  leaves is free before another file takes it; within a group, by path.

### A push, on the hub

Items are applied one at a time under the vault's lock, each through the
hub vault's write path ([vault.md](../design/vault.md#writes)) and each
revision in the same `Tx` as the hub's version.

1. **Repeats:** an item whose `push` this replica has sent before gets the
   first answer's status and reason, with the file's current state.
2. **Checks**, refusing with a reason:
   - `invalid`: a path the path guard refuses, a malformed hash, a create
     with `deleted`, or a `hash` with `deleted`;
   - `ignored`: the hub's settings ignore the path;
   - `missing`: the hub has no content for `hash`;
   - `unknown`: `file` is not a file of this vault;
   - `base`: `base` is not a revision of `file`, or its hash is not
     `baseHash`.
3. **Create** (no `file`):
   - at a path no live file has, ignoring case: a new file; `accepted`;
   - at a live file's exact path: merged into that file as two creates
     ([merge.md](merge.md#create)); `merged`, or `conflicted`;
   - at a path that differs only by case from a live file's: refused,
     `collision`.
4. **Delete:** if the head's content is the base's (its path may differ),
   the file is deleted; `accepted`. Otherwise refused, `edited`: an edit
   beats a delete, and the answer carries the head. A delete of a deleted
   file is `accepted` with no new revision.
5. **Put on a deleted file:** the file comes back, with its ID and the
   pushed content, at the pushed path, or at the first free of
   `name 1.ext`, `name 2.ext`, … if a live file has that path, ignoring
   case; `accepted`.
6. **Put on a live file:**
   - content: the pushed hash if the head's content is the base's or the
     pushed hash is the base's (then the head's content stays); otherwise
     a three-way merge of base, head and pushed content
     ([merge.md](merge.md)), or, for a file that is not UTF-8 or is over
     10 MB, the pushed content, with the file flagged;
   - path: the head's if the pushed path is the base's; otherwise the
     pushed path (the latest move wins), or `name 1.ext`, … if another
     live file has it, ignoring case;
   - a result equal to the head appends nothing; `accepted`.
7. **Base content pruned:** the content merges as two creates, head and
   pushed (ADR-0019), and the answer says `basePruned`.
8. **Conflicts:** a merge with conflicts appends the result (if it differs
   from the head) and then a `conflict` revision holding the pushed
   content, whose `lostTo` is the head that stands; the file is flagged,
   and the answer is `conflicted`. A file that is not text, or over 10 MB,
   is flagged when its later push wins; the content it replaced stays as
   its earlier revision.
9. A push that changes path and content appends a `move` and then a
   `put`.
10. Each appended revision emits `log.appended` after its `Tx` commits.

### The hub's own writers

On a served vault, every version the hub's notesd records appends a
revision in the same `Tx`: a local write or create (`put`, `create`), a
move (`move`), a deletion (`delete`), a conflict version (`conflict`), and
what a scan finds on disk (as the same ops, `source: outside`). Their
`base` is the file's head at the time; `device` is absent.

### Pulling and applying

- A device pulls after connecting, after each push, within 100 ms of a
  `log.appended` for the vault, and every 60 s without one. It asks for
  pages after its cursor until `more` is false. `next` is the last file's
  `seq` (or `after` for an empty page).
- For each `LogFile` (and each answer's `file`), in order:
  1. A file whose agreed revision is at `seq` or later is skipped.
  2. A path the device ignores, or whose guard refuses it, is skipped and
     listed in `sync.status`.
  3. **Unchanged locally** — the agreed row's file still has the agreed
     path and hash, or there is no agreed row and no file at the path —
     the head is applied (`vault.Apply`): created, rewritten, moved or
     deleted, expecting that local state.
  4. **Changed locally** — the file differs from the agreed state, or a
     file without an agreed row is at the path — the device pushes its
     change first and applies the answer.
- `vault.Apply` streams the content from the device's blob store,
  downloading it first if missing (`GET /sync/blobs/{hash}`, checked, 4 at
  a time), and compares the file with the expected state just before the
  rename; if it changed, nothing is written and rule 4 applies. It removes
  a folder that the delete or move leaves empty.
- In one `Tx` with the file's index rows: a version with `source: sync`
  and author `author@deviceName` (or `deviceName` when the author is
  empty, `hub` for the hub's own writers), and the agreed row set to the
  head (dropped for a delete). The cursor advances to a page's `next` once
  every file in the page is agreed or skipped.
- When an answer gives a local file a hub file that another local file is
  agreed with (a create merged into a file that moved onto its path), the
  local file at the head's path keeps it, and the other is deleted if it
  still holds its agreed content, or pushed first if not.

### Joining

`sync.join` calls `log.vaults` and `log.join`, records the remote, then
matches every file of the local vault by `log.match`: a file whose hash is
a revision of the file last at its path on the hub, live or deleted, is
agreed at that revision. Then the loop runs as always: unmatched files are
pushed as creates, and the pull from 0 brings the hub's heads, so a matched
older copy takes the head, and a file the hub deleted is deleted.
`sync.reset` drops the remote, its agreed rows and its push records, and
joins again with a new replica.

### Limits and timings

| What | Value |
| --- | --- |
| a pairing | 10 minutes; at most 8 pending |
| quiet before a push | 2 s, at most 30 s after the first change |
| items in one push | 100 |
| hashes in one `log.missing`, files in one `log.match` | 1,000 |
| pull page | 500 by default, at most 1,000 |
| concurrent transfers per hub | 4 |
| reconnect | after 1 s, doubling to 60 s, ±20% jitter; at once on a local change |
| a blob | 1 GiB by default |
| an unnamed blob | 24 hours |

## Derived artifacts

| Artifact | Derivation |
| --- | --- |
| `api/zz_generated.go`, `app/src/rpc/gen/api.ts`, [rpc-api.md](rpc-api.md) | `make gen` from `schema/rpc/hub.yaml`, `sync.yaml` and `log.yaml` |
| `internal/store/migrations/0002_sync.sql` | the hub's and devices' state in this spec |

## References

- Rationale: [ADR-0005](../adr/0005-sync-through-a-hub-with-revisions-and-three-way-merges.md), [ADR-0006](../adr/0006-every-write-names-the-version-it-changes.md), [ADR-0016](../adr/0016-pair-devices-with-a-one-time-code-and-a-revocable-token.md), [ADR-0017](../adr/0017-revisions-number-the-hubs-log-and-hashes-name-content.md), [ADR-0018](../adr/0018-move-blobs-over-http-named-by-their-hash.md), [ADR-0019](../adr/0019-keep-every-revisions-record-and-thin-the-content-by-age.md)
- Context: [design/sync.md](../design/sync.md), [design/vault.md](../design/vault.md#writes)
- Related: [rpc-protocol.md](rpc-protocol.md), [merge.md](merge.md)
- Built in: [plan 0004](../plans/0004-m2-sync.md)
