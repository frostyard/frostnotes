package store

import (
	"context"
	"errors"
	"time"
)

// PruneRevisions applies ADR-0019 to every served vault as of now, with
// days and months in loc: it sets pruned on each revision with content (a
// create, put, move or conflict) that is no longer kept, and returns,
// sorted, the hashes it pruned that no kept revision, no files row and no
// versions row names any more: the blobs that may go. A revision is kept
// when it is less than 30 days old; when it is its file's head; when it is
// a conflict revision after its file's last resolve, while the file is
// flagged; when it is less than 365 days old and its file's last revision
// with content of its day; or when it is its file's last revision with
// content of its month. A pruned revision stays pruned. Task card T-0028.
func (t *Tx) PruneRevisions(ctx context.Context, now time.Time, loc *time.Location) ([]string, error) {
	return nil, errors.New("store.PruneRevisions is not implemented yet (T-0028)")
}
