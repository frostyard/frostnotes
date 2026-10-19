-- 0004_hub_url.sql: the URL a hub records for its devices (hub.setURL,
-- docs/specs/lodge-app.md#device). Devices learn it with log.hub, and a
-- relocation proof names it; '' when none is recorded.
ALTER TABLE hub ADD COLUMN url TEXT NOT NULL DEFAULT '';
