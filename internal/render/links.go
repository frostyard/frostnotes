package render

import (
	"bytes"
	"net/url"
	"path"
	"regexp"
	"strings"

	"github.com/yuin/goldmark"
	"github.com/yuin/goldmark/ast"
	"github.com/yuin/goldmark/parser"
	"github.com/yuin/goldmark/renderer"
	"github.com/yuin/goldmark/text"
	"github.com/yuin/goldmark/util"

	"github.com/frostyard/frostnotes/internal/markdown"
)

// extendLinks registers how links, embeds and vault files render
// (docs/specs/render.md#links-and-embeds). Task card T-0045.
func (e *obsidianHTML) extendLinks(m goldmark.Markdown) {
	r := &linkRenderer{e}
	m.Renderer().AddOptions(renderer.WithNodeRenderers(util.Prioritized(r, 100)))
	m.Parser().AddOptions(parser.WithASTTransformers(util.Prioritized(r, 100)))
}

var (
	linkScheme = regexp.MustCompile(`^[A-Za-z][A-Za-z0-9+.-]*:`)
	imageSize  = regexp.MustCompile(`^([0-9]+)(?:x([0-9]+))?$`)
	// Registered once, like the Obsidian parser's node kinds; never changed.
	kindNoteEmbed = ast.NewNodeKind("FrostnotesNoteEmbed")
)

type linkRenderer struct{ *obsidianHTML }

func (r *linkRenderer) RegisterFuncs(reg renderer.NodeRendererFuncRegisterer) {
	reg.Register(markdown.WikilinkKind(), r.renderWiki)
	reg.Register(ast.KindLink, r.renderLink)
	reg.Register(ast.KindImage, r.renderImage)
	reg.Register(kindNoteEmbed, r.renderNoteEmbed)
}

func (r *linkRenderer) resolve(target string) string {
	if r.opts.Resolve == nil {
		return ""
	}
	return r.opts.Resolve(r.opts.Path, target)
}

func escapeLink(s string) string { return string(util.EscapeHTML([]byte(s))) }

func linkAttr(name, value string) string { return ` ` + name + `="` + escapeLink(value) + `"` }

func linkLabel(l markdown.Link) string {
	if l.Alias != "" {
		return l.Alias
	}
	label := l.Target
	if l.Subpath != "" {
		if label != "" {
			label += " > "
		}
		label += strings.TrimPrefix(l.Subpath, "#")
	}
	return label
}

func internalLink(l markdown.Link, resolved string) string {
	class := "internal-link"
	if resolved == "" {
		class += " is-unresolved"
	}
	s := "<a" + linkAttr("class", class) + linkAttr("data-href", l.Target+l.Subpath)
	if resolved != "" {
		s += linkAttr("data-path", resolved)
	}
	return s + ">"
}

func unresolvedEmbed(l markdown.Link) string {
	href := l.Target + l.Subpath
	return `<span class="embed is-unresolved"` + linkAttr("data-href", href) + ">" + escapeLink(href) + "</span>"
}

func markdownDestination(dest []byte) (markdown.Link, bool) {
	s := string(dest)
	if linkScheme.MatchString(s) || strings.HasPrefix(s, "//") {
		return markdown.Link{}, false
	}
	if decoded, err := url.PathUnescape(s); err == nil {
		s = decoded
	}
	target, sub, hasSub := strings.Cut(s, "#")
	l := markdown.Link{Kind: markdown.LinkMarkdown, Target: target}
	if hasSub {
		l.Subpath = "#" + sub
	}
	return l, true
}

func (r *linkRenderer) renderWiki(w util.BufWriter, _ []byte, n ast.Node, entering bool) (ast.WalkStatus, error) {
	if entering {
		l, _ := markdown.WikilinkOf(n)
		resolved := r.resolve(l.Target)
		if l.Embed {
			_, _ = w.WriteString(r.inlineEmbed(l, resolved))
		} else {
			_, _ = w.WriteString(internalLink(l, resolved) + escapeLink(linkLabel(l)) + "</a>")
		}
	}
	return ast.WalkSkipChildren, nil
}

