package query

import (
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/frostyard/frostnotes/internal/store"
)

// This is the file a base is embedded in, or the base file itself: what
// `this` folds to (docs/specs/bases.md#this).
type This struct {
	ID    int64
	Path  string
	Size  int64
	MTime time.Time
	Tags  []string // as the index spells them, without '#'
	Links []string // each distinct link target as written, in order
}

// Options are what a compile needs besides the base.
type Options struct {
	View        *string           // the view's name; nil is the first view
	Types       map[string]string // property types by key lower-cased (vault.LoadTypes)
	This        *This             // nil when there is none
	ResolveLink func(from, target string) (string, error)
	Now         time.Time
	Loc         *time.Location // the zone of dates without one; nil is time.Local
	Limit       int            // the caller's limit, 1 to 1000
}

// frag is a piece of SQL and the arguments of its '?', in order.
type frag struct {
	sql  string
	args []any
}

func lit(sql string, args ...any) frag { return frag{sql, args} }

var (
	fTrue  = lit("1")
	fFalse = lit("0")
)

// join writes the fragments as one condition, between parentheses, with
// word between them. No fragment: empty.
func join(word string, fs []frag) frag {
	var b strings.Builder
	var args []any
	b.WriteByte('(')
	for i, f := range fs {
		if i > 0 {
			b.WriteString(" " + word + " ")
		}
		b.WriteString(f.sql)
		args = append(args, f.args...)
	}
	b.WriteByte(')')
	return frag{b.String(), args}
}

func not(f frag) frag { return frag{"NOT (" + f.sql + ")", f.args} }

func and(fs ...frag) frag {
	if len(fs) == 0 {
		return fTrue
	}
	return join("AND", fs)
}

func or(fs ...frag) frag {
	if len(fs) == 0 {
		return fFalse
	}
	return join("OR", fs)
}

// compiler holds what one Compile knows; it lives for that call.
type compiler struct {
	o     Options
	loc   *time.Location
	zone  string // the SQL function's zone argument: "" for time.Local
	now   time.Time
	alias int
}

func newCompiler(o Options) *compiler {
	c := &compiler{o: o, loc: o.Loc}
	if c.loc == nil {
		c.loc = time.Local
	}
	if c.loc != time.Local {
		c.zone = c.loc.String()
	}
	c.now = o.Now.In(c.loc).Truncate(time.Millisecond)
	return c
}

// json returns a fresh alias for a json_each.
func (c *compiler) json() string {
	c.alias++
	return "j" + strconv.Itoa(c.alias)
}

// operand is what an expression is before it is used: a condition, a
// property, a constant, or `this`.
type operand struct {
	cond *frag
	prop *prop
	cst  *constant
	this int // 1 for this, 2 for this.file
}

// prop is a property: a note's frontmatter key, or a file property.
type prop struct {
	file string // the file property's name; empty for a note property
	key  string // a note property's key as written
	typ  string // a note property's type from the vault's settings; "" when it has none
	text string // as written, for messages
}

type propKind int

const (
	pText propKind = iota
	pNum
	pDate
	pList
)

const (
	// nameSQL is file.name: files.path from after its last '/'. files.name
	// is lower-cased, so it is not used. rtrim(path, replace(path, '/', ''))
	// is the path up to its last '/'.
	nameSQL = `substr(files.path, length(rtrim(files.path, replace(files.path, '/', ''))) + 1)`
	// folderSQL is file.folder: the path before its last '/', empty at the root.
	folderSQL = `CASE WHEN instr(files.path, '/') = 0 THEN '' ELSE substr(files.path, 1, length(rtrim(files.path, replace(files.path, '/', ''))) - 1) END`
	// basenameSQL and extSQL split file.name at its last '.'.
	basenameSQL = `CASE WHEN instr(` + nameSQL + `, '.') = 0 THEN ` + nameSQL +
		` ELSE substr(` + nameSQL + `, 1, length(rtrim(` + nameSQL + `, replace(` + nameSQL + `, '.', ''))) - 1) END`
	extSQL = `CASE WHEN instr(` + nameSQL + `, '.') = 0 THEN '' ELSE substr(` + nameSQL +
		`, length(rtrim(` + nameSQL + `, replace(` + nameSQL + `, '.', ''))) + 1) END`
)

