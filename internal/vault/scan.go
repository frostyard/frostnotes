package vault

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"

	"github.com/frostyard/frostnotes/api"
	"github.com/frostyard/frostnotes/internal/markdown"
	"github.com/frostyard/frostnotes/internal/store"
)

// candidate is a path a scan reads: new to the index, or with a stat that
// differs from the index's (every path, on a reindex).
type candidate struct {
	path string
	prev *store.File // the index's row; nil when the index lacks the path
}

// item is one path's change as a scan planned it.
type item struct {
	path string
	prev *store.File // the index's row when planned; nil when it lacked the path
	cur  *content    // what the file held when read; nil when it is gone
	from *store.File // for a move: the missing file this one is
}

// isGone reports whether err means a path holds no vault file: missing,
// or a symlink or folder, which are not vault files.
func isGone(err error) bool { return errors.Is(err, fs.ErrNotExist) || errors.Is(err, ErrPath) }

// sameStat reports whether row's size and time are fi's.
func sameStat(row *store.File, fi fs.FileInfo) bool {
	return row != nil && fi != nil && row.Size == fi.Size() && row.ModTime.Equal(fi.ModTime())
}

func kindOf(p string) store.FileKind { return store.File{Path: p}.Kind() }

// runScan scans the vault, keeping its status.
func (v *Vault) runScan(ctx context.Context, reindex bool) (Listing, error) {
	network, err := onNetwork(v.root)
	if err != nil {
		v.log.Debug("statfs", "err", err)
	}
	v.setStatus(func(s *api.ScanStatus) {
		s.State, s.Done, s.Total, s.Network = api.ScanStateScanning, 0, 0, network
	}, true)
	listing, err := v.scan(ctx, reindex)
	if ctx.Err() != nil {
		return Listing{}, ctx.Err()
	}
	now := v.cfg.Now()
	v.setStatus(func(s *api.ScanStatus) {
		s.Done, s.Total, s.ScannedAt, s.Error = 0, 0, &now, nil
		s.State = api.ScanStateIdle
		if err != nil {
			text := err.Error()
			s.State, s.Error = api.ScanStateFailed, &text
		}
	}, true)
	if err != nil {
		v.log.Warn("scan failed", "err", err)
	}
	return listing, err
}

// scan compares the vault's folder with its index and brings the index up
// to date (docs/design/vault.md#scan-and-watch): it reads the files whose
// size or time differ from the index's, pairs a missing file with a new
// one of the same content as a move, and applies the changes in batches
// of at most Config.Batch files, each in one transaction, so a crash
// leaves an index that the next scan continues. A reindex reads every file
// and rebuilds its rows. A folder that cannot be read fails the scan
// before anything changes.
func (v *Vault) scan(ctx context.Context, reindex bool) (Listing, error) {
	settings, err := LoadSettings(v.root)
	if err != nil {
		return Listing{}, err
	}
	ignore, err := NewIgnore(settings.Ignore)
	if err != nil {
		return Listing{}, err
	}
	listing, err := Walk(ctx, v.root, ignore)
	if err != nil {
		return Listing{}, err
	}
	r, err := v.open()
	if err != nil {
		return Listing{}, err
	}
	defer func() { _ = r.Close() }()
	v.mu.Lock()
	v.ignore = ignore
	for _, tmp := range listing.Temps {
		if err := r.Remove(tmp); err != nil && !errors.Is(err, fs.ErrNotExist) {
			v.log.Warn("remove a write's temporary file", "path", tmp, "err", err)
		}
	}
	v.mu.Unlock()

	indexed, err := v.cfg.DB.Files(ctx, v.id)
	if err != nil {
		return Listing{}, err
	}
	var cands []candidate
	var gone []store.File
	files := listing.Files[:0:0]
	for _, e := range listing.Files {
		if e.Kind != KindSettings {
			files = append(files, e)
		}
	}
	i := 0
	for _, e := range files {
		for i < len(indexed) && indexed[i].Path < e.Path {
			gone = append(gone, indexed[i])
			i++
		}
		var prev *store.File
		if i < len(indexed) && indexed[i].Path == e.Path {
			prev = &indexed[i]
			i++
		}
		if prev == nil || reindex || prev.Size != e.Size || !prev.ModTime.Equal(e.ModTime) {
			cands = append(cands, candidate{path: e.Path, prev: prev})
		}
	}
	gone = append(gone, indexed[i:]...)

	total := int64(len(files))
	done := total - int64(len(cands))
	v.setStatus(func(s *api.ScanStatus) { s.Done, s.Total = done, total }, true)
	items, err := v.plan(ctx, r, cands, gone, func() {
		done++
		v.setStatus(func(s *api.ScanStatus) { s.Done = done }, false)
	})
	if err != nil {
		return Listing{}, err
	}
	if err := v.applyAll(ctx, r, items, reindex); err != nil {
		return Listing{}, err
	}
	return listing, nil
}

