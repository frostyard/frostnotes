package render

import (
	"bytes"
	"net/url"
	"path"
	"regexp"
	"slices"
	"sort"
	"strings"

	"github.com/yuin/goldmark"
	"github.com/yuin/goldmark/ast"
	"github.com/yuin/goldmark/parser"
	"github.com/yuin/goldmark/text"
	"github.com/yuin/goldmark/util"

	"github.com/frostyard/frostnotes/internal/markdown"
)

// The languages of the fenced blocks Markdown writes for what CommonMark
// has no syntax for, which a native view draws itself
// (docs/specs/render.md#markdown-for-native-views).
const (
	EmbedLanguage   = "frostnotes-embed"
	CalloutLanguage = "frostnotes-callout"
	MediaLanguage   = "frostnotes-media"
)

// The schemes of the links Markdown writes.
const (
	NoteScheme = "note:" // a vault note or file, by path, with a subpath
	TagScheme  = "tag:"  // a tag, without '#'
	BlobScheme = "blob:" // a vault file's content, by hash and extension
)

// Markdown renders note as Markdown that a native view (MyGo's) reads
// with CommonMark and GitHub's extensions: Obsidian's syntax becomes links
// with the schemes above, <mark> and fenced blocks of the languages above,
// and everything else stays as written. It is what note.renderMarkdown
// returns. A native view shows raw HTML as text, so nothing needs
// sanitizing.
func Markdown(note []byte, opts Options) string {
	return markdownOf(note, opts, []string{opts.Path}, &budget{embeds: MaxEmbeds, bytes: MaxEmbedBytes})
}

func markdownOf(note []byte, opts Options, stack []string, left *budget) string {
	body, _ := markdown.RenderSource(note)
	return rewrite(body, opts, stack, left)
}

// edit replaces body[start:end] with text.
type edit struct {
	start, end int
	text       string
	lines      []int
}

type rewriter struct {
	opts    Options
	stack   []string
	left    *budget
	body    []byte
	origins []int
	edits   []edit
}

// rewrite rewrites a body (a note's, or a callout's inside) as Markdown.
// NativeMarkdown includes source-line provenance for interactive native views.
type NativeMarkdown struct {
	Markdown    string
	SourceLines []int
}

// MarkdownWithSourceLines renders the note and maps output lines to its source.
// Embedded notes map to zero: clicks must never write a different document.
func MarkdownWithSourceLines(note []byte, opts Options) NativeMarkdown {
	body, base := markdown.RenderSource(note)
	origins := make([]int, bytes.Count(body, []byte("\n"))+1)
	for i := range origins {
		origins[i] = base + i
	}
	return rewriteMapped(body, opts, []string{opts.Path}, &budget{embeds: MaxEmbeds, bytes: MaxEmbedBytes}, origins)
}

func rewrite(body []byte, opts Options, stack []string, left *budget) string {
	return rewriteMapped(body, opts, stack, left, nil).Markdown
}

func rewriteMapped(body []byte, opts Options, stack []string, left *budget, origins []int) NativeMarkdown {
	md := markdown.Goldmark(goldmark.WithParserOptions(parser.WithInlineParsers(util.Prioritized(highlightParser{}, 500))))
	root := md.Parser().Parse(text.NewReader(body))
	r := &rewriter{opts: opts, stack: stack, left: left, body: body, origins: origins}
	_ = ast.Walk(root, r.visit)
	slices.SortStableFunc(r.edits, func(a, b edit) int { return a.start - b.start })
	starts := []int{0}
	for i, c := range body {
		if c == '\n' {
			starts = append(starts, i+1)
		}
	}
	origin := func(at int) int {
		i := sort.Search(len(starts), func(i int) bool { return starts[i] > at }) - 1
		if i >= 0 && i < len(origins) {
			return origins[i]
		}
		return 0
	}
	var out strings.Builder
	lines := []int{origin(0)}
	// appendText assigns provenance to each newly started output line. Text
	// appended midway through a line keeps that line's existing provenance.
	appendText := func(value string, mapped []int, fallback int) {
		line := 0
		if out.Len() == 0 || strings.HasSuffix(out.String(), "\n") {
			if len(mapped) > 0 {
				lines[len(lines)-1] = mapped[0]
			} else {
				lines[len(lines)-1] = fallback
			}
		}
		for _, c := range value {
			if c == '\n' {
				line++
				n := fallback
				if line < len(mapped) {
					n = mapped[line]
				}
				lines = append(lines, n)
			}
		}
		out.WriteString(value)
	}
	unchanged := func(from, to int) {
		first := sort.Search(len(starts), func(i int) bool { return starts[i] > from }) - 1
		var mapped []int
		for i := first; i < len(starts) && starts[i] <= to; i++ {
			if i < len(origins) {
				mapped = append(mapped, origins[i])
			} else {
				mapped = append(mapped, 0)
			}
		}
		appendText(string(body[from:to]), mapped, origin(from))
	}
	pos := 0
	for _, e := range r.edits {
		if e.start < pos {
			continue
		}
		unchanged(pos, e.start)
		fallback := 0
		if !strings.Contains(e.text, "\n") {
			fallback = origin(e.start)
		}
		appendText(e.text, e.lines, fallback)
		pos = e.end
	}
	unchanged(pos, len(body))
	return NativeMarkdown{Markdown: out.String(), SourceLines: lines}
}

