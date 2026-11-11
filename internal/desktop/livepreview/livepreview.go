// Package livepreview works out what live preview shows
// (docs/design/app.md#live-preview, ADR-0039): which ranges of a note's
// source hide, and what stands in for the rest, on the lines no selection
// touches. It is plain Go over the text: the editor turns its pieces into
// MyGo's replacements and its blocks into MyGo's blocks.
package livepreview

import (
	"net/url"
	"regexp"
	"slices"
	"strconv"
	"strings"
)

// Kind says what a range of the source shows as.
type Kind uint8

// The kinds of piece.
const (
	// Hide shows nothing: a mark that goes off the selection's lines.
	Hide Kind = iota
	// Bullet is a list's dot.
	Bullet
	// Quote is a quote's bar.
	Quote
	// Task is a check box.
	Task
	// Rule is a horizontal rule.
	Rule
	// Link shows Label, which a click follows.
	Link
	// Tag shows Label, which a click searches for.
	Tag
	// Footnote shows Label, the reference's name, which a click jumps from.
	Footnote
	// Dim shows Label, a done task's text, dimmed.
	Dim

	// embedMark marks an embed in a line's cached pieces, which placeEmbeds
	// settles: what it draws as depends on the vault, not the line.
	embedMark
)

// Piece shows bytes Start to End of the text as its Kind.
type Piece struct {
	Start, End int
	Kind       Kind
	// Done is a Task's state.
	Done bool
	// Label is what a Link, Tag, Footnote or Dim shows.
	Label string
	// Target is the file a Link names, "" for one to the web.
	Target string
}

// BlockKind says what a Block draws.
type BlockKind uint8

// The kinds of block.
const (
	// Rendered draws Source as the reading view would.
	Rendered BlockKind = iota
	// CodeHeader draws a fence's language and a Copy button.
	CodeHeader
)

// Block is something drawn between lines, or in place of the lines a piece
// hides from their start through a newline.
type Block struct {
	Kind BlockKind
	// Line is the line it goes with, from 0.
	Line int
	// Label names it for assistive technology.
	Label string
	// Source is a Rendered block's Markdown.
	Source string
	// Width is the width an image's alias asks for, 0 for its own.
	Width int
	// Language and Code are a CodeHeader's.
	Language, Code string
}

// EmbedKind says what an embed's target draws as.
type EmbedKind uint8

// The kinds of embed.
const (
	// NoEmbed leaves the embed's source as written.
	NoEmbed EmbedKind = iota
	// ImageEmbed draws an image, under its line or in place of it.
	ImageEmbed
	// NoteEmbed draws a note, when the embed is alone on its line.
	NoteEmbed
)

// Options say what Scan shows.
type Options struct {
	// Revealed reports whether a selection touches a line, from 0: such a
	// line shows its source. Nil touches none.
	Revealed func(line int) bool
	// HideFront hides the frontmatter, the properties panel standing in.
	HideFront bool
	// Blocks draws callouts, tables and fences' headers as Blocks; without
	// it they keep their source.
	Blocks bool
	// Embed says what the target of an embed or image draws as; wikilink
	// says which it is. Nil, or without Blocks, none draw.
	Embed func(target string, wikilink bool) EmbedKind
}

// Result is what shows of a text.
type Result struct {
	// Pieces are sorted and apart.
	Pieces []Piece
	// Blocks are in the order of their lines.
	Blocks []Block
}

// The block a line is in, which decides whether its inline syntax counts.
type block uint8

const (
	blockText block = iota
	blockFront
	blockFence
	blockComment
	blockMath
)

// maxCache bounds the cached lines, as a multiple of the note's.
const maxCache = 4

// Scanner keeps each line's pieces, so that a keystroke reads again only
// the line it changed. Keep one per editor. It is not safe for concurrent
// use.
type Scanner struct {
	cache map[lineKey][]Piece
}