// scanPaths brings the index up to date with the given paths, as the
// watcher's events name them.
func (v *Vault) scanPaths(ctx context.Context, paths []string) error {
	r, err := v.open()
	if err != nil {
		return err
	}
	defer func() { _ = r.Close() }()
	v.mu.Lock()
	ignore := v.ignore
	v.mu.Unlock()
	var cands []candidate
	var gone []store.File
	for _, p := range paths {
		if CheckPath(p) != nil || IsSettings(p) || p == SettingsDir || ignore.Match(p, false) {
			continue
		}
		row, err := v.row(ctx, p)
		if err != nil {
			return err
		}
		// An event says the file changed, so it is read whatever its stat
		// says: two edits of the same size within the clock's granularity
		// have the same modification time.
		_, err = checkFile(r, p)
		switch {
		case isGone(err):
			if row != nil {
				gone = append(gone, *row)
			}
		case err != nil:
			v.log.Warn("cannot stat file", "path", p, "err", err)
		default:
			cands = append(cands, candidate{path: p, prev: row})
		}
	}
	items, err := v.plan(ctx, r, cands, gone, nil)
	if err != nil {
		return err
	}
	return v.applyAll(ctx, r, items, false)
}

// plan reads the candidates and pairs each new file with a missing file of
// the same kind and content as a move, in path order. A file that cannot
// be read keeps its index rows. progress, when set, runs after each read.
func (v *Vault) plan(ctx context.Context, r *os.Root, cands []candidate, gone []store.File, progress func()) ([]item, error) {
	var items []item
	for _, c := range cands {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		cur, err := read(r, c.path)
		if progress != nil {
			progress()
		}
		switch {
		case isGone(err):
			if c.prev != nil {
				gone = append(gone, *c.prev)
			}
			continue
		case err != nil:
			v.log.Warn("cannot read file", "path", c.path, "err", err)
			continue
		}
		items = append(items, item{path: c.path, prev: c.prev, cur: &cur})
	}
	byHash := map[string][]int{}
	for i, g := range gone {
		byHash[g.Hash] = append(byHash[g.Hash], i)
	}
	moved := make([]bool, len(gone))
	var moves, rest []item
	for _, it := range items {
		if it.prev == nil {
			for _, i := range byHash[it.cur.hash] {
				if !moved[i] && gone[i].Kind() == kindOf(it.path) {
					moved[i] = true
					it.from = &gone[i]
					break
				}
			}
		}
		if it.from != nil {
			moves = append(moves, it)
		} else {
			rest = append(rest, it)
		}
	}
	out := append(moves, rest...)
	for i := range gone {
		if !moved[i] {
			out = append(out, item{path: gone[i].Path, prev: &gone[i]})
		}
	}
	return out, nil
}

// applyAll applies items in batches of at most Config.Batch.
func (v *Vault) applyAll(ctx context.Context, r *os.Root, items []item, reindex bool) error {
	for len(items) > 0 {
		n := min(len(items), v.cfg.Batch)
		v.mu.Lock()
		err := v.applyLocked(ctx, r, items[:n], reindex)
		v.mu.Unlock()
		if err != nil {
			return err
		}
		items = items[n:]
	}
	return nil
}

type opKind int