// fileProps are the file properties of the subset
// (docs/specs/bases.md#file-properties).
var fileProps = map[string]struct {
	kind propKind
	sql  string
}{
	"path":     {pText, "files.path"},
	"name":     {pText, nameSQL},
	"basename": {pText, basenameSQL},
	"ext":      {pText, extSQL},
	"folder":   {pText, folderSQL},
	"size":     {pNum, "files.size"},
	"mtime":    {pDate, "(files.mtime / 1000000)"},
	"tags":     {pList, ""},
	"links":    {pList, ""},
}

// notExactly is the message for what the subset leaves out.
func outside(what string) *Error { return errf("%s is outside the subset", what) }

// filter compiles a filter: a statement, or and, or or not of filters.
func (c *compiler) filter(n *node, loc string, depth int) (frag, *Error) {
	if depth > maxFilterDepth {
		return frag{}, &Error{Location: loc, Msg: fmt.Sprintf("filters nest more than %d levels", maxFilterDepth)}
	}
	switch {
	case n.isString():
		e, perr := parseStatement(n.value)
		if perr != nil {
			return frag{}, perr.at(loc, n.value)
		}
		f, cerr := c.cond(e)
		if cerr != nil {
			return frag{}, cerr.at(loc, n.value)
		}
		return f, nil
	case n.isMapping():
		if len(n.pairs) != 1 {
			return frag{}, &Error{Location: loc, Msg: fmt.Sprintf("a filter mapping has exactly one key (and, or or not); this one has %d", len(n.pairs))}
		}
		key, val := n.pairs[0].key, n.pairs[0].val
		if key != "and" && key != "or" && key != "not" {
			return frag{}, &Error{Location: loc, Msg: fmt.Sprintf("the filter key %q is not and, or or not", key)}
		}
		here := loc + "." + key
		if !val.isSequence() {
			return frag{}, &Error{Location: here, Msg: fmt.Sprintf("%s holds a list of filters", key)}
		}
		var fs []frag
		for i, item := range val.items {
			f, err := c.filter(item, here+"["+strconv.Itoa(i)+"]", depth+1)
			if err != nil {
				return frag{}, err
			}
			fs = append(fs, f)
		}
		switch key {
		case "and":
			return and(fs...), nil
		case "or":
			return or(fs...), nil
		}
		// not: none is true.
		if len(fs) == 0 {
			return fTrue, nil
		}
		return not(or(fs...)), nil
	}
	return frag{}, &Error{Location: loc, Msg: "a filter is a statement string, or a mapping of and, or or not"}
}

// cond compiles an expression to a condition: SQL that is 0 or 1.
func (c *compiler) cond(e expr) (frag, *Error) {
	switch e := e.(type) {
	case notExpr:
		f, err := c.cond(e.x)
		if err != nil {
			return frag{}, err
		}
		return not(f), nil
	case binExpr:
		switch e.op {
		case "&&", "||":
			l, err := c.cond(e.l)
			if err != nil {
				return frag{}, err
			}
			r, err := c.cond(e.r)
			if err != nil {
				return frag{}, err
			}
			if e.op == "&&" {
				return and(l, r), nil
			}
			return or(l, r), nil
		case "==", "!=", ">", "<", ">=", "<=":
			return c.compare(e)
		}
	}
	op, err := c.operand(e)
	if err != nil {
		return frag{}, err
	}
	return c.truthy(op)
}

// operand classifies an expression.
func (c *compiler) operand(e expr) (operand, *Error) {
	switch e := e.(type) {
	case litExpr:
		return operand{cst: &e.c}, nil
	case refExpr:
		return c.reference(e)
	case callExpr:
		return c.call(e)
	case binExpr:
		if e.op == "+" || e.op == "-" {
			return c.arithmetic(e)
		}
	}
	f, err := c.cond(e)
	if err != nil {
		return operand{}, err
	}
	return operand{cond: &f}, nil
}