func (r *linkRenderer) renderLink(w util.BufWriter, _ []byte, node ast.Node, entering bool) (ast.WalkStatus, error) {
	if !entering {
		_, _ = w.WriteString("</a>")
		return ast.WalkContinue, nil
	}
	n := node.(*ast.Link)
	if l, internal := markdownDestination(n.Destination); internal {
		_, _ = w.WriteString(internalLink(l, r.resolve(l.Target)))
	} else {
		s := `<a class="external-link"` + linkAttr("href", string(n.Destination))
		if n.Title != nil {
			s += linkAttr("title", string(n.Title))
		}
		_, _ = w.WriteString(s + ` rel="noopener noreferrer">`)
	}
	return ast.WalkContinue, nil
}

func imageAlt(n ast.Node, source []byte) string {
	var b strings.Builder
	_ = ast.Walk(n, func(child ast.Node, entering bool) (ast.WalkStatus, error) {
		if entering {
			switch c := child.(type) {
			case *ast.Text:
				b.Write(c.Segment.Value(source))
			case *ast.String:
				b.Write(c.Value)
			}
		}
		return ast.WalkContinue, nil
	})
	return b.String()
}

func (r *linkRenderer) renderImage(w util.BufWriter, source []byte, node ast.Node, entering bool) (ast.WalkStatus, error) {
	if !entering {
		return ast.WalkSkipChildren, nil
	}
	n := node.(*ast.Image)
	alt := imageAlt(n, source)
	if l, internal := markdownDestination(n.Destination); internal {
		l.Alias, l.Embed = alt, true
		_, _ = w.WriteString(r.inlineEmbed(l, r.resolve(l.Target)))
	} else {
		s := "<img" + linkAttr("src", string(n.Destination)) + linkAttr("alt", alt)
		if n.Title != nil {
			s += linkAttr("title", string(n.Title))
		}
		_, _ = w.WriteString(s + ">")
	}
	return ast.WalkSkipChildren, nil
}

func fileExtension(p string) string { return strings.ToLower(strings.TrimPrefix(path.Ext(p), ".")) }

func fileType(p string) string {
	switch fileExtension(p) {
	case "md":
		return "note"
	case "bmp", "png", "jpg", "jpeg", "gif", "svg", "webp", "avif":
		return "img"
	case "mp3", "wav", "m4a", "3gp", "flac", "ogg", "oga", "opus":
		return "audio"
	case "mp4", "webm", "ogv", "mov", "mkv":
		return "video"
	default:
		return "file"
	}
}

func (r *linkRenderer) inlineEmbed(l markdown.Link, resolved string) string {
	if resolved == "" {
		return unresolvedEmbed(l)
	}
	if fileType(resolved) == "note" {
		if !r.exists(resolved) {
			return unresolvedEmbed(l)
		}
		return internalLink(l, resolved) + escapeLink(linkLabel(l)) + "</a>"
	}
	if r.opts.File == nil {
		return unresolvedEmbed(l)
	}
	hash, ok := r.opts.File(resolved)
	if !ok {
		return unresolvedEmbed(l)
	}
	return fileEmbed(l, resolved, hash)
}

// exists reports whether the vault has a note at path: by File, which
// does not read it, when File knows it.
func (r *linkRenderer) exists(path string) bool {
	if r.opts.File != nil {
		if _, ok := r.opts.File(path); ok {
			return true
		}
	}
	if r.opts.Note != nil {
		_, ok := r.opts.Note(path)
		return ok
	}
	return false
}

func fileEmbed(l markdown.Link, resolved, hash string) string {
	attrs := linkAttr("data-blob", hash+"."+fileExtension(resolved)) + linkAttr("data-path", resolved)
	label := l.Alias
	if label == "" && l.Kind != markdown.LinkMarkdown {
		label = path.Base(resolved)
	}
	switch kind := fileType(resolved); kind {
	case "img":
		var size string
		if dims := imageSize.FindStringSubmatch(l.Alias); dims != nil && l.Kind != markdown.LinkMarkdown {
			label = path.Base(resolved)
			size = linkAttr("width", dims[1])
			if dims[2] != "" {
				size += linkAttr("height", dims[2])
			}
		}
		return `<img class="embed"` + attrs + linkAttr("alt", label) + size + ">"
	case "audio", "video":
		return "<" + kind + ` class="embed" controls=""` + attrs + "></" + kind + ">"
	default:
		if label == "" {
			label = path.Base(resolved)
		}
		return `<a class="file-embed"` + attrs + ">" + escapeLink(label) + "</a>"
	}
}

