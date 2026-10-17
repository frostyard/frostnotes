package vault

import (
	"bytes"
	"context"
	"encoding/json/jsontext"
	"errors"
	"io/fs"
	"os"
	"path"
	"strings"
	"unicode/utf8"

	"github.com/frostyard/frostnotes/api"
	"github.com/frostyard/frostnotes/internal/blobs"
	"github.com/frostyard/frostnotes/internal/markdown"
	"github.com/frostyard/frostnotes/internal/merge"
	"github.com/frostyard/frostnotes/internal/store"
)

// Author says who made a write, for its version.
type Author struct {
	Source api.VersionSource // editor or agent
	Name   string            // the client's name; "" when unknown
	Push   *Push             // on a hub, the device's push this write is; nil otherwise
}

// checkNotePath is CheckPath for the note domain, which leaves the
// settings directory alone: settings are not notes.
func checkNotePath(p string) error {
	if err := CheckPath(p); err != nil {
		return api.InvalidParams("%v", err)
	}
	if p == SettingsDir || IsSettings(p) {
		return api.InvalidParams("%s is in the settings folder %s, which holds no notes", p, SettingsDir)
	}
	return nil
}

// checkIgnored refuses a path the vault's settings ignore: it is not a
// vault file, so the index would never see it. v.mu must be held.
func (v *Vault) checkIgnored(paths ...string) error {
	for _, p := range paths {
		if v.ignored(p) {
			return api.InvalidParams("%s is ignored by the vault's settings (%s)", p, SettingsFile)
		}
	}
	return nil
}

// apiError maps a file error to the API's: a path the guard refuses is
// invalidParams, a missing file notFound.
func apiError(p string, err error) error {
	var ae *api.Error
	switch {
	case err == nil:
		return nil
	case errors.As(err, &ae):
		return err
	case errors.Is(err, fs.ErrNotExist):
		return api.NotFound("%s does not exist", p)
	case errors.Is(err, ErrPath):
		return api.InvalidParams("%v", err)
	case errors.Is(err, errUnstable):
		return api.Conflict("%s kept changing on disk while it was read: read it again", p)
	}
	return err
}

// showText is content as the API carries it: text, where each sequence of
// bytes that is not UTF-8 reads as U+FFFD.
func showText(data []byte) string { return strings.ToValidUTF8(string(data), "�") }

// restore puts back the bytes of base that are not UTF-8, which the writer
// saw as U+FFFD, wherever the writer's text leaves the words around them
// as they were (ADR-0014). It merges by word, with the writer's text as
// the side that wins: base as shown is the base, and base's bytes the
// other side, which differs from it only where showText replaced bytes.
func restore(base, text []byte) []byte {
	if utf8.Valid(base) {
		return text
	}
	return merge.Text([]byte(showText(base)), text, base).Text
}

// mergeFile merges a write into a file that changed since its base: a note
// by frontmatter key, then body (merge.Note), other text by line; a file
// that is not text does not merge.
func mergeFile(p string, base, head, incoming []byte) (merge.Result, error) {
	if kindOf(p) == store.FileNote {
		return merge.Note(base, head, incoming), nil
	}
	if utf8.Valid(base) && utf8.Valid(head) && utf8.Valid(incoming) {
		return merge.Text(base, head, incoming), nil
	}
	return merge.Result{}, api.Conflict("%s changed since the base, and a file that is not text does not merge: read it again", p)
}

// Read returns a note or attachment as the API shows it, after bringing
// its index rows up to date with the file, so its hash is always a base a
// write can name.
func (v *Vault) Read(ctx context.Context, p string) (*api.Note, error) {
	if err := checkNotePath(p); err != nil {
		return nil, err
	}
	r, err := v.open()
	if err != nil {
		return nil, err
	}
	defer func() { _ = r.Close() }()
	if err := v.lock(); err != nil {
		return nil, err
	}
	defer v.mu.Unlock()
	if err := v.checkIgnored(p); err != nil {
		return nil, err
	}
	return v.read(ctx, r, p)
}

func (v *Vault) read(ctx context.Context, r *os.Root, p string) (*api.Note, error) {
	row, cur, err := v.refresh(ctx, r, p)
	if err != nil {
		return nil, apiError(p, err)
	}
	if cur.data == nil {
		return nil, api.InvalidParams("%s is %d bytes; the API reads files of at most %d", p, cur.size(), bigFile)
	}
	n := &api.Note{Path: row.Path, Title: path.Base(row.Path), Hash: row.Hash, ModifiedAt: row.ModTime,
		Content: showText(cur.data), Props: jsontext.Value("{}"), Aliases: []string{}, Tags: []string{}}
	if row.Kind() != store.FileNote {
		return n, nil
	}
	info, err := v.cfg.DB.NoteInfo(ctx, row.ID)
	if err != nil {
		return nil, err
	}
	n.Title, n.Props, n.Aliases, n.Tags = info.Title, info.Props, info.Aliases, info.Tags
	if info.PropError != "" {
		n.PropError = &info.PropError
	}
	return n, nil
}

