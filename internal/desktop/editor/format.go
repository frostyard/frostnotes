package editor

import (
	"regexp"
	"slices"
	"strconv"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/frostyard/frostnotes/internal/desktop/notes"
)

// Sel is a selection: the anchor stays, the head is the caret. Offsets
// count bytes, as MyGo's editor does.
type Sel struct{ Anchor, Head int }

// From and To are the selection's start and end.
func (s Sel) From() int { return min(s.Anchor, s.Head) }
func (s Sel) To() int   { return max(s.Anchor, s.Head) }

// Empty reports whether the selection is a caret.
func (s Sel) Empty() bool { return s.Anchor == s.Head }

func caret(pos int) Sel { return Sel{pos, pos} }

// mapSel moves a selection over cs as CodeMirror does: a caret by assoc,
// a range's start forward and its end back, so it does not grow over an
// insertion at its edges.
func mapSel(cs notes.ChangeSet, s Sel, assoc int) Sel {
	if s.Empty() {
		p := cs.MapPos(s.Head, assoc)
		return Sel{p, p}
	}
	from, to := cs.MapPos(s.From(), 1), cs.MapPos(s.To(), -1)
	if s.Anchor <= s.Head {
		return Sel{from, to}
	}
	return Sel{to, from}
}

func mapSels(cs notes.ChangeSet, sels []Sel, assoc int) []Sel {
	out := make([]Sel, len(sels))
	for i, s := range sels {
		out[i] = mapSel(cs, s, assoc)
	}
	return normalize(out)
}

// normalize sorts selections and merges those that overlap or meet at a
// caret, as CodeMirror's EditorSelection does.
func normalize(sels []Sel) []Sel {
	out := slices.Clone(sels)
	slices.SortStableFunc(out, func(a, b Sel) int { return a.From() - b.From() })
	merged := out[:0]
	for _, s := range out {
		if n := len(merged); n > 0 {
			p := merged[n-1]
			if s.From() < p.To() || s.From() == p.To() && (s.Empty() || p.Empty()) {
				from, to := min(p.From(), s.From()), max(p.To(), s.To())
				if p.Anchor <= p.Head {
					merged[n-1] = Sel{from, to}
				} else {
					merged[n-1] = Sel{to, from}
				}
				continue
			}
		}
		merged = append(merged, s)
	}
	return merged
}

// Edit is what a formatting command does: the changes, and where the
// selections are after them.
type Edit struct {
	Changes notes.ChangeSet
	Sels    []Sel
}

// line is one line of a text, without its newline.
type line struct {
	from, to int // to is before the "\n"
	number   int // from 0
	text     string
}

func lineAt(text string, pos int) line {
	pos = max(0, min(pos, len(text)))
	from := strings.LastIndexByte(text[:pos], '\n') + 1
	to := len(text)
	if i := strings.IndexByte(text[pos:], '\n'); i >= 0 {
		to = pos + i
	}
	return line{from: from, to: to, number: strings.Count(text[:from], "\n"), text: text[from:to]}
}

// touchedLines are the lines the selections touch, each once, in order.
func touchedLines(text string, sels []Sel) []line {
	seen := map[int]bool{}
	var out []line
	for _, s := range sels {
		for pos := s.From(); ; {
			l := lineAt(text, pos)
			if !seen[l.from] {
				seen[l.from] = true
				out = append(out, l)
			}
			if l.to >= s.To() || l.to >= len(text) {
				break
			}
			pos = l.to + 1
		}
	}
	slices.SortFunc(out, func(a, b line) int { return a.from - b.from })
	return out
}

// byRange runs f on each selection, last first, so that each sees the text
// as the ones after it left it; the selections f returns move with the
// changes made before them.
func byRange(text string, sels []Sel, f func(text string, s Sel) ([]notes.Change, Sel)) Edit {
	total := notes.EmptyChanges(len(text))
	order := slices.Clone(sels)
	slices.SortFunc(order, func(a, b Sel) int { return a.From() - b.From() })
	var out []Sel
	for i := len(order) - 1; i >= 0; i-- {
		changes, sel := f(text, order[i])
		cs, err := notes.ChangesOf(len(text), changes...)
		if err != nil {
			continue
		}
		next, _ := cs.Apply(text)
		total, _ = total.Compose(cs)
		for k := range out {
			out[k] = mapSel(cs, out[k], -1)
		}
		out = append(out, sel)
		text = next
	}
	slices.Reverse(out)
	return Edit{Changes: total, Sels: normalize(out)}
}

