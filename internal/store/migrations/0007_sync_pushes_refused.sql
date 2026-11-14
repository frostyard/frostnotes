-- 0007_sync_pushes_refused.sql: a device's refused pushes, by vault. After
-- every sync cycle, sync.status lists the refusals that still stand
-- (docs/specs/sync-protocol.md, Pending changes); with this index it reads only the
-- refused pushes, not every push the device made since it joined.
CREATE INDEX sync_pushes_refused ON sync_pushes (vault_id, file_id) WHERE status = 'refused';
