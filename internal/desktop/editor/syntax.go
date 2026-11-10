// Package editor is the desktop app's note editor on MyGo's code editor
// (plan 0009, Phase 2): source mode with Obsidian's syntax highlighted,
// folding by headings and lists, the app's own undo, Obsidian's keys,
// formatting commands, completions of links and tags, and links that
// follow. It joins a notes.Document to the MyGo buffer its tabs edit.
package editor

import (
	"regexp"
	"strings"
	"sync"
	"unicode"
	"unicode/utf8"

	code "github.com/egoist/mygo/ui/editor"
)

// The tokenizer's own states, above the bits MyGo's Markdown lexer uses
// (0 to 54).
const (
	stFront   uint64 = 1 << 55 // inside the frontmatter
	stComment uint64 = 1 << 56 // inside a %% comment
	stMath    uint64 = 1 << 57 // inside a $$ block
	stStarted uint64 = 1 << 58 // past the note's first line
	mdMask    uint64 = 1<<55 - 1
	mdFence   uint64 = 1 << 40 // MyGo's: a fenced block is open
)

// obsidian highlights Obsidian's Markdown (docs/specs/markdown.md): MyGo's
// Markdown lexer, with the frontmatter, comments, math, wikilinks and
// embeds, tags, highlights, callouts, footnotes and block IDs laid over
// its tokens.
type obsidian struct{ md code.Tokenizer }

func (o obsidian) Tokenize(line string, state uint64, tokens []code.Token) ([]code.Token, uint64) {
	first := state&stStarted == 0
	own := state | stStarted
	n := len(line)
	switch {
	case first && strings.TrimRight(line, " \t") == "---":
		return append(tokens, code.Token{End: n, Kind: code.TokenPunctuation}), stStarted | stFront
	case state&stFront != 0:
		if t := strings.TrimRight(line, " \t"); t == "---" || t == "..." {
			return append(tokens, code.Token{End: n, Kind: code.TokenPunctuation}), own &^ stFront
		}
		return frontLine(line, tokens), own
	case state&stComment != 0:
		end := strings.Index(line, "%%")
		if end < 0 {
			return append(tokens, code.Token{End: n, Kind: code.TokenComment}), own
		}
		tokens = append(tokens, code.Token{End: end + 2, Kind: code.TokenComment})
		rest, next := o.inline(line, end+2, state&mdMask, tokens)
		return rest, next | stStarted
	case state&stMath != 0:
		end := strings.Index(line, "$$")
		if end < 0 {
			return append(tokens, code.Token{End: n, Kind: code.TokenString}), own
		}
		tokens = append(tokens, code.Token{End: end + 2, Kind: code.TokenString})
		if end+2 < n {
			tokens = append(tokens, code.Token{End: n, Kind: code.TokenText})
		}
		return tokens, own &^ stMath
	}
	if state&mdFence == 0 {
		if t := strings.TrimSpace(line); strings.HasPrefix(t, "$$") && !strings.Contains(t[2:], "$$") {
			return append(tokens, code.Token{End: n, Kind: code.TokenString}), own | stMath
		}
	}
	out, next := o.inline(line, 0, state&mdMask, tokens)
	return out, next | stStarted
}

// inline lexes line from start with MyGo's lexer, then lays Obsidian's
// inline syntax over it. It returns the state after the line.
func (o obsidian) inline(line string, start int, md uint64, tokens []code.Token) ([]code.Token, uint64) {
	base := len(tokens)
	n := len(line)
	if start >= n {
		return tokens, md
	}
	// MyGo's lexer reads the rest of the line; its tokens' ends are
	// relative to that rest.
	sub, next := o.md.Tokenize(line[start:], md, nil)
	if next&mdFence != 0 && md&mdFence != 0 {
		// Inside a fenced block: its language's tokens, unchanged.
		for _, t := range sub {
			tokens = append(tokens, code.Token{End: start + t.End, Kind: t.Kind})
		}
		return tokens, next
	}
	kinds := make([]code.TokenKind, n-start)
	pos := 0
	for _, t := range sub {
		for ; pos < t.End && pos < len(kinds); pos++ {
			kinds[pos] = t.Kind
		}
	}
	comment := overlay(line[start:], kinds)
	if comment {
		next |= stComment
	}
	tokens = compress(tokens[:base], kinds, start)
	return tokens, next
}

