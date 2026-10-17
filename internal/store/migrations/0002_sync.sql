-- 0002_sync.sql: sync's state, on a hub and on a device
-- (docs/specs/sync-protocol.md, ADR-0017). From this migration on,
-- migrations are append-only: 0001 and this file no longer change, so the
-- sync state of M2's trial survives the fixes made during it (plan 0004).
-- Hashes are lowercase hex SHA-256; IDs of the hub's files are its own
-- files.id, kept across moves and never reused.

-- ---- on a hub ----

-- This notesd's identity as a hub: one row, made when it first serves a
-- vault.
CREATE TABLE hub (
  id         INTEGER PRIMARY KEY CHECK (id = 1),
  hub_id     TEXT NOT NULL, -- 32 lowercase hex digits, random
  created_at TEXT NOT NULL
) STRICT;

-- The vaults this hub serves (hub.serve).
CREATE TABLE hub_vaults (
  vault_id  INTEGER PRIMARY KEY REFERENCES vaults (id) ON DELETE CASCADE,
  sync_id   TEXT NOT NULL UNIQUE,      -- 32 lowercase hex digits, random
  seq       INTEGER NOT NULL DEFAULT 0, -- the log's latest sequence number
  served_at TEXT NOT NULL
) STRICT;

-- Paired devices (ADR-0016). A revoked device keeps its row, since
-- revisions name it, with its token hash cleared.
CREATE TABLE hub_devices (
  id         INTEGER PRIMARY KEY AUTOINCREMENT,
  name       TEXT NOT NULL,
  token_hash TEXT UNIQUE, -- SHA-256 of the whole device token; NULL once revoked
  paired_at  TEXT NOT NULL,
  seen_at    TEXT,        -- its last request, at most once a minute; NULL until one
  revoked_at TEXT
) STRICT;

-- One device's join of one vault; its push IDs count within it.
CREATE TABLE hub_replicas (
  id        INTEGER PRIMARY KEY AUTOINCREMENT,
  device_id INTEGER NOT NULL REFERENCES hub_devices (id),
  vault_id  INTEGER NOT NULL REFERENCES hub_vaults (vault_id) ON DELETE CASCADE,
  joined_at TEXT NOT NULL
) STRICT;

-- Each served vault's log. Rows are never deleted (ADR-0019): pruned is
-- set once a revision's content is no longer kept. A file's head is its
-- latest create, put, move or delete; conflict and resolve revisions only
-- set and clear its flag.
CREATE TABLE revisions (
  vault_id   INTEGER NOT NULL REFERENCES hub_vaults (vault_id) ON DELETE CASCADE,
  seq        INTEGER NOT NULL,
  file_id    INTEGER NOT NULL, -- not a foreign key: a deleted file's log outlives its row
  op         TEXT NOT NULL CHECK (op IN ('create', 'put', 'move', 'delete', 'conflict', 'resolve')),
  path       TEXT NOT NULL,    -- the file's path after this revision
  hash       TEXT NOT NULL,    -- the content; '' for a delete
  size       INTEGER NOT NULL,
  base       INTEGER NOT NULL, -- the revision it was based on; 0 for a create
  device_id  INTEGER REFERENCES hub_devices (id),  -- NULL for the hub's own writers
  replica_id INTEGER REFERENCES hub_replicas (id), -- with push_id, the push that made it
  push_id    INTEGER,
  source     TEXT NOT NULL CHECK (source IN ('editor', 'agent', 'outside', 'sync')),
  author     TEXT NOT NULL,
  at         TEXT NOT NULL,
  lost_to    INTEGER,          -- a conflict revision's winner
  pruned     INTEGER NOT NULL DEFAULT 0 CHECK (pruned IN (0, 1)),
  PRIMARY KEY (vault_id, seq)
) STRICT;

CREATE INDEX revisions_file ON revisions (vault_id, file_id, seq);
CREATE INDEX revisions_path ON revisions (vault_id, path, seq);
CREATE INDEX revisions_push ON revisions (replica_id, push_id);
CREATE INDEX revisions_hash ON revisions (hash);

-- Each file's place in the log, deleted files included: what log.pull
-- reads.
CREATE TABLE hub_files (
  vault_id   INTEGER NOT NULL REFERENCES hub_vaults (vault_id) ON DELETE CASCADE,
  file_id    INTEGER NOT NULL,
  seq        INTEGER NOT NULL, -- its latest revision of any kind
  revision   INTEGER NOT NULL, -- its head
  conflicted INTEGER NOT NULL DEFAULT 0 CHECK (conflicted IN (0, 1)),
  PRIMARY KEY (vault_id, file_id)
) STRICT;