func (c *compiler) reference(e refExpr) (operand, *Error) {
	switch e.scope {
	case "note":
		if e.name == "" {
			return operand{}, errf("a property needs a name")
		}
		return operand{prop: &prop{key: e.name, typ: c.o.Types[strings.ToLower(e.name)], text: e.name}}, nil
	case "file":
		if _, ok := fileProps[e.name]; !ok {
			return operand{}, outside("file." + e.name)
		}
		return operand{prop: &prop{file: e.name, text: "file." + e.name}}, nil
	case "formula":
		if e.name == "" {
			return operand{}, outside("formula")
		}
		return operand{}, outside("formula." + e.name)
	}
	return c.thisRef(e)
}

// thisRef folds this, this.file or this.file.NAME.
func (c *compiler) thisRef(e refExpr) (operand, *Error) {
	switch {
	case len(e.path) == 0:
		return c.needThis(operand{this: 1})
	case len(e.path) == 1 && e.path[0] == "file":
		return c.needThis(operand{this: 2})
	case len(e.path) == 2 && e.path[0] == "file":
	default:
		return operand{}, outside("this." + strings.Join(e.path, "."))
	}
	if _, ok := fileProps[e.path[1]]; !ok {
		return operand{}, outside("this.file." + e.path[1])
	}
	if _, err := c.needThis(operand{}); err != nil {
		return operand{}, err
	}
	t := c.o.This
	name := t.Path[strings.LastIndexByte(t.Path, '/')+1:]
	ext := ""
	if i := strings.LastIndexByte(name, '.'); i >= 0 {
		ext = name[i:] // with its '.'
	}
	var k constant
	switch e.path[1] {
	case "path":
		k = constant{k: kStr, str: t.Path}
	case "name":
		k = constant{k: kStr, str: name}
	case "basename":
		k = constant{k: kStr, str: strings.TrimSuffix(name, ext)}
	case "ext":
		k = constant{k: kStr, str: strings.TrimPrefix(ext, ".")}
	case "folder":
		if i := strings.LastIndexByte(t.Path, '/'); i >= 0 {
			k = constant{k: kStr, str: t.Path[:i]}
		} else {
			k = constant{k: kStr}
		}
	case "size":
		k = constant{k: kNum, num: float64(t.Size), numArg: t.Size}
	case "mtime":
		k = constant{k: kDate, ms: t.MTime.UnixMilli()}
	case "tags":
		k = constant{k: kList, list: t.Tags}
	case "links":
		k = constant{k: kList, list: t.Links}
	}
	return operand{cst: &k}, nil
}

func b2i(b bool) int {
	if b {
		return 1
	}
	return 0
}

func (c *compiler) needThis(op operand) (operand, *Error) {
	if c.o.This == nil {
		return operand{}, errf("this is not set: the base has no embedding note (give from, or read the base from a path)")
	}
	return op, nil
}

// arithmetic folds a date plus or minus a duration string.
func (c *compiler) arithmetic(e binExpr) (operand, *Error) {
	l, err := c.operand(e.l)
	if err != nil {
		return operand{}, err
	}
	r, err := c.operand(e.r)
	if err != nil {
		return operand{}, err
	}
	if l.cst == nil || l.cst.k != kDate || r.cst == nil || r.cst.k != kStr {
		return operand{}, errf("column %d: arithmetic is outside the subset, except a date plus or minus a duration string", e.col)
	}
	t, derr := addDuration(time.UnixMilli(l.cst.ms).In(c.loc), r.cst.str, e.op == "-")
	if derr != nil {
		return operand{}, errf("column %d: %v", e.col, derr)
	}
	return operand{cst: &constant{k: kDate, ms: t.UnixMilli()}}, nil
}

