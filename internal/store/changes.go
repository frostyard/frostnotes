package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"github.com/frostyard/frostnotes/api"
)

// LatestSeq is the highest durable sequence number ever assigned, or 0.
// It survives pruning because it reads AUTOINCREMENT's counter.
func (d *DB) LatestSeq(ctx context.Context) (int64, error) {
	var seq int64
	err := d.db.QueryRowContext(ctx, `SELECT seq FROM sqlite_sequence WHERE name = 'changes'`).Scan(&seq)
	if errors.Is(err, sql.ErrNoRows) {
		return 0, nil
	}
	if err != nil {
		return 0, fmt.Errorf("latest seq: %w", err)
	}
	return seq, nil
}

// OldestSeq is the lowest retained sequence number, or 0 when the log is empty.
func (d *DB) OldestSeq(ctx context.Context) (int64, error) {
	var seq int64
	if err := d.db.QueryRowContext(ctx, `SELECT COALESCE(MIN(seq), 0) FROM changes`).Scan(&seq); err != nil {
		return 0, fmt.Errorf("oldest seq: %w", err)
	}
	return seq, nil
}

// ChangesSince returns the retained durable events after seq, in order.
func (d *DB) ChangesSince(ctx context.Context, seq int64) ([]api.EventEnvelope, error) {
	rows, err := d.db.QueryContext(ctx, `SELECT seq, event, data FROM changes WHERE seq > ? ORDER BY seq`, seq)
	if err != nil {
		return nil, fmt.Errorf("changes since %d: %w", seq, err)
	}
	defer rows.Close()
	var out []api.EventEnvelope
	for rows.Next() {
		var env api.EventEnvelope
		var data string
		if err := rows.Scan(&env.Seq, &env.Event, &data); err != nil {
			return nil, fmt.Errorf("changes since %d: %w", seq, err)
		}
		env.Data = []byte(data)
		out = append(out, env)
	}
	return out, rows.Err()
}

// PruneChanges keeps the newest keep durable events.
func (d *DB) PruneChanges(ctx context.Context, keep int) error {
	return d.Tx(ctx, func(tx *Tx) error {
		_, err := tx.ExecContext(ctx,
			`DELETE FROM changes WHERE seq <= (SELECT COALESCE(MAX(seq), 0) FROM changes) - ?`, keep)
		return err
	})
}