CREATE INDEX hub_files_seq ON hub_files (vault_id, seq);

-- The answer to each push, so a repeated push ID gets it again.
CREATE TABLE hub_pushes (
  replica_id  INTEGER NOT NULL REFERENCES hub_replicas (id) ON DELETE CASCADE,
  push_id     INTEGER NOT NULL,
  file_id     INTEGER, -- NULL for a refused create
  status      TEXT NOT NULL CHECK (status IN ('accepted', 'merged', 'conflicted', 'refused')),
  reason      TEXT NOT NULL, -- a RefuseReason; '' unless refused
  conflicts   INTEGER NOT NULL,
  base_pruned INTEGER NOT NULL CHECK (base_pruned IN (0, 1)),
  at          TEXT NOT NULL,
  PRIMARY KEY (replica_id, push_id)
) STRICT;

-- ---- on a device ----

-- Hubs this device paired with. The device token is in the secret store
-- under hub/<hub_id>/token, never here.
CREATE TABLE sync_hubs (
  id        INTEGER PRIMARY KEY AUTOINCREMENT,
  hub_id    TEXT NOT NULL UNIQUE,
  url       TEXT NOT NULL,
  device_id INTEGER NOT NULL, -- this device's ID on the hub
  paired_at TEXT NOT NULL
) STRICT;

-- Local vaults synced with a hub vault (sync.join).
CREATE TABLE remotes (
  vault_id   INTEGER PRIMARY KEY REFERENCES vaults (id) ON DELETE CASCADE,
  hub        INTEGER NOT NULL REFERENCES sync_hubs (id),
  sync_id    TEXT NOT NULL,              -- the hub vault's sync ID
  name       TEXT NOT NULL,              -- the hub vault's name
  replica_id INTEGER NOT NULL,           -- this join's replica on the hub
  cursor     INTEGER NOT NULL DEFAULT 0, -- the highest sequence number whose heads are applied
  hub_seq    INTEGER NOT NULL DEFAULT 0, -- the hub log's latest, as last seen
  joined_at  TEXT NOT NULL,
  synced_at  TEXT                        -- when the device was last in step; NULL until it is
) STRICT;

-- The agreed state of each file (ADR-0017): what this device and the hub
-- last agreed it is. A file whose index row differs, or is gone, has a
-- change to push; a file without a row is a create.
CREATE TABLE sync_files (
  vault_id   INTEGER NOT NULL REFERENCES remotes (vault_id) ON DELETE CASCADE,
  hub_file   INTEGER NOT NULL, -- the hub's file ID
  file_id    INTEGER NOT NULL, -- the local files.id; not a foreign key: a row whose file is gone is a delete to push
  seq        INTEGER NOT NULL, -- the latest revision of the file applied or answered
  revision   INTEGER NOT NULL, -- the agreed head
  path       TEXT NOT NULL,
  hash       TEXT NOT NULL,
  conflicted INTEGER NOT NULL DEFAULT 0 CHECK (conflicted IN (0, 1)),
  PRIMARY KEY (vault_id, hub_file)
) STRICT;

CREATE UNIQUE INDEX sync_files_file ON sync_files (file_id);

-- Every push this device made, with the answer once it has one: the push
-- ID a retry reuses, the refusals not to send again, and sync.verify's
-- record.
CREATE TABLE sync_pushes (
  vault_id INTEGER NOT NULL REFERENCES remotes (vault_id) ON DELETE CASCADE,
  push_id  INTEGER NOT NULL, -- within the remote's replica, from 1
  file_id  INTEGER NOT NULL, -- local
  hub_file INTEGER,          -- NULL for a create
  base     INTEGER NOT NULL,
  path     TEXT NOT NULL,
  hash     TEXT NOT NULL,    -- '' for a delete
  at       TEXT NOT NULL,
  status   TEXT CHECK (status IN ('accepted', 'merged', 'conflicted', 'refused')), -- NULL until answered
  reason   TEXT NOT NULL DEFAULT '',
  PRIMARY KEY (vault_id, push_id)
) STRICT;

CREATE INDEX sync_pushes_file ON sync_pushes (file_id, push_id);
