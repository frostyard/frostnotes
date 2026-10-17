package markdown

import (
	"bytes"
	"cmp"
	"regexp"
	"slices"
	"sort"
	"strings"

	"github.com/yuin/goldmark"
	"github.com/yuin/goldmark/ast"
	"github.com/yuin/goldmark/extension"
	"github.com/yuin/goldmark/parser"
	"github.com/yuin/goldmark/text"
	"github.com/yuin/goldmark/util"
)

// Doc is what one note says, as docs/specs/markdown.md#parse-output
// defines it. Every string in it is valid UTF-8.
type Doc struct {
	Links     []Link         // wikilinks, markdown links to vault files and property links, in order of Start
	Tags      []Tag          // each once, compared ignoring case: frontmatter tags first, then inline ones
	Headings  []Heading      // in order
	Blocks    []Block        // block IDs, in order
	Tasks     []Task         // in order, nested ones included
	Aliases   []string       // from the aliases property, in order
	Props     map[string]any // the frontmatter mapping as decoded, tags and aliases included; nil without one
	PropError error          // why the frontmatter is not a YAML mapping; nil when it is or there is none
	Words     int            // words in the body outside comments
}

// LinkKind says where a link was written.
type LinkKind string

// The kinds of link.
const (
	LinkWiki     LinkKind = "wiki"     // [[wikilink]] or ![[embed]] in the body
	LinkMarkdown LinkKind = "markdown" // [text](path) or ![alt](path) to a vault file
	LinkProperty LinkKind = "property" // a wikilink in a frontmatter value
)

// Link is a link to a vault file.
type Link struct {
	Kind    LinkKind
	Target  string // the file named, before any '#'; empty for the note itself
	Subpath string // heading or block ID, with its leading '#'
	Alias   string // display text: after '|', or a markdown link's text
	Embed   bool   // ![[…]] or an image
	Key     string // the frontmatter key, for a property link
	Start   int    // byte offset in the note of '[[', '[' or '!'
	End     int    // byte offset just past the link
	Line    int    // 1-based line of Start; a property link's key's line
}

// Tag is a #tag or a frontmatter tag.
type Tag struct {
	Name  string // without '#', as first spelled
	Front bool   // from the frontmatter
	Line  int    // 1-based; the tags key's line for a frontmatter tag
}

// Heading is an ATX or setext heading.
type Heading struct {
	Level int    // 1–6
	Text  string // the content as written, trimmed, without an ATX closing sequence
	Line  int    // 1-based line of its first line
}

// Block is a block ID.
type Block struct {
	ID   string // without '^'
	Line int    // 1-based
}

// Task is a list item with a [ ] status.
type Task struct {
	Status string // the character between the brackets: " " open, "x" or "X" done, or another
	Text   string // the rest of the item's first line, trimmed
	Line   int    // 1-based
}

// Parse reads one note as docs/specs/markdown.md specifies. It never
// fails: frontmatter that is not a YAML mapping is a PropError, and the
// body is parsed whatever it holds.
func Parse(note []byte) Doc {
	split := SplitFrontmatter(note)
	s := newSource(note, split)
	var d Doc
	parseFront(&d, s, split)
	root := newParser(s).Parse(text.NewReader(s.body))
	s.comments = findComments(root, s)
	collectObsidian(&d, root, s)
	collectStructure(&d, root, s)
	finish(&d)
	return d
}

// source is a note being parsed. goldmark parses body, so an AST offset
// plus base is an offset in the note.
type source struct {
	note     []byte
	body     []byte // the body without a leading BOM
	base     int    // body's offset in note
	newlines []int  // offsets of '\n' in note

	// comments are the %%…%% ranges, in note offsets, sorted; findComments
	// sets them before the collectors run. Only Words leaves them out: the
	// index keeps what they hide, as Obsidian's does (ADR-0015).
	comments [][2]int
	// linkEnds holds the body offset just past each markdown link and
	// image, which goldmark does not record.
	linkEnds map[ast.Node]int
}

func newSource(note []byte, split Split) *source {
	s := &source{note: note, body: split.Body, base: split.BodyStart, linkEnds: map[ast.Node]int{}}
	if !split.HasFront && bytes.HasPrefix(note, []byte("\xef\xbb\xbf")) {
		s.body, s.base = note[3:], 3
	}
	for i, c := range note {
		if c == '\n' {
			s.newlines = append(s.newlines, i)
		}
	}
	return s
}

