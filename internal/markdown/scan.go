package markdown

import (
	"bytes"
	"regexp"
	"slices"
	"strings"
	"unicode"
	"unicode/utf8"
)

var (
	wikiPattern = regexp.MustCompile(`!?\[\[([^\[\]\n]+)\]\]`)
	tagPattern  = regexp.MustCompile(`#[\p{L}\p{Nd}_/-]+`)
	fencePrefix = regexp.MustCompile("^ {0,3}(`{3,}|~{3,})")
	tickRuns    = regexp.MustCompile("`+")
)

// Link is a wikilink or wiki embed in a note's body.
type Link struct {
	Target  string // the note or file named; empty for a heading of the same note
	Subpath string // heading or block ID, including its leading '#'
	Alias   string // the text after the first '|'
	Embed   bool   // written ![[...]]
	Start   int    // byte offset of '[[', or of '!' for an embed
	End     int    // byte offset just after ']]'
}

// Wikilinks returns the body's wikilinks and embeds in order.
func Wikilinks(body []byte) []Link {
	return scanLinks(withoutCode(body))
}

func scanLinks(body []byte) []Link {
	var links []Link
	for _, m := range wikiPattern.FindAllSubmatchIndex(body, -1) {
		target, alias, _ := strings.Cut(string(body[m[2]:m[3]]), "|")
		target, subpath, hasSubpath := strings.Cut(target, "#")
		link := Link{
			Target: strings.TrimSpace(target), Alias: strings.TrimSpace(alias),
			Embed: body[m[0]] == '!', Start: m[0], End: m[1],
		}
		if hasSubpath {
			link.Subpath = "#" + strings.TrimSpace(subpath)
		}
		if link.Target != "" || link.Subpath != "" || link.Alias != "" {
			links = append(links, link)
		}
	}
	return links
}

// InlineTags returns the body's #tags without the '#', each once, in order.
func InlineTags(body []byte) []string {
	text := withoutCode(body)
	for _, link := range scanLinks(text) {
		hide(text[link.Start:link.End])
	}
	var tags []string
	for _, m := range tagPattern.FindAllIndex(text, -1) {
		previous, _ := utf8.DecodeLastRune(body[:m[0]])
		if m[0] != 0 && !unicode.IsSpace(previous) {
			continue
		}
		tag := string(text[m[0]+1 : m[1]])
		if strings.IndexFunc(tag, func(r rune) bool { return !unicode.IsDigit(r) }) < 0 {
			continue
		}
		if !slices.ContainsFunc(tags, func(seen string) bool { return strings.EqualFold(seen, tag) }) {
			tags = append(tags, tag)
		}
	}
	return tags
}

// withoutCode preserves offsets in a copy, replacing code with newlines so
// neither a link nor a tag can match within or across a hidden span.
func withoutCode(body []byte) []byte {
	text := bytes.Clone(body)
	var fence []byte
	offset := 0
	for line := range bytes.SplitAfterSeq(body, []byte("\n")) {
		part := text[offset : offset+len(line)]
		offset += len(line)
		prefix := fencePrefix.FindSubmatchIndex(line)
		if len(fence) > 0 {
			if prefix != nil {
				run := line[prefix[2]:prefix[3]]
				if run[0] == fence[0] && len(run) >= len(fence) &&
					len(bytes.Trim(line[prefix[1]:], " \r\n")) == 0 {
					fence = nil
				}
			}
			hide(part)
			continue
		}
		if prefix != nil {
			fence = line[prefix[2]:prefix[3]]
			hide(part)
			continue
		}
		hideInlineCode(part)
	}
	return text
}

func hideInlineCode(line []byte) {
	runs := tickRuns.FindAllIndex(line, -1)
	// Index the next run of each length, then skip pairs from left to right.
	next := make([]int, len(runs))
	last := make(map[int]int)
	for i := len(runs) - 1; i >= 0; i-- {
		length := runs[i][1] - runs[i][0]
		next[i] = last[length]
		last[length] = i + 1
	}
	for i := 0; i < len(runs); i++ {
		if j := next[i] - 1; j >= 0 {
			hide(line[runs[i][0]:runs[j][1]])
			i = j
		}
	}
}

func hide(span []byte) {
	for i := range span {
		span[i] = '\n'
	}
}
