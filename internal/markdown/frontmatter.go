// Package markdown parses notes (docs/design/vault.md#parse).
package markdown

import "bytes"

// Split is a note divided at its frontmatter.
type Split struct {
	HasFront  bool   // the note has frontmatter
	Front     []byte // the YAML between the delimiter lines, with its line endings; empty without frontmatter
	Body      []byte // everything after the closing delimiter line; the whole note without frontmatter
	BodyStart int    // the byte offset of Body in the note
}

// SplitFrontmatter finds note's frontmatter.
func SplitFrontmatter(note []byte) Split {
	start := 0
	if bytes.HasPrefix(note, []byte("\xef\xbb\xbf")) {
		start = 3
	}
	switch {
	case bytes.HasPrefix(note[start:], []byte("---\n")):
		start += 4
	case bytes.HasPrefix(note[start:], []byte("---\r\n")):
		start += 5
	default:
		return Split{Body: note}
	}

	for pos := start; pos < len(note); {
		end, next := len(note), len(note)
		if newline := bytes.IndexByte(note[pos:], '\n'); newline >= 0 {
			end = pos + newline
			next = end + 1
			if end > pos && note[end-1] == '\r' {
				end--
			}
		}
		line := note[pos:end]
		if bytes.Equal(line, []byte("---")) || bytes.Equal(line, []byte("...")) {
			return Split{
				HasFront:  true,
				Front:     note[start:pos],
				Body:      note[next:],
				BodyStart: next,
			}
		}
		pos = next
	}
	return Split{Body: note}
}