// line is the 1-based line of a note offset: one more than the '\n' bytes
// before it.
func (s *source) line(off int) int {
	return sort.SearchInts(s.newlines, off) + 1
}

// inComment reports whether a note offset lies in a comment.
func (s *source) inComment(off int) bool {
	i := sort.Search(len(s.comments), func(i int) bool { return s.comments[i][1] > off })
	return i < len(s.comments) && s.comments[i][0] <= off
}

// newParser is goldmark with CommonMark, GFM (tables, strikethrough,
// autolinks, task list items), footnotes and the Obsidian extension. The
// link parser is goldmark's, wrapped to record where each link ends.
func newParser(s *source) parser.Parser {
	var inline []util.PrioritizedValue
	for _, v := range parser.DefaultInlineParsers() {
		if v.Value == parser.NewLinkParser() {
			v.Value = linkEnds{v.Value.(linkParser), s.linkEnds}
		}
		inline = append(inline, v)
	}
	p := parser.NewParser(
		parser.WithBlockParsers(parser.DefaultBlockParsers()...),
		parser.WithInlineParsers(inline...),
		parser.WithParagraphTransformers(parser.DefaultParagraphTransformers()...),
	)
	md := goldmark.New(
		goldmark.WithParser(p),
		goldmark.WithExtensions(extension.GFM, extension.Footnote, obsidian{}),
	)
	return md.Parser()
}

type linkParser interface {
	parser.InlineParser
	parser.CloseBlocker
}

// linkEnds is goldmark's link parser, recording the reader's position just
// past each link or image it returns.
type linkEnds struct {
	linkParser
	ends map[ast.Node]int
}

func (l linkEnds) Parse(parent ast.Node, block text.Reader, pc parser.Context) ast.Node {
	n := l.linkParser.Parse(parent, block, pc)
	switch n.(type) {
	case *ast.Link, *ast.Image:
		_, seg := block.Position()
		l.ends[n] = seg.Start
	}
	return n
}

// finish orders the links, keeps each tag once and makes every string
// valid UTF-8.
func finish(d *Doc) {
	slices.SortStableFunc(d.Links, func(a, b Link) int { return cmp.Compare(a.Start, b.Start) })
	seen := map[string]bool{}
	d.Tags = slices.DeleteFunc(d.Tags, func(t Tag) bool {
		key := strings.ToLower(t.Name)
		dup := seen[key]
		seen[key] = true
		return dup
	})
	for i := range d.Links {
		l := &d.Links[i]
		l.Target, l.Subpath, l.Alias, l.Key = valid(l.Target), valid(l.Subpath), valid(l.Alias), valid(l.Key)
	}
	for i := range d.Tags {
		d.Tags[i].Name = valid(d.Tags[i].Name)
	}
	for i := range d.Headings {
		d.Headings[i].Text = valid(d.Headings[i].Text)
	}
	for i := range d.Tasks {
		d.Tasks[i].Status, d.Tasks[i].Text = valid(d.Tasks[i].Status), valid(d.Tasks[i].Text)
	}
	for i := range d.Aliases {
		d.Aliases[i] = valid(d.Aliases[i])
	}
}

// wikilinkPattern is a wikilink or embed: "[[", one or more characters
// other than '[', ']' and a line break, then "]]", after an optional '!'.
var wikilinkPattern = regexp.MustCompile(`!?\[\[([^\[\]\n]+)\]\]`)

// splitWikilink splits a wikilink's inner text, between "[[" and "]]",
// into its Target, Subpath (with its '#') and Alias
// (docs/specs/markdown.md#wikilinks-and-embeds). The text splits at the
// first '|', dropping a '\' right before it (tables write "\|"), and the
// target part at its first '#'. ok is false when all three are empty:
// [[]] and [[ ]] are not links.
func splitWikilink(inner string) (target, subpath, alias string, ok bool) {
	if i := strings.IndexByte(inner, '|'); i >= 0 {
		inner, alias = strings.TrimSuffix(inner[:i], "\\"), strings.TrimSpace(inner[i+1:])
	}
	target, sub, hasSub := strings.Cut(inner, "#")
	target = strings.TrimSpace(target)
	if hasSub {
		subpath = "#" + strings.TrimSpace(sub)
	}
	return target, subpath, alias, target != "" || subpath != "" || alias != ""
}

// valid replaces each invalid UTF-8 sequence with U+FFFD, as the index
// sees it.
func valid(s string) string { return strings.ToValidUTF8(s, "�") }
