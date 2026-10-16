package store

import (
	"context"
	"database/sql"
	"encoding/json/v2"
	"fmt"
	"path"
	"strconv"
	"strings"

	"github.com/frostyard/frostnotes/internal/markdown"
)

// IndexNote replaces the index rows of note f (notes, props, tags, links,
// headings, blocks, tasks and its notes_fts row) with doc, which
// markdown.Parse returned for content. Each link is resolved with
// ResolveLink. f must be a note in the files table. Task card T-0015.
func (t *Tx) IndexNote(ctx context.Context, f File, content []byte, doc markdown.Doc) error {
	title := strings.TrimSuffix(path.Base(f.Path), ".md")
	if err := t.indexNoteRow(ctx, f.ID, title, doc); err != nil {
		return fmt.Errorf("index %s: %w", f.Path, err)
	}
	if err := t.indexProps(ctx, f.ID, doc.Props); err != nil {
		return fmt.Errorf("index %s: %w", f.Path, err)
	}
	if err := t.indexTags(ctx, f.ID, doc.Tags); err != nil {
		return fmt.Errorf("index %s: %w", f.Path, err)
	}
	if err := t.indexLinks(ctx, f, doc.Links); err != nil {
		return fmt.Errorf("index %s: %w", f.Path, err)
	}
	if err := t.indexStructure(ctx, f.ID, doc); err != nil {
		return fmt.Errorf("index %s: %w", f.Path, err)
	}
	if err := t.indexSearch(ctx, f.ID, title, content, doc.Aliases); err != nil {
		return fmt.Errorf("index %s: %w", f.Path, err)
	}
	return nil
}

// LinkName is links.name for a link's target: its last segment,
// lower-cased by strings.ToLower, without a final ".md"; "" for an empty
// target. A file whose FileName is n may resolve the links named
// strings.TrimSuffix(n, ".md"). Task card T-0015.
func LinkName(target string) string {
	if target == "" {
		return ""
	}
	return strings.TrimSuffix(strings.ToLower(path.Base(target)), ".md")
}

func (t *Tx) indexNoteRow(ctx context.Context, id int64, title string, doc markdown.Doc) error {
	if _, err := t.ExecContext(ctx, `DELETE FROM notes WHERE file_id = ?`, id); err != nil {
		return err
	}
	if doc.Props == nil {
		doc.Props = map[string]any{}
	}
	props, err := json.Marshal(doc.Props, json.Deterministic(true))
	if err != nil {
		return err
	}
	if doc.Aliases == nil {
		doc.Aliases = []string{}
	}
	aliases, err := json.Marshal(doc.Aliases)
	if err != nil {
		return err
	}
	propError := ""
	if doc.PropError != nil {
		propError = doc.PropError.Error()
	}
	_, err = t.ExecContext(ctx, `INSERT INTO notes (file_id, title, props, prop_error, aliases, words)
		VALUES (?, ?, ?, ?, ?, ?)`, id, title, string(props), propError, string(aliases), doc.Words)
	return err
}

func (t *Tx) indexProps(ctx context.Context, id int64, props map[string]any) error {
	for key, value := range props {
		items, ok := value.([]any)
		if !ok {
			items = []any{value}
		}
		for ord, item := range items {
			value, num, ok := propScalar(item)
			if !ok {
				continue
			}
			if _, err := t.ExecContext(ctx, `INSERT INTO props (file_id, key, ord, value, folded, num)
				VALUES (?, ?, ?, ?, ?, ?)`, id, key, ord, value, strings.ToLower(value), num); err != nil {
				return err
			}
		}
	}
	return nil
}

func propScalar(value any) (text string, num any, ok bool) {
	switch v := value.(type) {
	case string:
		return v, nil, true
	case bool:
		return strconv.FormatBool(v), nil, true
	case int64:
		return strconv.FormatInt(v, 10), float64(v), true
	case float64:
		return strconv.FormatFloat(v, 'g', -1, 64), v, true
	case nil:
		return "", nil, true
	default:
		return "", nil, false
	}
}

func (t *Tx) indexTags(ctx context.Context, id int64, tags []markdown.Tag) error {
	for ord, tag := range tags {
		if _, err := t.ExecContext(ctx, `INSERT INTO tags (file_id, ord, folded, name, front, line)
			VALUES (?, ?, ?, ?, ?, ?)`, id, ord, strings.ToLower(tag.Name), tag.Name, tag.Front, tag.Line); err != nil {
			return err
		}
	}
	return nil
}

func (t *Tx) indexLinks(ctx context.Context, f File, links []markdown.Link) error {
	for ord, link := range links {
		dest, ok, err := t.ResolveLink(ctx, f.VaultID, f.Path, link.Target)
		if err != nil {
			return err
		}
		_, err = t.ExecContext(ctx, `INSERT INTO links
			(file_id, ord, kind, target, subpath, alias, embed, key, start, stop, line, name, dest_id)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
			f.ID, ord, string(link.Kind), link.Target, link.Subpath, link.Alias, link.Embed, link.Key,
			link.Start, link.End, link.Line, LinkName(link.Target), sql.NullInt64{Int64: dest.ID, Valid: ok})
		if err != nil {
			return err
		}
	}
	return nil
}

func (t *Tx) indexStructure(ctx context.Context, id int64, doc markdown.Doc) error {
	for ord, heading := range doc.Headings {
		if _, err := t.ExecContext(ctx, `INSERT INTO headings (file_id, ord, level, text, line)
			VALUES (?, ?, ?, ?, ?)`, id, ord, heading.Level, heading.Text, heading.Line); err != nil {
			return err
		}
	}
	for ord, block := range doc.Blocks {
		if _, err := t.ExecContext(ctx, `INSERT INTO blocks (file_id, ord, block_id, line)
			VALUES (?, ?, ?, ?)`, id, ord, block.ID, block.Line); err != nil {
			return err
		}
	}
	for ord, task := range doc.Tasks {
		if _, err := t.ExecContext(ctx, `INSERT INTO tasks (file_id, ord, status, text, line)
			VALUES (?, ?, ?, ?, ?)`, id, ord, task.Status, task.Text, task.Line); err != nil {
			return err
		}
	}
	return nil
}

func (t *Tx) indexSearch(ctx context.Context, id int64, title string, content []byte, aliases []string) error {
	split := markdown.SplitFrontmatter(content)
	_, err := t.ExecContext(ctx, `INSERT INTO notes_fts (rowid, title, aliases, body, props)
		VALUES (?, ?, ?, ?, ?)`, id, title, strings.Join(aliases, "\n"),
		strings.ToValidUTF8(string(split.Body), "�"), strings.ToValidUTF8(string(split.Front), "�"))
	return err
}