// call compiles a function or a method.
func (c *compiler) call(e callExpr) (operand, *Error) {
	if e.recv != nil {
		recv, err := c.operand(e.recv)
		if err != nil {
			return operand{}, err
		}
		if recv.prop == nil {
			return operand{}, errf("column %d: .%s(…) applies to a property, not to a constant or a condition", e.col, e.name)
		}
		f, err := c.method(*recv.prop, e)
		if err != nil {
			return operand{}, err
		}
		return operand{cond: &f}, nil
	}
	if e.ns == "file" {
		f, err := c.fileFunc(e)
		if err != nil {
			return operand{}, err
		}
		return operand{cond: &f}, nil
	}
	switch e.name {
	case "now", "today":
		if len(e.args) != 0 {
			return operand{}, errf("column %d: %s() takes no argument", e.col, e.name)
		}
		if e.name == "now" {
			return operand{cst: &constant{k: kDate, ms: c.now.UnixMilli()}}, nil
		}
		y, m, d := c.now.Date()
		return operand{cst: &constant{k: kDate, ms: time.Date(y, m, d, 0, 0, 0, 0, c.loc).UnixMilli()}}, nil
	case "date":
		if len(e.args) != 1 {
			return operand{}, errf("column %d: date() takes 1 string", e.col)
		}
		a, err := c.operand(e.args[0])
		if err != nil {
			return operand{}, err
		}
		if a.cst == nil || a.cst.k != kStr {
			return operand{}, errf("column %d: date() takes a string such as \"2026-11-16\"", e.col)
		}
		ms, ok := store.ParseISOTime(a.cst.str, c.loc)
		if !ok {
			return operand{}, errf("column %d: date(%q) is not an ISO 8601 date (YYYY-MM-DD, with an optional time and zone)", e.col, a.cst.str)
		}
		return operand{cst: &constant{k: kDate, ms: ms}}, nil
	}
	return operand{}, outside("the function " + e.name)
}

// truthy is a bare reference or literal used as a condition.
func (c *compiler) truthy(op operand) (frag, *Error) {
	switch {
	case op.cond != nil:
		return *op.cond, nil
	case op.this != 0:
		return frag{}, errf("this is not a condition: use file.hasLink(this) or this.file.NAME")
	case op.cst != nil:
		k := op.cst
		switch k.k {
		case kNum:
			return boolFrag(k.num != 0), nil
		case kStr:
			return boolFrag(k.str != ""), nil
		case kBool:
			return boolFrag(k.b), nil
		case kList:
			return boolFrag(len(k.list) > 0), nil
		}
		return frag{}, errf("a date is not a condition: compare it")
	}
	p := op.prop
	if p.file == "" {
		j := c.json()
		return lit(`EXISTS (SELECT 1 FROM json_each(notes.props) `+j+` WHERE `+j+`.key = ? AND (`+j+`.type = 'true'
			OR (`+j+`.type IN ('integer', 'real') AND `+j+`.value != 0)
			OR (`+j+`.type = 'text' AND `+j+`.value != '')
			OR (`+j+`.type = 'array' AND EXISTS (SELECT 1 FROM json_each(`+arrayOf(j)+`)))))`, p.key), nil
	}
	fp := fileProps[p.file]
	switch {
	case p.file == "tags":
		return lit(`EXISTS (SELECT 1 FROM tags WHERE tags.file_id = files.id)`), nil
	case p.file == "links":
		return lit(`EXISTS (SELECT 1 FROM links WHERE links.file_id = files.id AND links.target != '')`), nil
	case fp.kind == pText:
		return lit(`(` + fp.sql + ` != '')`), nil
	case fp.kind == pNum:
		return lit(`(` + fp.sql + ` != 0)`), nil
	}
	return frag{}, errf("%s is a date, not a condition: compare it", p.text)
}

func boolFrag(b bool) frag {
	if b {
		return fTrue
	}
	return fFalse
}

// arrayOf is the JSON of j's value when it is an array, else an empty
// array, so that json_each never reads a value that is not JSON.
func arrayOf(j string) string {
	return `CASE WHEN ` + j + `.type = 'array' THEN ` + j + `.value ELSE '[]' END`
}