// byLines edits each touched line; selections move with the changes.
func byLines(text string, sels []Sel, edit func(l line) []notes.Change) Edit {
	var changes []notes.Change
	for _, l := range touchedLines(text, sels) {
		changes = append(changes, edit(l)...)
	}
	cs, err := notes.ChangesOf(len(text), changes...)
	if err != nil {
		return Edit{Changes: notes.EmptyChanges(len(text)), Sels: sels}
	}
	return Edit{Changes: cs, Sels: mapSels(cs, sels, 1)}
}

func slice(text string, from, to int) string {
	return text[max(0, min(from, len(text))):max(0, min(to, len(text)))]
}

var (
	starsEnd   = regexp.MustCompile(`\*{1,3}$`)
	starsStart = regexp.MustCompile(`^\*{1,3}`)
)

func starRuns(before, after string) (int, int) {
	return len(starsEnd.FindString(before)), len(starsStart.FindString(after))
}

func hasMark(before, after, mark string) bool {
	if mark == "*" || mark == "**" {
		a, b := starRuns(before, after)
		n := min(a, b)
		if mark == "**" {
			return n >= 2
		}
		return n%2 == 1
	}
	return strings.HasSuffix(before, mark) && strings.HasPrefix(after, mark)
}

func toggleSpan(text string, r Sel, mark string) ([]notes.Change, Sel) {
	from, to := r.From(), r.To()
	size := len(mark)
	before := slice(text, from-3, from)
	after := slice(text, to, to+3)
	inner := slice(text, from, to)
	start, end := from, to
	var ch []notes.Change
	switch {
	case hasMark(before, after, mark):
		ch = []notes.Change{{From: from - size, To: from}, {From: to, To: to + size}}
		start -= size
		end -= size
	case len(inner) > 2*size && hasMark(inner, inner, mark):
		trailing, leading := size, size
		if strings.HasPrefix(mark, "*") {
			trailing, leading = starRuns(inner, inner)
		}
		left := min(leading, len(inner)/2)
		right := min(trailing, len(inner)/2)
		ch = []notes.Change{{From: from + left - size, To: from + left}, {From: to - right, To: to - right + size}}
		end -= 2 * size
	case from == to:
		ch = []notes.Change{{From: from, To: from, Insert: mark + mark}}
		start += size
		end += size
	default:
		ch = []notes.Change{{From: from, To: from, Insert: mark}, {From: to, To: to, Insert: mark}}
		start += size
		end += size
	}
	if r.Anchor <= r.Head {
		return ch, Sel{start, end}
	}
	return ch, Sel{end, start}
}

// wordAt is the word around pos: letters, digits and underscores.
func wordAt(text string, pos int) (Sel, bool) {
	isWord := func(r rune) bool { return unicode.IsLetter(r) || unicode.IsDigit(r) || r == '_' }
	from, to := pos, pos
	for from > 0 {
		r, size := utf8.DecodeLastRuneInString(text[:from])
		if !isWord(r) {
			break
		}
		from -= size
	}
	for to < len(text) {
		r, size := utf8.DecodeRuneInString(text[to:])
		if !isWord(r) {
			break
		}
		to += size
	}
	return Sel{from, to}, from < to
}

// ToggleMark toggles mark (**, *, ~~, ==, `) around each selection, or
// around the word at a caret.
func ToggleMark(text string, sels []Sel, mark string) Edit {
	return byRange(text, sels, func(text string, r Sel) ([]notes.Change, Sel) {
		before := slice(text, r.From()-3, r.From())
		after := slice(text, r.To(), r.To()+3)
		if r.Empty() && !hasMark(before, after, mark) {
			if w, ok := wordAt(text, r.Head); ok {
				ch, res := toggleSpan(text, w, mark)
				return ch, caret(res.From() + r.Head - w.From())
			}
		}
		return toggleSpan(text, r, mark)
	})
}

var leadingSpace = regexp.MustCompile(`^\s*`)

