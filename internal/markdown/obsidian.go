package markdown

import (
	"bytes"
	"cmp"
	"regexp"
	"slices"
	"strings"
	"unicode"

	"github.com/yuin/goldmark"
	"github.com/yuin/goldmark/ast"
	"github.com/yuin/goldmark/parser"
	"github.com/yuin/goldmark/text"
	"github.com/yuin/goldmark/util"
)

// obsidian is the goldmark extension for the Obsidian syntax in a note's
// body: wikilinks and embeds, tags and math (docs/specs/markdown.md#body).
type obsidian struct{}

// Extend registers the extension's parsers.
func (obsidian) Extend(m goldmark.Markdown) {
	m.Parser().AddOptions(
		parser.WithInlineParsers(
			util.Prioritized(wikilinkParser{}, 199),
			util.Prioritized(tagParser{}, 199),
			util.Prioritized(inlineMathParser{}, 199),
		),
		parser.WithBlockParsers(util.Prioritized(mathBlockParser{}, 690)),
	)
}

var (
	kindWikilink   = ast.NewNodeKind("FrostnotesWikilink")
	kindTag        = ast.NewNodeKind("FrostnotesTag")
	kindInlineMath = ast.NewNodeKind("FrostnotesInlineMath")
	kindMathBlock  = ast.NewNodeKind("FrostnotesMathBlock")
	inlineWikiRE   = regexp.MustCompile("^" + wikilinkPattern.String())
	inlineTagRE    = regexp.MustCompile(`^#[\p{L}\p{M}\p{Nd}_/-]+`)
)

type wikilinkNode struct {
	ast.BaseInline
	link Link // Start and End are body offsets until collected.
}

func (*wikilinkNode) Kind() ast.NodeKind { return kindWikilink }
func (n *wikilinkNode) Dump(source []byte, level int) {
	ast.DumpHelper(n, source, level, nil, nil)
}

type wikilinkParser struct{}

func (wikilinkParser) Trigger() []byte { return []byte{'[', '!'} }
func (wikilinkParser) Parse(_ ast.Node, block text.Reader, _ parser.Context) ast.Node {
	line, seg := block.PeekLine()
	m := inlineWikiRE.FindSubmatchIndex(line)
	if m == nil {
		return nil
	}
	target, subpath, alias, ok := splitWikilink(string(line[m[2]:m[3]]))
	if !ok {
		return nil
	}
	n := &wikilinkNode{link: Link{
		Kind: LinkWiki, Target: target, Subpath: subpath, Alias: alias,
		Embed: line[0] == '!', Start: seg.Start, End: seg.Start + m[1],
	}}
	block.Advance(m[1])
	return n
}

type tagNode struct {
	ast.BaseInline
	name string
}

func (*tagNode) Kind() ast.NodeKind { return kindTag }
func (n *tagNode) Dump(source []byte, level int) {
	ast.DumpHelper(n, source, level, nil, nil)
}

type tagParser struct{}

func (tagParser) Trigger() []byte { return []byte{'#'} }
func (tagParser) Parse(_ ast.Node, block text.Reader, _ parser.Context) ast.Node {
	if !unicode.IsSpace(block.PrecendingCharacter()) {
		return nil
	}
	line, _ := block.PeekLine()
	match := inlineTagRE.Find(line)
	if len(match) == 0 {
		return nil
	}
	name := string(match[1:])
	if strings.IndexFunc(name, func(r rune) bool { return !unicode.IsDigit(r) }) < 0 {
		return nil
	}
	block.Advance(len(match))
	return &tagNode{name: name}
}

type inlineMathNode struct {
	ast.BaseInline
	end int
}

func (*inlineMathNode) Kind() ast.NodeKind { return kindInlineMath }
func (n *inlineMathNode) Dump(source []byte, level int) {
	ast.DumpHelper(n, source, level, nil, nil)
}

type inlineMathParser struct{}

func (inlineMathParser) Trigger() []byte { return []byte{'$'} }
func (inlineMathParser) Parse(_ ast.Node, block text.Reader, _ parser.Context) ast.Node {
	line, seg := block.PeekLine()
	end := inlineMathEnd(line)
	if end == 0 {
		return nil
	}
	block.Advance(end)
	return &inlineMathNode{end: seg.Start + end}
}

// inlineMathEnd includes the closing delimiter, or is zero without one.
func inlineMathEnd(line []byte) int {
	if bytes.HasPrefix(line, []byte("$$")) {
		if i := bytes.Index(line[2:], []byte("$$")); i >= 0 {
			return i + 4
		}
		return 0
	}
	if len(line) < 2 || bytes.ContainsAny(line[1:2], " \t\r\n") {
		return 0
	}
	for i := 1; i < len(line); i++ {
		if line[i] == '\\' {
			i++ // A backslash escapes the next byte, including another backslash.
			continue
		}
		if line[i] == '$' && line[i-1] != ' ' && line[i-1] != '\t' {
			return i + 1
		}
	}
	return 0
}

