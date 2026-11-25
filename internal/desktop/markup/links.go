package markup

import (
	"net/url"
	"regexp"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/frostyard/frostnotes/internal/markdown"
)

// Link is a link in a note: to a vault file (Target, and Subpath, a
// heading or block after '#'), or to the web (URL).
type Link struct {
	Target  string // as written, without subpath or alias; "" for the note itself
	Subpath string // with its leading '#'
	URL     string // a web link's address
	Embed   bool
	From    int // where the link is in the text
	To      int
}

// Target is a note [[ completes to.
type Target struct {
	Insert string // the link text, as the vault's link format writes it
	Label  string // what the list shows
	Detail string // its folder, dimmed
}

var (
	wikiLinkRe = regexp.MustCompile(`(!?)\[\[([^\[\]\n]+)\]\]`)
	mdLinkRe   = regexp.MustCompile(`(!?)\[([^\]\n]*)\]\(\s*(<[^>\n]*>|(?:[^()\s]|\((?:[^()\s]|\([^()\s]*\))*\))*)(?:\s+"[^"\n]*")?\s*\)`)
	autoLinkRe = regexp.MustCompile(`<([a-zA-Z][a-zA-Z0-9+.-]*:[^>\s]+)>`)
	bareURLRe  = regexp.MustCompile(`\bhttps?://[^\s<>()\[\]]+[^\s<>()\[\].,;:!?'"]`)
	schemeRe   = regexp.MustCompile(`^[a-zA-Z][a-zA-Z0-9+.-]*:`)
)

// LinkAt finds the link at pos in text, on pos's line.
func LinkAt(text string, pos int) (Link, bool) {
	l := lineAt(text, pos)
	at := pos - l.from
	for _, m := range wikiLinkRe.FindAllStringSubmatchIndex(l.text, -1) {
		if m[0] <= at && at <= m[1] {
			inner := l.text[m[4]:m[5]]
			if i := strings.IndexByte(inner, '|'); i >= 0 {
				inner = inner[:i]
			}
			target, sub := inner, ""
			if i := strings.IndexByte(inner, '#'); i >= 0 {
				target, sub = inner[:i], inner[i:]
			}
			return Link{Target: strings.TrimSpace(target), Subpath: sub, Embed: m[3] > m[2], From: l.from + m[0], To: l.from + m[1]}, true
		}
	}
	for _, m := range mdLinkRe.FindAllStringSubmatchIndex(l.text, -1) {
		if m[0] <= at && at <= m[1] {
			dest := strings.TrimSuffix(strings.TrimPrefix(l.text[m[6]:m[7]], "<"), ">")
			link := Link{Embed: m[3] > m[2], From: l.from + m[0], To: l.from + m[1]}
			if schemeRe.MatchString(dest) {
				link.URL = dest
				return link, true
			}
			if dest == "" {
				return Link{}, false
			}
			if i := strings.IndexByte(dest, '#'); i >= 0 { // before unescaping: %23 is part of the name
				dest, link.Subpath = dest[:i], dest[i:]
			}
			if p, err := url.PathUnescape(dest); err == nil {
				dest = p
			}
			link.Target = dest
			return link, true
		}
	}
	for _, re := range []*regexp.Regexp{autoLinkRe, bareURLRe} {
		for _, m := range re.FindAllStringSubmatchIndex(l.text, -1) {
			if m[0] <= at && at <= m[1] {
				u := l.text[m[0]:m[1]]
				if len(m) > 2 && m[2] >= 0 {
					u = l.text[m[2]:m[3]]
				}
				return Link{URL: u, From: l.from + m[0], To: l.from + m[1]}, true
			}
		}
	}
	return Link{}, false
}

// TagAt finds the #tag at pos, without its '#'.
func TagAt(text string, pos int) (string, bool) {
	l := lineAt(text, pos)
	at := pos - l.from
	for _, r := range TagRanges(l.text) {
		if r[0] <= at && at <= r[1] {
			return l.text[r[0]+1 : r[1]], true
		}
	}
	return "", false
}

// SubpathLine is the line, from 1, a link's "#Heading", "#A#B" or "#^block"
// leads to in text, 0 when the note has none.
func SubpathLine(text, subpath string) int {
	sub := strings.TrimPrefix(subpath, "#")
	doc := markdown.Parse([]byte(text))
	if id, ok := strings.CutPrefix(sub, "^"); ok {
		for _, b := range doc.Blocks {
			if strings.EqualFold(b.ID, id) {
				return b.Line
			}
		}
		return 0
	}
	parts := strings.Split(sub, "#")
	next := 0
	for _, h := range doc.Headings {
		if strings.EqualFold(strings.TrimSpace(h.Text), strings.TrimSpace(parts[next])) {
			if next++; next == len(parts) {
				return h.Line
			}
		}
	}
	return 0
}

// TagRanges finds #tags as specs/markdown.md reads them: after the start
// or a space, letters, combining marks, digits, _, - and /, not only digits.
func TagRanges(line string) [][2]int {
	var out [][2]int
	for i := 0; i < len(line); i++ {
		if line[i] != '#' || i > 0 && !isSpaceBefore(line, i) {
			continue
		}
		j := i + 1
		digits := true
		for j < len(line) {
			r, size := utf8.DecodeRuneInString(line[j:])
			if !unicode.IsLetter(r) && !unicode.IsMark(r) && !unicode.IsDigit(r) && r != '_' && r != '-' && r != '/' {
				break
			}
			if !unicode.IsDigit(r) {
				digits = false
			}
			j += size
		}
		if j > i+1 && !digits {
			out = append(out, [2]int{i, j})
			i = j - 1
		}
	}
	return out
}

func isSpaceBefore(line string, i int) bool {
	r, _ := utf8.DecodeLastRuneInString(line[:i])
	return unicode.IsSpace(r)
}