// ToggleComment toggles %% around each selection, or around a caret's
// line.
func ToggleComment(text string, sels []Sel) Edit {
	return byRange(text, sels, func(text string, r Sel) ([]notes.Change, Sel) {
		if !r.Empty() {
			return toggleSpan(text, r, "%%")
		}
		l := lineAt(text, r.Head)
		from := l.from + len(leadingSpace.FindString(l.text))
		var ch []notes.Change
		var res Sel
		if slice(text, from, l.to) == "%%%%" {
			ch, res = []notes.Change{{From: from, To: l.to}}, caret(from)
		} else {
			ch, res = toggleSpan(text, Sel{from, l.to}, "%%")
		}
		if from == l.to && r.Head == from {
			return ch, caret(res.From())
		}
		cs, err := notes.ChangesOf(len(text), ch...)
		if err != nil {
			return nil, r
		}
		assoc := -1
		if r.Head <= from {
			assoc = 1
		}
		return ch, caret(cs.MapPos(r.Head, assoc))
	})
}

var urlRe = regexp.MustCompile(`(?i)^[a-z][a-z0-9+.-]*://\S+$`)

// InsertLink writes a Markdown link around each selection: its text, or
// its URL when it is one.
func InsertLink(text string, sels []Sel) Edit {
	return byRange(text, sels, func(text string, r Sel) ([]notes.Change, Sel) {
		inner := slice(text, r.From(), r.To())
		url := urlRe.MatchString(inner)
		insert := "[" + inner + "]()"
		head := r.From() + len(inner) + 3
		if url {
			insert = "[](" + inner + ")"
		}
		if r.Empty() || url {
			head = r.From() + 1
		}
		return []notes.Change{{From: r.From(), To: r.To(), Insert: insert}}, caret(head)
	})
}

var (
	itemRe = regexp.MustCompile(`^(?:[-*+]|\d+[.)])(?:[ \t]+|$)`)
	taskRe = regexp.MustCompile(`^\[(.)\](?:\s|$)`)
)

// ToggleChecklist checks or unchecks each touched line's task, makes a
// list item a task, and any other line a task item.
func ToggleChecklist(text string, sels []Sel) Edit {
	return byLines(text, sels, func(l line) []notes.Change {
		indent := leadingSpace.FindString(l.text)
		from := l.from + len(indent)
		item := itemRe.FindString(l.text[len(indent):])
		if item == "" && !itemRe.MatchString(l.text[len(indent):]) {
			return []notes.Change{{From: from, To: from, Insert: "- [ ] "}}
		}
		content := from + len(item)
		if m := taskRe.FindStringSubmatchIndex(text[content:l.to]); m != nil {
			mark := "x"
			if text[content+m[2]:content+m[3]] != " " {
				mark = " "
			}
			return []notes.Change{{From: content + m[2], To: content + m[3], Insert: mark}}
		}
		insert := " [ ] "
		if strings.HasSuffix(item, " ") || strings.HasSuffix(item, "\t") {
			insert = "[ ] "
		}
		return []notes.Change{{From: content, To: content, Insert: insert}}
	})
}

// DeleteParagraph deletes each line the selections touch.
func DeleteParagraph(text string, sels []Sel) Edit {
	var changes []notes.Change
	lastLine := strings.Count(text, "\n")
	for _, l := range touchedLines(text, sels) {
		from, to := l.from, l.to+1
		if l.number == lastLine {
			from, to = max(0, l.from-1), l.to
		}
		if n := len(changes); n > 0 && from <= changes[n-1].To {
			changes[n-1].To = to
			continue
		}
		changes = append(changes, notes.Change{From: from, To: to})
	}
	cs, err := notes.ChangesOf(len(text), changes...)
	if err != nil {
		return Edit{Changes: notes.EmptyChanges(len(text)), Sels: sels}
	}
	return Edit{Changes: cs, Sels: mapSels(cs, sels, -1)}
}

var headingRe = regexp.MustCompile(`^( {0,3})(#{1,6})( +|$)`)

// SetHeading makes each touched line a heading of level 1 to 6, or plain
// text with 0.
func SetHeading(text string, sels []Sel, level int) Edit {
	return byLines(text, sels, func(l line) []notes.Change {
		m := headingRe.FindStringSubmatch(l.text)
		if m != nil && len(m[2]) == level {
			return nil
		}
		indent := leadingSpace.FindString(l.text)
		if m != nil {
			indent = m[1]
		}
		from := l.from + len(indent)
		to := from
		if m != nil {
			to = l.from + len(m[0])
		}
		insert := ""
		if level > 0 {
			insert = strings.Repeat("#", level) + " "
		}
		return []notes.Change{{From: from, To: to, Insert: insert}}
	})
}