// lineKey is a line and what about the lines before it changes its pieces.
type lineKey struct {
	text      string
	afterText bool // the line before is a paragraph's: "---" underlines it
	afterGap  bool // the line before is blank: four spaces start code
}

// Pieces lists what shows of text, sorted and apart, without blocks.
// revealed reports whether a selection touches a line, from 0: such a line
// shows its source. With hideFront the frontmatter hides, the properties
// panel standing in for it.
func (s *Scanner) Pieces(text string, revealed func(line int) bool, hideFront bool) []Piece {
	return s.Scan(text, Options{Revealed: revealed, HideFront: hideFront}).Pieces
}

// Scan works out what shows of text.
func (s *Scanner) Scan(text string, o Options) Result {
	if s.cache == nil {
		s.cache = map[lineKey][]Piece{}
	}
	var res Result
	st := state{text: text, closedFront: closedFront(text)}
	line, start := 0, 0
	lines := 0
	for start <= len(text) {
		end := len(text)
		next := end + 1
		if i := strings.IndexByte(text[start:], '\n'); i >= 0 {
			end = start + i
			next = end + 1
		}
		content := strings.TrimSuffix(text[start:end], "\r")
		if o.Blocks && st.block == blockText {
			if n, after := wholeBlock(text, content, start, next, line, st, o, &res); n > 0 {
				st.afterGap, st.afterText = false, false
				line, start = line+n, after
				lines += n
				continue
			}
		}
		was := st.block
		st.advance(content, line, start, next, min(next, len(text)), o, &res)
		if was == blockText && st.block == blockText && !st.structural {
			from := len(res.Pieces)
			res.Pieces = s.line(res.Pieces, content, start, st, line, o.Revealed)
			placeEmbeds(&res, from, text, content, start, min(next, len(text)), line, o)
		}
		st.afterGap = strings.TrimSpace(content) == ""
		st.afterText = was == blockText && !st.afterGap && !st.structural
		line, start = line+1, next
		lines++
	}
	if o.Blocks && st.block == blockFence && !spanRevealed(o.Revealed, st.fromLine, line-1) {
		st.endFence(len(text), len(text), &res) // a fence that runs on to the end
	}
	if len(s.cache) > maxCache*lines+64 {
		clear(s.cache)
	}
	slices.SortStableFunc(res.Pieces, func(a, b Piece) int { return a.Start - b.Start })
	res.Pieces = apart(res.Pieces)
	return res
}

// apart drops pieces that overlap one before them.
func apart(ps []Piece) []Piece {
	out := ps[:0]
	end := -1
	for _, p := range ps {
		if p.Start < end {
			continue
		}
		out = append(out, p)
		end = p.End
	}
	return out
}

func (s *Scanner) line(out []Piece, content string, start int, st state, line int, revealed func(int) bool) []Piece {
	if revealed != nil && revealed(line) {
		return out
	}
	key := lineKey{content, st.afterText, st.afterGap}
	ps, ok := s.cache[key]
	if !ok {
		// A copy: a substring in the cache would keep the whole note alive,
		// a new note each edit.
		key.text = strings.Clone(content)
		ps = linePieces(key.text, st.afterText, st.afterGap)
		s.cache[key] = ps
	}
	for _, p := range ps {
		p.Start += start
		p.End += start
		out = append(out, p)
	}
	return out
}

// state follows the blocks that span lines.
type state struct {
	text       string
	block      block
	fence      string // the marks that close a fence
	language   string // a fence's language
	body       int    // where a fence's code starts
	from       int    // where a hidden block starts
	fromLine   int
	afterText  bool
	afterGap   bool
	structural bool // this line opened or closed a block
	// closedFront says the note's first "---" has its closing line, without
	// which it is no frontmatter.
	closedFront bool
}