// compare compiles a comparison.
func (c *compiler) compare(e binExpr) (frag, *Error) {
	l, err := c.operand(e.l)
	if err != nil {
		return frag{}, err
	}
	r, err := c.operand(e.r)
	if err != nil {
		return frag{}, err
	}
	switch {
	case l.this != 0 || r.this != 0:
		return frag{}, errf("column %d: this cannot be compared: use file.hasLink(this) or this.file.NAME", e.col)
	case l.cst != nil && r.cst != nil:
		return foldCompare(*l.cst, e.op, *r.cst, e.col)
	case l.prop != nil && r.cst != nil:
		return c.propCompare(*l.prop, e.op, *r.cst, e.col)
	case l.cst != nil && r.prop != nil:
		return c.propCompare(*r.prop, mirror(e.op), *l.cst, e.col)
	case l.prop != nil && r.prop != nil:
		return frag{}, errf("column %d: a comparison of two properties is outside the subset", e.col)
	case l.cond != nil && r.cst != nil && r.cst.k == kBool:
		return boolCompare(*l.cond, e.op, r.cst.b, e.col)
	case r.cond != nil && l.cst != nil && l.cst.k == kBool:
		return boolCompare(*r.cond, e.op, l.cst.b, e.col)
	}
	return frag{}, errf("column %d: this comparison is outside the subset: a property or a condition against a constant", e.col)
}

func mirror(op string) string {
	switch op {
	case "<":
		return ">"
	case ">":
		return "<"
	case "<=":
		return ">="
	case ">=":
		return "<="
	}
	return op
}

func boolCompare(f frag, op string, b bool, col int) (frag, *Error) {
	if op != "==" && op != "!=" {
		return frag{}, errf("column %d: a boolean compares only with == and !=", col)
	}
	if (op == "==") == b {
		return f, nil
	}
	return not(f), nil
}

// foldCompare compares two constants.
func foldCompare(l constant, op string, r constant, col int) (frag, *Error) {
	if l.k == kList || r.k == kList {
		return frag{}, errf("column %d: a list cannot be compared: use contains, containsAny or isEmpty", col)
	}
	ordering := op != "==" && op != "!="
	if (l.k == kBool || r.k == kBool) && ordering {
		return frag{}, errf("column %d: a boolean compares only with == and !=", col)
	}
	if l.k != r.k {
		// A value that does not convert: == and the orders are false.
		return boolFrag(op == "!="), nil
	}
	var cmp int
	switch l.k {
	case kNum:
		cmp = compareOrdered(l.num, r.num)
	case kStr:
		cmp = compareOrdered(l.str, r.str)
	case kDate:
		cmp = compareOrdered(l.ms, r.ms)
	case kBool:
		cmp = compareOrdered(b2i(l.b), b2i(r.b))
	}
	return boolFrag(cmpHolds(cmp, op)), nil
}

func compareOrdered[T int | int64 | float64 | string](a, b T) int {
	switch {
	case a < b:
		return -1
	case a > b:
		return 1
	}
	return 0
}

func cmpHolds(cmp int, op string) bool {
	switch op {
	case "==":
		return cmp == 0
	case "!=":
		return cmp != 0
	case "<":
		return cmp < 0
	case ">":
		return cmp > 0
	case "<=":
		return cmp <= 0
	}
	return cmp >= 0
}

// sqlOp is a comparison operator in SQL; != is compiled as NOT (==).
func sqlOp(op string) string {
	if op == "==" || op == "!=" {
		return "="
	}
	return op
}

// dateOf reads a constant as a date: a date, or a string in an ISO 8601
// form, as a date-typed property or file.mtime takes it.
func (c *compiler) dateOf(k constant, what string) (int64, *Error) {
	switch k.k {
	case kDate:
		return k.ms, nil
	case kStr:
		if ms, ok := store.ParseISOTime(k.str, c.loc); ok {
			return ms, nil
		}
		return 0, errf("%s is a date: %q is not an ISO 8601 date", what, k.str)
	}
	return 0, errf("%s is a date: it cannot compare with %s", what, k.k)
}

