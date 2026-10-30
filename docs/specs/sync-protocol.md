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
[ADR-0019](../adr/0019-keep-every-revisions-record-and-thin-the-content-by-age.md),
[ADR-0034](../adr/0034-a-device-records-the-head-it-applies-before-it-changes-the-disk.md);
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
| `/sync/hub` | `POST` | none | the hub's ID, the URL it records and a relocation proof (M4, [lodge-app.md](lodge-app.md#device)) |
| `/sync/device` | `DELETE` | `Bearer` device token | a device that cannot keep its token has the hub forget its pairing |

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
  8 pairings are pending, or with `Retry-After` past a peer's requests
  without a token (30 at once, then one a second, shared with
  `POST /sync/hub`; [lodge-app.md](lodge-app.md#limits)).
- The code is 8 characters of Crockford's base32 alphabet
  (`0123456789ABCDEFGHJKMNPQRSTVWXYZ`), shown as two groups of four. A
  pairing lasts 10 minutes. Pending pairings live in the hub's memory: a
  restart forgets them. The 8 are the hub's, not a caller's: anyone who
  can reach the hub can hold them for 10 minutes, which delays a pairing
  but grants nothing.
- A device keys a hub by the hub ID the hub's answers carry, which is no
  secret. So `sync.paired` refuses (`conflict`) an approval whose hub ID
  is that of a hub this device syncs vaults with at another URL, before it
  keeps any token: otherwise a hub the user was led to pair with could
  take another hub's token and vaults. To pair with a hub that moved,
  leave its vaults (`sync.leave`) first.
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
SHA-256 (hex) of the whole token, never the token.

- **Before it asks to pair,** a device writes and deletes a probe key in its
  secret store. A store that cannot keep the probe cannot keep a token
  either, and the pairing fails as `unavailable` without a request to the
  hub: the store's prompt comes before the hub knows the device.
- **A device that cannot keep the token** after the hub handed it out (it
  refuses the hub's ID, the token does not store, or the hub does not
  record) withdraws the pairing: `DELETE /sync/device` with the token. Best
  effort, with a 10 s deadline.
- **A withdrawn pairing** is a device that never made a request: the hub
  deletes it and answers 204. A device that made a request is never
  deleted, since the hub's revisions may name it: the hub answers 409, and
  its owner revokes it with `hub.revoke`. A missing, unknown or revoked
  token is 401, as at every other device endpoint.
- A device that loses the approved answer pairs again; the unused device
  shows in `hub.devices` as never seen and can be revoked.

### The sync connection

`GET /sync` upgrades to a WebSocket with the device token. The hub serves
JSON-RPC on it as specified in [rpc-protocol.md](rpc-protocol.md), caller
`device`, for the token's device. The device calls `rpc.hello`, then
`events.subscribe` without `sinceSeq`, then `log` methods. Revoking the
device closes its connections; a connection whose token was checked as
the device was revoked is closed too, since the hub checks the device
again once it counts the connection. A device holds at most 64
connections at once, one per synced vault. Events reach a connection only when their
domain admits its caller, so a device receives `log.appended` and none of
the hub's `note` or `vault` events.

### `log` methods (caller `device`)

Every method but `log.vaults` and `log.hub` names a hub vault by its sync ID (`vault`);
an unknown one, or one the hub no longer serves, is `notFound`. A replica
must be one of the connection's device (`notFound` otherwise).

| Method | Params | Result |
| --- | --- | --- |
| `log.hub` | | `LogHub` `{hub, url?}`: the hub's ID and the URL it records for devices (`hub.setURL`, M4) |
| `log.vaults` | | `[]LogVault` `{sync, name, seq}`: every served vault, `seq` its latest |
| `log.join` | `vault` | `Replica` `{id, vault, seq}`: a new replica for this device |
| `log.missing` | `vault`, `hashes` (at most 1,000) | `[]string`: the hashes the hub has no content for |
| `log.push` | `vault`, `replica`, `items []PushItem` (at most 100) | `[]PushAnswer`, one per item, in order |
| `log.pull` | `vault`, `after`, `limit` (default 500, at most 1,000) | `LogPage` `{files []LogFile, next, more}` |
| `log.match` | `vault`, `files []MatchQuery` `{path, hash}` (at most 1,000) | `[]LogMatch` `{path, hash, file?, revision?, revisionHash?}` |
| `log.history` | `vault`, `file`, `before?`, `limit` (default 50, at most 200) | `[]Revision`, newest first, below `before` |
| `log.revision` | `vault`, `seq` | `Revision`: one revision, for `sync.readRevision` (M3) |
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
| `conflict` | bool | no | `hash` is a local conflict version: a writer's content that lost hunks in a merge on the device, for the hub's log ([Pending changes](#pending-changes)) |
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

- `PUT` streams the body into the hub's blob store, reading and checking
  it whether or not the hub holds the blob already: 201 when it matches;
  400 when the body's SHA-256 is not `hash` (nothing is stored); 411
  without `Content-Length`; 413 over the hub's limit (1 GiB unless
  `notesd --hub-max-blob` says otherwise), checked before reading. The
  answer is the same either way, so an upload tells a device nothing of
  what else the hub holds; the hub remembers for 24 hours which device
  uploaded what.
- A device may name content in a push, and `log.missing` reports it as
  present, only when the vault's log names it with its content kept, or
  the device uploaded it in the last 24 hours. The hub's blob store is
  shared by every vault on its notesd, served or not, and its other
  content is not the device's to learn or read. A push that names other
  content is refused `missing`; the device uploads it and pushes again.
- A device's download stops, keeping nothing, once it is longer than the
  device takes (1 GiB), so a hub cannot fill its disk.
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
| `sync.relocate {hub, url}` | moves the hub paired at `hub` to `url` when the hub there proves itself; `conflict` otherwise (M4, [lodge-app.md](lodge-app.md#device)) |
| `sync.join {vaultId, url, name}` | syncs the local vault with the hub vault called `name` on the paired hub at `url`; answers a `SyncStatus` that names the hub vault and the URL; `revoked` when the hub refuses the device's token or the device holds none (`unavailable` when the hub cannot be reached) |
| `sync.leave {vaultId}` | stops syncing it and drops its sync state; the files stay |
| `sync.status {vaultId?}` | `[]SyncStatus`: state (`idle`, `syncing`, `offline`, `revoked`, `reset`), cursor, hub sequence, pending and conflicted counts, refused files, last sync, error |
| `sync.conflicts {vaultId}` | flagged files, each with its conflict revisions since its last resolve |
| `sync.resolve {vaultId, path}` | clears a file's flag on the hub (`log.resolve`) |
| `sync.reset {vaultId}` | leave, then join again |
| `sync.verify {vaultId}` | `VerifyReport`: files that differ from the hub's heads, pushes the hub has no record of, flagged files |
| `sync.history {vaultId, path, limit?}` | the hub's revisions of a file (`log.history`) |
| `sync.readRevision {vaultId, seq}` | `RevisionContent`: a revision (`log.revision`) and its content, from the device's blobs or downloaded into them from the hub; `notFound` once pruned, at most 10 MB |

Event `sync.changed {status}` (transient) when a vault's sync state
changes.

On a hub, `hub` (caller `local`; from M4 also `session`, but for
`serve`, `approve` and `setURL`, [lodge-app.md](lodge-app.md#methods-and-callers)):

| Method | Does |
| --- | --- |
| `hub.status` | `{hub, listen?, url?, vaults, devices, connected}` |
| `hub.setURL {url}` | records the URL devices should use (M4): `https://`, or `http://` on loopback; empty forgets it; answers `hub.status` |
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
  device pairs again. A call that dials the hub (`sync.join`,
  `sync.conflicts`, `sync.resolve`, `sync.reset`, `sync.verify`,
  `sync.history`, `sync.readRevision`) answers `revoked` when the hub
  refuses the token or the device holds none. Pairing again with the
  hub's URL (`sync.pair`, `sync.paired`) replaces its row and token and
  wakes its loops; `notesctl hub join` does so by itself when `sync.join`
  answers `revoked`, at most once.
- A device that pairs again is a new device to the hub, and its old
  replicas are not its own. At each connection a device checks its
  replica (`log.pushes` with no pushes); answered `notFound`, it calls
  `log.join` and keeps the new replica, its agreed state and its cursor.
  A push still without an answer goes again under the new replica, whose
  push IDs the hub has not seen: it applies it anew, and a state the head
  already holds changes nothing.
- The hub records a device's last request time, at most once a minute.
- From M4, a device asks `log.hub` at each connection. When the hub
  records a URL that differs from the device's (scheme and host without
  case, the path exactly), the device would pair with it, and the host
  there proves it is the hub (`POST /sync/hub`'s proof, as
  `sync.relocate` checks it), the device keeps it and connects there,
  once per dial, with the same token, replicas, agreed states and
  cursors. A URL's host is kept in lower case. A hub from before M4 answers
  `methodNotFound`, and the device stays
  ([lodge-app.md](lodge-app.md#device)).

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
- A push without an answer (its connection dropped) is sent again, with
  the same push ID, before anything else of its file: no new change of a
  file is pushed until the answer to its last push is applied. A second
  create of one file would make two files on the hub, and a second change
  is pushed from the state the answer agrees.
- A local conflict version of a file with an agreed row (content a writer
  with a stale base lost in a merge on the device,
  [vault.md](../design/vault.md#writes)) that no push has carried is
  pushed as a `conflict` item: base the agreed revision, path the agreed
  path, hash the version's. So every device's history keeps it, and the
  file is flagged on the hub too. A delete that drops the agreed row (a
  pulled head, an answer, or a copy dropped for another local file)
  first records, in its `Tx`, each such version that no push has carried
  as an unanswered `conflict` push from the row: its hub file, revision,
  the revision's hash and path. Its unanswered conflict pushes take the
  row as their base too. They are sent as every unanswered push is, also
  after a restart, with that base while the file has no agreed row (the
  hub keeps a conflict item for a deleted file), or from the agreed row
  a later pull of the file makes. No delete is pushed for it: the device
  deleted nothing itself, and the hub's file may come back meanwhile. A
  file the device never agreed (the hub never had it, or it changed
  before the answer to its create applied) has no agreed row: its
  conflict versions stay in the device's history.

### A push, on the hub

Items are applied one at a time under the vault's lock, each through the
hub vault's write path ([vault.md](../design/vault.md#writes)) and each
revision in the same `Tx` as the hub's version. They apply in the order
they come, but a put that moves its file onto a path another item of the
push moves a file away from (or a create there) goes after that item, so
a chain of renames takes no numbers. In a ring of such moves, a swap the
simplest, one item's file first moves aside to `name.sync-<file ID>.ext`,
a `move` revision of that item's push, and the ring then applies in
order. Answers keep the items' order. A push applies as a whole: no other
`log` call on the vault (a push, pull, match or resolve) sees it half
done, such as a file still parked.

1. **Repeats:** an item whose `push` this replica has sent before gets the
   first answer's status and reason, with the file's current state.
2. **Checks**, refusing with a reason:
   - `invalid`: a path the path guard refuses (among its rules, no name
     over 255 bytes and no path over 4,000), a name the hub's file system
     cannot hold, a malformed hash, a create with `deleted`, or a `hash`
     with `deleted`; the item is refused, and the push's others apply;
   - `ignored`: the hub's settings ignore the path;
   - `missing`: the hub has no content for `hash`, or none this device
     may name ([Blobs](#blobs)); unlike other refusals, the change stays
     pending, and the device uploads it and pushes again;
   - `unknown`: `file` is not a file of this vault;
   - `base`: `base` is not a revision of `file`, or its hash is not
     `baseHash`.
3. **Create** (no `file`):
   - at a path no live file has, ignoring case: a new file; `accepted`;
   - at a live file's exact path: merged into that file as two creates
     ([merge.md](merge.md#create)); `merged`, or `conflicted`;
   - at a path that differs only by case from a live file's: a new file
     at the first free of `name 1.ext`, `name 2.ext`, …, since a file
     system that ignores case could not hold the two; `accepted`, and the
     answer's head moves the device's file there. (`collision`, a refusal
     of protocol 1, is no longer given.)
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
10. **Conflict item:** a `conflict` item appends a `conflict` revision
    holding the pushed content to its file, live or deleted, whatever the
    head (`lostTo` the head), and flags the file; `accepted`. It changes
    no head. A conflict item without `file` is refused, `invalid`.
11. Each appended revision emits `log.appended` after its `Tx` commits.

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
  2. A path the device ignores, whose guard refuses it, or whose name the
     device's file system cannot hold, is skipped and listed in
     `sync.status`. So is an answer's head at such a path: the push is
     recorded as refused on the device (`ignored` or `invalid`), the file
     leaves the device's care with its change merged on the hub, and it is
     pushed again only once it changes again.
  3. **Unchanged locally** — the agreed row's file still has the agreed
     path and hash, or there is no agreed row and no file at the path —
     the head is applied (`vault.Apply`): created, rewritten, moved or
     deleted, expecting that local state.
  4. **Changed locally** — the file differs from the agreed state, or a
     file without an agreed row is at the path — the device pushes its
     change first and applies the answer.
  5. A head whose path a copy of another file still holds here, that
     file's own head being later in the pull, parks the copy first when it
     is unchanged: it moves to `name.sync-<hub file>.ext`, its agreed row
     following it, and its own head then moves it on, so a swap or a ring
     of moves never waits on itself. A changed copy is pushed first, and
     the head waits: the cursor stays below the waiting head while the
     rest of the pull applies.
- `vault.Apply` streams the content from the device's blob store,
  downloading it first if missing (`GET /sync/blobs/{hash}`, checked, 4 at
  a time), and compares the file with the expected state just before the
  rename; if it changed, nothing is written and rule 4 applies. It removes
  a folder that the delete or move leaves empty.
- In one `Tx` with the file's index rows: a version with `source: sync`
  and author `author@deviceName` (or `deviceName` when the author is
  empty, `hub` for the hub's own writers), and the agreed row set to the
  head (dropped for a delete, its unpushed conflict versions recorded as
  pushes first, [Pending changes](#pending-changes)). The cursor advances
  to a page's `next` once every file in the page is agreed or skipped.
- Before `vault.Apply` changes the disk for a head (a pulled head, an
  answer, or a copy dropped for another local file), the device records
  the head in a `Tx` of its own (`sync_applying`): the hub file; the
  local file, path and hash it replaces; the agreed row's `seq` then
  (none: 0); the head's `seq`, revision, path, hash, deletion and flag;
  the path the change puts the file at; and, for an answer, the push and
  the answer's status and reason. The `Tx` that agrees the head deletes
  the record; so does a change that wrote nothing (the file changed, or
  the device cannot hold the path). Before each sync session starts, the
  loop settles the records a crash left, so before any push, and only
  after a scan of the vault that succeeded since notesd started. While
  none has (a scan fails as a whole, changing nothing, on a folder it
  cannot read or an empty folder of an indexed vault), the session does
  not start: `sync.status` reports the vault `offline` with the error,
  and the loop tries again after its reconnect backoff, without dialing
  the hub, until a rescan succeeds
  ([ADR-0034](../adr/0034-a-device-records-the-head-it-applies-before-it-changes-the-disk.md)):
  1. A record whose agreed row's `seq` is no longer the recorded one is
     discarded.
  2. If the file is the head (at the head's path with its hash; for a
     delete, nothing at the old path and the local file gone from the
     index), the head is agreed, and its push answered, as the change's
     `Tx` would have done (a delete drops the row, its unpushed conflict
     versions recorded as pushes first). A head that moves and rewrites
     its file, found at the head's path with the old content, is applied
     there first.
  3. Otherwise the record is discarded: the agreed row stays, so the
     file's own change is pushed first, or the next pull, from the cursor
     still below the head, applies the head again.

  So a device never pushes back, as its own change, a head it only
  applied: a crash between the rename and the `Tx` leaves no change to
  push from the old base.
- The answers to one push apply in order. An answer that moves a file
  onto the path of a file a later item of the same push carried, still as
  pushed, parks that file first; its own answer moves it on from there. An
  answer whose path an unchanged copy of another file holds parks it as
  a pulled head does (rule 5), and the following pull moves it on. A
  parked file is never pushed; one nothing moved on goes back after the
  pull, or to a numbered name beside its old path if that was taken. A
  crash meanwhile leaves it at its parked name, which the device then
  pushes as a move: a name to fix by hand, nothing lost.
- When an answer gives a local file a hub file that another local file is
  agreed with (a create merged into a file that moved onto its path), the
  local file at the head's path keeps it, and the other is deleted if it
  still holds its agreed content, or pushed first if not.
- An answer is applied like a pulled head, expecting the pushed path and
  hash. If the file changed since it was pushed, nothing is written and
  the agreed state stays (none, for a create), so the next push, from the
  same base, merges again on the hub. One exception: the answer to a
  create the hub merged, whose file has since moved without changing, is
  applied at the file's new path, and the move is pushed next; without an
  agreed state it would go out as a second create.

### Joining

`sync.join` calls `log.vaults` and `log.join`, records the remote, then
matches every file of the local vault by `log.match`: a file whose hash is
a revision of the file last at its path on the hub, live or deleted, is
agreed at that file's newest create, put or move at its path no later
than the newest revision with its hash, or at that revision when the file
was not at the path before it, with that revision's content
(`revisionHash`). Then the loop runs as always: unmatched files are pushed
as creates, and the pull from 0 brings the hub's heads, so a matched older
copy takes the head, and a file the hub deleted is deleted. A copy with a
later revision's content at the path its file left differs from its
agreed revision, so it is pushed as a put from it, and the answer brings
the head's path.
`sync.reset` drops the remote, its agreed rows and its push records, and
joins again with a new replica.

### Limits and timings

| What | Value |
| --- | --- |
| a pairing | 10 minutes; at most 8 pending |
| a path | at most 4,000 bytes, a name in it at most 255 |
| a device's connections | 64 |
| quiet before a push | 2 s, at most 30 s after the first change |
| items in one push | 100 |
| hashes in one `log.missing`, files in one `log.match` | 1,000 |
| pull page | 500 by default, at most 1,000 |
| concurrent transfers per hub | 4 |
| reconnect | after 1 s, doubling to 60 s, ±20% jitter; at once on a local change |
| a blob | 1 GiB by default |
| an unnamed blob | 24 hours |
| a download | 1 GiB on the device |

## Derived artifacts

| Artifact | Derivation |
| --- | --- |
| `api/zz_generated.go`, `app/src/rpc/gen/api.ts`, [rpc-api.md](rpc-api.md) | `make gen` from `schema/rpc/hub.yaml`, `sync.yaml` and `log.yaml` |
| `internal/store/migrations/0002_sync.sql`, `0003_sync_conflict_pushes.sql`, `0005_sync_push_base_hash.sql`, `0006_sync_applying.sql` | the hub's and devices' state in this spec |

## References

- Rationale: [ADR-0005](../adr/0005-sync-through-a-hub-with-revisions-and-three-way-merges.md), [ADR-0006](../adr/0006-every-write-names-the-version-it-changes.md), [ADR-0016](../adr/0016-pair-devices-with-a-one-time-code-and-a-revocable-token.md), [ADR-0017](../adr/0017-revisions-number-the-hubs-log-and-hashes-name-content.md), [ADR-0018](../adr/0018-move-blobs-over-http-named-by-their-hash.md), [ADR-0019](../adr/0019-keep-every-revisions-record-and-thin-the-content-by-age.md), [ADR-0026](../adr/0026-notesd-runs-as-a-lodge-app.md) (from M4, the hub behind lodge and relocation), [ADR-0031](../adr/0031-the-desktop-app-ships-as-a-flatpak-with-its-own-notesd.md) (the probe and a withdrawn pairing, from the Flatpak's test on Snow), [ADR-0034](../adr/0034-a-device-records-the-head-it-applies-before-it-changes-the-disk.md) (the head a device is applying)
- Context: [design/sync.md](../design/sync.md), [design/vault.md](../design/vault.md#writes), [design/hub.md](../design/hub.md) (the hub behind lodge, moving devices)
- Related: [rpc-protocol.md](rpc-protocol.md), [merge.md](merge.md), [lodge-app.md](lodge-app.md) (from M4, these routes under `/device/` on lodge's app socket, and relocation)
- Built in: [plan 0004](../plans/0004-m2-sync.md)