// closedFront reports whether text opens frontmatter and closes it.
func closedFront(text string) bool {
	text = strings.TrimPrefix(text, "\uFEFF")
	first, rest, ok := strings.Cut(text, "\n")
	if !ok || strings.TrimRight(first, " \t\r") != "---" {
		return false
	}
	for line := range strings.SplitSeq(rest, "\n") {
		if t := strings.TrimRight(line, " \t\r"); t == "---" || t == "..." {
			return true
		}
	}
	return false
}

func spanRevealed(revealed func(int) bool, from, to int) bool {
	if revealed == nil {
		return false
	}
	for l := from; l <= to; l++ {
		if revealed(l) {
			return true
		}
	}
	return false
}

// advance moves the state over a line that starts at start and whose next
// line starts at next (limit is that, held to the text), and adds the
// pieces of a block it ends.
func (st *state) advance(content string, line, start, next, limit int, o Options, res *Result) {
	revealed := o.Revealed
	st.structural = false
	trim := strings.TrimRight(content, " \t")
	switch st.block {
	case blockFront:
		st.structural = true
		if trim == "---" || trim == "..." {
			st.block = blockText
			if o.HideFront && !spanRevealed(revealed, st.fromLine, line) {
				res.Pieces = append(res.Pieces, Piece{Start: st.from, End: limit})
			}
		}
		return
	case blockFence:
		st.structural = true
		if t := strings.TrimSpace(content); strings.HasPrefix(t, st.fence) && strings.Trim(t, st.fence[:1]) == "" {
			st.block = blockText
			if o.Blocks && !spanRevealed(revealed, st.fromLine, line) {
				st.endFence(start, limit, res)
			}
		}
		return
	case blockComment:
		st.structural = true
		if i := strings.Index(content, "%%"); i >= 0 {
			st.block = blockText
			end := start + i + 2
			if strings.TrimSpace(content[i+2:]) == "" {
				end = limit // the whole line goes, and its row
			}
			if !spanRevealed(revealed, st.fromLine, line) {
				res.Pieces = append(res.Pieces, Piece{Start: st.from, End: end})
			}
		}
		return
	case blockMath:
		st.structural = true
		if strings.Contains(content, "$$") {
			st.block = blockText
		}
		return
	}
	body := strings.TrimPrefix(trim, "\uFEFF")
	switch t := strings.TrimLeft(content, " "); {
	case line == 0 && body == "---" && st.closedFront:
		st.block, st.from, st.fromLine, st.structural = blockFront, start, line, true
	case len(content)-len(t) <= 3 && (strings.HasPrefix(t, "```") || strings.HasPrefix(t, "~~~")):
		mark := t[:1]
		n := len(t) - len(strings.TrimLeft(t, mark))
		st.block, st.fence, st.structural = blockFence, strings.Repeat(mark, n), true
		st.from, st.fromLine, st.body = start, line, limit
		st.language = ""
		if f := strings.Fields(t[n:]); len(f) > 0 {
			st.language = f[0]
		}
	case strings.HasPrefix(t, "$$") && !strings.Contains(t[2:], "$$"):
		st.block, st.structural = blockMath, true
	default:
		if at := openComment(content); at >= 0 {
			st.block, st.from, st.fromLine, st.structural = blockComment, start+at, line, true
		}
	}
}

// openComment finds a "%%" on the line that no later "%%" closes, outside
// code spans: where a comment that runs on starts, -1 for none.
func openComment(line string) int {
	if !strings.Contains(line, "%%") {
		return -1
	}
	open := -1
	for i := 0; i < len(line); {
		switch {
		case line[i] == '\\':
			i += 2
		case line[i] == '`':
			n := run(line, i, len(line), '`')
			if j := closing(line, i+n, len(line), n); j >= 0 {
				i = j + n
			} else {
				i += n
			}
		case strings.HasPrefix(line[i:], "%%"):
			if open < 0 {
				open = i
			} else {
				open = -1
			}
			i += 2
		default:
			i++
		}
	}
	return open
}