const (
	opStat    opKind = iota + 1 // same content, new size or time
	opReindex                   // same content, rows rebuilt
	opContent                   // new or changed content: an outside version
	opMove                      // a missing file found at a new path
	opGone                      // the file is gone: an outside deletion
)

type op struct {
	kind opKind
	path string
	row  *store.File // the index's row at path now
	from *store.File // opMove: the moved file's row
	cur  *content
	doc  markdown.Doc
}

// applyLocked checks each item against the disk and the index again, since
// a write may have run since it was planned, then applies what still
// holds in one transaction. v.mu must be held.
func (v *Vault) applyLocked(ctx context.Context, r *os.Root, batch []item, reindex bool) error {
	var ops []op
	for _, it := range batch {
		o, ok, err := v.decide(ctx, r, it, reindex)
		if err != nil {
			return err
		}
		if !ok {
			continue
		}
		if o.kind == opContent {
			if err := v.storeBlob(ctx, r, o.path, o.cur); errors.Is(err, errUnstable) {
				continue // it changed again; its next event or scan reads it
			} else if err != nil {
				return err
			}
		}
		if (o.kind == opContent || o.kind == opReindex) && kindOf(o.path) == store.FileNote && o.cur.data != nil {
			o.doc = markdown.Parse(o.cur.data)
		}
		ops = append(ops, o)
	}
	if len(ops) == 0 {
		return nil
	}
	return v.cfg.DB.Tx(ctx, func(tx *store.Tx) error {
		for _, o := range ops {
			if err := v.applyOp(ctx, tx, o); err != nil {
				return err
			}
		}
		return nil
	})
}

// decide turns an item into the change it makes now, or false when it
// makes none.
func (v *Vault) decide(ctx context.Context, r *os.Root, it item, reindex bool) (op, bool, error) {
	row, err := v.row(ctx, it.path)
	if err != nil {
		return op{}, false, err
	}
	fi, ferr := checkFile(r, it.path)
	if isGone(ferr) {
		return op{kind: opGone, path: it.path, row: row}, row != nil, nil
	}
	if ferr != nil {
		v.log.Warn("cannot stat file", "path", it.path, "err", ferr)
		return op{}, false, nil
	}
	if it.cur == nil {
		return op{}, false, nil // planned as gone, but it is back: its next event or scan reads it
	}
	cur := it.cur
	if !cur.sameStat(fi) {
		c, err := read(r, it.path)
		if isGone(err) {
			return op{kind: opGone, path: it.path, row: row}, row != nil, nil
		}
		if err != nil {
			v.log.Warn("cannot read file", "path", it.path, "err", err)
			return op{}, false, nil
		}
		cur = &c
	}
	if row != nil && row.Hash == cur.hash {
		switch {
		case reindex:
			return op{kind: opReindex, path: it.path, row: row, cur: cur}, true, nil
		case !sameStat(row, cur.info):
			return op{kind: opStat, path: it.path, row: row, cur: cur}, true, nil
		}
		return op{}, false, nil
	}
	if row == nil && it.from != nil {
		from, err := v.row(ctx, it.from.Path)
		if err != nil {
			return op{}, false, err
		}
		if from != nil && from.ID == it.from.ID && from.Hash == cur.hash {
			if _, err := checkFile(r, from.Path); isGone(err) {
				return op{kind: opMove, path: it.path, from: from, cur: cur}, true, nil
			}
		}
	}
	return op{kind: opContent, path: it.path, row: row, cur: cur}, true, nil
}

