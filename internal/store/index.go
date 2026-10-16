package store

import (
	"context"
	"errors"

	"github.com/frostyard/frostnotes/internal/markdown"
)

// IndexNote replaces the index rows of note f (notes, props, tags, links,
// headings, blocks, tasks and its notes_fts row) with doc, which
// markdown.Parse returned for content. Each link is resolved with
// ResolveLink. f must be a note in the files table. Task card T-0015.
func (t *Tx) IndexNote(ctx context.Context, f File, content []byte, doc markdown.Doc) error {
	return errors.New("store.IndexNote is not implemented yet (T-0015)")
}

// LinkName is links.name for a link's target: its last segment,
// lower-cased by strings.ToLower, without a final ".md"; "" for an empty
// target. A file whose FileName is n may resolve the links named
// strings.TrimSuffix(n, ".md"). Task card T-0015.
func LinkName(target string) string { return "" }
