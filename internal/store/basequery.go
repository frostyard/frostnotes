package store

import (
	"context"
	"encoding/json/jsontext"
	"encoding/json/v2"
	"fmt"
	"time"
)

// ViewQuery is a compiled base view (docs/specs/bases.md#sql). Where, OrderBy
// and their arguments come from internal/query, which writes every
// property key and literal of a base as an argument: no text of a base is
// in them.
type ViewQuery struct {
	Where     string // a condition that is 0 or 1 over files LEFT JOIN notes; empty means every file
	Args      []any
	OrderBy   string // a list of ORDER BY terms ending in files.path
	OrderArgs []any
	Limit     int  // at least 1
	Tags      bool // read each row's tags
	Links     bool // read each row's link targets
}

// QueryRow is a file a query matched, with what the columns, groups and
// image of a view are read from.
type QueryRow struct {
	ID    int64
	Path  string
	Kind  FileKind
	Size  int64
	MTime time.Time
	Title string         // a note's title; empty for an attachment
	Props jsontext.Value // the frontmatter as a JSON object; {} for an attachment
	Tags  []string       // as the index spells them, in order; only when ViewQuery.Tags
	Links []string       // each distinct link target as written, in order; only when ViewQuery.Links
}

const queryFrom = ` FROM files LEFT JOIN notes ON notes.file_id = files.id WHERE files.vault_id = ?`

// RunQuery returns the files of vault that q matches, sorted by q.OrderBy,
// at most q.Limit of them, and how many files match in all.
func (d *DB) RunQuery(ctx context.Context, vaultID int64, q ViewQuery) ([]QueryRow, int, error) {
	where, args := queryFrom, []any{vaultID}
	if q.Where != "" {
		where += " AND (" + q.Where + ")"
		args = append(args, q.Args...)
	}
	// The rows and the count of every match come from 1 statement, so they
	// are 1 snapshot of the index: COUNT(*) OVER () is computed before LIMIT.
	cols := `files.id, files.path, files.kind, files.size, files.mtime, COALESCE(notes.title, ''), COALESCE(notes.props, '{}')`
	if q.Tags {
		cols += `, (SELECT json_group_array(name) FROM (SELECT name FROM tags WHERE tags.file_id = files.id ORDER BY ord))`
	}
	if q.Links {
		cols += `, (SELECT json_group_array(target) FROM (SELECT target FROM links WHERE links.file_id = files.id AND target != ''
			GROUP BY target ORDER BY MIN(ord)))`
	}
	cols += `, COUNT(*) OVER ()`
	order := " ORDER BY files.path"
	if q.OrderBy != "" {
		order = " ORDER BY " + q.OrderBy
		args = append(args, q.OrderArgs...)
	}
	args = append(args, max(q.Limit, 1))
	rows, err := d.db.QueryContext(ctx, `SELECT `+cols+where+order+` LIMIT ?`, args...)
	if err != nil {
		return nil, 0, fmt.Errorf("run query: %w", err)
	}
	defer rows.Close()
	var out []QueryRow
	total := 0 // no row, no match
	for rows.Next() {
		var r QueryRow
		var kind, props string
		var tags, links *string
		var mtime int64
		dest := []any{&r.ID, &r.Path, &kind, &r.Size, &mtime, &r.Title, &props}
		if q.Tags {
			dest = append(dest, &tags)
		}
		if q.Links {
			dest = append(dest, &links)
		}
		dest = append(dest, &total)
		if err := rows.Scan(dest...); err != nil {
			return nil, 0, fmt.Errorf("scan query row: %w", err)
		}
		r.Kind, r.MTime, r.Props = FileKind(kind), time.Unix(0, mtime).UTC(), jsontext.Value(props)
		for _, list := range []struct {
			text *string
			into *[]string
		}{{tags, &r.Tags}, {links, &r.Links}} {
			if list.text == nil {
				continue
			}
			if err := json.Unmarshal([]byte(*list.text), list.into); err != nil {
				return nil, 0, fmt.Errorf("read %s's tags or links: %w", r.Path, err)
			}
		}
		out = append(out, r)
	}
	if err := rows.Err(); err != nil {
		return nil, 0, fmt.Errorf("read query rows: %w", err)
	}
	return out, total, nil
}