// Create makes a new file at p with content, and its missing folders. It
// fails with conflict when p exists (ADR-0006: only a create has no base).
func (v *Vault) Create(ctx context.Context, p string, content []byte, who Author) (*api.WriteResult, error) {
	if err := checkNotePath(p); err != nil {
		return nil, err
	}
	r, err := v.open()
	if err != nil {
		return nil, err
	}
	defer func() { _ = r.Close() }()
	if err := v.lock(); err != nil {
		return nil, err
	}
	defer v.mu.Unlock()
	if err := v.checkIgnored(p); err != nil {
		return nil, err
	}
	return v.create(ctx, r, p, content, who, 0)
}

// create writes a new file; a non-zero id brings back the deleted file
// with that ID (note.revert). v.mu must be held.
func (v *Vault) create(ctx context.Context, r *os.Root, p string, content []byte, who Author, id int64) (*api.WriteResult, error) {
	// A file the index has but the disk lacks is an outside deletion,
	// recorded before the new file takes its path.
	if _, _, err := v.refresh(ctx, r, p); err == nil {
		return nil, api.Conflict("%s exists", p)
	} else if !errors.Is(err, fs.ErrNotExist) {
		return nil, apiError(p, err)
	}
	if _, err := r.Lstat(p); err == nil {
		// A folder, a symlink, or the same file under another case.
		return nil, api.Conflict("%s exists", p)
	}
	if err := mkdirs(r, p); err != nil {
		return nil, apiError(p, err)
	}
	hash, err := v.cfg.Blobs.Put(ctx, bytes.NewReader(content))
	if err != nil {
		return nil, err
	}
	st, err := stage(r, v.root, p, content, nil)
	if err != nil {
		return nil, err
	}
	if err := v.settled(st.commit(true)); errors.Is(err, fs.ErrExist) {
		return nil, api.Conflict("%s exists", p)
	} else if err != nil {
		return nil, err
	}
	// The file is on disk: index it even if the client has gone.
	ctx = context.WithoutCancel(ctx)
	err = v.cfg.DB.Tx(ctx, func(tx *store.Tx) error {
		f := store.File{ID: id, VaultID: v.id, Path: p, Size: st.info.Size(), ModTime: st.info.ModTime(), Hash: hash}
		var err error
		if id != 0 {
			f, err = tx.RestoreFile(ctx, f)
		} else {
			f, err = tx.PutFile(ctx, f)
		}
		if err != nil {
			return err
		}
		return v.indexWrite(ctx, tx, f, content, who)
	})
	if errors.Is(err, store.ErrConflict) {
		return nil, api.Conflict("%s: its file ID is taken", p) // a scan's move won the race
	}
	if err != nil {
		return nil, err
	}
	return &api.WriteResult{Path: p, Hash: hash}, nil
}

// settled passes err on, except errNotSynced: the change happened, so the
// caller records it, and the log says it may not survive a power cut.
func (v *Vault) settled(err error) error {
	if errors.Is(err, errNotSynced) {
		v.log.Warn("a change is on disk but not yet durable", "err", err)
		return nil
	}
	return err
}

// indexWrite indexes a written file's new content, appends its version and
// emits note.changed, in the write's transaction.
func (v *Vault) indexWrite(ctx context.Context, tx *store.Tx, f store.File, content []byte, who Author) error {
	if f.Kind() == store.FileNote {
		var doc markdown.Doc
		data := content
		if f.Size <= bigFile {
			doc = markdown.Parse(content)
		} else {
			data = nil // indexed by name only
		}
		if err := tx.IndexNote(ctx, f, data, doc); err != nil {
			return err
		}
	}
	if _, err := tx.AddVersion(ctx, store.Version{VaultID: v.id, FileID: f.ID, Path: f.Path, Hash: f.Hash,
		Size: f.Size, Source: who.Source, Author: who.Name}); err != nil {
		return err
	}
	if err := v.logRevision(ctx, tx, api.RevisionOpPut, f.ID, f.Path, f.Hash, f.Size, who); err != nil {
		return err
	}
	return tx.Emit(ctx, api.NoteChanged{VaultID: v.id, Path: f.Path, Hash: f.Hash, Source: who.Source})
}