type noteEmbed struct {
	ast.BaseBlock
	link markdown.Link
	path string
}

func (*noteEmbed) Kind() ast.NodeKind { return kindNoteEmbed }

func (n *noteEmbed) Dump(source []byte, level int) {
	ast.DumpHelper(n, source, level, nil, nil)
}

func (r *linkRenderer) paragraphEmbeds(n ast.Node, source []byte) []*noteEmbed {
	var embeds []*noteEmbed
	for c := n.FirstChild(); c != nil; c = c.NextSibling() {
		if t, ok := c.(*ast.Text); ok && len(bytes.TrimSpace(t.Segment.Value(source))) == 0 {
			continue
		}
		l, ok := markdown.WikilinkOf(c)
		if img, image := c.(*ast.Image); image {
			l, ok = markdownDestination(img.Destination)
			l.Alias, l.Embed = imageAlt(img, source), true
		}
		if !ok || !l.Embed {
			return nil
		}
		resolved := r.resolve(l.Target)
		if fileType(resolved) != "note" {
			return nil
		}
		embeds = append(embeds, &noteEmbed{link: l, path: resolved})
	}
	return embeds
}

func (r *linkRenderer) Transform(doc *ast.Document, reader text.Reader, _ parser.Context) {
	var paragraphs []ast.Node
	_ = ast.Walk(doc, func(n ast.Node, entering bool) (ast.WalkStatus, error) {
		if entering && n.Kind() == ast.KindParagraph {
			paragraphs = append(paragraphs, n)
		}
		return ast.WalkContinue, nil
	})
	for _, p := range paragraphs {
		embeds := r.paragraphEmbeds(p, reader.Source())
		if len(embeds) == 0 {
			continue
		}
		parent := p.Parent()
		for _, embed := range embeds {
			parent.InsertBefore(parent, p, embed)
		}
		parent.RemoveChild(parent, p)
	}
}

func noteSection(note []byte, subpath string) []byte {
	if subpath == "" || strings.HasPrefix(subpath, "#^") {
		return note
	}
	headings := markdown.Parse(note).Headings
	lines := bytes.SplitAfter(note, []byte("\n"))
	for i, h := range headings {
		if !strings.EqualFold(h.Text, strings.TrimPrefix(subpath, "#")) {
			continue
		}
		end := len(lines)
		for _, next := range headings[i+1:] {
			if next.Level <= h.Level {
				end = next.Line - 1
				break
			}
		}
		return bytes.Join(lines[h.Line-1:end], nil)
	}
	return note
}

func (r *linkRenderer) renderNoteEmbed(w util.BufWriter, _ []byte, node ast.Node, entering bool) (ast.WalkStatus, error) {
	if !entering {
		return ast.WalkSkipChildren, nil
	}
	n := node.(*noteEmbed)
	var note []byte
	var ok bool
	if r.opts.Note != nil {
		note, ok = r.opts.Note(n.path)
	}
	if !ok && !r.exists(n.path) {
		_, _ = w.WriteString("<p>" + unresolvedEmbed(n.link) + "</p>\n")
	} else if rendered, allowed := r.embed(n.path, noteSection(note, n.link.Subpath)); ok && allowed {
		_, _ = w.WriteString(`<div class="embed"` + linkAttr("data-href", n.link.Target+n.link.Subpath) +
			linkAttr("data-path", n.path) + ">\n" + rendered + "</div>\n")
	} else {
		// Too deep, a loop, past the budget, or too large to embed: a link.
		label := n.link
		label.Target, label.Alias = strings.TrimSuffix(path.Base(n.path), path.Ext(n.path)), ""
		_, _ = w.WriteString("<p>" + internalLink(n.link, n.path) + escapeLink(linkLabel(label)) + "</a></p>\n")
	}
	return ast.WalkSkipChildren, nil
}