func (r *rewriter) add(start, end int, text string) {
	r.edits = append(r.edits, edit{start: start, end: end, text: text})
}

func (r *rewriter) resolve(target string) string {
	if r.opts.Resolve == nil || target == "" {
		return ""
	}
	return r.opts.Resolve(r.stack[len(r.stack)-1], target)
}

func (r *rewriter) visit(n ast.Node, entering bool) (ast.WalkStatus, error) {
	if !entering {
		return ast.WalkContinue, nil
	}
	switch n.Kind() {
	case ast.KindFencedCodeBlock, ast.KindCodeBlock, ast.KindHTMLBlock, ast.KindCodeSpan,
		ast.KindRawHTML, ast.KindAutoLink, markdown.MathBlockKind(), markdown.InlineMathKind():
		return ast.WalkSkipChildren, nil
	case ast.KindBlockquote:
		if r.callout(n) {
			return ast.WalkSkipChildren, nil
		}
	case ast.KindParagraph, ast.KindTextBlock:
		if r.loneEmbed(n) {
			return ast.WalkSkipChildren, nil
		}
		r.blockID(n)
	case markdown.WikilinkKind():
		r.wikilink(n)
		return ast.WalkSkipChildren, nil
	case markdown.TagKind():
		if s, e, ok := markdown.TagSpan(n); ok {
			name, _ := markdown.TagOf(n)
			r.add(s, e, "[#"+escapeText(name)+"]("+dest(TagScheme+name)+")")
		}
		return ast.WalkSkipChildren, nil
	case ast.KindLink, ast.KindImage:
		r.markdownLink(n)
	case kindHighlight:
		if s, e, ok := childSpan(n); ok && s >= 2 && e+2 <= len(r.body) {
			r.add(s-2, s, "<mark>")
			r.add(e, e+2, "</mark>")
		}
	}
	return ast.WalkContinue, nil
}

// childSpan is where a node's text is: from its first text's start to its
// last text's end.
func childSpan(n ast.Node) (start, end int, ok bool) {
	start, end = -1, -1
	_ = ast.Walk(n, func(c ast.Node, entering bool) (ast.WalkStatus, error) {
		if t, isText := c.(*ast.Text); entering && isText {
			if start < 0 {
				start = t.Segment.Start
			}
			end = t.Segment.Stop
		}
		return ast.WalkContinue, nil
	})
	return start, end, start >= 0
}

// lineSpan is the whole lines a block's text is on.
func (r *rewriter) lineSpan(n ast.Node) (start, end int, ok bool) {
	start, end = -1, -1
	grow := func(s, e int) {
		if start < 0 || s < start {
			start = s
		}
		end = max(end, e)
	}
	_ = ast.Walk(n, func(c ast.Node, entering bool) (ast.WalkStatus, error) {
		if !entering {
			return ast.WalkContinue, nil
		}
		if c.Type() == ast.TypeBlock {
			lines := c.Lines()
			for i := range lines.Len() {
				seg := lines.At(i)
				grow(seg.Start, seg.Stop)
			}
		}
		if t, isText := c.(*ast.Text); isText {
			grow(t.Segment.Start, t.Segment.Stop)
		}
		return ast.WalkContinue, nil
	})
	if start < 0 {
		return 0, 0, false
	}
	start = bytes.LastIndexByte(r.body[:start], '\n') + 1
	if i := bytes.IndexByte(r.body[end:], '\n'); i >= 0 {
		end += i + 1
	} else {
		end = len(r.body)
	}
	return start, end, true
}

