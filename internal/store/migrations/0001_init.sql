-- 0001_init.sql: the initial notesd schema (docs/design/vault.md).
-- Pre-release, this file is edited in place; from the first daily-driver
-- release on, schema changes are new append-only migrations. Times are UTC
-- text in the fixed format 2006-01-02T15:04:05.000Z so they sort lexically.

-- The vaults this notesd owns (ADR-0003). Paths are absolute and cleaned;
-- no vault contains another (checked by notesd, not SQL).
CREATE TABLE vaults (
  id         INTEGER PRIMARY KEY AUTOINCREMENT, -- never reused: clients hold IDs
  name       TEXT NOT NULL,
  path       TEXT NOT NULL UNIQUE,
  created_at TEXT NOT NULL
) STRICT;

-- The durable event log behind events.subscribe(sinceSeq). AUTOINCREMENT
-- keeps sequence numbers unique after pruning.
CREATE TABLE changes (
  seq   INTEGER PRIMARY KEY AUTOINCREMENT,
  event TEXT NOT NULL,
  data  TEXT NOT NULL,
  at    TEXT NOT NULL
) STRICT;
