package markdown

import (
	"bytes"
	"net/url"
	"regexp"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/yuin/goldmark/ast"
	extast "github.com/yuin/goldmark/extension/ast"
	"github.com/yuin/goldmark/util"
)

var (
	linkSchemePattern = regexp.MustCompile(`^[A-Za-z][A-Za-z0-9+.-]*:`)
	taskPattern       = regexp.MustCompile(`^\[([^\n\r])\][ \t]`)
	blockIDPattern    = regexp.MustCompile(`\s\^([A-Za-z0-9-]+)$`)
	loneBlockPattern  = regexp.MustCompile(`^\^([A-Za-z0-9-]+)$`)
)

// linkLabel accumulates descendant text during the structure walk. A stack
// lets an image's text contribute to both its own alias and an enclosing link.
type linkLabel struct {
	node  ast.Node
	index int
	text  strings.Builder
}

// collectStructure adds the markdown links to vault files, headings, block
// IDs and tasks outside comments to d, and counts its words
// (docs/specs/markdown.md#body).
func collectStructure(d *Doc, root ast.Node, s *source) {
	var labels []*linkLabel
	_ = ast.Walk(root, func(n ast.Node, entering bool) (ast.WalkStatus, error) {
		if !entering {
			if len(labels) > 0 && labels[len(labels)-1].node == n {
				label := labels[len(labels)-1]
				d.Links[label.index].Alias = strings.TrimSpace(label.text.String())
				labels = labels[:len(labels)-1]
			}
			return ast.WalkContinue, nil
		}
		if text, ok := n.(*ast.Text); ok {
			for _, label := range labels {
				label.text.Write(text.Value(s.body))
				if text.SoftLineBreak() {
					label.text.WriteByte(' ')
				}
			}
		}
		if s.inComment(n.Pos() + s.base) {
			return ast.WalkContinue, nil
		}
		switch n := n.(type) {
		case *ast.Link:
			if label := collectMarkdownLink(d, n, n.Destination, false, s); label != nil {
				labels = append(labels, label)
			}
		case *ast.Image:
			if label := collectMarkdownLink(d, n, n.Destination, true, s); label != nil {
				labels = append(labels, label)
			}
		case *ast.Heading:
			collectHeading(d, n, s)
		case *ast.ListItem:
			collectTask(d, n, s)
		case *ast.Paragraph, *ast.TextBlock:
			collectBlockID(d, n, s)
		}
		return ast.WalkContinue, nil
	})
	d.Words = countWords(s)
}

func collectMarkdownLink(d *Doc, n ast.Node, dest []byte, embed bool, s *source) *linkLabel {
	if len(dest) == 0 || linkSchemePattern.Match(dest) || bytes.HasPrefix(dest, []byte("//")) {
		return nil
	}
	dest = util.UnescapePunctuations(dest)
	dest = util.ResolveNumericReferences(dest)
	dest = util.ResolveEntityNames(dest)
	target, subpath, hasSubpath := strings.Cut(string(dest), "#")
	if hasSubpath {
		subpath = "#" + subpath
	}
	start := n.Pos() + s.base
	label := &linkLabel{node: n, index: len(d.Links)}
	d.Links = append(d.Links, Link{
		Kind: LinkMarkdown, Target: unescapePath(target), Subpath: unescapePath(subpath),
		Embed: embed, Start: start, End: s.linkEnds[n] + s.base, Line: s.line(start),
	})
	return label
}

func unescapePath(path string) string {
	if decoded, err := url.PathUnescape(path); err == nil {
		return decoded
	}
	return path
}

func collectHeading(d *Doc, n *ast.Heading, s *source) {
	lines := n.Lines()
	parts := make([]string, 0, lines.Len())
	start := n.Pos()
	if lines.Len() > 0 {
		start = lines.At(0).Start
	}
	for i := 0; i < lines.Len(); i++ {
		seg := lines.At(i)
		parts = append(parts, strings.TrimSpace(string(seg.Value(s.body))))
	}
	d.Headings = append(d.Headings, Heading{
		Level: n.Level, Text: strings.Join(parts, " "), Line: s.line(start + s.base),
	})
}

func collectTask(d *Doc, n *ast.ListItem, s *source) {
	first := n.FirstChild()
	switch first.(type) {
	case *ast.Paragraph, *ast.TextBlock:
	default:
		return
	}
	if first.Lines().Len() == 0 {
		return
	}
	seg := first.Lines().At(0)
	line := seg.Value(s.body)
	match := taskPattern.FindSubmatchIndex(line)
	if match == nil || s.inComment(seg.Start+s.base) {
		return
	}
	d.Tasks = append(d.Tasks, Task{
		Status: string(line[match[2]:match[3]]),
		Text:   strings.TrimSpace(string(line[match[1]:])),
		Line:   s.line(seg.Start + s.base),
	})
}

func collectBlockID(d *Doc, n ast.Node, s *source) {
	lines := n.Lines()
	if lines.Len() == 0 {
		return
	}
	start, stop := lines.At(0).Start, lines.At(lines.Len()-1).Stop
	text := bytes.TrimRightFunc(s.body[start:stop], unicode.IsSpace)
	match := blockIDPattern.FindSubmatchIndex(text)
	if match == nil {
		switch n.PreviousSibling().(type) {
		case *ast.List, *ast.Blockquote, *extast.Table:
			match = loneBlockPattern.FindSubmatchIndex(text)
		}
	}
	if match == nil {
		return
	}
	off := start + match[2] - 1 + s.base // The '^' immediately precedes the ID.
	if !s.inComment(off) {
		d.Blocks = append(d.Blocks, Block{ID: string(text[match[2]:match[3]]), Line: s.line(off)})
	}
}

func countWords(s *source) int {
	words := 0
	hasLetterOrDigit := false
	for off := s.base; off < len(s.note); {
		r, size := utf8.DecodeRune(s.note[off:])
		if unicode.IsSpace(r) || s.inComment(off) {
			hasLetterOrDigit = false
		} else if !hasLetterOrDigit && (unicode.IsLetter(r) || unicode.IsDigit(r)) {
			words++
			hasLetterOrDigit = true
		}
		off += size
	}
	return words
}
