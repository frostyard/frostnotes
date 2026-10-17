// Package store is notesd's SQLite database: migrations, the single writer,
// the durable event log and the queries (docs/design/vault.md).
//
// Reads go through DB methods and may run concurrently. Every write runs in
// DB.Tx, which serializes writers, records the events the transaction emits
// and hands them to OnCommit after commit, in commit order.
package store

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"embed"
	"encoding/hex"
	"encoding/json/v2"
	"errors"
	"fmt"
	"io/fs"
	"slices"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/frostyard/frostnotes/api"

	"modernc.org/sqlite" // also registers the "sqlite" driver
	sqlite3 "modernc.org/sqlite/lib"
)

//go:embed migrations/*.sql
var migrations embed.FS

var (
	// ErrNotFound means the row does not exist.
	ErrNotFound = errors.New("store: not found")
	// ErrConflict means a uniqueness rule rejected the write.
	ErrConflict = errors.New("store: conflict")
)

// TimeFormat is how times are stored: UTC, fixed width, lexically sortable.
const TimeFormat = "2006-01-02T15:04:05.000Z"

// DB is the notesd database.
type DB struct {
	db      *sql.DB
	writeMu sync.Mutex

	// Now is the clock for stored timestamps; tests replace it.
	Now func() time.Time
	// OnCommit receives each committed transaction's events, in commit
	// order, while the write lock is held. It must not block.
	OnCommit func([]api.EventEnvelope)
}

// Open opens or creates the database at path and applies pending migrations.
// path must not live on a network or virtiofs share: WAL needs shared memory.
func Open(ctx context.Context, path string) (*DB, error) {
	if strings.ContainsAny(path, "?#") {
		return nil, fmt.Errorf("open database: path %q contains ? or #", path)
	}
	dsn := "file:" + path +
		"?_pragma=journal_mode(WAL)&_pragma=foreign_keys(1)&_pragma=busy_timeout(5000)" +
		"&_pragma=synchronous(NORMAL)&_txlock=immediate"
	sqlDB, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, fmt.Errorf("open database: %w", err)
	}
	sqlDB.SetMaxOpenConns(8)
	d := &DB{db: sqlDB, Now: time.Now}
	if err := d.migrate(ctx); err != nil {
		_ = sqlDB.Close()
		return nil, fmt.Errorf("open database %s: %w", path, err)
	}
	return d, nil
}

// Close closes the database.
func (d *DB) Close() error { return d.db.Close() }

// SchemaVersion is the highest applied migration.
func (d *DB) SchemaVersion(ctx context.Context) (int, error) {
	var v int
	err := d.db.QueryRowContext(ctx, `SELECT COALESCE(MAX(version), 0) FROM schema_migrations`).Scan(&v)
	return v, err
}

type migration struct {
	version  int
	name     string
	sql      string
	checksum string // lowercase hex SHA-256 of sql
}

func loadMigrations() ([]migration, error) {
	entries, err := fs.ReadDir(migrations, "migrations")
	if err != nil {
		return nil, err
	}
	var ms []migration
	for _, e := range entries {
		prefix, _, ok := strings.Cut(e.Name(), "_")
		v, err := strconv.Atoi(prefix)
		if !ok || err != nil || !strings.HasSuffix(e.Name(), ".sql") {
			return nil, fmt.Errorf("migration %s: name must be NNNN_name.sql", e.Name())
		}
		body, err := fs.ReadFile(migrations, "migrations/"+e.Name())
		if err != nil {
			return nil, err
		}
		sum := sha256.Sum256(body)
		ms = append(ms, migration{v, e.Name(), string(body), hex.EncodeToString(sum[:])})
	}
	slices.SortFunc(ms, func(a, b migration) int { return a.version - b.version })
	for i, m := range ms {
		if m.version != i+1 {
			return nil, fmt.Errorf("migration %s: versions must run 1, 2, 3 without gaps", m.name)
		}
	}
	return ms, nil
}

