package markup

import (
	"regexp"
	"strings"
)

// Kind is what a run of a note's source is, for an editor's colors: the
// syntax of Obsidian's Markdown (docs/specs/markdown.md) that source mode
// shows, as the MyGo client's overlay on MyGo's Markdown lexer
// (internal/desktop/editor/syntax.go) shows it.
type Kind uint8

// The kinds of run. KindNone is plain text.
const (
	KindNone Kind = iota
	// KindFrontmatter is a line of --- or ... around the frontmatter;
	// KindProperty a key in it and KindValue what follows the key.
	KindFrontmatter
	KindProperty
	KindValue
	// KindHeading is an ATX heading's line.
	KindHeading
	// KindEmphasis, KindStrong and KindStrike are the text between their marks, and
	// KindMarks those marks: *, _, **, __ and ~~.
	KindEmphasis
	KindStrong
	KindStrike
	KindMarks
	// KindCode is a code span, and a fenced or indented code block's line.
	KindCode
	// KindLink is a Markdown link or image's text and KindURL its destination, an
	// autolink or a bare web address.
	KindLink
	KindURL
	// KindWikilink is [[Note]], KindEmbed ![[Note]] and an image ![alt](url).
	KindWikilink
	KindEmbed
	// KindTag is a #tag.
	KindTag
	// KindHighlight is ==text==, with its marks.
	KindHighlight
	// KindComment is a %% comment, which may span lines.
	KindComment
	// KindCallout is a callout's [!type] and its fold sign.
	KindCallout
	// KindFootnote is a reference [^1] or a definition's label.
	KindFootnote
	// KindMath is $inline$ math and a $$ block's lines.
	KindMath
	// KindBlockID is a ^block-id at a line's end.
	KindBlockID
	// KindQuote is a blockquote's > marks, KindList a list's marker and its task
	// box, and KindRule a thematic break.
	KindQuote
	KindList
	KindRule
)

// Token is bytes Start to End of a line, of a Kind. A line's tokens are
// in order and apart; a byte of no token is plain.
type Token struct {
	Start, End int
	Kind       Kind
}

// scan is what is open where a line starts: the frontmatter, a fence, a
// math block, a comment, and what decides whether an indented line is
// code.
type scan struct {
	started  bool   // past the note's first line
	front    bool   // inside the frontmatter
	fence    string // an open fence's marks, "" for none
	math     bool   // inside a $$ block
	comment  bool   // inside a %% comment
	code     bool   // the indented run so far is code
	prev     string // the line before
	lastText string // the last line that is not blank
}

// Highlight returns the tokens of lines from to to (exclusive) of lines, a
// note's whole text split at its line breaks: what is open at line from
// is read from the lines before it. Lines outside lines are left out.
func Highlight(lines []string, from, to int) [][]Token {
	from, to = max(from, 0), min(to, len(lines))
	if from >= to {
		return nil
	}
	var st scan
	for i := range from {
		st.line(lines[i], nil)
	}
	out := make([][]Token, to-from)
	for i := from; i < to; i++ {
		var kinds []Kind
		if n := len(lines[i]); n > 0 {
			kinds = make([]Kind, n)
		}
		st.line(lines[i], kinds)
		out[i-from] = runs(kinds)
	}
	return out
}

// runs is kinds' runs of a kind other than KindNone.
func runs(kinds []Kind) []Token {
	var out []Token
	for i := 0; i < len(kinds); {
		j := i + 1
		for j < len(kinds) && kinds[j] == kinds[i] {
			j++
		}
		if kinds[i] != KindNone {
			out = append(out, Token{Start: i, End: j, Kind: kinds[i]})
		}
		i = j
	}
	return out
}

func fill(kinds []Kind, s, e int, k Kind) {
	for i := s; i < e && i < len(kinds); i++ {
		kinds[i] = k
	}
}

