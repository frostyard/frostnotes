package merge

import "strings"

// Note merges a markdown note: frontmatter by top-level key, then the body
// as Text. Each key's lines (its value, nested lines, and the comments and
// blank lines after it) are one unit, so a key only one side touched keeps
// its bytes. A key both sides changed merges as Text, and if that
// conflicts the head's value stands. A key one side deleted and the other
// changed is kept, changed, and reported as a conflict. Frontmatter that
// cannot be split into keys (a repeated key) makes the whole note merge as
// Text.
func Note(base, head, incoming []byte) Result {
	b, h, in := parseNote(string(base)), parseNote(string(head)), parseNote(string(incoming))
	if !b.ok || !h.ok || !in.ok {
		return Text(base, head, incoming)
	}
	var m merger
	units, conflicts := frontmatter(b, h, in)
	if len(units) > 0 || (h.open != "" && in.open != "") {
		open, closing := h.open, h.close
		for _, n := range []note{in, b} {
			if open == "" {
				open, closing = n.open, n.close
			}
		}
		m.out.WriteString(open)
		for _, u := range units {
			m.out.WriteString(u)
		}
		m.out.WriteString(closing)
		for _, c := range conflicts {
			c.Line++ // the opening delimiter
			m.conflicts = append(m.conflicts, c)
		}
	}
	m.text(b.body, h.body, in.body)
	return m.result()
}

// Create merges two files created at the same path with no common
// revision, using the lines they share as the base. Two notes with nothing
// in common conflict as a whole, and the head stands.
func Create(head, incoming []byte) Result {
	if string(head) == string(incoming) {
		return Result{Text: head}
	}
	hl, il := lines(string(head)), lines(string(incoming))
	var common strings.Builder
	for i, j := range lcs(hl, il) {
		if j >= 0 {
			common.WriteString(hl[i])
		}
	}
	if common.Len() == 0 {
		return Result{Text: head, Conflicts: []Conflict{{Line: 1, Head: string(head), Incoming: string(incoming)}}}
	}
	return Note([]byte(common.String()), head, incoming)
}

type note struct {
	ok          bool
	open, close string // delimiter lines with their line endings; "" without frontmatter
	keys        []string
	units       map[string]string
	body        string
}

// parseNote splits a note into frontmatter units and body. Frontmatter
// starts at the first byte with a "---" line and ends at the next "---" or
// "..." line; anything else is all body.
func parseNote(s string) note {
	n := note{ok: true, body: s, units: map[string]string{}}
	ls := lines(s)
	if len(ls) == 0 || strings.TrimRight(ls[0], "\r\n") != "---" {
		return n
	}
	end := -1
	for i := 1; i < len(ls); i++ {
		if t := strings.TrimRight(ls[i], "\r\n"); (t == "---" || t == "...") && strings.HasSuffix(ls[i], "\n") {
			end = i
			break
		}
	}
	if end < 0 {
		return n
	}
	key := "" // lines before the first key belong to the preamble unit ""
	for _, l := range ls[1:end] {
		if k, ok := topKey(l); ok {
			if _, dup := n.units[k]; dup {
				n.ok = false
				return n
			}
			key = k
			n.keys = append(n.keys, k)
		} else if len(n.keys) == 0 && key == "" && n.units[""] == "" {
			n.keys = append(n.keys, "")
		}
		n.units[key] += l
	}
	n.open, n.close = ls[0], ls[end]
	n.body = strings.Join(ls[end+1:], "")
	return n
}

// topKey reports whether l starts a top-level mapping key, and which.
func topKey(l string) (string, bool) {
	if l == "" || strings.ContainsRune(" \t#-\r\n", rune(l[0])) {
		return "", false
	}
	t := strings.TrimRight(l, "\r\n")
	i := strings.Index(t, ": ")
	if i < 0 && strings.HasSuffix(t, ":") {
		i = len(t) - 1
	}
	if i <= 0 {
		return "", false
	}
	return t[:i], true
}

// frontmatter merges the units of three notes, in the head's key order with
// the incoming side's other keys after them. It returns the merged units'
// text and conflicts whose lines count from the first unit's line.
func frontmatter(b, h, in note) (units []string, conflicts []Conflict) {
	order := append([]string(nil), h.keys...)
	for _, k := range in.keys {
		if _, inHead := h.units[k]; !inHead {
			order = append(order, k)
		}
	}

	line := 1
	for _, k := range order {
		bv, pb := b.units[k]
		hv, ph := h.units[k]
		iv, pi := in.units[k]
		var v string
		var keep bool
		switch {
		case ph && pi && hv == iv:
			v, keep = hv, true
		case (pb && ph && hv == bv) || (!pb && !ph): // the head left it alone
			v, keep = iv, pi
		case (pb && pi && iv == bv) || (!pb && !pi): // the incoming side left it alone
			v, keep = hv, ph
		case ph && pi && pb: // both changed it
			var sub merger
			sub.text(bv, hv, iv)
			v, keep = sub.out.String(), true
			for _, c := range sub.conflicts {
				c.Line += line - 1
				conflicts = append(conflicts, c)
			}
		case ph && pi: // both added it, differently
			v, keep = hv, true
			conflicts = append(conflicts, Conflict{Line: line, Head: hv, Incoming: iv})
		case ph: // the incoming side deleted it; the head changed it
			v, keep = hv, true
			conflicts = append(conflicts, Conflict{Line: line, Base: bv, Head: hv})
		default: // the head deleted it; the incoming side changed it
			v, keep = iv, true
			conflicts = append(conflicts, Conflict{Line: line, Base: bv, Incoming: iv})
		}
		if keep {
			units = append(units, v)
			line += strings.Count(v, "\n")
		}
	}
	return units, conflicts
}