// migrate checks that the migrations the database applied are this
// notesd's, byte for byte, and applies the pending ones. Pre-release,
// 0001_init.sql is edited in place, so a database that applied an earlier
// text of it lacks what the code expects: Open refuses it.
func (d *DB) migrate(ctx context.Context) error {
	ms, err := loadMigrations()
	if err != nil {
		return err
	}
	if err := d.Tx(ctx, func(tx *Tx) error {
		_, err := tx.ExecContext(ctx, `CREATE TABLE IF NOT EXISTS schema_migrations (
			version INTEGER PRIMARY KEY, name TEXT NOT NULL, applied_at TEXT NOT NULL,
			checksum TEXT NOT NULL) STRICT`)
		return err
	}); err != nil {
		return fmt.Errorf("migrate: %w", err)
	}
	current, err := d.SchemaVersion(ctx)
	if err != nil {
		return fmt.Errorf("migrate: %w", err)
	}
	if current > len(ms) {
		return fmt.Errorf("migrate: database schema %d is newer than this notesd (%d)", current, len(ms))
	}
	if err := d.adoptChecksums(ctx, ms[:current]); err != nil {
		return err
	}
	if err := d.checkChecksums(ctx, ms); err != nil {
		return err
	}
	for _, m := range ms[current:] {
		if err := d.Tx(ctx, func(tx *Tx) error {
			if _, err := tx.ExecContext(ctx, m.sql); err != nil {
				return err
			}
			_, err := tx.ExecContext(ctx, `INSERT INTO schema_migrations (version, name, applied_at, checksum)
				VALUES (?, ?, ?, ?)`, m.version, m.name, FormatTime(d.Now()), m.checksum)
			return err
		}); err != nil {
			return fmt.Errorf("migrate %s: %w", m.name, err)
		}
	}
	return nil
}

// checkChecksums refuses a database that applied a migration whose SQL is
// not this notesd's.
func (d *DB) checkChecksums(ctx context.Context, ms []migration) error {
	rows, err := d.db.QueryContext(ctx, `SELECT version, checksum FROM schema_migrations ORDER BY version`)
	if err != nil {
		return fmt.Errorf("migrate: %w", err)
	}
	defer func() { _ = rows.Close() }()
	for rows.Next() {
		var v int
		var sum string
		if err := rows.Scan(&v, &sum); err != nil {
			return fmt.Errorf("migrate: %w", err)
		}
		if v < 1 || v > len(ms) {
			return fmt.Errorf("migrate: database records unknown migration %d", v)
		}
		if m := ms[v-1]; sum != m.checksum {
			return errStaleSchema("migration " + m.name + " differs from the one this database applied")
		}
	}
	if err := rows.Err(); err != nil {
		return fmt.Errorf("migrate: %w", err)
	}
	return nil
}

