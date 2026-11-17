package query

import (
	"fmt"
	"math"
	"strconv"
)

// Limits that keep a hostile statement small.
const (
	maxStatement = 4096 // bytes
	maxDepth     = 64   // nested parentheses, '!' and arguments
)

// expr is a parsed statement or part of one.
type expr any

type (
	// litExpr is a string, number or boolean literal.
	litExpr struct{ c constant }
	// refExpr is a reference: scope "note" (note.KEY, note["KEY"] or a bare
	// KEY, name KEY), "file" (file.NAME), "formula" or "this" (path holds
	// what follows this, such as ["file", "name"]).
	refExpr struct {
		scope string
		name  string
		path  []string
		col   int
	}
	// callExpr is a function (recv nil; ns "file" for file.NAME(…), else "")
	// or a method of recv.
	callExpr struct {
		recv expr
		ns   string
		name string
		args []expr
		col  int
	}
	notExpr struct{ x expr }
	// binExpr is && || == != < <= > >= + or -.
	binExpr struct {
		op   string
		l, r expr
		col  int
	}
)

type parser struct {
	toks  []token
	pos   int
	depth int
}

// parseStatement parses one statement (docs/specs/bases.md#statements).
func parseStatement(src string) (expr, *Error) {
	if len(src) > maxStatement {
		return nil, errf("the statement is %d bytes; the most is %d", len(src), maxStatement)
	}
	toks, err := lex(src)
	if err != nil {
		return nil, err
	}
	p := &parser{toks: toks}
	e, err := p.or()
	if err != nil {
		return nil, err
	}
	if t := p.peek(); t.kind != tEOF {
		return nil, p.unexpected(t)
	}
	return e, nil
}

func (p *parser) peek() token { return p.toks[p.pos] }

// peekAt looks n tokens ahead; past the end it is the end token.
func (p *parser) peekAt(n int) token {
	return p.toks[min(p.pos+n, len(p.toks)-1)]
}

func (p *parser) next() token {
	t := p.toks[p.pos]
	if t.kind != tEOF {
		p.pos++
	}
	return t
}

func (p *parser) isPunct(s string) bool {
	t := p.peek()
	return t.kind == tPunct && t.text == s
}

func (p *parser) accept(s string) bool {
	if p.isPunct(s) {
		p.pos++
		return true
	}
	return false
}

func (p *parser) expect(s string) *Error {
	if !p.accept(s) {
		return p.unexpected(p.peek(), s)
	}
	return nil
}

func (p *parser) unexpected(t token, want ...string) *Error {
	what := fmt.Sprintf("%q", t.text)
	switch t.kind {
	case tEOF:
		what = "the end of the statement"
	case tString:
		what = "a string"
	case tNumber:
		what = "the number " + t.text
	}
	msg := fmt.Sprintf("column %d: unexpected %s", t.col, what)
	if len(want) > 0 {
		msg += fmt.Sprintf(", expected %q", want[0])
	}
	return errf("%s", msg)
}

func (p *parser) enter() *Error {
	if p.depth++; p.depth > maxDepth {
		return errf("the statement nests more than %d levels", maxDepth)
	}
	return nil
}

func (p *parser) or() (expr, *Error) {
	l, err := p.and()
	for err == nil && p.isPunct("||") {
		col := p.next().col
		var r expr
		if r, err = p.and(); err == nil {
			l = binExpr{"||", l, r, col}
		}
	}
	return l, err
}

func (p *parser) and() (expr, *Error) {
	l, err := p.unary()
	for err == nil && p.isPunct("&&") {
		col := p.next().col
		var r expr
		if r, err = p.unary(); err == nil {
			l = binExpr{"&&", l, r, col}
		}
	}
	return l, err
}

func (p *parser) unary() (expr, *Error) {
	if p.isPunct("!") {
		p.next()
		if err := p.enter(); err != nil {
			return nil, err
		}
		defer func() { p.depth-- }()
		x, err := p.unary()
		if err != nil {
			return nil, err
		}
		return notExpr{x}, nil
	}
	return p.compare()
}

func (p *parser) compare() (expr, *Error) {
	l, err := p.sum()
	if err != nil {
		return nil, err
	}
	if t := p.peek(); t.kind == tPunct {
		switch t.text {
		case "==", "!=", ">", "<", ">=", "<=":
			p.next()
			r, err := p.sum()
			if err != nil {
				return nil, err
			}
			return binExpr{t.text, l, r, t.col}, nil
		}
	}
	return l, nil
}

func (p *parser) sum() (expr, *Error) {
	l, err := p.postfix()
	for err == nil && (p.isPunct("+") || p.isPunct("-")) {
		t := p.next()
		var r expr
		if r, err = p.postfix(); err == nil {
			l = binExpr{t.text, l, r, t.col}
		}
	}
	return l, err
}

