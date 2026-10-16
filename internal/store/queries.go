package store

import (
	"context"
	"encoding/json/jsontext"
	"errors"

	"github.com/frostyard/frostnotes/api"
)

// NoteFilter selects notes for ListNotes. A zero field selects every note;
// a note must match every field that is set.
type NoteFilter struct {
	Folder   string // a vault-relative folder: notes in it or below it
	Tag      string // a tag, or the parent of nested tags, without '#'; case is ignored
	Property string // a frontmatter key the note has
	Value    string // with Property: the key's value, or an item of its list; case is ignored
}

// ListNotes returns vault's notes that match f, by path. Task card T-0017.
func (d *DB) ListNotes(ctx context.Context, vaultID int64, f NoteFilter) ([]api.NoteEntry, error) {
	return nil, errors.New("store.ListNotes is not implemented yet (T-0017)")
}

// NoteInfo is what the index holds for one note beyond its file.
type NoteInfo struct {
	Title     string
	Props     jsontext.Value // the frontmatter as a JSON object
	PropError string         // "" without one
	Aliases   []string
	Tags      []string // as first spelled, in Doc order
	Words     int
}

// NoteInfo returns what the index holds for note fileID, or ErrNotFound.
// Task card T-0017.
func (d *DB) NoteInfo(ctx context.Context, fileID int64) (NoteInfo, error) {
	return NoteInfo{}, errors.New("store.NoteInfo is not implemented yet (T-0017)")
}

// Links returns note fileID's links in the order they are written, each
// with the path it resolves to. Task card T-0017.
func (d *DB) Links(ctx context.Context, fileID int64) ([]api.Link, error) {
	return nil, errors.New("store.Links is not implemented yet (T-0017)")
}

// Backlinks returns the links that resolve to file fileID, by the linking
// note's path, then line, then position. Task card T-0017.
func (d *DB) Backlinks(ctx context.Context, fileID int64) ([]api.Backlink, error) {
	return nil, errors.New("store.Backlinks is not implemented yet (T-0017)")
}
