package vault

import (
	"context"
	"strings"
	"unicode/utf8"

	"github.com/frostyard/frostnotes/api"
)

// contextReach is how many characters of a link's line Backlinks keeps
// on each side of the link before cutting it with '…'.
const contextReach = 120

// Backlinks returns the links to file fileID, as the store lists them,
// each with its line in the source (docs/specs/ui.md, Right sidebar
// panes): the index's spans index the content of the source's indexed
// hash, which the blob store holds.
func (v *Vault) Backlinks(ctx context.Context, fileID int64) ([]api.Backlink, error) {
	spans, err := v.cfg.DB.BacklinkSpans(ctx, fileID)
	if err != nil {
		return nil, err
	}
	content := map[string][]byte{}
	out := make([]api.Backlink, 0, len(spans))
	for _, sp := range spans {
		data, ok := content[sp.SourceHash]
		if !ok {
			// A source whose blob is gone shows its links without their lines.
			data, _ = v.cfg.Blobs.Get(sp.SourceHash)
			content[sp.SourceHash] = data
		}
		b := sp.Backlink
		b.Context = backlinkContext(data, sp.Start, sp.Stop)
		out = append(out, b)
	}
	return out, nil
}

// backlinkContext is the line of data holding the link at [start, stop):
// the text before it, the link (marked as matching) up to the line's end,
// and the text after it, each side cut with '…' contextReach characters
// from the link. Empty when the span is not in data.
func backlinkContext(data []byte, start, stop int) []api.SnippetPart {
	if start < 0 || start >= stop || stop > len(data) {
		return []api.SnippetPart{}
	}
	lineStart := strings.LastIndexByte(string(data[:start]), '\n') + 1
	lineEnd := len(data)
	if i := strings.IndexByte(string(data[start:]), '\n'); i >= 0 {
		lineEnd = start + i
	}
	end := min(stop, lineEnd)
	before := strings.TrimLeft(valid(data[lineStart:start]), " \t")
	link := valid(data[start:end])
	after := strings.TrimRight(valid(data[end:lineEnd]), " \t\r")
	if utf8.RuneCountInString(before) > contextReach {
		r := []rune(before)
		before = "…" + string(r[len(r)-contextReach:])
	}
	if utf8.RuneCountInString(after) > contextReach {
		after = string([]rune(after)[:contextReach]) + "…"
	}
	parts := []api.SnippetPart{}
	if before != "" {
		parts = append(parts, api.SnippetPart{Text: before})
	}
	parts = append(parts, api.SnippetPart{Text: link, Match: true})
	if after != "" {
		parts = append(parts, api.SnippetPart{Text: after})
	}
	return parts
}

func valid(b []byte) string {
	return strings.ToValidUTF8(string(b), "�")
}