// compress appends the runs of kinds as tokens, offset by start.
func compress(tokens []code.Token, kinds []code.TokenKind, start int) []code.Token {
	for i := 0; i < len(kinds); {
		j := i + 1
		for j < len(kinds) && kinds[j] == kinds[i] {
			j++
		}
		tokens = append(tokens, code.Token{End: start + j, Kind: kinds[i]})
		i = j
	}
	return tokens
}

var (
	wikiRe     = regexp.MustCompile(`!?\[\[[^\[\]\n]+\]\]`)
	highRe     = regexp.MustCompile(`==[^=\s](?:[^=]*[^=\s])?==`)
	mathRe     = regexp.MustCompile(`\$[^$\s](?:[^$]*[^$\s])?\$`)
	calloutRe  = regexp.MustCompile(`^\s*(?:>\s*)+(\[![^\]]+\][+-]?)`)
	footnoteRe = regexp.MustCompile(`\[\^[^\]\s]+\]`)
	blockIDRe  = regexp.MustCompile(`\s(\^[A-Za-z0-9-]+)\s*$`)
)

// overlay sets Obsidian's inline syntax in kinds, leaving code spans be;
// it reports whether a %% comment stays open at the end of the line.
func overlay(line string, kinds []code.TokenKind) (open bool) {
	free := func(s, e int) bool {
		for i := s; i < e; i++ {
			if kinds[i] == code.TokenCode || kinds[i] == code.TokenComment {
				return false
			}
		}
		return true
	}
	set := func(s, e int, k code.TokenKind) {
		for i := s; i < e; i++ {
			kinds[i] = k
		}
	}
	if m := calloutRe.FindStringSubmatchIndex(line); m != nil {
		set(m[2], m[3], code.TokenKeyword)
	}
	for _, m := range wikiRe.FindAllStringIndex(line, -1) {
		if free(m[0], m[1]) {
			set(m[0], m[1], code.TokenLink)
		}
	}
	for _, m := range footnoteRe.FindAllStringIndex(line, -1) {
		if free(m[0], m[1]) {
			set(m[0], m[1], code.TokenLink)
		}
	}
	for _, m := range highRe.FindAllStringIndex(line, -1) {
		if free(m[0], m[1]) {
			set(m[0], m[1], code.TokenInserted)
		}
	}
	for _, m := range mathRe.FindAllStringIndex(line, -1) {
		if free(m[0], m[1]) {
			set(m[0], m[1], code.TokenString)
		}
	}
	for _, t := range tagRanges(line) {
		if free(t[0], t[1]) && kinds[t[0]] != code.TokenLink {
			set(t[0], t[1], code.TokenType)
		}
	}
	if m := blockIDRe.FindStringSubmatchIndex(line); m != nil && free(m[2], m[3]) {
		set(m[2], m[3], code.TokenComment)
	}
	// %% comments last: they hide the rest.
	for i := 0; i < len(line); {
		s := strings.Index(line[i:], "%%")
		if s < 0 {
			break
		}
		s += i
		if !free(s, s+2) {
			i = s + 2
			continue
		}
		e := strings.Index(line[s+2:], "%%")
		if e < 0 {
			set(s, len(line), code.TokenComment)
			return true
		}
		e += s + 4
		set(s, e, code.TokenComment)
		i = e
	}
	return false
}

