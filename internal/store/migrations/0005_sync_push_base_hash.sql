-- 0005_sync_push_base_hash.sql: the hash of a push's base revision, for a
-- conflict item a device sends after its file's agreed row is gone: a
-- delete that applied first carries the file's conflict versions as
-- pushes from that row (docs/specs/sync-protocol.md, Pending changes).
-- '' when the push takes it from the agreed row as it is sent.
ALTER TABLE sync_pushes ADD COLUMN base_hash TEXT NOT NULL DEFAULT '';
