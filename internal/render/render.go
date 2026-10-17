// Package render renders a note to HTML for the reading view and embeds
// (ADR-0007; docs/specs/render.md): goldmark over the tree the index
// reads, with Obsidian's syntax drawn as the spec pins it, then
// sanitized.
package render

import (
	"bytes"
	"slices"

	"github.com/yuin/goldmark"
	"github.com/yuin/goldmark/ast"
	"github.com/yuin/goldmark/renderer"
	"github.com/yuin/goldmark/renderer/html"
	"github.com/yuin/goldmark/util"

	"github.com/frostyard/frostnotes/internal/markdown"
)

// MaxEmbedDepth is how deep embedded notes nest below the note rendered:
// embeds three levels down show their notes, and a fourth level is a link.
const MaxEmbedDepth = 3

// Options say where a note sits and how to reach what it links to.
type Options struct {
	// Path is the note's vault path; links resolve from its folder.
	Path string
	// Resolve returns the vault path a link target (without subpath or
	// alias) leads to from the note at from, or "" when it is unresolved.
	// An embedded note's links resolve from the embedded note.
	Resolve func(from, target string) string
	// File returns a vault file's content hash, or false when the vault
	// has no file at path.
	File func(path string) (hash string, ok bool)
	// Note returns an embedded note's content, or false when there is
	// none at path.
	Note func(path string) ([]byte, bool)
}

// Render renders note to sanitized HTML: what note.render returns.
func Render(note []byte, opts Options) string {
	return Sanitize(HTML(note, opts))
}

// HTML renders note to HTML before it is sanitized: what the spec's
// fixtures pin.
func HTML(note []byte, opts Options) string {
	return render(note, opts, []string{opts.Path})
}

// render renders note, embedded in the notes of stack (the outermost
// first, the note itself last).
func render(note []byte, opts Options, stack []string) string {
	body, line := markdown.RenderSource(note)
	ext := &obsidianHTML{opts: opts, stack: stack, line: line}
	md := markdown.Goldmark(
		goldmark.WithRendererOptions(html.WithHardWraps(), html.WithUnsafe()),
		goldmark.WithExtensions(ext),
	)
	var buf bytes.Buffer
	if err := md.Convert(body, &buf); err != nil {
		// goldmark's renderers write to a buffer, which never fails.
		panic(err)
	}
	return buf.String()
}

// obsidianHTML is the goldmark extension that draws Obsidian's syntax.
// Its renderers live in links.go (task card T-0045) and syntax.go (task
// card T-0046).
type obsidianHTML struct {
	opts  Options
	stack []string // the notes being rendered, the outermost first
	line  int      // the note's line on which the rendered source starts
}

// lineless reports whether the note being rendered is embedded: an
// embed's lines are another note's, so nothing in it names a line.
func (e *obsidianHTML) lineless() bool { return len(e.stack) > 1 }

// lineOf returns the note's 1-based line of an offset in source, the text
// being rendered.
func (e *obsidianHTML) lineOf(source []byte, offset int) int {
	return e.line + bytes.Count(source[:offset], []byte("\n"))
}

// Extend registers the extension's parsers, transformers and renderers.
func (e *obsidianHTML) Extend(m goldmark.Markdown) {
	m.Renderer().AddOptions(renderer.WithNodeRenderers(util.Prioritized(fallback{}, 900)))
	e.extendLinks(m)
	e.extendSyntax(m)
}

// fallback draws nothing for the Obsidian extension's nodes, which
// goldmark cannot render on its own; the cards' renderers, registered at
// a higher priority, replace it.
type fallback struct{}

func (fallback) RegisterFuncs(reg renderer.NodeRendererFuncRegisterer) {
	skip := func(util.BufWriter, []byte, ast.Node, bool) (ast.WalkStatus, error) {
		return ast.WalkSkipChildren, nil
	}
	for _, k := range []ast.NodeKind{markdown.WikilinkKind(), markdown.TagKind(), markdown.InlineMathKind(), markdown.MathBlockKind()} {
		reg.Register(k, skip)
	}
}

// embed renders the note at path as an embed of the note being rendered,
// or returns false when that would nest deeper than MaxEmbedDepth or
// embed a note that is already being rendered.
func (e *obsidianHTML) embed(path string, note []byte) (string, bool) {
	if len(e.stack) >= MaxEmbedDepth+1 || slices.Contains(e.stack, path) {
		return "", false
	}
	opts := e.opts
	opts.Path = path
	return render(note, opts, append(slices.Clone(e.stack), path)), true
}