// Write replaces p's content with text, which the writer made from the
// content whose hash is base (ADR-0006). When the file changed since base,
// the write merges into it (docs/design/vault.md#writes); a conflicting
// hunk keeps the file's side, and text is kept as a conflict version.
// Bytes of the file that are not UTF-8 survive where text leaves them be
// (restore).
func (v *Vault) Write(ctx context.Context, p, base string, text []byte, who Author) (*api.WriteResult, error) {
	if err := checkNotePath(p); err != nil {
		return nil, err
	}
	r, err := v.open()
	if err != nil {
		return nil, err
	}
	defer func() { _ = r.Close() }()
	if err := v.lock(); err != nil {
		return nil, err
	}
	defer v.mu.Unlock()
	if err := v.checkIgnored(p); err != nil {
		return nil, err
	}
	return v.write(ctx, r, p, base, text, true, who)
}

// write is Write's path. incoming is the writer's text when isText, or
// bytes to write as they are (a revert's version, a link rewrite). v.mu
// must be held.
func (v *Vault) write(ctx context.Context, r *os.Root, p, base string, incoming []byte, isText bool, who Author) (*api.WriteResult, error) {
	kept := "" // the conflict version kept by an earlier attempt
	for range 5 {
		row, cur, err := v.refresh(ctx, r, p)
		if err != nil {
			return nil, apiError(p, err)
		}
		head := cur.data
		if head == nil {
			return nil, api.InvalidParams("%s is %d bytes; the API writes files of at most %d", p, cur.size(), bigFile)
		}
		baseData := head
		if base != row.Hash {
			if baseData, err = v.baseContent(ctx, row, base); err != nil {
				return nil, err
			}
		}
		in := incoming
		if isText {
			in = restore(baseData, incoming)
		}
		result, conflicts, merged := in, 0, false
		if base != row.Hash {
			res, err := mergeFile(p, baseData, head, in)
			if err != nil {
				return nil, err
			}
			result, conflicts, merged = res.Text, len(res.Conflicts), true
		}
		hash := blobs.ID(result)
		wr := &api.WriteResult{Path: p, Hash: hash, Merged: merged, Conflicts: int64(conflicts)}
		if merged {
			shown := showText(result)
			wr.Content = &shown
		}
		if hash == row.Hash && conflicts == 0 {
			return wr, nil // the file already holds it
		}
		// What the writer sent and lost in a conflict is kept first, in a
		// transaction of its own, so neither a crash nor a client that
		// goes away loses it once the file has changed.
		if conflicts > 0 && kept != blobs.ID(in) {
			if err := v.keepConflict(ctx, row, in, who); err != nil {
				return nil, err
			}
			kept = blobs.ID(in)
		}
		if hash == row.Hash {
			return wr, nil // the file's side stood everywhere
		}
		if _, err := v.cfg.Blobs.Put(ctx, bytes.NewReader(result)); err != nil {
			return nil, err
		}
		st, err := stage(r, v.root, p, result, cur.info)
		if err != nil {
			return nil, err
		}
		if v.staged != nil {
			v.staged(p)
		}
		// The disk is compared just before the rename (ADR-0006): an edit
		// made since refresh read the file is merged in, not lost.
		now, err := read(r, p)
		if err != nil || now.hash != row.Hash || !now.sameStat(cur.info) {
			st.discard()
			continue
		}
		if err := v.settled(st.commit(false)); err != nil {
			return nil, err
		}
		// The file holds the new content: index it even if the client
		// has gone, or the next scan would take it for another program's.
		ctx := context.WithoutCancel(ctx)
		err = v.cfg.DB.Tx(ctx, func(tx *store.Tx) error {
			f, err := tx.PutFile(ctx, store.File{VaultID: v.id, Path: p, Size: st.info.Size(), ModTime: st.info.ModTime(), Hash: hash})
			if err != nil {
				return err
			}
			return v.indexWrite(ctx, tx, f, result, who)
		})
		if err != nil {
			return nil, err
		}
		return wr, nil
	}
	return nil, api.Conflict("%s kept changing on disk while it was written; read it again", p)
}

// keepConflict stores content, a writer's text that lost a conflicting
// hunk, as a conflict version of file row (ADR-0005).
func (v *Vault) keepConflict(ctx context.Context, row store.File, content []byte, who Author) error {
	hash, err := v.cfg.Blobs.Put(ctx, bytes.NewReader(content))
	if err != nil {
		return err
	}
	return v.flagConflict(ctx, row, hash, int64(len(content)), who)
}

// flagConflict records the blob hash, content that lost to file row's
// head, as a conflict version and, on a hub, a conflict revision, which
// flags the file.
func (v *Vault) flagConflict(ctx context.Context, row store.File, hash string, size int64, who Author) error {
	return v.cfg.DB.Tx(ctx, func(tx *store.Tx) error {
		if _, err := tx.AddVersion(ctx, store.Version{VaultID: v.id, FileID: row.ID, Path: row.Path, Hash: hash,
			Size: size, Source: who.Source, Author: who.Name, Conflict: true}); err != nil {
			return err
		}
		return v.logRevision(ctx, tx, api.RevisionOpConflict, row.ID, row.Path, hash, size, who)
	})
}

