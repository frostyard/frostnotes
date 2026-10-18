package store

import (
	"context"
	"fmt"
	"maps"
	"slices"
	"strings"

	"github.com/frostyard/frostnotes/api"
)

// TagCounts returns every tag of vault's notes and how many notes have it,
// one entry per tag ignoring case (tags.folded), ordered by that folded
// name. A tag's spelling is the one that sorts first in byte order among
// its notes' spellings. Nested tags are counted whole: a note tagged a/b
// counts for a/b, not for a. Task card T-0043.
func (d *DB) TagCounts(ctx context.Context, vaultID int64) ([]api.TagCount, error) {
	rows, err := d.db.QueryContext(ctx, `SELECT MIN(tags.name), COUNT(*)
		FROM files JOIN tags ON tags.file_id = files.id
		WHERE files.vault_id = ?
		GROUP BY tags.folded ORDER BY tags.folded`, vaultID)
	if err != nil {
		return nil, fmt.Errorf("list tag counts: %w", err)
	}
	defer rows.Close()
	counts := []api.TagCount{}
	for rows.Next() {
		var count api.TagCount
		if err := rows.Scan(&count.Tag, &count.Count); err != nil {
			return nil, fmt.Errorf("scan tag count: %w", err)
		}
		counts = append(counts, count)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("read tag counts: %w", err)
	}
	return counts, nil
}

// PropertyCounts returns every frontmatter key of vault's notes and how
// many notes have it, one entry per key ignoring case as strings.ToLower
// folds it, ordered by the lower-cased key in byte order. A key's spelling is the one that sorts first in byte
// order among its notes' spellings; a key counts even when its value is
// null or an empty list. types maps lower-cased keys to the vault's
// property types; a key without one has type "". Task card T-0043.
func (d *DB) PropertyCounts(ctx context.Context, vaultID int64, types map[string]string) ([]api.PropertyCount, error) {
	rows, err := d.db.QueryContext(ctx, `SELECT notes.file_id, property.key
		FROM files JOIN notes ON notes.file_id = files.id, json_each(notes.props) AS property
		WHERE files.vault_id = ? ORDER BY notes.file_id`, vaultID)
	if err != nil {
		return nil, fmt.Errorf("list property counts: %w", err)
	}
	defer rows.Close()
	byKey := make(map[string]api.PropertyCount)
	lastFile := make(map[string]int64)
	for rows.Next() {
		var fileID int64
		var key string
		if err := rows.Scan(&fileID, &key); err != nil {
			return nil, fmt.Errorf("scan property key: %w", err)
		}
		folded := strings.ToLower(key)
		count := byKey[folded]
		if count.Count == 0 || key < count.Key {
			count.Key = key
		}
		// Rows are ordered by note, so alternate spellings in one note count once.
		if count.Count == 0 || lastFile[folded] != fileID {
			count.Count++
			lastFile[folded] = fileID
		}
		byKey[folded] = count
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("read property keys: %w", err)
	}
	counts := make([]api.PropertyCount, 0, len(byKey))
	for _, folded := range slices.Sorted(maps.Keys(byKey)) {
		count := byKey[folded]
		count.Type = types[folded]
		counts = append(counts, count)
	}
	return counts, nil
}
