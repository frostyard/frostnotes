package store

import (
	"context"
	"fmt"
	"slices"
	"time"

	"github.com/frostyard/frostnotes/api"
)

// PruneRevisions applies ADR-0019 to every served vault as of now, with
// days and months in loc: it sets pruned on each revision with content (a
// create, put, move or conflict) that is no longer kept, and returns,
// sorted, the hashes it pruned that no kept create, put, move or conflict
// revision, no files row and no versions row names any more: the blobs
// that may go. A revision is kept
// when it is less than 30 days old (at > now - 30 days); when it is its
// file's head; when it is a conflict revision after its file's last
// resolve while the file is flagged; when it is a create, put or move less
// than 365 days old and its file's last create, put or move of its day; or
// when it is a create, put or move and its file's last of its month. A
// pruned revision stays pruned. Task card T-0028.
func (t *Tx) PruneRevisions(ctx context.Context, now time.Time, loc *time.Location) ([]string, error) {
	revisions, err := t.revisionsToPrune(ctx, now, loc)
	if err != nil {
		return nil, err
	}
	hashes := map[string]struct{}{}
	for _, r := range revisions {
		if _, err := t.ExecContext(ctx, `UPDATE revisions SET pruned = 1 WHERE vault_id = ? AND seq = ?`,
			r.VaultID, r.Seq); err != nil {
			return nil, fmt.Errorf("prune revision %d of vault %d: %w", r.Seq, r.VaultID, err)
		}
		hashes[r.Hash] = struct{}{}
	}
	freed := []string{}
	for hash := range hashes {
		var used bool
		if err := t.QueryRowContext(ctx, `SELECT EXISTS (SELECT 1 FROM versions WHERE hash = ?)
			OR EXISTS (SELECT 1 FROM files WHERE hash = ?)
			OR EXISTS (SELECT 1 FROM revisions WHERE hash = ? AND op IN ('create', 'put', 'move', 'conflict') AND pruned = 0)`,
			hash, hash, hash).Scan(&used); err != nil {
			return nil, fmt.Errorf("check pruned revision hash %s: %w", hash, err)
		}
		if !used {
			freed = append(freed, hash)
		}
	}
	slices.Sort(freed)
	return freed, nil
}

// revisionsToPrune closes its result set before the caller updates the log.
func (t *Tx) revisionsToPrune(ctx context.Context, now time.Time, loc *time.Location) ([]Revision, error) {
	rows, err := t.QueryContext(ctx, `SELECT r.vault_id, r.file_id, r.seq, r.op, r.hash, r.at, r.pruned,
		COALESCE(f.revision, 0), COALESCE(f.conflicted, 0)
		FROM revisions r LEFT JOIN hub_files f ON f.vault_id = r.vault_id AND f.file_id = r.file_id
		WHERE r.op != 'delete' ORDER BY r.vault_id, r.file_id, r.seq DESC`)
	if err != nil {
		return nil, fmt.Errorf("list revisions to prune: %w", err)
	}
	defer rows.Close()
	var vaultID, fileID int64
	var state revisionRetention
	var out []Revision
	for rows.Next() {
		var r Revision
		var at string
		var head int64
		var conflicted bool
		if err := rows.Scan(&r.VaultID, &r.FileID, &r.Seq, &r.Op, &r.Hash, &at, &r.Pruned, &head, &conflicted); err != nil {
			return nil, fmt.Errorf("read revision to prune: %w", err)
		}
		r.At, err = ParseTime(at)
		if err != nil {
			return nil, fmt.Errorf("parse revision time: %w", err)
		}
		if r.VaultID != vaultID || r.FileID != fileID {
			vaultID, fileID = r.VaultID, r.FileID
			state = revisionRetention{days: map[string]bool{}, months: map[string]bool{},
				recent: now.Add(-30 * 24 * time.Hour), daily: now.Add(-365 * 24 * time.Hour), loc: loc}
		}
		// Even already-pruned states participate in daily and monthly selection.
		keep := state.keep(r, head, conflicted)
		if !r.Pruned && !keep {
			out = append(out, r)
		}
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("list revisions to prune: %w", err)
	}
	if err := rows.Close(); err != nil {
		return nil, fmt.Errorf("close revisions to prune: %w", err)
	}
	return out, nil
}

// revisionRetention tracks one file in descending sequence order. The first
// state in each local day or month has the highest seq, regardless of its age.
type revisionRetention struct {
	days, months  map[string]bool
	resolved      bool
	recent, daily time.Time
	loc           *time.Location
}

func (s *revisionRetention) keep(r Revision, head int64, conflicted bool) bool {
	if r.Op == api.RevisionOpResolve {
		s.resolved = true
		return true
	}
	keep := r.At.After(s.recent) || r.Seq == head
	if r.Op == api.RevisionOpConflict {
		// Once we pass the latest resolve, older conflicts are no longer pinned.
		return keep || (conflicted && !s.resolved)
	}
	local := r.At.In(s.loc)
	day, month := local.Format("2006-01-02"), local.Format("2006-01")
	keep = keep || !s.months[month] || (!s.days[day] && r.At.After(s.daily))
	s.days[day], s.months[month] = true, true
	return keep
}
