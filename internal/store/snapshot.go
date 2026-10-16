package store

import (
	"context"
	"fmt"
	"slices"
)

// snapshotQueries render each index table of one vault as text rows, with
// file IDs replaced by paths, so that two indexes of the same files give
// the same rows whatever IDs they assigned. Versions and the changes log
// are history, not index, and are left out.
var snapshotQueries = []string{
	`SELECT 'file ' || path || ' ' || kind || ' ' || size || ' ' || mtime || ' ' || hash || ' ' || name
		FROM files WHERE vault_id = ?1`,
	`SELECT 'note ' || f.path || ' ' || n.title || ' ' || n.props || ' ' || quote(n.prop_error) || ' ' ||
		n.aliases || ' ' || n.words
		FROM notes n JOIN files f ON f.id = n.file_id WHERE f.vault_id = ?1`,
	`SELECT 'prop ' || f.path || ' ' || quote(p.key) || ' ' || p.ord || ' ' || quote(p.value) || ' ' ||
		quote(p.folded) || ' ' || quote(p.num)
		FROM props p JOIN files f ON f.id = p.file_id WHERE f.vault_id = ?1`,
	`SELECT 'tag ' || f.path || ' ' || t.ord || ' ' || quote(t.name) || ' ' || t.front || ' ' || t.line
		FROM tags t JOIN files f ON f.id = t.file_id WHERE f.vault_id = ?1`,
	`SELECT 'link ' || f.path || ' ' || l.ord || ' ' || l.kind || ' ' || quote(l.target) || ' ' ||
		quote(l.subpath) || ' ' || quote(l.alias) || ' ' || l.embed || ' ' || quote(l.key) || ' ' ||
		l.start || ' ' || l.stop || ' ' || l.line || ' ' || quote(l.name) || ' -> ' || quote(d.path)
		FROM links l JOIN files f ON f.id = l.file_id LEFT JOIN files d ON d.id = l.dest_id
		WHERE f.vault_id = ?1`,
	`SELECT 'heading ' || f.path || ' ' || h.ord || ' ' || h.level || ' ' || quote(h.text) || ' ' || h.line
		FROM headings h JOIN files f ON f.id = h.file_id WHERE f.vault_id = ?1`,
	`SELECT 'block ' || f.path || ' ' || b.ord || ' ' || quote(b.block_id) || ' ' || b.line
		FROM blocks b JOIN files f ON f.id = b.file_id WHERE f.vault_id = ?1`,
	`SELECT 'task ' || f.path || ' ' || t.ord || ' ' || quote(t.status) || ' ' || quote(t.text) || ' ' || t.line
		FROM tasks t JOIN files f ON f.id = t.file_id WHERE f.vault_id = ?1`,
	`SELECT 'search ' || f.path || ' ' || quote(s.title) || ' ' || quote(s.aliases) || ' ' ||
		quote(s.body) || ' ' || quote(s.props)
		FROM notes_fts s JOIN files f ON f.id = s.rowid WHERE f.vault_id = ?1`,
}

// IndexSnapshot returns everything vault's index holds as sorted text
// rows, by path rather than ID: an index after a crash and restart must
// equal a fresh rebuild of the same files row for row (plan 0003's kill -9
// harness).
func (d *DB) IndexSnapshot(ctx context.Context, vaultID int64) ([]string, error) {
	var out []string
	for _, q := range snapshotQueries {
		rows, err := d.db.QueryContext(ctx, q, vaultID)
		if err != nil {
			return nil, fmt.Errorf("snapshot index: %w", err)
		}
		for rows.Next() {
			var row string
			if err := rows.Scan(&row); err != nil {
				_ = rows.Close()
				return nil, fmt.Errorf("snapshot index: %w", err)
			}
			out = append(out, row)
		}
		if err := rows.Close(); err != nil {
			return nil, fmt.Errorf("snapshot index: %w", err)
		}
		if err := rows.Err(); err != nil {
			return nil, fmt.Errorf("snapshot index: %w", err)
		}
	}
	slices.Sort(out)
	return out, nil
}