var (
	hlHeadingRe = regexp.MustCompile(`^ {0,3}#{1,6}(?:[ \t]|$)`)
	ruleRe      = regexp.MustCompile(`^ {0,3}(?:(?:-[ \t]*){3,}|(?:\*[ \t]*){3,}|(?:_[ \t]*){3,})$`)
	hlQuoteRe   = regexp.MustCompile(`^(?:[ \t]*>[ \t]?)+`)
	markerRe    = regexp.MustCompile(`^[ \t]*(?:[-*+]|\d{1,9}[.)])(?:[ \t]+\[.\])?(?:[ \t]+|$)`)
	calloutRe   = regexp.MustCompile(`^\[![^\]]+\][+-]?`)
	footDefRe   = regexp.MustCompile(`^\[\^[^\]\s]+\]:`)
	wikiRe      = regexp.MustCompile(`!?\[\[[^\[\]\n]+\]\]`)
	hlLinkRe    = regexp.MustCompile(`(!?)\[([^\[\]]*)\](\([^()\s]*(?:\s+"[^"]*")?\))`)
	autoRe      = regexp.MustCompile(`<[a-zA-Z][a-zA-Z0-9+.-]{1,31}:[^<>\s]*>|https?://[^\s<>()\[\]]*[^\s<>()\[\].,;:!?'"]`)
	footRefRe   = regexp.MustCompile(`\[\^[^\]\s]+\]`)
	highRe      = regexp.MustCompile(`==[^=\s](?:[^=]*[^=\s])?==`)
	strikeRe    = regexp.MustCompile(`~~[^~\s](?:[^~]*[^~\s])?~~`)
	strongRe    = regexp.MustCompile(`\*\*[^*\s](?:[^*]*[^*\s])?\*\*|__[^_\s](?:[^_]*[^_\s])?__`)
	emRe        = regexp.MustCompile(`\*[^*\s](?:[^*]*[^*\s])?\*|_[^_\s](?:[^_]*[^_\s])?_`)
	mathSpanRe  = regexp.MustCompile(`\$\$[^$]+\$\$|\$[^$\s](?:[^$]*[^$\s])?\$`)
	blockIDRe   = regexp.MustCompile(`\s(\^[A-Za-z0-9-]+)\s*$`)
)

func isIndented(s string) bool { return strings.HasPrefix(s, "    ") || strings.HasPrefix(s, "\t") }

// fenceOf is the fence a line opens, "" for none: 3 or more backticks or
// tildes, indented at most 3 spaces.
func fenceOf(line string) string {
	t := strings.TrimLeft(line, " ")
	if len(line)-len(t) > 3 || !strings.HasPrefix(t, "```") && !strings.HasPrefix(t, "~~~") {
		return ""
	}
	return t[:len(t)-len(strings.TrimLeft(t, t[:1]))]
}

// line reads one line, and with kinds (as long as the line) sets the kind
// of each of its bytes.
func (st *scan) line(line string, kinds []Kind) {
	first := !st.started
	st.started = true
	defer func() {
		st.prev = line
		if strings.TrimSpace(line) != "" {
			st.lastText = line
		}
	}()
	trimmed := strings.TrimRight(line, " \t")
	switch {
	case first && trimmed == "---":
		st.front = true
		fill(kinds, 0, len(line), KindFrontmatter)
		return
	case st.front:
		if trimmed == "---" || trimmed == "..." {
			st.front = false
			fill(kinds, 0, len(line), KindFrontmatter)
			return
		}
		frontLine(line, kinds)
		return
	case st.fence != "":
		if t := strings.TrimSpace(line); strings.HasPrefix(t, st.fence) && strings.Trim(t, st.fence[:1]) == "" {
			st.fence = ""
		}
		fill(kinds, 0, len(line), KindCode)
		return
	case st.math:
		end := strings.Index(line, "$$")
		if end < 0 {
			fill(kinds, 0, len(line), KindMath)
			return
		}
		st.math = false
		fill(kinds, 0, end+2, KindMath)
		return
	case st.comment:
		end := strings.Index(line, "%%")
		if end < 0 {
			fill(kinds, 0, len(line), KindComment)
			return
		}
		st.comment = false
		fill(kinds, 0, end+2, KindComment)
		if kinds != nil {
			st.comment = inline(line, end+2, kinds)
		} else if strings.Contains(line[end+2:], "%%") {
			st.comment = openComment(line[end+2:])
		}
		return
	}
	if f := fenceOf(line); f != "" {
		st.fence = f
		st.code = false
		fill(kinds, 0, len(line), KindCode)
		return
	}
	switch {
	case strings.TrimSpace(line) == "":
		return // a blank line does not end indented code
	case isIndented(line):
		if !st.code {
			st.code = strings.TrimSpace(st.prev) == "" && !markerRe.MatchString(strings.TrimLeft(st.lastText, "> \t")) && !isIndented(st.lastText)
		}
		if st.code {
			fill(kinds, 0, len(line), KindCode)
			return
		}
	default:
		st.code = false
	}
	if t := strings.TrimSpace(line); strings.HasPrefix(t, "$$") && !strings.Contains(t[2:], "$$") {
		st.math = true
		fill(kinds, 0, len(line), KindMath)
		return
	}
	if kinds == nil {
		// Only a comment that stays open matters past this line.
		if strings.Contains(line, "%%") {
			st.comment = openComment(line)
		}
		return
	}
	st.comment = inline(line, 0, kinds)
}

