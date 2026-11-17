package query

import (
	"strings"
	"unicode"
	"unicode/utf8"
)

// token kinds.
const (
	tEOF = iota
	tIdent
	tString
	tNumber
	tPunct // an operator or a bracket; text is the symbol
)

type token struct {
	kind int
	text string // an identifier, a string's value, a number's text or a symbol
	col  int    // 1-based character column of its start
}

// lex splits a statement into tokens. It is hand-written and small: the
// grammar is docs/specs/bases.md#statements.
func lex(src string) ([]token, *Error) {
	var toks []token
	col := 0
	for i := 0; i < len(src); {
		r, size := utf8.DecodeRuneInString(src[i:])
		col = utf8.RuneCountInString(src[:i]) + 1
		switch {
		case r == utf8.RuneError && size == 1:
			return nil, errf("column %d: the statement is not valid UTF-8", col)
		case unicode.IsSpace(r):
			i += size
		case r == '"' || r == '\'':
			val, n, err := lexString(src[i:], r)
			if err != nil {
				return nil, errf("column %d: %s", col, err)
			}
			toks = append(toks, token{tString, val, col})
			i += n
		case r >= '0' && r <= '9':
			n := lexNumber(src[i:])
			toks = append(toks, token{tNumber, src[i : i+n], col})
			i += n
		case r == '_' || unicode.IsLetter(r):
			j := i + size
			for j < len(src) {
				r2, s2 := utf8.DecodeRuneInString(src[j:])
				if r2 != '_' && !unicode.IsLetter(r2) && !unicode.IsDigit(r2) {
					break
				}
				j += s2
			}
			toks = append(toks, token{tIdent, src[i:j], col})
			i = j
		default:
			sym := ""
			for _, two := range []string{"==", "!=", ">=", "<=", "&&", "||"} {
				if strings.HasPrefix(src[i:], two) {
					sym = two
				}
			}
			if sym == "" && strings.ContainsRune("()[].,!<>+-", r) {
				sym = string(r)
			}
			if sym == "" {
				return nil, errf("column %d: unexpected %q", col, string(r))
			}
			toks = append(toks, token{tPunct, sym, col})
			i += len(sym)
		}
	}
	return append(toks, token{tEOF, "", col + 1}), nil
}

// lexNumber returns the length of the decimal number at the start of s:
// digits, an optional fraction and an optional exponent. A '.' or an 'e'
// that no digit follows is left out of the number.
func lexNumber(s string) int {
	i := digitsLen(s, 0)
	if i+1 < len(s) && s[i] == '.' && isDigit(s[i+1]) {
		i = digitsLen(s, i+1)
	}
	if i < len(s) && (s[i] == 'e' || s[i] == 'E') {
		j := i + 1
		if j < len(s) && (s[j] == '+' || s[j] == '-') {
			j++
		}
		if j < len(s) && isDigit(s[j]) {
			i = digitsLen(s, j)
		}
	}
	return i
}

func isDigit(c byte) bool { return c >= '0' && c <= '9' }

func digitsLen(s string, from int) int {
	for from < len(s) && isDigit(s[from]) {
		from++
	}
	return from
}

// lexString reads the quoted string at the start of s, with the escapes \"
// \' \\ \n and \t. It returns the value and the bytes used.
func lexString(s string, quote rune) (string, int, error) {
	var b strings.Builder
	for i := 1; i < len(s); {
		r, size := utf8.DecodeRuneInString(s[i:])
		switch {
		case r == utf8.RuneError && size == 1:
			return "", 0, errString("the string is not valid UTF-8")
		case r == quote:
			return b.String(), i + size, nil
		case r == '\\':
			if i+1 >= len(s) {
				return "", 0, errString("the string is not closed")
			}
			switch s[i+1] {
			case '"', '\'', '\\':
				b.WriteByte(s[i+1])
			case 'n':
				b.WriteByte('\n')
			case 't':
				b.WriteByte('\t')
			default:
				return "", 0, errString("unknown escape \\" + string(rune(s[i+1])))
			}
			i += 2
		default:
			b.WriteRune(r)
			i += size
		}
	}
	return "", 0, errString("the string is not closed")
}

type errString string

func (e errString) Error() string { return string(e) }