// endFence hides a fence's marks, whose closing line runs from close to
// limit, and puts its header in place of its opening line.
func (st *state) endFence(close, limit int, res *Result) {
	res.Pieces = append(res.Pieces, Piece{Start: st.from, End: st.body})
	if limit > close {
		res.Pieces = append(res.Pieces, Piece{Start: close, End: limit})
	}
	code := strings.TrimSuffix(st.text[st.body:max(st.body, close)], "\n")
	res.Blocks = append(res.Blocks, Block{Kind: CodeHeader, Line: st.fromLine, Label: "Code block", Language: st.language, Code: code})
}

// wholeBlock draws a callout or a table that starts at this line, unless a
// selection touches it: how many lines it took and where the next starts.
func wholeBlock(text, content string, start, next, line int, st state, o Options, res *Result) (lines, after int) {
	if st.afterGap && (strings.HasPrefix(content, "    ") || strings.HasPrefix(content, "\t")) {
		return 0, 0 // indented code
	}
	var label string
	switch {
	case strings.Contains(content, "[!") && calloutRe.MatchString(content):
		label = "Callout"
		lines, after = runOfLines(text, start, func(i int, l string) bool { return i == 0 || quotedRe.MatchString(l) })
	case strings.IndexByte(content, '|') >= 0 && tableStarts(text, content, next):
		label = "Table"
		lines, after = runOfLines(text, start, func(i int, l string) bool { return i < 2 || tableRow(l) })
	default:
		return 0, 0
	}
	if spanRevealed(o.Revealed, line, line+lines-1) {
		return 0, 0
	}
	limit := min(after, len(text))
	res.Pieces = append(res.Pieces, Piece{Start: start, End: limit})
	res.Blocks = append(res.Blocks, Block{Kind: Rendered, Line: line, Label: label, Source: strings.TrimRight(text[start:limit], "\r\n")})
	return lines, after
}

// runOfLines counts the lines from start that keep holds of, and where the
// line after them starts.
func runOfLines(text string, start int, keep func(i int, line string) bool) (lines, after int) {
	for start <= len(text) {
		end, next := len(text), len(text)+1
		if i := strings.IndexByte(text[start:], '\n'); i >= 0 {
			end, next = start+i, start+i+1
		}
		if !keep(lines, strings.TrimSuffix(text[start:end], "\r")) {
			break
		}
		lines++
		start = next
	}
	return lines, start
}

// tableStarts reports whether header is a table's first row: the next line
// is its delimiter row, with as many cells.
func tableStarts(text, header string, next int) bool {
	if next >= len(text) {
		return false
	}
	delim := text[next:]
	if i := strings.IndexByte(delim, '\n'); i >= 0 {
		delim = delim[:i]
	}
	delim = strings.TrimSuffix(delim, "\r")
	return delimiterRe.MatchString(delim) && cells(header) == cells(delim)
}

// cells counts a table row's cells.
func cells(row string) int {
	row = strings.TrimSpace(row)
	row = strings.TrimPrefix(row, "|")
	if strings.HasSuffix(row, "|") && !strings.HasSuffix(row, "\\|") {
		row = row[:len(row)-1]
	}
	n := 1
	for i := 0; i < len(row); i++ {
		switch row[i] {
		case '\\':
			i++
		case '|':
			n++
		}
	}
	return n
}

// tableRow reports whether a line goes on a table: not blank, and no other
// block's start.
func tableRow(line string) bool {
	t := strings.TrimLeft(line, " ")
	return strings.TrimSpace(line) != "" && !strings.HasPrefix(t, ">") && !strings.HasPrefix(t, "```") &&
		!strings.HasPrefix(t, "~~~") && !headingRe.MatchString(line)
}