// baseContent returns the content whose hash is base, a version of file
// row; a base notesd has no version of, or whose content was pruned, is a
// conflict: the writer reads again.
func (v *Vault) baseContent(ctx context.Context, row store.File, base string) ([]byte, error) {
	if base == "" {
		return nil, api.Conflict("%s exists: name the hash it had when it was read as the base", row.Path)
	}
	ok, err := v.cfg.DB.HasVersion(ctx, row.ID, base)
	if err == nil && !ok {
		// A hub keeps content in its log longer than its own versions.
		ok, err = v.cfg.DB.HasContent(ctx, base)
	}
	if err != nil {
		return nil, err
	}
	if !ok {
		return nil, api.Conflict("%s has no version %q to merge from: read it again", row.Path, base)
	}
	data, err := v.cfg.Blobs.Get(base)
	if errors.Is(err, fs.ErrNotExist) {
		return nil, api.Conflict("the content of %s's version %s is gone (versions are kept 30 days): read it again", row.Path, base)
	}
	return data, err
}

// Delete removes p. base must be its current hash: a file changed since is
// not deleted, because an edit beats a delete (ADR-0005). Its versions
// stay, and a deletion version is added.
func (v *Vault) Delete(ctx context.Context, p, base string, who Author) error {
	if err := checkNotePath(p); err != nil {
		return err
	}
	r, err := v.open()
	if err != nil {
		return err
	}
	defer func() { _ = r.Close() }()
	if err := v.lock(); err != nil {
		return err
	}
	defer v.mu.Unlock()
	if err := v.checkIgnored(p); err != nil {
		return err
	}
	row, _, err := v.refresh(ctx, r, p)
	if err != nil {
		return apiError(p, err)
	}
	if base != row.Hash {
		return api.Conflict("%s changed since %q, and an edit beats a delete: read it again", p, base)
	}
	if now, err := read(r, p); err != nil || now.hash != row.Hash {
		return api.Conflict("%s changed while it was deleted, and an edit beats a delete: read it again", p)
	}
	if err := v.settled(remove(r, p)); err != nil {
		return err
	}
	ctx = context.WithoutCancel(ctx) // the file is gone: record it whatever the client does
	return v.cfg.DB.Tx(ctx, func(tx *store.Tx) error { return v.deleteRow(ctx, tx, row, who) })
}

// Revert makes p's content that of version id, as a write with base; a
// deleted file comes back at p, its last path, with its ID.
func (v *Vault) Revert(ctx context.Context, p string, id int64, base string, who Author) (*api.WriteResult, error) {
	if err := checkNotePath(p); err != nil {
		return nil, err
	}
	r, err := v.open()
	if err != nil {
		return nil, err
	}
	defer func() { _ = r.Close() }()
	if err := v.lock(); err != nil {
		return nil, err
	}
	defer v.mu.Unlock()
	if err := v.checkIgnored(p); err != nil {
		return nil, err
	}
	ver, err := v.cfg.DB.GetVersion(ctx, id)
	if errors.Is(err, store.ErrNotFound) || (err == nil && ver.VaultID != v.id) {
		return nil, api.NotFound("version %d does not exist", id)
	}
	if err != nil {
		return nil, err
	}
	if ver.Hash == "" {
		return nil, api.InvalidParams("version %d records a deletion; note.delete deletes", id)
	}
	data, err := v.cfg.Blobs.Get(ver.Hash)
	if errors.Is(err, fs.ErrNotExist) {
		return nil, api.NotFound("the content of version %d is gone", id)
	}
	if err != nil {
		return nil, err
	}
	row, _, err := v.refresh(ctx, r, p)
	switch {
	case err == nil:
		if ver.FileID != row.ID {
			return nil, api.NotFound("version %d is not a version of %s", id, p)
		}
		if base == "" {
			return nil, api.Conflict("%s exists: name its hash as the base", p)
		}
		return v.write(ctx, r, p, base, data, false, who)
	case !errors.Is(err, fs.ErrNotExist):
		return nil, apiError(p, err)
	}
	if base != "" {
		return nil, api.Conflict("%s is deleted: revert it with an empty base", p)
	}
	last, err := v.cfg.DB.LastFileAt(ctx, v.id, p)
	if errors.Is(err, store.ErrNotFound) || (err == nil && last != ver.FileID) {
		return nil, api.NotFound("version %d is not a version of %s", id, p)
	}
	if err != nil {
		return nil, err
	}
	if f, err := v.cfg.DB.FileByID(ctx, ver.FileID); err == nil {
		return nil, api.Conflict("the file of version %d is at %s now", id, f.Path)
	} else if !errors.Is(err, store.ErrNotFound) {
		return nil, err
	}
	return v.create(ctx, r, p, data, who, ver.FileID)
}