type mathBlockNode struct {
	ast.BaseBlock
	closed bool
}

func (*mathBlockNode) Kind() ast.NodeKind { return kindMathBlock }
func (*mathBlockNode) IsRaw() bool        { return true }
func (n *mathBlockNode) Dump(source []byte, level int) {
	ast.DumpHelper(n, source, level, nil, nil)
}

type mathBlockParser struct{}

func (mathBlockParser) Trigger() []byte                             { return []byte{'$'} }
func (mathBlockParser) CanInterruptParagraph() bool                 { return true }
func (mathBlockParser) CanAcceptIndentedLine() bool                 { return false }
func (mathBlockParser) Close(ast.Node, text.Reader, parser.Context) {}

func (mathBlockParser) Open(_ ast.Node, reader text.Reader, pc parser.Context) (ast.Node, parser.State) {
	line, seg := reader.PeekLine()
	pos := pc.BlockOffset()
	if pos < 0 || pos > 3 || !bytes.HasPrefix(line[pos:], []byte("$$")) {
		return nil, parser.NoChildren
	}
	n := &mathBlockNode{closed: bytes.Contains(line[pos+2:], []byte("$$"))}
	n.Lines().Append(seg)
	reader.AdvanceToEOL()
	return n, parser.NoChildren
}

func (mathBlockParser) Continue(node ast.Node, reader text.Reader, _ parser.Context) parser.State {
	n := node.(*mathBlockNode)
	if n.closed {
		return parser.Close
	}
	line, seg := reader.PeekLine()
	n.Lines().Append(seg)
	n.closed = bytes.Contains(line, []byte("$$"))
	reader.AdvanceToEOL()
	return parser.Continue | parser.NoChildren
}

// findComments returns the body's %%…%% comments as sorted, disjoint
// ranges of note offsets. Literal ranges suppress openers, never closers.
func findComments(root ast.Node, s *source) [][2]int {
	literal := literalRanges(root)
	var comments [][2]int
	for off, r := 0, 0; off < len(s.body); {
		i := bytes.Index(s.body[off:], []byte("%%"))
		if i < 0 {
			break
		}
		start := off + i
		for r < len(literal) && literal[r][1] <= start {
			r++
		}
		if r < len(literal) && literal[r][0] <= start {
			off = literal[r][1]
			continue
		}
		end := len(s.body)
		if j := bytes.Index(s.body[start+2:], []byte("%%")); j >= 0 {
			end = start + 2 + j + 2
		}
		comments = append(comments, [2]int{start + s.base, end + s.base})
		off = end
	}
	return comments
}

func literalRanges(root ast.Node) [][2]int {
	var ranges [][2]int
	_ = ast.Walk(root, func(n ast.Node, entering bool) (ast.WalkStatus, error) {
		if !entering {
			return ast.WalkContinue, nil
		}
		switch n := n.(type) {
		case *ast.FencedCodeBlock, *ast.CodeBlock, *ast.HTMLBlock, *mathBlockNode:
			lines := n.Lines()
			if lines.Len() > 0 {
				start := min(n.Pos(), lines.At(0).Start)
				ranges = append(ranges, [2]int{start, lines.At(lines.Len() - 1).Stop})
			}
		case *ast.CodeSpan:
			if last, ok := n.LastChild().(*ast.Text); ok {
				ranges = append(ranges, [2]int{n.Pos(), last.Segment.Stop})
			}
		case *ast.RawHTML:
			for i := 0; i < n.Segments.Len(); i++ {
				seg := n.Segments.At(i)
				ranges = append(ranges, [2]int{seg.Start, seg.Stop})
			}
		case *inlineMathNode:
			ranges = append(ranges, [2]int{n.Pos(), n.end})
		}
		return ast.WalkContinue, nil
	})
	slices.SortFunc(ranges, func(a, b [2]int) int { return cmp.Compare(a[0], b[0]) })
	return ranges
}

// collectObsidian adds the wikilinks, embeds and inline tags to d, those
// inside comments included, as Obsidian indexes them (ADR-0015).
func collectObsidian(d *Doc, root ast.Node, s *source) {
	_ = ast.Walk(root, func(n ast.Node, entering bool) (ast.WalkStatus, error) {
		if !entering {
			return ast.WalkContinue, nil
		}
		switch n := n.(type) {
		case *wikilinkNode:
			link := n.link
			link.Start += s.base
			link.End += s.base
			link.Line = s.line(link.Start)
			d.Links = append(d.Links, link)
		case *tagNode:
			if !insideLink(n) {
				d.Tags = append(d.Tags, Tag{Name: n.name, Line: s.line(n.Pos() + s.base)})
			}
		}
		return ast.WalkContinue, nil
	})
}

func insideLink(n ast.Node) bool {
	for p := n.Parent(); p != nil; p = p.Parent() {
		switch p.(type) {
		case *ast.Link, *ast.Image:
			return true
		}
	}
	return false
}