// placeEmbeds settles the embeds among the line's pieces from from on:
// those that draw hide, and the rest keep their source.
func placeEmbeds(res *Result, from int, text, content string, start, limit, line int, o Options) {
	if !slices.ContainsFunc(res.Pieces[from:], func(p Piece) bool { return p.Kind == embedMark }) {
		return
	}
	kept := res.Pieces[:from]
	pieces := slices.Clone(res.Pieces[from:])
	for _, p := range pieces {
		if p.Kind != embedMark {
			kept = append(kept, p)
			continue
		}
		src := text[p.Start:p.End]
		target, wikilink, width := parseEmbed(src)
		kind := NoEmbed
		if o.Blocks && o.Embed != nil {
			kind = o.Embed(target, wikilink)
		}
		alone := strings.TrimSpace(content) == src
		if kind == NoEmbed || kind == NoteEmbed && !alone {
			continue
		}
		label := "Image"
		if kind == NoteEmbed {
			label = "Embedded note"
		}
		res.Blocks = append(res.Blocks, Block{Kind: Rendered, Line: line, Label: label, Source: src, Width: width})
		if alone {
			kept = append(kept[:from], Piece{Start: start, End: limit})
			break
		}
		kept = append(kept, Piece{Start: p.Start, End: p.End})
	}
	res.Pieces = kept
}

// parseEmbed reads the source of an embed, an image or a link: its target, without a
// subpath, whether it is a wikilink, and the width its alias asks for.
func parseEmbed(src string) (target string, wikilink bool, width int) {
	if inner, ok := strings.CutPrefix(src, "![["); ok {
		inner = strings.TrimSuffix(inner, "]]")
		name, alias, _ := strings.Cut(inner, "|")
		name, _, _ = strings.Cut(name, "#")
		return strings.TrimSpace(name), true, sizeWidth(alias)
	}
	open := strings.IndexByte(src, '[')
	closeBracket, _, ok := linkAt(src, open, len(src))
	if !ok {
		return "", false, 0
	}
	if i := strings.LastIndexByte(src[open+1:closeBracket], '|'); i >= 0 {
		width = sizeWidth(src[open+1+i+1 : closeBracket])
	}
	dest := strings.TrimSpace(src[closeBracket+2 : len(src)-1])
	if rest, ok := strings.CutPrefix(dest, "<"); ok {
		dest, _, _ = strings.Cut(rest, ">")
	} else if f := strings.Fields(dest); len(f) > 0 {
		dest = f[0]
	}
	if d, err := url.PathUnescape(dest); err == nil && !strings.Contains(dest, "://") {
		dest = d
	}
	if !strings.Contains(dest, "://") {
		dest, _, _ = strings.Cut(dest, "#")
	}
	return dest, false, width
}

// sizeWidth is the width of a size alias, "300" or "300x200", 0 for other
// text.
func sizeWidth(alias string) int {
	m := sizeRe.FindStringSubmatch(strings.TrimSpace(alias))
	if m == nil {
		return 0
	}
	n, err := strconv.Atoi(m[1])
	if err != nil {
		return 0
	}
	return n
}

var (
	calloutRe   = regexp.MustCompile(`^ {0,3}> ?\[![^\]]+\]`)
	quotedRe    = regexp.MustCompile(`^ {0,3}>`)
	delimiterRe = regexp.MustCompile(`^ {0,3}\|?[ \t]*:?-+:?[ \t]*(?:\|[ \t]*:?-+:?[ \t]*)*\|?[ \t]*$`)
	sizeRe      = regexp.MustCompile(`^(\d+)(?:x\d+)?$`)
	headingRe   = regexp.MustCompile(`^ {0,3}#{1,6}(?:[ \t]+|$)`)
	quoteRe     = regexp.MustCompile(`^ {0,3}(?:> ?)+`)
	taskRe      = regexp.MustCompile(`^[ \t]*([-*+]|\d+[.)])[ \t]+(\[(.)\])(?:[ \t]|$)`)
	bulletRe    = regexp.MustCompile(`^[ \t]*([-*+])[ \t]+`)
	ruleRe      = regexp.MustCompile(`^ {0,3}(?:(?:-[ \t]*){3,}|(?:\*[ \t]*){3,}|(?:_[ \t]*){3,})$`)
	blockIDRe   = regexp.MustCompile(`[ \t]\^[A-Za-z0-9-]+[ \t]*$`)
	tagRe       = regexp.MustCompile(`^#[\p{L}\p{N}_/-]*[\p{L}_/-][\p{L}\p{N}_/-]*`)
)