// adoptChecksums records the checksums of applied in a database from before
// schema_migrations kept them, once its schema proves to be what applied
// creates today.
func (d *DB) adoptChecksums(ctx context.Context, applied []migration) error {
	var n int
	if err := d.db.QueryRowContext(ctx,
		`SELECT count(*) FROM pragma_table_info('schema_migrations') WHERE name = 'checksum'`).Scan(&n); err != nil {
		return fmt.Errorf("migrate: %w", err)
	}
	if n > 0 {
		return nil
	}
	if len(applied) > 0 {
		got, err := schemaOf(ctx, d.db)
		if err != nil {
			return fmt.Errorf("migrate: %w", err)
		}
		want, err := freshSchema(ctx, applied)
		if err != nil {
			return fmt.Errorf("migrate: %w", err)
		}
		if !slices.Equal(got, want) {
			return errStaleSchema("its schema is not what migrations through " + applied[len(applied)-1].name + " create")
		}
	}
	err := d.Tx(ctx, func(tx *Tx) error {
		if _, err := tx.ExecContext(ctx, `ALTER TABLE schema_migrations ADD COLUMN checksum TEXT NOT NULL DEFAULT ''`); err != nil {
			return err
		}
		for _, m := range applied {
			if _, err := tx.ExecContext(ctx, `UPDATE schema_migrations SET checksum = ? WHERE version = ?`,
				m.checksum, m.version); err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		return fmt.Errorf("migrate: record checksums: %w", err)
	}
	return nil
}

// freshSchema is the schema ms create in an empty database.
func freshSchema(ctx context.Context, ms []migration) ([]string, error) {
	mem, err := sql.Open("sqlite", "file::memory:")
	if err != nil {
		return nil, err
	}
	defer func() { _ = mem.Close() }()
	mem.SetMaxOpenConns(1) // each connection would be a database of its own
	for _, m := range ms {
		if _, err := mem.ExecContext(ctx, m.sql); err != nil {
			return nil, fmt.Errorf("%s: %w", m.name, err)
		}
	}
	return schemaOf(ctx, mem)
}

// schemaOf lists the tables, indexes, triggers and views of db as SQLite
// stores their SQL, leaving out schema_migrations and SQLite's own objects.
func schemaOf(ctx context.Context, db *sql.DB) ([]string, error) {
	rows, err := db.QueryContext(ctx, `SELECT type || ' ' || name || ' ' || COALESCE(sql, '') FROM sqlite_schema
		WHERE tbl_name != 'schema_migrations' AND name NOT LIKE 'sqlite\_%' ESCAPE '\'
		ORDER BY type, name`)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	var schema []string
	for rows.Next() {
		var s string
		if err := rows.Scan(&s); err != nil {
			return nil, err
		}
		schema = append(schema, s)
	}
	return schema, rows.Err()
}

// errStaleSchema explains why Open refuses a database and what to do.
func errStaleSchema(why string) error {
	return fmt.Errorf("%s: the database predates an in-place edit of the schema; "+
		"move it aside, with any -wal and -shm files, and restart notesd to create a new one", why)
}

// Tx is a write transaction. Emit records events delivered after commit.
type Tx struct {
	*sql.Tx
	d      *DB
	events []api.EventEnvelope
}

// Tx runs fn in a write transaction: committed if fn returns nil, rolled
// back otherwise. Writers are serialized.
func (d *DB) Tx(ctx context.Context, fn func(*Tx) error) error {
	d.writeMu.Lock()
	defer d.writeMu.Unlock()
	sqlTx, err := d.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin: %w", err)
	}
	t := &Tx{Tx: sqlTx, d: d}
	if err := fn(t); err != nil {
		_ = sqlTx.Rollback()
		return err
	}
	if err := sqlTx.Commit(); err != nil {
		return fmt.Errorf("commit: %w", err)
	}
	if d.OnCommit != nil && len(t.events) > 0 {
		d.OnCommit(t.events)
	}
	return nil
}

// Now is the database clock, for rows written in this transaction.
func (t *Tx) Now() time.Time { return t.d.Now() }

// Emit records ev. A durable event is appended to the changes log in this
// transaction and gets its sequence number; every event is delivered to
// OnCommit only if the transaction commits.
func (t *Tx) Emit(ctx context.Context, ev api.Event) error {
	var seq int64
	if ev.Durable() {
		data, err := json.Marshal(ev)
		if err != nil {
			return fmt.Errorf("emit %s: %w", ev.EventName(), err)
		}
		err = t.QueryRowContext(ctx, `INSERT INTO changes (event, data, at) VALUES (?, ?, ?) RETURNING seq`,
			ev.EventName(), string(data), FormatTime(t.d.Now())).Scan(&seq)
		if err != nil {
			return fmt.Errorf("emit %s: %w", ev.EventName(), err)
		}
	}
	env, err := api.NewEnvelope(seq, ev)
	if err != nil {
		return err
	}
	t.events = append(t.events, env)
	return nil
}

// FormatTime renders t in TimeFormat.
func FormatTime(t time.Time) string { return t.UTC().Format(TimeFormat) }

// ParseTime parses a TimeFormat value.
func ParseTime(s string) (time.Time, error) {
	t, err := time.Parse(TimeFormat, s)
	if err != nil {
		return time.Time{}, fmt.Errorf("stored time %q: %w", s, err)
	}
	return t, nil
}

// IsUniqueViolation reports whether err is a SQLite UNIQUE or PRIMARY KEY
// constraint failure; callers map it to ErrConflict.
func IsUniqueViolation(err error) bool {
	var se *sqlite.Error
	if !errors.As(err, &se) {
		return false
	}
	return se.Code() == sqlite3.SQLITE_CONSTRAINT_UNIQUE || se.Code() == sqlite3.SQLITE_CONSTRAINT_PRIMARYKEY
}

// IsForeignKeyViolation reports whether err is SQLite's foreign key
// constraint failure: a row names a parent that does not exist.
func IsForeignKeyViolation(err error) bool {
	var se *sqlite.Error
	return errors.As(err, &se) && se.Code() == sqlite3.SQLITE_CONSTRAINT_FOREIGNKEY
}
