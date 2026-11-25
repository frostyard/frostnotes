package markup

import (
	"strings"
	"unicode/utf16"
	"unicode/utf8"
)

// Fold is lines Start to End of a note, from 0, that fold into Start.
type Fold struct{ Start, End int }

// Folds are what folds in a note's text, by Start, as the MyGo client
// folds a note (internal/desktop/editor/syntax.go, FoldEnd): the
// frontmatter; a heading's section, to the line before the next heading
// of its level or above, blank lines at its end left out; and a line's
// run of deeper-indented lines after it, as lists nest. A # in a fence is
// no heading. 1 line starts at most 1 fold, a heading's before an
// indentation's.
func Folds(text string) []Fold {
	var out []Fold
	type open struct{ level, start, last int }
	var headings, indents []open
	lastText := -1 // the last line that is not blank
	fence := ""
	front := false
	line := 0
	closeIndents := func(below int) {
		for len(indents) > 0 && indents[len(indents)-1].level >= below {
			o := indents[len(indents)-1]
			indents = indents[:len(indents)-1]
			if o.last > o.start {
				out = append(out, Fold{o.start, o.last})
			}
		}
	}
	for rest := text; ; line++ {
		s, next, more := strings.Cut(rest, "\n")
		rest = next
		trimmed := strings.TrimRight(s, " \t")
		switch {
		case line == 0 && trimmed == "---":
			front = true
		case front:
			if trimmed == "---" || trimmed == "..." {
				front = false
				out = append(out, Fold{0, line})
			}
		case fence != "":
			if t := strings.TrimSpace(s); strings.HasPrefix(t, fence) && strings.Trim(t, fence[:1]) == "" {
				fence = ""
			}
		default:
			fence = fenceOf(s)
		}
		inFront := front || line == 0 && trimmed == "---"
		if !inFront && strings.TrimSpace(s) != "" {
			level := 0
			if fence == "" {
				level = headingLevel(s)
			}
			if level > 0 {
				closeIndents(0)
				for len(headings) > 0 && headings[len(headings)-1].level >= level {
					o := headings[len(headings)-1]
					headings = headings[:len(headings)-1]
					if lastText > o.start {
						out = append(out, Fold{o.start, lastText})
					}
				}
				headings = append(headings, open{level, line, line})
			} else {
				ind := indentOf(s)
				closeIndents(ind)
				for i := range indents {
					indents[i].last = line
				}
				indents = append(indents, open{ind, line, line})
			}
			lastText = line
		}
		if !more {
			break
		}
	}
	closeIndents(0)
	for i := len(headings) - 1; i >= 0; i-- {
		if lastText > headings[i].start {
			out = append(out, Fold{headings[i].start, lastText})
		}
	}
	// By start; a heading's fold first where 2 start on a line.
	sortFolds(out)
	return out
}

// headingLevel is the level of an ATX heading, indented at most 3
// spaces, and 0 for a line that is none.
func headingLevel(s string) int {
	i := 0
	for i < len(s) && i < 4 && s[i] == ' ' {
		i++
	}
	n := 0
	for i+n < len(s) && s[i+n] == '#' {
		n++
	}
	if i > 3 || n == 0 || n > 6 || i+n < len(s) && s[i+n] != ' ' && s[i+n] != '\t' {
		return 0
	}
	return n
}

// sortFolds orders folds by start, keeping the first of each start.
func sortFolds(fs []Fold) {
	// Insertion sort: folds come nearly in order.
	for i := 1; i < len(fs); i++ {
		for j := i; j > 0 && fs[j].Start < fs[j-1].Start; j-- {
			fs[j], fs[j-1] = fs[j-1], fs[j]
		}
	}
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

// Offset is the byte offset in text of line, from 0, and column col in
// UTF-16 units, as an editor that counts as a language server does gives a
// place; both are clamped to the text.
func Offset(text string, line, col int) int {
	at := 0
	for ; line > 0; line-- {
		i := strings.IndexByte(text[at:], '\n')
		if i < 0 {
			break
		}
		at += i + 1
	}
	for col > 0 && at < len(text) && text[at] != '\n' {
		r, size := utf8.DecodeRuneInString(text[at:])
		n := utf16.RuneLen(r)
		if n < 0 {
			n = 1
		}
		if col < n {
			break
		}
		col -= n
		at += size
	}
	return at
}

// LineCol is the line, from 0, and the column in UTF-16 units of byte
// offset off in text, clamped to it.
func LineCol(text string, off int) (line, col int) {
	off = min(max(off, 0), len(text))
	start := strings.LastIndexByte(text[:off], '\n') + 1
	line = strings.Count(text[:start], "\n")
	for _, r := range text[start:off] {
		n := utf16.RuneLen(r)
		if n < 0 {
			n = 1
		}
		col += n
	}
	return line, col
}