func (v *Vault) applyOp(ctx context.Context, tx *store.Tx, o op) error {
	file := func() store.File {
		return store.File{VaultID: v.id, Path: o.path, Size: o.cur.size(), ModTime: o.cur.modTime(), Hash: o.cur.hash}
	}
	switch o.kind {
	case opStat:
		_, err := tx.PutFile(ctx, file())
		return err
	case opReindex:
		f, err := tx.PutFile(ctx, file())
		if err != nil {
			return err
		}
		if f.Kind() == store.FileNote {
			return tx.IndexNote(ctx, f, o.cur.data, o.doc)
		}
		return nil
	case opContent:
		f, err := tx.PutFile(ctx, file())
		if err != nil {
			return err
		}
		if f.Kind() == store.FileNote {
			if err := tx.IndexNote(ctx, f, o.cur.data, o.doc); err != nil {
				return err
			}
		}
		if _, err := tx.AddVersion(ctx, store.Version{VaultID: v.id, FileID: f.ID, Path: f.Path, Hash: f.Hash,
			Size: f.Size, Source: api.VersionSourceOutside}); err != nil {
			return err
		}
		return tx.Emit(ctx, api.NoteChanged{VaultID: v.id, Path: f.Path, Hash: f.Hash, Source: api.VersionSourceOutside})
	case opMove:
		if _, err := tx.MoveFile(ctx, o.from.ID, o.path); err != nil {
			return err
		}
		if _, err := tx.PutFile(ctx, file()); err != nil {
			return err
		}
		return tx.Emit(ctx, api.NoteMoved{VaultID: v.id, From: o.from.Path, To: o.path})
	case opGone:
		return v.deleteRow(ctx, tx, *o.row, api.VersionSourceOutside, "")
	}
	return fmt.Errorf("unknown change %d", o.kind)
}

// deleteRow records file row's deletion as a version, removes it from the
// index and emits note.removed.
func (v *Vault) deleteRow(ctx context.Context, tx *store.Tx, row store.File, source api.VersionSource, author string) error {
	if _, err := tx.AddVersion(ctx, store.Version{VaultID: v.id, FileID: row.ID, Path: row.Path,
		Source: source, Author: author}); err != nil {
		return err
	}
	if err := tx.DeleteFile(ctx, row.ID); err != nil {
		return err
	}
	return tx.Emit(ctx, api.NoteRemoved{VaultID: v.id, Path: row.Path})
}

// storeBlob keeps a file's content in the blob store, checking that what
// it stored is what was read.
func (v *Vault) storeBlob(ctx context.Context, r *os.Root, p string, c *content) error {
	if c.data != nil {
		_, err := v.cfg.Blobs.Put(ctx, bytes.NewReader(c.data))
		return err
	}
	f, err := r.Open(p)
	if err != nil {
		return fmt.Errorf("%w: %w", errUnstable, err)
	}
	defer func() { _ = f.Close() }()
	id, err := v.cfg.Blobs.Put(ctx, f)
	if err != nil {
		return err
	}
	if id != c.hash {
		return errUnstable
	}
	return nil
}

// row returns the index's row at p, or nil when there is none.
func (v *Vault) row(ctx context.Context, p string) (*store.File, error) {
	f, err := v.cfg.DB.FileByPath(ctx, v.id, p)
	if errors.Is(err, store.ErrNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &f, nil
}

// refresh brings the index's row for p up to date with the file on disk,
// as a scan would, and returns the row and what the file holds: a file the
// index lacks is added, a changed one indexed as an outside version, and a
// missing one removed as an outside deletion, when refresh returns an
// error matching fs.ErrNotExist. v.mu must be held.
func (v *Vault) refresh(ctx context.Context, r *os.Root, p string) (store.File, content, error) {
	for range 5 {
		row, err := v.row(ctx, p)
		if err != nil {
			return store.File{}, content{}, err
		}
		cur, err := read(r, p)
		if isGone(err) {
			if row != nil {
				if err := v.applyLocked(ctx, r, []item{{path: p, prev: row}}, false); err != nil {
					return store.File{}, content{}, err
				}
			}
			return store.File{}, content{}, fmt.Errorf("%s: %w", p, fs.ErrNotExist)
		}
		if err != nil {
			return store.File{}, content{}, err
		}
		if row != nil && row.Hash == cur.hash && sameStat(row, cur.info) {
			return *row, cur, nil
		}
		if err := v.applyLocked(ctx, r, []item{{path: p, prev: row, cur: &cur}}, false); err != nil {
			return store.File{}, content{}, err
		}
		// Read again: the row now holds this content, unless the file
		// changed once more while it was indexed.
	}
	return store.File{}, content{}, fmt.Errorf("read %s: %w", p, errUnstable)
}
