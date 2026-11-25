package markup

import (
	"regexp"
	"strings"
)

// CompleteKind is what the text before a caret is being completed to.
type CompleteKind uint8

// The kinds of completion.
const (
	// CompleteNote is a note after [[ or ![[.
	CompleteNote CompleteKind = iota
	// CompleteHeading is a heading of a note after [[note#.
	CompleteHeading
	// CompleteBlock is a block ID of a note after [[note#^.
	CompleteBlock
	// CompleteTag is a tag after its #.
	CompleteTag
)

// Completing is what the text before a caret is being completed to: the
// MyGo client's completions (internal/desktop/editor, openCompletions).
type Completing struct {
	Kind CompleteKind
	// Target is the note a heading or a block is of, as written; "" is the
	// note being edited.
	Target string
	// Query is what is typed of the completion so far, and From where it
	// starts, a byte offset: a completion replaces From to the caret.
	Query string
	From  int
	// Closed says "]]" follows the caret, so a link's completion does not
	// add it again.
	Closed bool
}

var (
	openLinkRe = regexp.MustCompile(`!?\[\[([^\[\]|#\n]*)$`)
	openSubRe  = regexp.MustCompile(`!?\[\[([^\[\]|#\n]*)#(\^?)([^\[\]|\n]*)$`)
	openTagRe  = regexp.MustCompile(`(?:^|\s)#([\p{L}\p{N}_/-]*)$`)
)

// CompletionAt reads what the line before byte offset at is being
// completed to: a note in an open [[ link, a heading or block of one after
// its #, or a tag after its #. ok is false for anything else.
func CompletionAt(text string, at int) (c Completing, ok bool) {
	at = max(0, min(at, len(text)))
	before := text[strings.LastIndexByte(text[:at], '\n')+1 : at]
	closed := strings.HasPrefix(text[at:], "]]")
	if m := openSubRe.FindStringSubmatch(before); m != nil {
		kind := CompleteHeading
		if m[2] != "" {
			kind = CompleteBlock
		}
		return Completing{Kind: kind, Target: strings.TrimSpace(m[1]), Query: m[3], From: at - len(m[3]), Closed: closed}, true
	}
	if m := openLinkRe.FindStringSubmatch(before); m != nil {
		return Completing{Kind: CompleteNote, Query: m[1], From: at - len(m[1]), Closed: closed}, true
	}
	if m := openTagRe.FindStringSubmatch(before); m != nil {
		return Completing{Kind: CompleteTag, Query: m[1], From: at - len(m[1])}, true
	}
	return Completing{}, false
}

// FuzzyMatch reports whether every rune of query appears in s in order,
// ignoring case: how a tag's completion matches.
func FuzzyMatch(s, query string) bool {
	s, query = strings.ToLower(s), strings.ToLower(query)
	for _, r := range query {
		i := strings.IndexRune(s, r)
		if i < 0 {
			return false
		}
		s = s[i+len(string(r)):]
	}
	return true
}