// toggleLines gives every touched line a prefix, or takes it off when
// every one has it: a quote's "> ", a list item's marker.
func toggleLines(text string, sels []Sel, matched func(rest string) (string, bool), insert func(i int) string) Edit {
	all := touchedLines(text, sels)
	var lines []line
	for _, l := range all {
		if len(all) == 1 || strings.TrimSpace(l.text) != "" {
			lines = append(lines, l)
		}
	}
	every := true
	for _, l := range lines {
		if _, ok := matched(l.text[len(leadingSpace.FindString(l.text)):]); !ok {
			every = false
		}
	}
	var changes []notes.Change
	for i, l := range lines {
		ind := len(leadingSpace.FindString(l.text))
		from := l.from + ind
		rest := l.text[ind:]
		has, ok := matched(rest)
		switch {
		case every:
			changes = append(changes, notes.Change{From: from, To: from + len(has)})
		case ok:
		default:
			other := bulletRe.FindString(rest)
			changes = append(changes, notes.Change{From: from, To: from + len(other), Insert: insert(i)})
		}
	}
	cs, err := notes.ChangesOf(len(text), changes...)
	if err != nil {
		return Edit{Changes: notes.EmptyChanges(len(text)), Sels: sels}
	}
	return Edit{Changes: cs, Sels: mapSels(cs, sels, 1)}
}

var (
	bulletRe   = regexp.MustCompile(`^(?:[-*+]|\d+[.)])[ \t]+`)
	quoteRe    = regexp.MustCompile(`^> ?`)
	dashRe     = regexp.MustCompile(`^[-*+][ \t]+`)
	numberedRe = regexp.MustCompile(`^\d+[.)][ \t]+`)
)

func prefixOf(re *regexp.Regexp) func(string) (string, bool) {
	return func(rest string) (string, bool) {
		m := re.FindString(rest)
		return m, m != "" || re.MatchString(rest)
	}
}

// ToggleBlockquote quotes each touched line with "> ", or unquotes them
// when all are quoted.
func ToggleBlockquote(text string, sels []Sel) Edit {
	return toggleLines(text, sels, prefixOf(quoteRe), func(int) string { return "> " })
}

// ToggleBulletList makes each touched line a "- " item, or plain when all
// are bullets.
func ToggleBulletList(text string, sels []Sel) Edit {
	return toggleLines(text, sels, prefixOf(dashRe), func(int) string { return "- " })
}

// ToggleNumberedList numbers the touched lines from 1, or makes them plain
// when all are numbered.
func ToggleNumberedList(text string, sels []Sel) Edit {
	return toggleLines(text, sels, prefixOf(numberedRe), func(i int) string { return strconv.Itoa(i+1) + ". " })
}

// InsertPair writes before and after around each selection, the caret
// after the selection: [[ ]], ![[ ]], #.
func InsertPair(text string, sels []Sel, before, after string) Edit {
	return byRange(text, sels, func(text string, r Sel) ([]notes.Change, Sel) {
		inner := slice(text, r.From(), r.To())
		return []notes.Change{{From: r.From(), To: r.To(), Insert: before + inner + after}},
			caret(r.From() + len(before) + len(inner))
	})
}

// quotesRe reads a line's quote marks.
var quotesRe = regexp.MustCompile(`^(\s*>\s?)+`)

// listRe reads a line's quote, list marker and task box.
var listRe = regexp.MustCompile(`^((?:\s*>\s?)*)(\s*)(?:([-*+])|(\d+)([.)]))([ \t]+)(\[.\][ \t]+)?`)

// inCode reports whether the line starting at from is code: inside a fence,
// or indented after a blank line with no list above it to continue.
func inCode(text string, from int) bool {
	fence := ""
	prev, lastText := "", ""
	indentedCode := false // the indented run so far is code
	isIndented := func(s string) bool { return strings.HasPrefix(s, "    ") || strings.HasPrefix(s, "\t") }
	startsCode := func() bool {
		return strings.TrimSpace(prev) == "" && !listRe.MatchString(lastText) && !isIndented(lastText)
	}
	for at := 0; at < from; {
		end := strings.IndexByte(text[at:], '\n')
		if end < 0 {
			break
		}
		content := text[at : at+end]
		t := strings.TrimLeft(content, " ")
		switch {
		case fence != "":
			if strings.HasPrefix(t, fence) && strings.Trim(strings.TrimSpace(t), fence[:1]) == "" {
				fence = ""
			}
		case len(content)-len(t) <= 3 && (strings.HasPrefix(t, "```") || strings.HasPrefix(t, "~~~")):
			fence = t[:len(t)-len(strings.TrimLeft(t, t[:1]))]
		}
		switch {
		case fence != "":
			indentedCode = false
		case strings.TrimSpace(content) == "": // a blank line does not end it
		case isIndented(content):
			indentedCode = indentedCode || startsCode()
		default:
			indentedCode = false
		}
		prev = content
		if strings.TrimSpace(content) != "" {
			lastText = content
		}
		at += end + 1
	}
	if fence != "" {
		return true
	}
	line := text[from:]
	if i := strings.IndexByte(line, '\n'); i >= 0 {
		line = line[:i]
	}
	return isIndented(line) && (indentedCode || startsCode())
}

