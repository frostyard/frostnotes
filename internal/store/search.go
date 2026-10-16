package store

import (
	"context"
	"errors"

	"github.com/frostyard/frostnotes/api"
)

// SearchQuery turns what a user typed into an FTS5 MATCH expression: each
// double-quoted phrase, and each other word with a letter or digit, is
// quoted as an FTS5 phrase, a word with "*" after it to match as a prefix,
// and all are required. It returns "" when nothing is left to match. Task
// card T-0016.
func SearchQuery(text string) string { return "" }

// Search returns vault's notes that match query, an expression from
// SearchQuery, best first, at most limit of them, each with a snippet
// around its best match. Task card T-0016.
func (d *DB) Search(ctx context.Context, vaultID int64, query string, limit int) ([]api.SearchHit, error) {
	return nil, errors.New("store.Search is not implemented yet (T-0016)")
}
