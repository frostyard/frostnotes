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

-- Every file in a vault, notes and attachments (docs/design/vault.md#index).
-- A file keeps its ID across moves, and IDs are never reused, so versions
-- still name a file after it is deleted.
CREATE TABLE files (
  id       INTEGER PRIMARY KEY AUTOINCREMENT,
  vault_id INTEGER NOT NULL REFERENCES vaults (id) ON DELETE CASCADE,
  path     TEXT NOT NULL,    -- vault-relative and slash-separated, as on disk
  name     TEXT NOT NULL,    -- the last path segment lower-cased by Go's strings.ToLower: markdown.Resolve's lookup
  kind     TEXT NOT NULL CHECK (kind IN ('note', 'attachment')), -- a note's name ends in .md
  size     INTEGER NOT NULL,
  mtime    INTEGER NOT NULL, -- Unix nanoseconds as stat reports them, compared exactly by scans
  hash     TEXT NOT NULL,    -- lowercase hex SHA-256 of the content: its blob ID
  UNIQUE (vault_id, path)
) STRICT;

CREATE INDEX files_name ON files (vault_id, name);

-- What markdown.Parse found in each note.
CREATE TABLE notes (
  file_id    INTEGER PRIMARY KEY REFERENCES files (id) ON DELETE CASCADE,
  title      TEXT NOT NULL, -- the file name without .md
  props      TEXT NOT NULL, -- Doc.Props as a JSON object; '{}' without frontmatter
  prop_error TEXT NOT NULL, -- Doc.PropError's message; '' without one
  aliases    TEXT NOT NULL, -- Doc.Aliases as a JSON array
  words      INTEGER NOT NULL
) STRICT;

-- Frontmatter values, for listing notes by property: one row per key with
-- a scalar value (null is ''), and one per scalar item of a list value.
-- Mappings and nested lists are not listed.
CREATE TABLE props (
  file_id INTEGER NOT NULL REFERENCES notes (file_id) ON DELETE CASCADE,
  key     TEXT NOT NULL,
  ord     INTEGER NOT NULL, -- the item's position in a list; 0 for a scalar
  value   TEXT NOT NULL,    -- a string as decoded; a number or boolean as JSON writes it
  folded  TEXT NOT NULL,    -- value lower-cased by Go's strings.ToLower
  num     REAL,             -- the value when it is a number
  PRIMARY KEY (file_id, key, ord)
) STRICT;

CREATE INDEX props_value ON props (key, folded);

-- Each note's tags, once each (Doc.Tags).
CREATE TABLE tags (
  file_id INTEGER NOT NULL REFERENCES notes (file_id) ON DELETE CASCADE,
  ord     INTEGER NOT NULL, -- the tag's position in Doc.Tags
  folded  TEXT NOT NULL, -- the name lower-cased by Go's strings.ToLower: a tag's identity
  name    TEXT NOT NULL, -- as the note first spells it, without '#'
  front   INTEGER NOT NULL CHECK (front IN (0, 1)),
  line    INTEGER NOT NULL,
  PRIMARY KEY (file_id, folded)
) STRICT;

CREATE INDEX tags_folded ON tags (folded);

-- Each note's links (Doc.Links), resolved by markdown.Resolve. Unresolved
-- links are kept: the app and the graph show them.
CREATE TABLE links (
  file_id INTEGER NOT NULL REFERENCES notes (file_id) ON DELETE CASCADE, -- the linking note
  ord     INTEGER NOT NULL, -- the link's position in Doc.Links
  kind    TEXT NOT NULL CHECK (kind IN ('wiki', 'markdown', 'property')),
  target  TEXT NOT NULL,
  subpath TEXT NOT NULL,
  alias   TEXT NOT NULL,
  embed   INTEGER NOT NULL CHECK (embed IN (0, 1)),
  key     TEXT NOT NULL,    -- the frontmatter key of a property link; '' otherwise
  start   INTEGER NOT NULL, -- byte offsets in the note
  stop    INTEGER NOT NULL,
  line    INTEGER NOT NULL,
  -- name is the target's last segment, lower-cased, without a final .md:
  -- a file whose name is name or name + '.md' may resolve it, so creating,
  -- moving or deleting that file re-resolves the link.
  name    TEXT NOT NULL,
  dest_id INTEGER REFERENCES files (id) ON DELETE SET NULL, -- NULL when unresolved
  PRIMARY KEY (file_id, ord)
) STRICT;

CREATE INDEX links_dest ON links (dest_id);
CREATE INDEX links_name ON links (name);

CREATE TABLE headings (
  file_id INTEGER NOT NULL REFERENCES notes (file_id) ON DELETE CASCADE,
  ord     INTEGER NOT NULL,
  level   INTEGER NOT NULL CHECK (level BETWEEN 1 AND 6),
  text    TEXT NOT NULL,
  line    INTEGER NOT NULL,
  PRIMARY KEY (file_id, ord)
) STRICT;

CREATE TABLE blocks (
  file_id  INTEGER NOT NULL REFERENCES notes (file_id) ON DELETE CASCADE,
  ord      INTEGER NOT NULL,
  block_id TEXT NOT NULL, -- without '^'
  line     INTEGER NOT NULL,
  PRIMARY KEY (file_id, ord)
) STRICT;

CREATE TABLE tasks (
  file_id INTEGER NOT NULL REFERENCES notes (file_id) ON DELETE CASCADE,
  ord     INTEGER NOT NULL,
  status  TEXT NOT NULL, -- the character between the brackets
  text    TEXT NOT NULL,
  line    INTEGER NOT NULL,
  PRIMARY KEY (file_id, ord)
) STRICT;

-- Every version of every file notesd wrote or scanned, and each deletion,
-- kept 30 days (ADR-0006); the content is the blob named by hash. file_id
-- is not a foreign key: a deleted file's versions outlive its row, so
-- note.revert can bring it back.
CREATE TABLE versions (
  id       INTEGER PRIMARY KEY AUTOINCREMENT,
  vault_id INTEGER NOT NULL REFERENCES vaults (id) ON DELETE CASCADE,
  file_id  INTEGER NOT NULL,
  path     TEXT NOT NULL, -- the file's path when this version was made
  hash     TEXT NOT NULL, -- the content's blob ID; '' records a deletion
  size     INTEGER NOT NULL,
  at       TEXT NOT NULL,
  source   TEXT NOT NULL CHECK (source IN ('editor', 'agent', 'outside', 'sync')),
  author   TEXT NOT NULL, -- the writer's name (a client's, or an MCP client's); '' for an outside edit
  -- 1 for a writer's content that lost a conflicting hunk in a merge: kept
  -- so nothing is lost, but never the file's content (ADR-0005).
  conflict INTEGER NOT NULL DEFAULT 0 CHECK (conflict IN (0, 1))
) STRICT;

CREATE INDEX versions_file ON versions (file_id, id);
CREATE INDEX versions_path ON versions (vault_id, path, id);
CREATE INDEX versions_at ON versions (at);
CREATE INDEX versions_hash ON versions (hash);

-- Full-text search over each note's title, aliases, body (after the
-- frontmatter, as written) and frontmatter text; rowid = notes.file_id.
-- It stores its content, so snippet() works.
CREATE VIRTUAL TABLE notes_fts USING fts5 (
  title, aliases, body, props,
  tokenize = 'unicode61 remove_diacritics 2', prefix = '2 3'
);

-- A note's search row leaves with it, cascades included.
CREATE TRIGGER notes_fts_delete AFTER DELETE ON notes BEGIN
  DELETE FROM notes_fts WHERE rowid = old.file_id;
END;