func (c *compiler) propCompare(p prop, op string, k constant, col int) (frag, *Error) {
	if k.k == kList {
		return frag{}, errf("column %d: a list cannot be compared: use contains, containsAny or isEmpty", col)
	}
	if p.file != "" {
		return c.fileCompare(p, op, k, col)
	}
	if isListType(p.typ) {
		return frag{}, errf("column %d: %s has the type %s: a list cannot be compared: use contains, containsAny or isEmpty", col, p.text, p.typ)
	}
	// What the comparison compares as: the property's type, else the constant.
	ck := k.k
	switch p.typ {
	case "number":
		ck = kNum
	case "checkbox":
		ck = kBool
	case "text":
		ck = kStr
	case "date", "datetime":
		ck = kDate
	}
	if ck == kDate && k.k == kStr && p.typ != "" {
		ms, err := c.dateOf(k, p.text+" ("+p.typ+")")
		if err != nil {
			return frag{}, err
		}
		k = constant{k: kDate, ms: ms}
	}
	if k.k != ck {
		return frag{}, errf("column %d: %s has the type %s: it cannot compare with %s", col, p.text, p.typ, k.k)
	}
	if ck == kBool && op != "==" && op != "!=" {
		return frag{}, errf("column %d: a boolean compares only with == and !=", col)
	}
	j := c.json()
	var pred frag
	switch ck {
	case kNum:
		pred = lit(j+`.type IN ('integer', 'real') AND `+j+`.value `+sqlOp(op)+` ?`, k.numArg)
	case kStr:
		pred = lit(j+`.type = 'text' AND `+j+`.value `+sqlOp(op)+` ?`, k.str)
	case kBool:
		pred = lit(j+`.type = ?`, strconv.FormatBool(k.b))
	case kDate:
		pred = lit(j+`.type = 'text' AND `+timeCall(j+".value")+` `+sqlOp(op)+` ?`, c.zone, k.ms)
	}
	f := frag{`EXISTS (SELECT 1 FROM json_each(notes.props) ` + j + ` WHERE ` + j + `.key = ? AND ` + pred.sql + `)`,
		append([]any{p.key}, pred.args...)}
	if op == "!=" {
		return not(f), nil
	}
	return f, nil
}

// timeCall reads the text expr as a date; its arguments are the zone.
func timeCall(expr string) string { return "frostnotes_time(" + expr + ", ?)" }

func (c *compiler) fileCompare(p prop, op string, k constant, col int) (frag, *Error) {
	fp := fileProps[p.file]
	var arg any
	switch fp.kind {
	case pText:
		if k.k != kStr {
			return frag{}, errf("column %d: %s is text: it cannot compare with %s", col, p.text, k.k)
		}
		arg = k.str
	case pNum:
		if k.k != kNum {
			return frag{}, errf("column %d: %s is a number: it cannot compare with %s", col, p.text, k.k)
		}
		arg = k.numArg
	case pDate:
		ms, err := c.dateOf(k, p.text)
		if err != nil {
			return frag{}, err
		}
		arg = ms
	default:
		return frag{}, errf("column %d: %s is a list: use contains, containsAny or isEmpty", col, p.text)
	}
	sqlop := sqlOp(op)
	if op == "!=" {
		sqlop = "!="
	}
	return lit(`COALESCE(`+fp.sql+` `+sqlop+` ?, 0)`, arg), nil
}

// textArgs reads a method's arguments as constants. A list constant
// (this.file.tags) stands for its items. Each item is a string or a number.
func (c *compiler) constArgs(e callExpr) ([]constant, *Error) {
	var out []constant
	for _, a := range e.args {
		op, err := c.operand(a)
		if err != nil {
			return nil, err
		}
		if op.cst == nil {
			return nil, errf("column %d: the arguments of %s are strings, numbers or this.file.tags; a property or a condition is outside the subset", e.col, e.name)
		}
		switch op.cst.k {
		case kStr, kNum:
			out = append(out, *op.cst)
		case kList:
			for _, item := range op.cst.list {
				out = append(out, constant{k: kStr, str: item})
			}
		default:
			return nil, errf("column %d: the arguments of %s are strings or numbers, not %s", e.col, e.name, op.cst.k)
		}
	}
	return out, nil
}

