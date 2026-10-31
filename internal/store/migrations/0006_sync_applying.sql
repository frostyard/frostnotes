-- 0006_sync_applying.sql: the head a device is applying to its vault
-- (ADR-0034, docs/specs/sync-protocol.md, Pulling and applying). The
-- device records it before vault.Apply changes the disk; the transaction
-- that agrees the head deletes it. A row left by a crash is finished or
-- discarded when the vault's sync loop connects again, so a head the
-- device only applied is never pushed back as its own change.
CREATE TABLE sync_applying (
  vault_id   INTEGER NOT NULL REFERENCES remotes (vault_id) ON DELETE CASCADE,
  hub_file   INTEGER NOT NULL, -- the hub's file ID
  file_id    INTEGER NOT NULL, -- the local file the head replaces; 0 for a new file
  from_path  TEXT NOT NULL,    -- the path the head replaces the file at (a delete removes it there)
  from_hash  TEXT NOT NULL,    -- the content it replaces there; '' for a new file
  base_seq   INTEGER NOT NULL, -- the agreed row's seq when the head was recorded; 0 without a row
  seq        INTEGER NOT NULL, -- the head: the agreed row it makes
  revision   INTEGER NOT NULL,
  path       TEXT NOT NULL,
  hash       TEXT NOT NULL,    -- '' for a delete
  at_path    TEXT NOT NULL,    -- where the change puts the file: the head's path, or where a merged create moved to
  deleted    INTEGER NOT NULL CHECK (deleted IN (0, 1)),
  conflicted INTEGER NOT NULL CHECK (conflicted IN (0, 1)),
  push_id    INTEGER NOT NULL, -- the push the head answers; 0 for a pulled head
  status     TEXT NOT NULL,    -- that answer's status and reason; '' for a pulled head
  reason     TEXT NOT NULL,
  PRIMARY KEY (vault_id, hub_file)
) STRICT;