// maxObject bounds a label drawn as one object, which cannot wrap: a longer
// link keeps to hidden marks.
const maxObject = 80

// linePieces are the pieces of a line of text, from its start.
func linePieces(line string, afterText, afterGap bool) []Piece {
	if afterGap && (strings.HasPrefix(line, "    ") || strings.HasPrefix(line, "\t")) && !taskRe.MatchString(line) && !bulletRe.MatchString(line) {
		return nil // indented code
	}
	var ps []Piece
	at := 0
	done := false
	if m := quoteRe.FindString(line); m != "" {
		ps = append(ps, Piece{Start: 0, End: len(m), Kind: Quote})
		at = len(m)
	}
	rest := line[at:]
	switch {
	case ruleRe.MatchString(rest) && (!afterText || !strings.HasPrefix(strings.TrimLeft(rest, " "), "-")):
		return append(ps, Piece{Start: at, End: len(line), Kind: Rule})
	case headingRe.MatchString(rest):
		n := len(headingRe.FindString(rest))
		ps = append(ps, Piece{Start: at, End: at + n})
		at += n
	default:
		if m := taskRe.FindStringSubmatchIndex(rest); m != nil {
			done = rest[m[6]:m[7]] != " "
			from := m[2]
			if rest[m[2]] >= '0' && rest[m[2]] <= '9' {
				from = m[4] // an ordered task keeps its number
			}
			mark := rest[m[6]:m[7]]
			ps = append(ps, Piece{Start: at + from, End: at + m[5], Kind: Task, Done: mark != " "})
			at += m[5]
		} else if m := bulletRe.FindStringSubmatchIndex(rest); m != nil {
			ps = append(ps, Piece{Start: at + m[2], End: at + m[3], Kind: Bullet})
			at += m[3]
		}
	}
	end := len(line)
	if m := blockIDRe.FindStringIndex(line[at:]); m != nil {
		ps = append(ps, Piece{Start: at + m[0], End: end})
		end = at + m[0]
	}
	before := len(ps)
	ps = inline(ps, line, at, end)
	if done && end > at {
		if label, ok := plainText(line, at, end, ps[before:]); ok {
			return append(ps[:before], Piece{Start: at, End: end, Kind: Dim, Label: label})
		}
	}
	return ps
}

// plainText is line[at:end] without the marks ps hide; false when ps show
// something else there, which a label cannot hold.
func plainText(line string, at, end int, ps []Piece) (string, bool) {
	slices.SortStableFunc(ps, func(a, b Piece) int { return a.Start - b.Start })
	var b strings.Builder
	for _, p := range ps {
		if p.Kind != Hide || p.Start < at {
			return "", false
		}
		b.WriteString(line[at:p.Start])
		at = p.End
	}
	b.WriteString(line[at:end])
	return b.String(), true
}

// wikiLabel is what a wikilink's inside shows: its alias, or its target
// with " > " before a subpath.
func wikiLabel(inner string) string {
	if _, alias, ok := strings.Cut(inner, "|"); ok {
		return strings.TrimSpace(alias)
	}
	return strings.TrimSpace(strings.ReplaceAll(inner, "#", " > "))
}