// oneString reads the single string argument of a method.
func (c *compiler) oneString(e callExpr) (string, *Error) {
	name := e.name
	if e.ns != "" {
		name = e.ns + "." + name
	}
	if len(e.args) != 1 {
		return "", errf("column %d: %s takes 1 string", e.col, name)
	}
	args, err := c.constArgs(e)
	if err != nil {
		return "", err
	}
	if len(args) != 1 || args[0].k != kStr {
		return "", errf("column %d: %s takes 1 string", e.col, name)
	}
	return args[0].str, nil
}

// fileFunc compiles file.hasTag, file.inFolder, file.hasLink and
// file.hasProperty.
func (c *compiler) fileFunc(e callExpr) (frag, *Error) {
	switch e.name {
	case "hasTag":
		if len(e.args) == 0 {
			return frag{}, errf("column %d: file.hasTag takes at least 1 tag", e.col)
		}
		args, err := c.constArgs(e)
		if err != nil {
			return frag{}, err
		}
		var alts []frag
		for _, a := range args {
			if a.k != kStr {
				return frag{}, errf("column %d: file.hasTag takes strings", e.col)
			}
			tag := strings.ToLower(strings.TrimPrefix(a.str, "#"))
			if tag == "" {
				return frag{}, errf("column %d: file.hasTag takes a tag, not an empty string", e.col)
			}
			alts = append(alts, lit(`(tags.folded = ? OR (tags.folded >= ? AND tags.folded < ?))`, tag, tag+"/", tag+"0"))
		}
		if len(alts) == 0 {
			return fFalse, nil
		}
		w := or(alts...)
		return lit(`EXISTS (SELECT 1 FROM tags WHERE tags.file_id = files.id AND `+w.sql+`)`, w.args...), nil
	case "inFolder":
		folder, err := c.oneString(e)
		if err != nil {
			return frag{}, err
		}
		folder = strings.Trim(folder, "/")
		if folder == "" {
			return fTrue, nil
		}
		return lit(`(files.path >= ? AND files.path < ?)`, folder+"/", folder+"0"), nil
	case "hasProperty":
		key, err := c.oneString(e)
		if err != nil {
			return frag{}, err
		}
		return lit(`EXISTS (SELECT 1 FROM json_each(notes.props) WHERE key = ?)`, key), nil
	case "hasLink":
		return c.hasLink(e)
	}
	return frag{}, outside("file." + e.name + "(…)")
}

func (c *compiler) hasLink(e callExpr) (frag, *Error) {
	if len(e.args) != 1 {
		return frag{}, errf("column %d: file.hasLink takes 1 argument: a string, this or this.file", e.col)
	}
	a, err := c.operand(e.args[0])
	if err != nil {
		return frag{}, err
	}
	switch {
	case a.this != 0:
		return lit(`EXISTS (SELECT 1 FROM links WHERE links.file_id = files.id AND links.dest_id = ?)`, c.o.This.ID), nil
	case a.cst != nil && a.cst.k == kStr:
	default:
		return frag{}, errf("column %d: file.hasLink takes a string, this or this.file", e.col)
	}
	target := a.cst.str
	from := ""
	if c.o.This != nil {
		from = c.o.This.Path
	}
	dest := ""
	if c.o.ResolveLink != nil {
		var rerr error
		if dest, rerr = c.o.ResolveLink(from, target); rerr != nil {
			return frag{}, errf("column %d: resolve %q: %v", e.col, target, rerr)
		}
	}
	if dest != "" {
		return lit(`EXISTS (SELECT 1 FROM links WHERE links.file_id = files.id
			AND links.dest_id = (SELECT f.id FROM files f WHERE f.vault_id = files.vault_id AND f.path = ?))`, dest), nil
	}
	name := store.LinkName(target)
	return lit(`EXISTS (SELECT 1 FROM links WHERE links.file_id = files.id AND links.dest_id IS NULL AND links.name = ?)`, name), nil
}
