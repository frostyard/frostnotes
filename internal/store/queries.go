package store

import (
	"context"
	"database/sql"
	"encoding/json/jsontext"
	"encoding/json/v2"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/frostyard/frostnotes/api"
)

// NoteFilter selects notes for ListNotes. A zero field selects every note;
// a note must match every field that is set.
type NoteFilter struct {
	Folder   string // a vault-relative folder: notes in it or below it; "/" is the whole vault
	Tag      string // a tag, or the parent of nested tags, without '#'; case is ignored
	Property string // a frontmatter key the note has
	Value    string // with Property: the key's value, or an item of its list; case is ignored
}

// ListNotes returns vault's notes that match f, by path. Task card T-0017.
func (d *DB) ListNotes(ctx context.Context, vaultID int64, f NoteFilter) ([]api.NoteEntry, error) {
	where, args := noteConditions(vaultID, f)
	rows, err := d.db.QueryContext(ctx, `SELECT files.path, notes.title, files.hash, files.size, files.mtime
		FROM files JOIN notes ON notes.file_id = files.id
		WHERE `+where+` ORDER BY files.path`, args...)
	if err != nil {
		return nil, fmt.Errorf("list notes: %w", err)
	}
	defer rows.Close()
	entries := []api.NoteEntry{}
	for rows.Next() {
		var entry api.NoteEntry
		var mtime int64
		if err := rows.Scan(&entry.Path, &entry.Title, &entry.Hash, &entry.Size, &mtime); err != nil {
			return nil, fmt.Errorf("scan note entry: %w", err)
		}
		entry.ModifiedAt = time.Unix(0, mtime).UTC()
		entries = append(entries, entry)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("read note entries: %w", err)
	}
	return entries, nil
}

func noteConditions(vaultID int64, f NoteFilter) (string, []any) {
	where := "files.vault_id = ?"
	args := []any{vaultID}
	// "/" and "" are the vault's root folder: every note is in it.
	if folder := strings.Trim(f.Folder, "/"); folder != "" {
		where += " AND files.path >= ? AND files.path < ?"
		args = append(args, folder+"/", folder+"0")
	}
	if f.Tag != "" {
		tag := strings.ToLower(strings.TrimPrefix(f.Tag, "#"))
		where += ` AND EXISTS (SELECT 1 FROM tags WHERE tags.file_id = notes.file_id
			AND (tags.folded = ? OR (tags.folded >= ? AND tags.folded < ?)))`
		args = append(args, tag, tag+"/", tag+"0")
	}
	if f.Property != "" {
		if f.Value == "" {
			where += " AND EXISTS (SELECT 1 FROM json_each(notes.props) WHERE key = ?)"
			args = append(args, f.Property)
		} else {
			where += ` AND EXISTS (SELECT 1 FROM props WHERE props.file_id = notes.file_id
				AND props.key = ? AND props.folded = ?)`
			args = append(args, f.Property, strings.ToLower(f.Value))
		}
	}
	return where, args
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
	info := NoteInfo{Aliases: []string{}, Tags: []string{}}
	var props, aliases string
	err := d.db.QueryRowContext(ctx, `SELECT title, props, prop_error, aliases, words
		FROM notes WHERE file_id = ?`, fileID).
		Scan(&info.Title, &props, &info.PropError, &aliases, &info.Words)
	if errors.Is(err, sql.ErrNoRows) {
		return NoteInfo{}, fmt.Errorf("note %d: %w", fileID, ErrNotFound)
	}
	if err != nil {
		return NoteInfo{}, fmt.Errorf("read note %d: %w", fileID, err)
	}
	info.Props = jsontext.Value(props)
	if err := json.Unmarshal([]byte(aliases), &info.Aliases); err != nil {
		return NoteInfo{}, fmt.Errorf("decode note %d aliases: %w", fileID, err)
	}
	rows, err := d.db.QueryContext(ctx, `SELECT name FROM tags WHERE file_id = ? ORDER BY ord`, fileID)
	if err != nil {
		return NoteInfo{}, fmt.Errorf("list note %d tags: %w", fileID, err)
	}
	defer rows.Close()
	for rows.Next() {
		var tag string
		if err := rows.Scan(&tag); err != nil {
			return NoteInfo{}, fmt.Errorf("scan note %d tag: %w", fileID, err)
		}
		info.Tags = append(info.Tags, tag)
	}
	if err := rows.Err(); err != nil {
		return NoteInfo{}, fmt.Errorf("read note %d tags: %w", fileID, err)
	}
	return info, nil
}