// dest writes a link destination in angle brackets, so spaces stay.
func dest(s string) string {
	s = strings.NewReplacer("%", "%25", "<", "%3C", ">", "%3E", "\n", "%0A").Replace(s)
	return "<" + s + ">"
}

// escapeText escapes what would end a link's text.
func escapeText(s string) string {
	return strings.NewReplacer(`\`, `\\`, "[", `\[`, "]", `\]`).Replace(s)
}

func (r *rewriter) wikilink(n ast.Node) {
	start, end, ok := markdown.WikilinkSpan(n)
	if !ok {
		return
	}
	l, _ := markdown.WikilinkOf(n)
	resolved := r.resolve(l.Target)
	if l.Target == "" && l.Subpath != "" {
		resolved = "" // a heading of the note itself
	}
	r.add(start, end, r.inline(l, resolved))
}

// inline is a wikilink, or an embed within a line: a link, an image, or a
// link to a file.
func (r *rewriter) inline(l markdown.Link, resolved string) string {
	label := linkLabel(l)
	if l.Embed && resolved != "" && fileType(resolved) != "note" && r.opts.File != nil {
		if hash, ok := r.opts.File(resolved); ok {
			name := path.Base(resolved)
			if fileType(resolved) == "img" {
				alt := name
				if l.Alias != "" && !imageSize.MatchString(l.Alias) {
					alt = l.Alias
				}
				return "![" + escapeText(alt) + "](" + dest(BlobScheme+hash+"."+fileExtension(resolved)) + ")"
			}
			if l.Alias == "" {
				label = name
			}
		}
	}
	target := NoteScheme + l.Target + l.Subpath
	if resolved != "" {
		target = NoteScheme + resolved + l.Subpath
	}
	return "[" + escapeText(label) + "](" + dest(target) + ")"
}

// markdownLink rewrites the destination of a Markdown link or image to a
// vault file: a link to note:, an image to blob:. A web link, or one whose
// destination cannot be found in the source (an empty text, a reference),
// stays as written.
func (r *rewriter) markdownLink(n ast.Node) {
	var raw []byte
	image := false
	switch l := n.(type) {
	case *ast.Link:
		raw = l.Destination
	case *ast.Image:
		raw, image = l.Destination, true
	}
	l, internal := markdownDestination(raw)
	if !internal || l.Target == "" {
		return
	}
	_, textEnd, ok := childSpan(n)
	if !ok && image {
		textEnd, ok = r.emptyAltAt(n, raw)
	}
	if !ok {
		return
	}
	start, end, ok := destinationAt(r.body, textEnd)
	if !ok {
		return
	}
	resolved := r.resolve(l.Target)
	if resolved == "" {
		return
	}
	if image && fileType(resolved) == "img" && r.opts.File != nil {
		if hash, ok := r.opts.File(resolved); ok {
			r.add(start, end, dest(BlobScheme+hash+"."+fileExtension(resolved)))
			return
		}
	}
	r.add(start, end, dest(NoteScheme+resolved+l.Subpath))
}

// emptyAltAt uses the parser's source position, never a later image or
// code span that happens to contain the same destination.
func (r *rewriter) emptyAltAt(n ast.Node, _ []byte) (int, bool) {
	image, ok := n.(*ast.Image)
	if !ok || image.Reference != nil {
		return 0, false
	}
	start := image.Pos()
	if start < 0 || start+4 > len(r.body) || !bytes.HasPrefix(r.body[start:], []byte("![](")) {
		return 0, false
	}
	return start + 2, true
}

// destinationAt finds the destination of an inline link whose text ends at
// i: "](" then the destination, in angle brackets or up to a space or the
// closing parenthesis, parentheses balanced. A reference link has no "("
// after its own "]", and has no destination here.
func destinationAt(body []byte, i int) (start, end int, ok bool) {
	// The text may end before emphasis closes: skip to the link's own "]".
	j := i
	for j < len(body) && body[j] != ']' {
		switch body[j] {
		case '\n':
			return 0, 0, false
		case '\\':
			j++
		}
		j++
	}
	if j+1 >= len(body) || body[j+1] != '(' {
		return 0, 0, false
	}
	start = j + 2
	for start < len(body) && (body[start] == ' ' || body[start] == '\t') {
		start++
	}
	if start < len(body) && body[start] == '<' {
		k := bytes.IndexByte(body[start:], '>')
		if k < 0 {
			return 0, 0, false
		}
		return start, start + k + 1, true
	}
	depth := 0
	for end = start; end < len(body); end++ {
		switch c := body[end]; {
		case c == '(':
			depth++
		case c == ')' && depth == 0, c == ' ', c == '\t', c == '\n':
			return start, end, end > start
		case c == ')':
			depth--
		case c == '\\' && end+1 < len(body):
			end++
		}
	}
	return 0, 0, false
}

// loneEmbed writes a paragraph of note embeds alone as fenced embed
// blocks, each holding the embedded note's Markdown.
func (r *rewriter) loneEmbed(p ast.Node) bool {
	var embeds []string
	for c := p.FirstChild(); c != nil; c = c.NextSibling() {
		if t, ok := c.(*ast.Text); ok && len(bytes.TrimSpace(t.Segment.Value(r.body))) == 0 {
			continue
		}
		l, ok := markdown.WikilinkOf(c)
		if img, isImage := c.(*ast.Image); isImage {
			l, ok = markdownDestination(img.Destination)
			l.Embed = true
		}
		if !ok || !l.Embed {
			return false
		}
		resolved := r.resolve(l.Target)
		switch fileType(resolved) {
		case "note":
			embeds = append(embeds, r.embedBlock(l, resolved))
		case "audio", "video":
			if r.opts.File == nil {
				return false
			}
			hash, found := r.opts.File(resolved)
			if !found {
				return false
			}
			embeds = append(embeds, fence(MediaLanguage+" "+path.Base(resolved), BlobScheme+hash+"."+fileExtension(resolved)))
		default:
			return false
		}
	}
	start, end, ok := r.lineSpan(p)
	if len(embeds) == 0 || !ok {
		return false
	}
	var b strings.Builder
	for _, e := range embeds {
		b.WriteString(e)
	}
	first := string(r.body[start:p.Lines().At(0).Start])
	r.add(start, end, inContainer(b.String(), first))
	return true
}

// inContainer writes a block's lines inside the quote or list its first
// line is in: first is that line's prefix, as "> " or "- ".
func inContainer(block, first string) string {
	if first == "" {
		return block
	}
	rest := continuation(first)
	var b strings.Builder
	for i, line := range strings.SplitAfter(block, "\n") {
		switch {
		case line == "":
		case i == 0:
			b.WriteString(first + line)
		default:
			b.WriteString(rest + line)
		}
	}
	return b.String()
}

// continuation is what the lines after a container's first start with:
// its quote marks and its indentation, a list's marker becoming spaces.
func continuation(first string) string {
	return strings.Map(func(c rune) rune {
		if c == '>' || c == ' ' || c == '\t' {
			return c
		}
		return ' '
	}, first)
}

// stripContainer takes a container's prefix off a line, as far as the line
// has it: a lazy continuation line may not.
func stripContainer(line, prefix string) string {
	for _, c := range prefix {
		if c == '>' {
			if t := strings.TrimLeft(line, " "); strings.HasPrefix(t, ">") {
				line = t[1:]
			}
			continue
		}
		line = strings.TrimPrefix(line, string(c))
	}
	return line
}

func (r *rewriter) embedBlock(l markdown.Link, p string) string {
	var note []byte
	var ok bool
	if r.opts.Note != nil {
		note, ok = r.opts.Note(p)
	}
	section := noteSection(note, l.Subpath)
	if ok && len(r.stack) < MaxEmbedDepth+1 && !slices.Contains(r.stack, p) &&
		r.left.embeds > 0 && len(section) <= r.left.bytes {
		r.left.embeds--
		r.left.bytes -= len(section)
		opts := r.opts
		opts.Path = p
		inner := markdownOf(section, opts, append(slices.Clone(r.stack), p), r.left)
		return fence(EmbedLanguage+" "+p+l.Subpath, inner)
	}
	// Missing, too deep, a loop, past the budget, or too large: a link.
	label := l
	label.Alias = ""
	if p != "" {
		label.Target = strings.TrimSuffix(path.Base(p), path.Ext(p))
	}
	target := NoteScheme + l.Target + l.Subpath
	if ok || p != "" {
		target = NoteScheme + p + l.Subpath
	}
	return "[" + escapeText(linkLabel(label)) + "](" + dest(target) + ")\n"
}

// fence wraps content in a fenced block of info, with a fence longer than
// any run of its mark inside. The mark is a backtick, or a tilde when info
// has a backtick, which CommonMark does not allow after a backtick fence.
func fence(info, content string) string {
	mark := '`'
	if strings.ContainsRune(info, '`') {
		mark = '~'
	}
	longest, run := 0, 0
	for _, c := range content {
		if c == mark {
			run++
			longest = max(longest, run)
		} else {
			run = 0
		}
	}
	f := strings.Repeat(string(mark), max(3, longest+1))
	if content != "" && !strings.HasSuffix(content, "\n") {
		content += "\n"
	}
	return f + info + "\n" + content + f + "\n"
}

var quoteMark = regexp.MustCompile(`^[ \t]{0,3}>[ \t]?`)

// callout writes a block quote whose first line is [!type] as a fenced
// callout block: its info is the type, the fold mark and the title, and
// it holds the callout's text, rewritten.
func (r *rewriter) callout(q ast.Node) bool {
	first := q.FirstChild()
	if first == nil || first.Kind() != ast.KindParagraph || first.Lines().Len() == 0 {
		return false
	}
	seg := first.Lines().At(0)
	m := calloutMarker.FindSubmatch(bytes.TrimRight(seg.Value(r.body), "\r\n"))
	if m == nil {
		return false
	}
	start, end, ok := r.lineSpan(q)
	if !ok {
		return false
	}
	// What is before this quote's own mark on its first line is the
	// container it is in.
	container := string(r.body[start:seg.Start])
	container = container[:max(0, strings.LastIndexByte(container, '>'))]
	rest := continuation(container)
	lines := strings.SplitAfter(string(r.body[start:end]), "\n")
	var inner strings.Builder
	for i, line := range lines {
		if i == 0 {
			continue // the [!type] line
		}
		inner.WriteString(quoteMark.ReplaceAllString(stripContainer(line, rest), ""))
	}
	info := CalloutLanguage + " " + strings.ToLower(string(m[1])) + string(m[2])
	if title := strings.TrimSpace(string(m[3])); title != "" {
		info += " " + title
	}
	firstLine := bytes.Count(r.body[:start], []byte("\n"))
	var origins []int
	if firstLine+1 < len(r.origins) {
		origins = r.origins[firstLine+1 : min(firstLine+len(lines), len(r.origins))]
	}
	body := rewriteMapped([]byte(inner.String()), r.opts, r.stack, r.left, origins)
	output := inContainer(fence(info, body.Markdown), container)
	mapped := append([]int{0}, body.SourceLines...)
	mapped = append(mapped, 0)
	r.edits = append(r.edits, edit{start: start, end: end, text: output, lines: mapped})
	return true
}

// blockID removes a paragraph's block ID (" ^id" at its end), which the
// reading view hides.
func (r *rewriter) blockID(p ast.Node) {
	lines := p.Lines()
	if lines.Len() == 0 {
		return
	}
	last := lines.At(lines.Len() - 1)
	text := last.Value(r.body)
	if m := loneBlockID.FindIndex(text); m != nil && lines.Len() == 1 {
		r.add(last.Start, last.Stop, "")
		return
	}
	if m := blockSuffix.FindIndex(bytes.TrimRight(text, "\r\n")); m != nil {
		r.add(last.Start+m[0], last.Start+m[1], "")
	}
}

// NoteLink reads a link Markdown wrote: a vault path and subpath (note:)
// or a tag (tag:); ok is false for any other link.
func NoteLink(href string) (path, subpath, tag string, ok bool) {
	switch {
	case strings.HasPrefix(href, TagScheme):
		return "", "", strings.TrimPrefix(href, TagScheme), true
	case strings.HasPrefix(href, NoteScheme):
		p := strings.TrimPrefix(href, NoteScheme)
		if u, err := url.PathUnescape(p); err == nil {
			p = u
		}
		p, sub, _ := strings.Cut(p, "#")
		if sub != "" {
			sub = "#" + sub
		}
		return p, sub, "", true
	}
	return "", "", "", false
}
