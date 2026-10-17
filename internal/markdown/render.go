package markdown

import (
	"bytes"
	"strings"

	"github.com/yuin/goldmark"
	"github.com/yuin/goldmark/ast"
	"github.com/yuin/goldmark/extension"
	"github.com/yuin/goldmark/text"
)

// What internal/render needs to render a note from the tree the index
// reads (docs/specs/render.md): the same goldmark, the Obsidian
// extension's nodes, and the body without its comments.

// Goldmark returns goldmark as Parse reads notes: CommonMark, GFM,
// footnotes and the Obsidian extension, with opts after them.
func Goldmark(opts ...goldmark.Option) goldmark.Markdown {
	base := []goldmark.Option{goldmark.WithExtensions(extension.GFM, extension.Footnote, obsidian{})}
	return goldmark.New(append(base, opts...)...)
}

// RenderSource returns what a note renders from, its body after any
// frontmatter and a leading BOM with each %%…%% comment outside code
// replaced by the newlines it held, and the 1-based line of the note on
// which the body starts, so a body offset's line is that line plus the
// '\n' bytes before it.
func RenderSource(note []byte) (body []byte, line int) {
	split := SplitFrontmatter(note)
	s := newSource(note, split)
	line = s.line(s.base)
	root := newParser(s).Parse(text.NewReader(s.body))
	comments := findComments(root, s)
	out := make([]byte, 0, len(s.body))
	from := 0
	for _, c := range comments {
		start, end := c[0]-s.base, c[1]-s.base
		out = append(out, s.body[from:start]...)
		out = append(out, bytes.Repeat([]byte("\n"), bytes.Count(s.body[start:end], []byte("\n")))...)
		from = end
	}
	return append(out, s.body[from:]...), line
}

// WikilinkKind is the kind of a [[wikilink]] or ![[embed]] node.
func WikilinkKind() ast.NodeKind { return kindWikilink }

// TagKind is the kind of an inline #tag node.
func TagKind() ast.NodeKind { return kindTag }

// InlineMathKind is the kind of a $…$ or $$…$$ node within a line.
func InlineMathKind() ast.NodeKind { return kindInlineMath }

// MathBlockKind is the kind of a $$ block.
func MathBlockKind() ast.NodeKind { return kindMathBlock }

// WikilinkOf returns a wikilink or embed node's link: its target,
// subpath, alias and embed mark. Its offsets are not set.
func WikilinkOf(n ast.Node) (Link, bool) {
	w, ok := n.(*wikilinkNode)
	if !ok {
		return Link{}, false
	}
	l := w.link
	l.Start, l.End, l.Line = 0, 0, 0
	return l, true
}

// TagOf returns a tag node's name, without '#'.
func TagOf(n ast.Node) (string, bool) {
	t, ok := n.(*tagNode)
	if !ok {
		return "", false
	}
	return t.name, true
}

// MathOf returns a math node's TeX without its delimiters, read from
// source, the text the node was parsed from.
func MathOf(n ast.Node, source []byte) (string, bool) {
	switch m := n.(type) {
	case *inlineMathNode:
		tex := string(source[m.Pos():m.end])
		delim := "$"
		if strings.HasPrefix(tex, "$$") {
			delim = "$$"
		}
		return strings.TrimSuffix(strings.TrimPrefix(tex, delim), delim), true
	case *mathBlockNode:
		var b strings.Builder
		lines := m.Lines()
		for i := range lines.Len() {
			seg := lines.At(i)
			b.Write(seg.Value(source))
		}
		tex := strings.TrimSpace(b.String())
		tex = strings.TrimPrefix(tex, "$$")
		if i := strings.LastIndex(tex, "$$"); i >= 0 {
			tex = tex[:i]
		}
		return strings.TrimSpace(tex), true
	}
	return "", false
}
