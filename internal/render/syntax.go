package render

import (
	"bytes"
	"regexp"
	"strconv"
	"strings"

	"github.com/yuin/goldmark"
	"github.com/yuin/goldmark/ast"
	extast "github.com/yuin/goldmark/extension/ast"
	"github.com/yuin/goldmark/parser"
	"github.com/yuin/goldmark/renderer"
	"github.com/yuin/goldmark/text"
	"github.com/yuin/goldmark/util"

	"github.com/frostyard/frostnotes/internal/markdown"
)

// extendSyntax registers how the rest of Obsidian's syntax renders: tags,
// highlights, math, callouts, task checkboxes, headings and block IDs
// (docs/specs/render.md#obsidians-syntax). Task card T-0046.
func (e *obsidianHTML) extendSyntax(m goldmark.Markdown) {
	r := &syntaxRenderer{e}
	m.Parser().AddOptions(
		parser.WithInlineParsers(util.Prioritized(highlightParser{}, 500)),
		// Remove callout markers before links.go looks for standalone embeds.
		parser.WithASTTransformers(util.Prioritized(r, 50)),
	)
	m.Renderer().AddOptions(renderer.WithNodeRenderers(util.Prioritized(r, 100)))
}

var (
	kindHighlight = ast.NewNodeKind("FrostnotesHighlight")
	kindCallout   = ast.NewNodeKind("FrostnotesCallout")
	calloutMarker = regexp.MustCompile(`^\[!([A-Za-z0-9_-]+)\]([+-]?)(?:[ \t]+(.*?))?[ \t]*$`)
	blockSuffix   = regexp.MustCompile(`[\s]+\^([A-Za-z0-9-]+)[\s]*$`)
	loneBlockID   = regexp.MustCompile(`^[\s]*\^([A-Za-z0-9-]+)[\s]*$`)
)

type highlight struct{ ast.BaseInline }

func (*highlight) Kind() ast.NodeKind { return kindHighlight }

func (n *highlight) Dump(source []byte, level int) {
	ast.DumpHelper(n, source, level, nil, nil)
}

type highlightParser struct{}

func (highlightParser) Trigger() []byte { return []byte{'='} }

func (highlightParser) Parse(_ ast.Node, block text.Reader, pc parser.Context) ast.Node {
	before := block.PrecendingCharacter()
	line, segment := block.PeekLine()
	n := parser.ScanDelimiter(line, before, 2, highlightDelimiter{})
	if n == nil || n.OriginalLength != 2 || before == '=' {
		return nil
	}
	n.Segment = segment.WithStop(segment.Start + 2)
	block.Advance(2)
	pc.PushDelimiter(n)
	return n
}

func (highlightParser) CloseBlock(ast.Node, parser.Context) {}

type highlightDelimiter struct{}

func (highlightDelimiter) IsDelimiter(b byte) bool { return b == '=' }

func (highlightDelimiter) CanOpenCloser(opener, closer *parser.Delimiter) bool {
	return opener.Char == closer.Char
}

func (highlightDelimiter) OnMatch(int) ast.Node { return &highlight{} }

type callout struct {
	ast.BaseBlock
	typ, fold, title string
}

func (*callout) Kind() ast.NodeKind { return kindCallout }

func (n *callout) Dump(source []byte, level int) {
	ast.DumpHelper(n, source, level, nil, nil)
}

type syntaxRenderer struct{ *obsidianHTML }

func (r *syntaxRenderer) RegisterFuncs(reg renderer.NodeRendererFuncRegisterer) {
	reg.Register(markdown.TagKind(), r.renderTag)
	reg.Register(markdown.InlineMathKind(), r.renderMath)
	reg.Register(markdown.MathBlockKind(), r.renderMath)
	reg.Register(kindHighlight, r.renderHighlight)
	reg.Register(kindCallout, r.renderCallout)
	reg.Register(extast.KindTaskCheckBox, r.renderTask)
}

func (*syntaxRenderer) renderTag(w util.BufWriter, _ []byte, n ast.Node, entering bool) (ast.WalkStatus, error) {
	if entering {
		name, _ := markdown.TagOf(n)
		_, _ = w.WriteString(`<a class="tag"` + linkAttr("data-tag", name) + ">#" + escapeLink(name) + "</a>")
	}
	return ast.WalkSkipChildren, nil
}

func (*syntaxRenderer) renderMath(w util.BufWriter, source []byte, n ast.Node, entering bool) (ast.WalkStatus, error) {
	if entering {
		tex, _ := markdown.MathOf(n, source)
		tag, end := "span", ""
		if n.Kind() == markdown.MathBlockKind() {
			tag, end = "div", "\n"
		}
		_, _ = w.WriteString("<" + tag + ` class="math"` + linkAttr("data-tex", tex) + "></" + tag + ">" + end)
	}
	return ast.WalkSkipChildren, nil
}

func (*syntaxRenderer) renderHighlight(w util.BufWriter, _ []byte, _ ast.Node, entering bool) (ast.WalkStatus, error) {
	if entering {
		_, _ = w.WriteString("<mark>")
	} else {
		_, _ = w.WriteString("</mark>")
	}
	return ast.WalkContinue, nil
}