// Links returns note fileID's links in the order they are written, each
// with the path it resolves to. Task card T-0017.
func (d *DB) Links(ctx context.Context, fileID int64) ([]api.Link, error) {
	rows, err := d.db.QueryContext(ctx, `SELECT l.kind, l.target, l.subpath, l.alias,
		l.embed, l.key, l.line, dest.path
		FROM links l LEFT JOIN files dest ON dest.id = l.dest_id
		WHERE l.file_id = ? ORDER BY l.ord`, fileID)
	if err != nil {
		return nil, fmt.Errorf("list links for file %d: %w", fileID, err)
	}
	defer rows.Close()
	links := []api.Link{}
	for rows.Next() {
		var link api.Link
		if err := rows.Scan(&link.Kind, &link.Target, &link.Subpath, &link.Alias,
			&link.Embed, &link.Key, &link.Line, &link.Resolved); err != nil {
			return nil, fmt.Errorf("scan link for file %d: %w", fileID, err)
		}
		links = append(links, link)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("read links for file %d: %w", fileID, err)
	}
	return links, nil
}

// BacklinkSpan is a backlink with where it sits in its source: the byte
// offsets of the link in the content whose hash is SourceHash.
type BacklinkSpan struct {
	Backlink   api.Backlink
	SourceHash string
	Start      int
	Stop       int
}

// BacklinkSpans returns Backlinks' links with their spans, in the same
// order, for showing each link's line.
func (d *DB) BacklinkSpans(ctx context.Context, fileID int64) ([]BacklinkSpan, error) {
	rows, err := d.db.QueryContext(ctx, `SELECT source.path, source.hash, l.start, l.stop, l.kind,
		l.target, l.subpath, l.alias, l.embed, l.key, l.line, dest.path
		FROM links l JOIN files source ON source.id = l.file_id
		JOIN files dest ON dest.id = l.dest_id
		WHERE l.dest_id = ? AND l.file_id != l.dest_id
		ORDER BY source.path, l.line, l.start`, fileID)
	if err != nil {
		return nil, fmt.Errorf("list backlink spans for file %d: %w", fileID, err)
	}
	defer rows.Close()
	spans := []BacklinkSpan{}
	for rows.Next() {
		var sp BacklinkSpan
		link := &sp.Backlink.Link
		if err := rows.Scan(&sp.Backlink.Source, &sp.SourceHash, &sp.Start, &sp.Stop, &link.Kind,
			&link.Target, &link.Subpath, &link.Alias, &link.Embed, &link.Key, &link.Line, &link.Resolved); err != nil {
			return nil, fmt.Errorf("scan backlink span for file %d: %w", fileID, err)
		}
		spans = append(spans, sp)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("read backlink spans for file %d: %w", fileID, err)
	}
	return spans, nil
}

// Backlinks returns the links that resolve to file fileID, by the linking
// note's path, then line, then position. Task card T-0017.
func (d *DB) Backlinks(ctx context.Context, fileID int64) ([]api.Backlink, error) {
	rows, err := d.db.QueryContext(ctx, `SELECT source.path, l.kind, l.target, l.subpath,
		l.alias, l.embed, l.key, l.line, dest.path
		FROM links l JOIN files source ON source.id = l.file_id
		JOIN files dest ON dest.id = l.dest_id
		WHERE l.dest_id = ? AND l.file_id != l.dest_id
		ORDER BY source.path, l.line, l.start`, fileID)
	if err != nil {
		return nil, fmt.Errorf("list backlinks for file %d: %w", fileID, err)
	}
	defer rows.Close()
	backlinks := []api.Backlink{}
	for rows.Next() {
		var backlink api.Backlink
		link := &backlink.Link
		if err := rows.Scan(&backlink.Source, &link.Kind, &link.Target, &link.Subpath,
			&link.Alias, &link.Embed, &link.Key, &link.Line, &link.Resolved); err != nil {
			return nil, fmt.Errorf("scan backlink for file %d: %w", fileID, err)
		}
		backlinks = append(backlinks, backlink)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("read backlinks for file %d: %w", fileID, err)
	}
	return backlinks, nil
}