// ContinueMarkup is Enter with smart lists: a list item, task or quote
// continues on the new line; an empty one ends its list. ok is false when
// a caret is not in one, and Enter does what it always does.
func ContinueMarkup(text string, sels []Sel) (Edit, bool) {
	for _, s := range sels {
		if !s.Empty() {
			return Edit{}, false
		}
	}
	handled := true
	e := byRange(text, sels, func(text string, r Sel) ([]notes.Change, Sel) {
		l := lineAt(text, r.Head)
		if inCode(text, l.from) {
			handled = false
			return nil, r
		}
		if m := listRe.FindStringSubmatch(l.text); m != nil {
			prefix := m[0]
			if r.Head-l.from < len(prefix) {
				handled = false
				return nil, r
			}
			if strings.TrimSpace(l.text[len(prefix):]) == "" && r.Head == l.to {
				// An empty item ends the list: the marker goes.
				return []notes.Change{{From: l.from + len(m[1]), To: l.to}}, caret(l.from + len(m[1]))
			}
			marker := m[3]
			if m[4] != "" {
				n, _ := strconv.Atoi(m[4])
				marker = strconv.Itoa(n+1) + m[5]
			}
			next := m[1] + m[2] + marker + m[6]
			if m[7] != "" {
				next += "[ ] "
			}
			ins := "\n" + next
			return []notes.Change{{From: r.Head, To: r.Head, Insert: ins}}, caret(r.Head + len(ins))
		}
		if q := quotesRe.FindString(l.text); q != "" {
			if r.Head-l.from < len(q) {
				handled = false
				return nil, r
			}
			if strings.TrimSpace(l.text[len(q):]) == "" && r.Head == l.to {
				return []notes.Change{{From: l.from, To: l.to}}, caret(l.from)
			}
			ins := "\n" + q
			return []notes.Change{{From: r.Head, To: r.Head, Insert: ins}}, caret(r.Head + len(ins))
		}
		handled = false
		return nil, r
	})
	if !handled {
		return Edit{}, false
	}
	return e, true
}

// DeleteMarkupBackward is Backspace with smart lists: at the end of a list
// item's marker or a quote's '>', it removes the marker. ok is false
// otherwise.
func DeleteMarkupBackward(text string, sels []Sel) (Edit, bool) {
	if len(sels) != 1 || !sels[0].Empty() {
		return Edit{}, false
	}
	pos := sels[0].Head
	l := lineAt(text, pos)
	m := listRe.FindStringSubmatch(l.text)
	if m == nil || pos-l.from != len(m[0]) {
		return Edit{}, false
	}
	from := l.from + len(m[1]) + len(m[2])
	cs, err := notes.ChangesOf(len(text), notes.Change{From: from, To: pos})
	if err != nil {
		return Edit{}, false
	}
	return Edit{Changes: cs, Sels: []Sel{caret(from)}}, true
}

// WrapInMark wraps each selection in a Markdown mark typed over it (*, _,
// ~, =, `), as Auto-pair Markdown syntax does; ok is false for a caret or
// another character.
func WrapInMark(text string, sels []Sel, typed string) (Edit, bool) {
	if !strings.Contains("*_~=`", typed) || len(typed) != 1 {
		return Edit{}, false
	}
	for _, s := range sels {
		if s.Empty() {
			return Edit{}, false
		}
	}
	return byRange(text, sels, func(_ string, r Sel) ([]notes.Change, Sel) {
		return []notes.Change{{From: r.From(), To: r.From(), Insert: typed}, {From: r.To(), To: r.To(), Insert: typed}},
			Sel{r.From() + 1, r.To() + 1}
	}), true
}