// inline adds the marks of inline syntax in line[at:end].
func inline(ps []Piece, line string, at, end int) []Piece {
	var used []int // closing marks already taken
	for i := at; i < end; {
		if slices.Contains(used, i) {
			i++
			continue
		}
		c := line[i]
		switch {
		case c == '\\':
			i += 2
		case c == '`':
			n := run(line, i, end, '`')
			if j := closing(line, i+n, end, n); j >= 0 {
				ps = append(ps, Piece{Start: i, End: i + n}, Piece{Start: j, End: j + n})
				i = j + n
			} else {
				i += n
			}
		case strings.HasPrefix(line[i:end], "%%"):
			if j := strings.Index(line[i+2:end], "%%"); j >= 0 {
				ps = append(ps, Piece{Start: i, End: i + 2 + j + 2})
				i += 2 + j + 2
			} else {
				i = end
			}
		case c == '$':
			if j := mathEnd(line, i, end); j >= 0 {
				i = j + 1 // shown as written until the app draws TeX
			} else {
				i++
			}
		case strings.HasPrefix(line[i:end], "![["):
			if j := strings.Index(line[i:end], "]]"); j >= 0 {
				ps = append(ps, Piece{Start: i, End: i + j + 2, Kind: embedMark})
				i += j + 2
			} else {
				i += 3
			}
		case strings.HasPrefix(line[i:end], "[["):
			j := strings.Index(line[i:end], "]]")
			if j < 0 {
				i += 2
				break
			}
			close := i + j
			open := i + 2
			if bar := strings.IndexByte(line[i:close], '|'); bar >= 0 {
				open = i + bar + 1
			}
			if label := wikiLabel(line[i+2 : close]); label != "" && len(label) <= maxObject {
				name, _, _ := strings.Cut(line[i+2:close], "|")
				name, _, _ = strings.Cut(name, "#")
				ps = append(ps, Piece{Start: i, End: close + 2, Kind: Link, Label: label, Target: strings.TrimSpace(name)})
			} else {
				ps = append(ps, Piece{Start: i, End: open}, Piece{Start: close, End: close + 2})
			}
			i = close + 2
		case c == '!' && i+1 < end && line[i+1] == '[':
			if _, dest, ok := linkAt(line, i+1, end); ok {
				ps = append(ps, Piece{Start: i, End: dest, Kind: embedMark})
				i = dest
			} else {
				i += 2
			}
		case c == '[':
			if i+1 < end && line[i+1] == '^' {
				if j := strings.IndexByte(line[i:end], ']'); j > 0 {
					definition := strings.TrimSpace(line[:i]) == "" && i+j+1 < end && line[i+j+1] == ':'
					if !definition {
						ps = append(ps, Piece{Start: i, End: i + j + 1, Kind: Footnote, Label: line[i+2 : i+j]})
					}
					i += j + 1
					break
				}
			}
			if closeBracket, dest, ok := linkAt(line, i, end); ok {
				inner := inline(nil, line, i+1, closeBracket) // once: nested links would double the work
				if label, plain := plainText(line, i+1, closeBracket, inner); plain && label != "" && len(label) <= maxObject {
					target, _, _ := parseEmbed(line[i:dest])
					if strings.Contains(target, ":") {
						target = ""
					}
					ps = append(ps, Piece{Start: i, End: dest, Kind: Link, Label: label, Target: target})
					i = dest
					break
				}
				ps = append(ps, Piece{Start: i, End: i + 1}, Piece{Start: closeBracket, End: dest})
				used = append(used, closeBracket)
				ps = append(ps, inner...)
				i = dest
			} else {
				i++
			}
		case c == '#' && (i == 0 || line[i-1] == ' ' || line[i-1] == '\t'):
			if m := tagRe.FindString(line[i:end]); m != "" {
				ps = append(ps, Piece{Start: i, End: i + len(m), Kind: Tag, Label: m})
				i += len(m)
			} else {
				i++
			}
		case c == '*' || c == '_' || c == '~' || c == '=':
			mark := marker(line, i, end)
			if mark == "" {
				i++
				break
			}
			j := emphasisEnd(line, i, end, mark, used)
			if j < 0 {
				i += len(mark)
				break
			}
			ps = append(ps, Piece{Start: i, End: i + len(mark)}, Piece{Start: j, End: j + len(mark)})
			for k := j; k < j+len(mark); k++ {
				used = append(used, k)
			}
			i += len(mark)
		default:
			i++
		}
	}
	return ps
}

