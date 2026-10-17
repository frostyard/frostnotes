package vault

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/frostyard/frostnotes/internal/store"
)

// PushOrder orders a batch of pushed changes to this hub vault
// (docs/specs/sync-protocol.md, A push, on the hub). A change that moves a
// file onto a path another change of the batch moves a file away from
// goes after that change, so a batch that renames a chain of files takes
// no numbers. In a ring of such moves, a swap the simplest, one change is
// parked first: Park moves its file aside, and the ring then applies in
// order. It returns the order of cs's indexes, and the indexes to park.
func (v *Vault) PushOrder(ctx context.Context, cs []PushChange) ([]int, []int, error) {
	moving := map[int64]int{} // a file, and the change that moves it
	moves := map[int]bool{}
	for i, c := range cs {
		if c.FileID == 0 || c.Deleted || c.Conflict {
			continue
		}
		row, err := v.cfg.DB.FileByID(ctx, c.FileID)
		if errors.Is(err, store.ErrNotFound) {
			continue
		} else if err != nil {
			return nil, nil, err
		}
		// A put moves its file only when its path is not its base's
		// (pushPut): a device's edit names the path it last agreed. A put
		// Push would refuse moves nothing.
		base, err := v.cfg.DB.Revision(ctx, v.id, c.Base)
		if errors.Is(err, store.ErrNotFound) {
			continue
		} else if err != nil {
			return nil, nil, err
		}
		if base.FileID != c.FileID || base.Hash != c.BaseHash || base.Path == c.Path ||
			checkNotePath(c.Path) != nil || v.ignored(c.Path) || !hashRE.MatchString(c.Hash) {
			continue
		}
		if ok, err := v.cfg.Blobs.Has(c.Hash); err != nil || !ok {
			continue
		}
		if row.VaultID == v.id && row.Path != c.Path {
			moving[c.FileID], moves[i] = i, true
		}
	}
	after := map[int]int{} // a change, and the one that frees its path
	for i := range cs {
		if !moves[i] && cs[i].FileID != 0 {
			continue
		}
		taken, err := v.livePath(ctx, cs[i].Path)
		if err != nil {
			return nil, nil, err
		}
		if taken == "" {
			continue
		}
		holder, err := v.cfg.DB.FileByPath(ctx, v.id, taken)
		if err != nil {
			continue
		}
		if j, ok := moving[holder.ID]; ok && j != i && !strings.EqualFold(cs[j].Path, taken) {
			after[i] = j
		}
	}
	// Each change waits for at most one other, so the waits are chains,
	// some closing into rings.
	var order, park []int
	state := make([]int, len(cs)) // 0 not yet placed, 1 being placed, 2 placed
	var place func(i int)
	place = func(i int) {
		state[i] = 1
		if j, ok := after[i]; ok {
			switch state[j] {
			case 0:
				place(j)
			case 1:
				park = append(park, j) // a ring: j's file steps aside first
			}
		}
		state[i] = 2
		order = append(order, i)
	}
	for i := range cs {
		if state[i] == 0 {
			place(i)
		}
	}
	return order, park, nil
}

// Park moves the file change c moves aside, beside where it is, as the
// first step of a ring of moves: the change itself then moves it on. The
// move appends its revision as part of c's push. It returns where the file
// was and where it went.
func (v *Vault) Park(ctx context.Context, c PushChange, who Author) (string, string, error) {
	r, err := v.open()
	if err != nil {
		return "", "", err
	}
	defer func() { _ = r.Close() }()
	if err := v.lock(); err != nil {
		return "", "", err
	}
	defer v.mu.Unlock()
	row, err := v.cfg.DB.FileByID(ctx, c.FileID)
	if err != nil {
		return "", "", err
	}
	to, err := v.freePath(ctx, r, Beside(row.Path, fmt.Sprintf(".sync-%d", row.ID)), row.ID)
	if err != nil {
		return "", "", err
	}
	if _, err := v.relocate(ctx, r, row.Path, row.Hash, to, who, nil); err != nil {
		return "", "", err
	}
	return row.Path, to, nil
}

// Unpark moves file id back from parked to from, when its change did not
// move it on (Push refused it) and from is still free.
func (v *Vault) Unpark(ctx context.Context, id int64, from, parked string, who Author) error {
	r, err := v.open()
	if err != nil {
		return err
	}
	defer func() { _ = r.Close() }()
	if err := v.lock(); err != nil {
		return err
	}
	defer v.mu.Unlock()
	row, err := v.cfg.DB.FileByID(ctx, id)
	if err != nil || row.Path != parked {
		return nil
	}
	if to, err := v.freePath(ctx, r, from, id); err != nil || to != from {
		return err
	}
	_, err = v.relocate(ctx, r, parked, row.Hash, from, who, nil)
	return err
}
