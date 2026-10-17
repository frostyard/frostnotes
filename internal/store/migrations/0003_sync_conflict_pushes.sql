-- 0003_sync_conflict_pushes.sql: a device's pushes of local conflict
-- versions (docs/specs/sync-protocol.md, Pending changes): a writer's
-- content that lost hunks in a merge on the device goes to the hub's log
-- as a conflict item, and its push record says so.
ALTER TABLE sync_pushes ADD COLUMN conflict INTEGER NOT NULL DEFAULT 0 CHECK (conflict IN (0, 1));
