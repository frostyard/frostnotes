package editor

import (
	"net/url"
	"regexp"
	"strings"
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

var (
	wikiLinkRe = regexp.MustCompile(`(!?)\[\[([^\[\]\n]+)\]\]`)
	mdLinkRe   = regexp.MustCompile(`(!?)\[([^\]\n]*)\]\(\s*(<[^>\n]*>|[^)\s]*)(?:\s+"[^"\n]*")?\s*\)`)
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
			if p, err := url.PathUnescape(dest); err == nil {
				dest = p
			}
			if i := strings.IndexByte(dest, '#'); i >= 0 {
				dest, link.Subpath = dest[:i], dest[i:]
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
	for _, r := range tagRanges(l.text) {
		if r[0] <= at && at <= r[1] {
			return l.text[r[0]+1 : r[1]], true
		}
	}
	return "", false
}