// tagRanges finds #tags as specs/markdown.md reads them: after the start
// or a space, letters, digits, _, - and /, not only digits.
func tagRanges(line string) [][2]int {
	var out [][2]int
	for i := 0; i < len(line); i++ {
		if line[i] != '#' || i > 0 && !isSpaceBefore(line, i) {
			continue
		}
		j := i + 1
		digits := true
		for j < len(line) {
			r, size := utf8.DecodeRuneInString(line[j:])
			if !unicode.IsLetter(r) && !unicode.IsDigit(r) && r != '_' && r != '-' && r != '/' {
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

// frontLine highlights a line of YAML frontmatter: a key and its value.
func frontLine(line string, tokens []code.Token) []code.Token {
	t := strings.TrimLeft(line, " ")
	indent := len(line) - len(t)
	if strings.HasPrefix(t, "- ") {
		tokens = append(tokens, code.Token{End: indent + 2, Kind: code.TokenPunctuation})
		return append(tokens, code.Token{End: len(line), Kind: code.TokenString})
	}
	if c := strings.Index(t, ":"); c > 0 {
		tokens = append(tokens, code.Token{End: indent + c, Kind: code.TokenProperty})
		tokens = append(tokens, code.Token{End: indent + c + 1, Kind: code.TokenPunctuation})
		return append(tokens, code.Token{End: len(line), Kind: code.TokenString})
	}
	return append(tokens, code.Token{End: len(line), Kind: code.TokenText})
}

// highlighter is the language's: MyGo's token highlighter with folding by
// headings and by indentation.
type highlighter struct {
	code.Highlighter
	text code.Snapshot
}

func (h *highlighter) Edit(old, new code.Snapshot, changes []code.Change) {
	h.Highlighter.Edit(old, new, changes)
	h.text = new
}

// FoldEnd folds a heading's section, up to the next heading of its level
// or above, and a line's deeper-indented lines, as lists nest.
func (h *highlighter) FoldEnd(line int) (int, bool) {
	text := h.text
	n := text.LineCount()
	if line < 0 || line >= n {
		return 0, false
	}
	if lvl := h.heading(line); lvl > 0 {
		end := line
		for i := line + 1; i < n; i++ {
			if l := h.heading(i); l > 0 && l <= lvl {
				break
			}
			end = i
		}
		for end > line && strings.TrimSpace(text.Line(end)) == "" {
			end--
		}
		return end, end > line
	}
	s := text.Line(line)
	if strings.TrimSpace(s) == "" {
		return 0, false
	}
	ind := indentOf(s)
	end := line
	for i := line + 1; i < n; i++ {
		t := text.Line(i)
		if strings.TrimSpace(t) == "" {
			continue
		}
		if indentOf(t) <= ind {
			break
		}
		end = i
	}
	return end, end > line
}

// heading returns line's ATX heading level, 0 when it is none (or a '#'
// inside a fenced block, which the tokens tell).
func (h *highlighter) heading(line int) int {
	s := h.text.Line(line)
	lvl := 0
	for lvl < len(s) && s[lvl] == '#' {
		lvl++
	}
	if lvl == 0 || lvl > 6 || lvl < len(s) && s[lvl] != ' ' && s[lvl] != '\t' {
		return 0
	}
	toks, _ := h.Tokens(line, s, nil)
	if len(toks) == 0 || toks[0].Kind != code.TokenHeading {
		return 0
	}
	return lvl
}

func indentOf(s string) int {
	n := 0
	for _, r := range s {
		switch r {
		case ' ':
			n++
		case '\t':
			n += 4
		default:
			return n
		}
	}
	return n
}

var (
	langOnce sync.Once
	lang     *code.Language
)

// Language is Obsidian's Markdown for MyGo's editor: one value, as
// editors compare languages by pointer.
func Language() *code.Language {
	langOnce.Do(func() {
		md := code.LanguageNamed("Markdown")
		l := *md
		l.Name = "Frostnotes Markdown"
		l.Extensions = nil
		l.Filenames = nil
		tok := obsidian{md: md.Tokenizer}
		l.Tokenizer = tok
		l.Highlighter = func(text code.Snapshot, _ func()) code.Highlighter {
			return &highlighter{Highlighter: code.NewTokenHighlighter(tok, text), text: text}
		}
		lang = &l
	})
	return lang
}