func (*syntaxRenderer) renderCallout(w util.BufWriter, _ []byte, node ast.Node, entering bool) (ast.WalkStatus, error) {
	if !entering {
		_, _ = w.WriteString("</div>\n</div>\n")
		return ast.WalkContinue, nil
	}
	n := node.(*callout)
	s := `<div class="callout"` + linkAttr("data-callout", n.typ)
	if n.fold != "" {
		s += linkAttr("data-fold", n.fold)
	}
	if id, ok := n.AttributeString("data-block"); ok {
		s += linkAttr("data-block", id.(string))
	}
	_, _ = w.WriteString(s + ">\n" + `<div class="callout-title">` + escapeLink(n.title) +
		"</div>\n" + "<div class=\"callout-content\">\n")
	return ast.WalkContinue, nil
}

func (r *syntaxRenderer) renderTask(w util.BufWriter, source []byte, node ast.Node, entering bool) (ast.WalkStatus, error) {
	if entering {
		n := node.(*extast.TaskCheckBox)
		s := `<input type="checkbox" disabled=""`
		if n.IsChecked {
			s += ` checked=""`
		}
		if !r.lineless() {
			s += linkAttr("data-line", strconv.Itoa(r.lineOf(source, n.Parent().Lines().At(0).Start)))
		}
		_, _ = w.WriteString(s + "> ")
	}
	return ast.WalkSkipChildren, nil
}

func (r *syntaxRenderer) Transform(doc *ast.Document, reader text.Reader, _ parser.Context) {
	source := reader.Source()
	var blocks, quotes []ast.Node
	_ = ast.Walk(doc, func(n ast.Node, entering bool) (ast.WalkStatus, error) {
		if !entering {
			return ast.WalkContinue, nil
		}
		switch n.Kind() {
		case ast.KindHeading:
			if !r.lineless() {
				n.SetAttributeString("data-line", strconv.Itoa(r.lineOf(source, n.Lines().At(0).Start)))
			}
		case ast.KindParagraph:
			blocks = append(blocks, n)
		case ast.KindTextBlock:
			if n.Parent().Kind() == ast.KindListItem {
				blocks = append(blocks, n)
			}
		case ast.KindBlockquote:
			quotes = append(quotes, n)
		}
		return ast.WalkContinue, nil
	})
	// Collect first: removing ID paragraphs or replacing quotes during a walk
	// would sever the sibling links the walker still needs.
	for _, n := range blocks {
		transformBlockID(n, source)
	}
	for _, n := range quotes {
		transformCallout(n, source)
	}
}

func transformBlockID(n ast.Node, source []byte) {
	last, ok := n.LastChild().(*ast.Text)
	if !ok {
		return
	}
	value := last.Segment.Value(source)
	target := n
	if n.Kind() == ast.KindTextBlock {
		target = n.Parent()
	}
	if match := loneBlockID.FindSubmatch(value); match != nil {
		if prev, ok := last.PreviousSibling().(*ast.Text); ok && (prev.SoftLineBreak() || prev.HardLineBreak()) {
			prev.SetSoftLineBreak(false)
			prev.SetHardLineBreak(false)
			n.RemoveChild(n, last)
			target.SetAttributeString("data-block", string(match[1]))
			return
		}
		if n.Kind() == ast.KindParagraph && n.FirstChild() == last && blockIDTarget(n.PreviousSibling()) {
			n.PreviousSibling().SetAttributeString("data-block", string(match[1]))
			n.Parent().RemoveChild(n.Parent(), n)
			return
		}
	}
	if match := blockSuffix.FindSubmatchIndex(value); match != nil {
		target.SetAttributeString("data-block", string(value[match[2]:match[3]]))
		last.Segment.Stop = last.Segment.Start + match[0]
	}
}

func blockIDTarget(n ast.Node) bool {
	if n == nil {
		return false
	}
	switch n.Kind() {
	case ast.KindList, ast.KindBlockquote, extast.KindTable:
		return true
	}
	return false
}

func transformCallout(quote ast.Node, source []byte) {
	p, ok := quote.FirstChild().(*ast.Paragraph)
	if !ok || p.Lines().Len() == 0 {
		return
	}
	line := p.Lines().At(0)
	match := calloutMarker.FindSubmatch(bytes.TrimRight(line.Value(source), "\r\n"))
	if match == nil {
		return
	}
	n := &callout{typ: strings.ToLower(string(match[1])), fold: string(match[2]), title: string(match[3])}
	if n.title == "" {
		n.title = strings.ToUpper(n.typ[:1]) + n.typ[1:]
	}
	if id, ok := quote.AttributeString("data-block"); ok {
		n.SetAttributeString("data-block", id)
	}
	for child := p.FirstChild(); child != nil; child = p.FirstChild() {
		p.RemoveChild(p, child)
		if t, ok := child.(*ast.Text); ok && (t.SoftLineBreak() || t.HardLineBreak()) {
			break
		}
	}
	if !p.HasChildren() {
		quote.RemoveChild(quote, p)
	}
	for child := quote.FirstChild(); child != nil; child = quote.FirstChild() {
		n.AppendChild(n, child)
	}
	quote.Parent().ReplaceChild(quote.Parent(), quote, n)
}