func (p *parser) postfix() (expr, *Error) {
	e, err := p.primary()
	for err == nil && p.isPunct(".") {
		p.next()
		name := p.next()
		if name.kind != tIdent {
			return nil, p.unexpected(name, "a method name")
		}
		if !p.isPunct("(") {
			return nil, errf("column %d: %s is not a method call; a property of a property is outside the subset", name.col, name.text)
		}
		var args []expr
		if args, err = p.args(); err != nil {
			return nil, err
		}
		e = callExpr{recv: e, name: name.text, args: args, col: name.col}
	}
	return e, err
}

// args reads "(" args? ")".
func (p *parser) args() ([]expr, *Error) {
	if err := p.expect("("); err != nil {
		return nil, err
	}
	if err := p.enter(); err != nil {
		return nil, err
	}
	defer func() { p.depth-- }()
	var args []expr
	if p.accept(")") {
		return args, nil
	}
	for {
		a, err := p.or()
		if err != nil {
			return nil, err
		}
		args = append(args, a)
		if p.accept(")") {
			return args, nil
		}
		if err := p.expect(","); err != nil {
			return nil, err
		}
	}
}

func (p *parser) primary() (expr, *Error) {
	t := p.next()
	switch t.kind {
	case tString:
		return litExpr{constant{k: kStr, str: t.text}}, nil
	case tNumber:
		return p.number(t, "")
	case tPunct:
		switch t.text {
		case "(":
			if err := p.enter(); err != nil {
				return nil, err
			}
			defer func() { p.depth-- }()
			e, err := p.or()
			if err != nil {
				return nil, err
			}
			return e, p.expect(")")
		case "-", "+":
			if n := p.peek(); n.kind == tNumber {
				p.next()
				return p.number(n, t.text)
			}
		}
	case tIdent:
		return p.identifier(t)
	}
	return nil, p.unexpected(t)
}

func (p *parser) number(t token, sign string) (expr, *Error) {
	text := t.text
	if sign == "-" {
		text = "-" + text
	}
	f, err := strconv.ParseFloat(text, 64)
	if err != nil || math.IsInf(f, 0) {
		return nil, errf("column %d: the number %s is out of range", t.col, text)
	}
	c := constant{k: kNum, num: f, numArg: f}
	if i, err := strconv.ParseInt(text, 10, 64); err == nil {
		c.numArg = i
	}
	return litExpr{c}, nil
}

func (p *parser) identifier(t token) (expr, *Error) {
	switch t.text {
	case "true":
		return litExpr{constant{k: kBool, b: true}}, nil
	case "false":
		return litExpr{constant{k: kBool}}, nil
	case "null":
		return nil, errf("column %d: null is outside the subset: test a missing value with isEmpty() or file.hasProperty()", t.col)
	case "date", "today", "now":
		if !p.isPunct("(") {
			return nil, errf("column %d: %s is a function: write %s(…)", t.col, t.text, t.text)
		}
		args, err := p.args()
		return callExpr{name: t.text, args: args, col: t.col}, err
	case "note":
		if p.accept("[") {
			s := p.next()
			if s.kind != tString {
				return nil, p.unexpected(s, "a string")
			}
			return refExpr{scope: "note", name: s.text, col: t.col}, p.expect("]")
		}
		name, err := p.dotIdent(t)
		if err != nil {
			return nil, err
		}
		if p.isPunct("(") {
			return nil, errf("column %d: note.%s(…): a function is outside the subset", t.col, name)
		}
		return refExpr{scope: "note", name: name, col: t.col}, nil
	case "file":
		name, err := p.dotIdent(t)
		if err != nil {
			return nil, err
		}
		if p.isPunct("(") {
			args, err := p.args()
			return callExpr{ns: "file", name: name, args: args, col: t.col}, err
		}
		return refExpr{scope: "file", name: name, col: t.col}, nil
	case "formula":
		ref := refExpr{scope: "formula", col: t.col}
		if p.isPunct(".") && p.peekAt(1).kind == tIdent {
			p.next()
			ref.name = p.next().text
		}
		return ref, nil
	case "this":
		ref := refExpr{scope: "this", col: t.col}
		// this.file.NAME: take up to 2 names, but leave a method's name.
		for len(ref.path) < 2 && p.isPunct(".") && p.peekAt(1).kind == tIdent && (p.peekAt(2).kind != tPunct || p.peekAt(2).text != "(") {
			p.next()
			ref.path = append(ref.path, p.next().text)
		}
		return ref, nil
	}
	if p.isPunct("(") {
		return nil, errf("column %d: the function %s is outside the subset", t.col, t.text)
	}
	return refExpr{scope: "note", name: t.text, col: t.col}, nil
}

// dotIdent reads ".NAME" after the word at t.
func (p *parser) dotIdent(t token) (string, *Error) {
	if err := p.expect("."); err != nil {
		return "", errf("column %d: %s must be followed by .NAME", t.col, t.text)
	}
	n := p.next()
	if n.kind != tIdent {
		return "", p.unexpected(n, "a name")
	}
	return n.text, nil
}
