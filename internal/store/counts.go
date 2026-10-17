package store

import (
	"context"

	"github.com/frostyard/frostnotes/api"
)

// TagCounts returns every tag of vault's notes and how many notes have it,
// one entry per tag ignoring case (tags.folded), ordered by that folded
// name. A tag's spelling is the one that sorts first in byte order among
// its notes' spellings. Nested tags are counted whole: a note tagged a/b
// counts for a/b, not for a. Task card T-0043.
func (d *DB) TagCounts(ctx context.Context, vaultID int64) ([]api.TagCount, error) {
	_, _ = ctx, vaultID
	return []api.TagCount{}, nil
}

// PropertyCounts returns every frontmatter key of vault's notes and how
// many notes have it, one entry per key ignoring case as strings.ToLower
// folds it, ordered by the lower-cased key in byte order. A key's spelling is the one that sorts first in byte
// order among its notes' spellings; a key counts even when its value is
// null or an empty list. types maps lower-cased keys to the vault's
// property types; a key without one has type "". Task card T-0043.
func (d *DB) PropertyCounts(ctx context.Context, vaultID int64, types map[string]string) ([]api.PropertyCount, error) {
	_, _, _ = ctx, vaultID, types
	return []api.PropertyCount{}, nil
}