// run counts the bytes c from i.
func run(line string, i, end int, c byte) int {
	n := 0
	for i+n < end && line[i+n] == c {
		n++
	}
	return n
}

// closing finds a run of exactly n backticks from i.
func closing(line string, i, end, n int) int {
	for i < end {
		if line[i] != '`' {
			i++
			continue
		}
		m := run(line, i, end, '`')
		if m == n {
			return i
		}
		i += m
	}
	return -1
}

// mathEnd finds the '$' that closes inline math opened at i.
func mathEnd(line string, i, end int) int {
	if i+1 >= end || line[i+1] == ' ' || line[i+1] == '$' {
		return -1
	}
	for j := i + 1; j < end; j++ {
		if line[j] == '\\' {
			j++
			continue
		}
		if line[j] == '$' && line[j-1] != ' ' && (j+1 == end || line[j+1] < '0' || line[j+1] > '9') {
			return j
		}
	}
	return -1
}

// linkAt reads a Markdown link whose '[' is at i: where its ']' is and
// where it ends.
func linkAt(line string, i, end int) (closeBracket, dest int, ok bool) {
	depth := 0
	for j := i; j < end; j++ {
		switch line[j] {
		case '\\':
			j++
		case '[':
			depth++
		case ']':
			depth--
			if depth > 0 {
				continue
			}
			if j+1 >= end || line[j+1] != '(' {
				return 0, 0, false
			}
			parens := 0
			for k := j + 1; k < end; k++ {
				switch line[k] {
				case '\\':
					k++
				case '(':
					parens++
				case ')':
					parens--
					if parens == 0 {
						return j, k + 1, true
					}
				}
			}
			return 0, 0, false
		}
	}
	return 0, 0, false
}

// marker is the emphasis mark at i: "**", "__", "*", "_", "~~" or "==".
func marker(line string, i, end int) string {
	c := line[i]
	n := run(line, i, end, c)
	switch c {
	case '~', '=':
		if n == 2 {
			return line[i : i+2]
		}
		return ""
	}
	if n >= 2 {
		return line[i : i+2]
	}
	return line[i : i+1]
}

func wordByte(c byte) bool {
	return c >= '0' && c <= '9' || c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' || c >= 0x80
}

// emphasisEnd finds the mark that closes one opened at i.
func emphasisEnd(line string, i, end int, mark string, used []int) int {
	after := i + len(mark)
	if after >= end || line[after] == ' ' || line[after] == '\t' {
		return -1 // not left-flanking
	}
	if mark[0] == '_' && i > 0 && wordByte(line[i-1]) {
		return -1 // inside a word
	}
	for j := after + 1; j+len(mark) <= end; j++ {
		switch line[j] {
		case '\\':
			j++
			continue
		case '`':
			n := run(line, j, end, '`')
			if k := closing(line, j+n, end, n); k >= 0 {
				j = k + n - 1
				continue
			}
		}
		if line[j:j+len(mark)] != mark || slices.Contains(used, j) {
			continue
		}
		if line[j-1] == ' ' || line[j-1] == '\t' {
			continue // not right-flanking
		}
		if len(mark) == 1 && (j+1 < end && line[j+1] == mark[0] || line[j-1] == mark[0]) {
			continue // half of a longer run
		}
		if mark[0] == '_' && j+len(mark) < end && wordByte(line[j+len(mark)]) {
			continue
		}
		return j
	}
	return -1
}
