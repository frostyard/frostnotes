// Package livepreview works out what live preview shows
// (docs/design/app.md#live-preview, ADR-0039): which ranges of a note's
// source hide, and what stands in for the rest, on the lines no selection
// touches. It is plain Go over the text: the editor turns its pieces into
// MyGo's replacements.
package livepreview

import (
	"regexp"
	"slices"
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
)

// Piece shows bytes Start to End of the text as its Kind.
type Piece struct {
	Start, End int
	Kind       Kind
	// Done is a Task's state.
	Done bool
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

// Pieces lists what shows of text, sorted and apart. revealed reports
// whether a selection touches a line, from 0: such a line shows its
// source. With hideFront the frontmatter hides, the properties panel
// standing in for it.
func (s *Scanner) Pieces(text string, revealed func(line int) bool, hideFront bool) []Piece {
	if s.cache == nil {
		s.cache = map[lineKey][]Piece{}
	}
	var out []Piece
	st := state{closedFront: closedFront(text)}
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
		was := st.block
		st.advance(content, line, start, next, min(next, len(text)), revealed, hideFront, &out)
		if was == blockText && st.block == blockText && !st.structural {
			out = s.line(out, content, start, st, line, revealed)
		}
		st.afterGap = strings.TrimSpace(content) == ""
		st.afterText = was == blockText && !st.afterGap && !st.structural
		line, start = line+1, next
		lines++
	}
	if len(s.cache) > maxCache*lines+64 {
		clear(s.cache)
	}
	slices.SortStableFunc(out, func(a, b Piece) int { return a.Start - b.Start })
	return apart(out)
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
		ps = linePieces(content, st.afterText, st.afterGap)
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
	block      block
	fence      string // the marks that close a fence
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
func (st *state) advance(content string, line, start, next, limit int, revealed func(int) bool, hideFront bool, out *[]Piece) {
	st.structural = false
	trim := strings.TrimRight(content, " \t")
	switch st.block {
	case blockFront:
		st.structural = true
		if trim == "---" || trim == "..." {
			st.block = blockText
			if hideFront && !spanRevealed(revealed, st.fromLine, line) {
				*out = append(*out, Piece{Start: st.from, End: limit})
			}
		}
		return
	case blockFence:
		st.structural = true
		if t := strings.TrimSpace(content); strings.HasPrefix(t, st.fence) && strings.Trim(t, st.fence[:1]) == "" {
			st.block = blockText
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
				*out = append(*out, Piece{Start: st.from, End: end})
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
	case strings.HasPrefix(t, "$$") && !strings.Contains(t[2:], "$$"):
		st.block, st.structural = blockMath, true
	case strings.Count(content, "%%")%2 == 1 && strings.HasPrefix(t, "%%"):
		st.block, st.from, st.fromLine, st.structural = blockComment, start, line, true
	}
}

var (
	headingRe = regexp.MustCompile(`^ {0,3}#{1,6}(?:[ \t]+|$)`)
	quoteRe   = regexp.MustCompile(`^ {0,3}(?:> ?)+`)
	taskRe    = regexp.MustCompile(`^[ \t]*([-*+]|\d+[.)])[ \t]+(\[(.)\])(?:[ \t]|$)`)
	bulletRe  = regexp.MustCompile(`^[ \t]*([-*+])[ \t]+`)
	ruleRe    = regexp.MustCompile(`^ {0,3}(?:(?:-[ \t]*){3,}|(?:\*[ \t]*){3,}|(?:_[ \t]*){3,})$`)
	blockIDRe = regexp.MustCompile(`[ \t]\^[A-Za-z0-9-]+[ \t]*$`)
)

// linePieces are the pieces of a line of text, from its start.
func linePieces(line string, afterText, afterGap bool) []Piece {
	if afterGap && (strings.HasPrefix(line, "    ") || strings.HasPrefix(line, "\t")) && !taskRe.MatchString(line) && !bulletRe.MatchString(line) {
		return nil // indented code
	}
	var ps []Piece
	at := 0
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
	return inline(ps, line, at, end)
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
				i += j + 2 // an embed: a block, shown as written for now
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
			ps = append(ps, Piece{Start: i, End: open}, Piece{Start: close, End: close + 2})
			i = close + 2
		case c == '!' && i+1 < end && line[i+1] == '[':
			if _, dest, ok := linkAt(line, i+1, end); ok {
				i = dest // an image: a block, shown as written for now
			} else {
				i += 2
			}
		case c == '[':
			if i+1 < end && line[i+1] == '^' {
				if j := strings.IndexByte(line[i:end], ']'); j > 0 {
					ps = append(ps, Piece{Start: i + 1, End: i + 2})
					i += j + 1
					break
				}
			}
			if closeBracket, dest, ok := linkAt(line, i, end); ok {
				ps = append(ps, Piece{Start: i, End: i + 1}, Piece{Start: closeBracket, End: dest})
				used = append(used, closeBracket)
				i++
				end2 := closeBracket
				ps = inline(ps, line, i, end2)
				i = dest
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