// openComment reports whether a %% comment stays open at the end of line,
// %% inside a code span aside.
func openComment(line string) bool {
	kinds := make([]Kind, len(line))
	codeSpans(line, 0, kinds)
	open := false
	for i := 0; i+1 < len(line); i++ {
		if line[i] == '%' && line[i+1] == '%' && kinds[i] != KindCode {
			open = !open
			i++
		}
	}
	return open
}

// frontLine sets a frontmatter line's kinds: a key and its value, or a
// list item.
func frontLine(line string, kinds []Kind) {
	t := strings.TrimLeft(line, " ")
	indent := len(line) - len(t)
	if strings.HasPrefix(t, "- ") || t == "-" {
		fill(kinds, indent, indent+1, KindList)
		fill(kinds, indent+1, len(line), KindValue)
		return
	}
	if c := strings.Index(t, ":"); c > 0 {
		fill(kinds, indent, indent+c, KindProperty)
		fill(kinds, indent+c+1, len(line), KindValue)
		return
	}
	fill(kinds, indent, len(line), KindValue)
}

// inline sets the kinds of line from start, a line outside the
// frontmatter, code and math blocks; it reports whether a %% comment stays
// open at its end.
func inline(line string, start int, kinds []Kind) bool {
	at := start
	heading := false
	// What starts a line: a heading, a rule, quote marks with a callout,
	// a list's marker and task box, and a footnote's definition.
	if start == 0 {
		if ruleRe.MatchString(line) {
			fill(kinds, 0, len(line), KindRule)
			return false
		}
		if m := hlQuoteRe.FindStringIndex(line); m != nil {
			fill(kinds, 0, m[1], KindQuote)
			at = m[1]
			if c := calloutRe.FindStringIndex(line[at:]); c != nil {
				fill(kinds, at, at+c[1], KindCallout)
				at += c[1]
			}
		}
		if hlHeadingRe.MatchString(line[at:]) {
			heading = true
			fill(kinds, at, len(line), KindHeading)
		} else if m := markerRe.FindStringIndex(line[at:]); m != nil {
			fill(kinds, at, at+m[1], KindList)
			at += m[1]
		}
		if m := footDefRe.FindStringIndex(line[at:]); m != nil {
			fill(kinds, at, at+m[1]-1, KindFootnote)
		}
	}
	// What hides the syntax inside it comes first: code spans, math,
	// links and addresses.
	codeSpans(line, at, kinds)
	free := func(s, e int) bool {
		for i := s; i < e; i++ {
			switch kinds[i] {
			case KindCode, KindMath, KindWikilink, KindEmbed, KindLink, KindURL, KindComment:
				return false
			}
		}
		return true
	}
	each := func(re *regexp.Regexp, need string, f func(m []int)) {
		if need != "" && !strings.Contains(line[at:], need) {
			return
		}
		for _, m := range re.FindAllStringSubmatchIndex(line[at:], -1) {
			for i := range m {
				if m[i] >= 0 {
					m[i] += at
				}
			}
			if free(m[0], m[1]) {
				f(m)
			}
		}
	}
	each(mathSpanRe, "$", func(m []int) { fill(kinds, m[0], m[1], KindMath) })
	each(wikiRe, "[[", func(m []int) {
		k := KindWikilink
		if line[m[0]] == '!' {
			k = KindEmbed
		}
		fill(kinds, m[0], m[1], k)
	})
	each(hlLinkRe, "](", func(m []int) {
		if m[3] > m[2] {
			fill(kinds, m[0], m[1], KindEmbed) // an image
			return
		}
		fill(kinds, m[0], m[6], KindLink)
		fill(kinds, m[6], m[7], KindURL)
	})
	each(autoRe, ":", func(m []int) { fill(kinds, m[0], m[1], KindURL) })
	each(footRefRe, "[^", func(m []int) { fill(kinds, m[0], m[1], KindFootnote) })
	// Marks over text: the text keeps the kind its line gives it under a
	// heading, so only the marks show there.
	marked := func(m []int, n int, k Kind) {
		fill(kinds, m[0], m[0]+n, KindMarks)
		fill(kinds, m[1]-n, m[1], KindMarks)
		if !heading {
			fill(kinds, m[0]+n, m[1]-n, k)
		}
	}
	each(highRe, "==", func(m []int) { fill(kinds, m[0], m[1], KindHighlight) })
	each(strikeRe, "~~", func(m []int) { marked(m, 2, KindStrike) })
	each(strongRe, "", func(m []int) {
		if line[m[0]] == '_' && !wordEdge(line, m[0], m[1]) {
			return
		}
		marked(m, 2, KindStrong)
	})
	each(emRe, "", func(m []int) {
		if kinds[m[0]] == KindMarks || kinds[m[0]] == KindStrong {
			return // part of a strong run
		}
		if line[m[0]] == '_' && !wordEdge(line, m[0], m[1]) {
			return
		}
		marked(m, 1, KindEmphasis)
	})
	if strings.Contains(line[at:], "#") {
		for _, t := range TagRanges(line[at:]) {
			s, e := at+t[0], at+t[1]
			if free(s, e) && kinds[s] != KindFootnote {
				fill(kinds, s, e, KindTag)
			}
		}
	}
	if strings.Contains(line[at:], "^") {
		if m := blockIDRe.FindStringSubmatchIndex(line[at:]); m != nil && free(at+m[2], at+m[3]) {
			fill(kinds, at+m[2], at+m[3], KindBlockID)
		}
	}
	// %% comments last: they hide what is inside them.
	for i := at; i+1 < len(line); {
		s := strings.Index(line[i:], "%%")
		if s < 0 {
			break
		}
		s += i
		if kinds[s] == KindCode {
			i = s + 2
			continue
		}
		e := strings.Index(line[s+2:], "%%")
		if e < 0 {
			fill(kinds, s, len(line), KindComment)
			return true
		}
		e += s + 4
		fill(kinds, s, e, KindComment)
		i = e
	}
	return false
}

// wordEdge reports whether line[s:e], a run between _ marks, starts and
// ends at a word's edge: _ inside a word marks nothing.
func wordEdge(line string, s, e int) bool {
	word := func(b byte) bool {
		return b >= '0' && b <= '9' || b >= 'a' && b <= 'z' || b >= 'A' && b <= 'Z' || b >= 0x80
	}
	return (s == 0 || !word(line[s-1])) && (e == len(line) || !word(line[e]))
}

// codeSpans sets the kinds of line's code spans from at: a run of
// backticks closed by a run of the same length.
func codeSpans(line string, at int, kinds []Kind) {
	if !strings.Contains(line[at:], "`") {
		return
	}
	for i := at; i < len(line); {
		if line[i] != '`' {
			i++
			continue
		}
		n := 0
		for i+n < len(line) && line[i+n] == '`' {
			n++
		}
		closed := -1
		for j := i + n; j < len(line); {
			if line[j] != '`' {
				j++
				continue
			}
			m := 0
			for j+m < len(line) && line[j+m] == '`' {
				m++
			}
			if m == n {
				closed = j + m
				break
			}
			j += m
		}
		if closed < 0 {
			i += n
			continue
		}
		fill(kinds, i, closed, KindCode)
		i = closed
	}
}
