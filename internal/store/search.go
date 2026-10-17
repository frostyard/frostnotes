package store

import (
	"context"
	"fmt"
	"strings"
	"unicode"

	"github.com/frostyard/frostnotes/api"
)

// SearchQuery turns what a user typed into an FTS5 MATCH expression: each
// double-quoted phrase, and each other word with a letter or digit, is
// quoted as an FTS5 phrase, a word with "*" after it to match as a prefix,
// and all are required. It returns "" when nothing is left to match. Task
// card T-0016.
func SearchQuery(text string) string {
	var terms []string
	for {
		text = strings.TrimLeft(text, " \t\r\n")
		if text == "" {
			break
		}
		phrase := text[0] == '"'
		var term string
		if phrase {
			term, text, _ = strings.Cut(text[1:], `"`)
		} else {
			end := strings.IndexAny(text, " \t\r\n\"")
			if end < 0 {
				end = len(text)
			}
			term, text = text[:end], text[end:]
		}
		if strings.IndexFunc(term, func(r rune) bool {
			return unicode.IsLetter(r) || unicode.IsDigit(r)
		}) < 0 {
			continue
		}
		term = `"` + strings.ReplaceAll(term, `"`, `""`) + `"`
		if !phrase {
			term += "*"
		}
		terms = append(terms, term)
	}
	return strings.Join(terms, " ")
}

// Search returns vault's notes that match query, an expression from
// SearchQuery, best first, at most limit of them, each with a snippet
// around its best match. Task card T-0016.
func (d *DB) Search(ctx context.Context, vaultID int64, query string, limit int) ([]api.SearchHit, error) {
	hits := []api.SearchHit{}
	if query == "" {
		return hits, nil
	}
	limit = min(max(limit, 1), 100)
	rows, err := d.db.QueryContext(ctx, `SELECT files.path, notes.title,
		snippet(notes_fts, -1, char(2), char(3), '…', 16)
		FROM notes_fts
		JOIN notes ON notes.file_id = notes_fts.rowid
		JOIN files ON files.id = notes.file_id
		WHERE files.vault_id = ? AND notes_fts MATCH ?
		ORDER BY bm25(notes_fts, 10.0, 5.0, 1.0, 2.0), files.path
		LIMIT ?`, vaultID, query, limit)
	if err != nil {
		return nil, fmt.Errorf("search notes: %w", err)
	}
	defer rows.Close()
	for rows.Next() {
		var hit api.SearchHit
		var snippet string
		if err := rows.Scan(&hit.Path, &hit.Title, &snippet); err != nil {
			return nil, fmt.Errorf("scan search hit: %w", err)
		}
		hit.Snippet = splitSnippet(snippet)
		hits = append(hits, hit)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("read search hits: %w", err)
	}
	return hits, nil
}

// Query is a search as note.search takes it: words and phrases to match,
// and filters (schema/rpc/note.yaml, note.search).
type Query struct {
	// Match is an FTS5 MATCH expression, as SearchQuery makes it from the
	// words and phrases that are not filters; "" when there are none.
	Match string
	// Paths hold path:TEXT filters: a note's path must contain each,
	// ignoring case.
	Paths []string
	// Files hold file:TEXT filters: a note's file name, its last path
	// segment, must contain each, ignoring case.
	Files []string
	// Tags hold tag:#TAG filters, without '#' and lower-cased: a note must
	// have each tag or one nested under it.
	Tags []string
}

// ParseQuery splits what a user typed into a Query. A filter is a word
// that starts with path:, file: or tag: (case ignored) followed by its
// value, which may be a double-quoted phrase; an empty value is no
// filter. Everything else goes to SearchQuery. Task card T-0044.
func ParseQuery(text string) Query {
	return Query{Match: SearchQuery(text)}
}

// SearchNotes returns vault's notes that match q, at most limit of them.
// With Match, it ranks and snippets as Search does, keeping only notes the
// filters admit; with filters alone it lists the notes they admit by path,
// each without a snippet; with neither it returns none. Task card T-0044.
func (d *DB) SearchNotes(ctx context.Context, vaultID int64, q Query, limit int) ([]api.SearchHit, error) {
	return d.Search(ctx, vaultID, q.Match, limit)
}

func splitSnippet(text string) []api.SnippetPart {
	parts := []api.SnippetPart{}
	match := false
	start := 0
	for i := 0; i < len(text); i++ {
		if text[i] != '\x02' && text[i] != '\x03' {
			continue
		}
		if i > start {
			parts = append(parts, api.SnippetPart{Text: text[start:i], Match: match})
		}
		match = text[i] == '\x02'
		start = i + 1
	}
	if start < len(text) {
		parts = append(parts, api.SnippetPart{Text: text[start:], Match: match})
	}
	return parts
}
