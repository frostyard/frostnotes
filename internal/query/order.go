package query

import (
	"fmt"
	"strconv"
	"strings"
)

// sortKey is a property to order by.
type sortKey struct {
	col  column
	desc bool
}

// readOrder reads groupBy and sort, and compiles them to ORDER BY terms:
// the group key, then each sort key, then file.path ascending
// (docs/specs/bases.md#columns-sort-groups-and-limit).
func (p *Plan) readOrder(c *compiler, v *node, loc string) (frag, *Error) {
	var keys []sortKey
	if g := v.get("groupBy"); !g.isNull() {
		k, err := readSortKey(g, loc+".groupBy")
		if err != nil {
			return frag{}, err
		}
		keys = append(keys, k)
		p.group = &k.col
	}
	if s := v.get("sort"); !s.isNull() {
		if !s.isSequence() {
			return frag{}, &Error{Location: loc + ".sort", Msg: "sort is a list of mappings"}
		}
		for i, item := range s.items {
			k, err := readSortKey(item, loc+".sort["+strconv.Itoa(i)+"]")
			if err != nil {
				return frag{}, err
			}
			keys = append(keys, k)
		}
	}
	var terms []frag
	for _, k := range keys {
		terms = append(terms, k.terms()...)
	}
	terms = append(terms, lit("files.path ASC"))
	var b strings.Builder
	var args []any
	for i, t := range terms {
		if i > 0 {
			b.WriteString(", ")
		}
		b.WriteString(t.sql)
		args = append(args, t.args...)
	}
	return frag{b.String(), args}, nil
}

// readSortKey reads {property, direction}.
func readSortKey(n *node, loc string) (sortKey, *Error) {
	if !n.isMapping() {
		return sortKey{}, &Error{Location: loc, Msg: "a sort key is a mapping of property and direction"}
	}
	prop := n.get("property")
	if !prop.isString() {
		return sortKey{}, &Error{Location: loc + ".property", Msg: "property is a property key"}
	}
	k := sortKey{col: classify(prop.value)}
	switch {
	case k.col.kind == colNone:
		return sortKey{}, &Error{Location: loc, Expr: prop.value, Msg: fmt.Sprintf("%s is outside the subset: a sort or group key is a note property or file.path, name, basename, ext, folder, size or mtime", prop.value)}
	case k.col.kind == colFile && (k.col.name == "tags" || k.col.name == "links"):
		return sortKey{}, &Error{Location: loc, Expr: prop.value, Msg: prop.value + " is a list: it cannot order rows"}
	case k.col.kind == colNote && k.col.name == "":
		return sortKey{}, &Error{Location: loc, Expr: prop.value, Msg: "the property has no name"}
	}
	if d := n.get("direction"); !d.isNull() {
		switch {
		case !d.isString():
			return sortKey{}, &Error{Location: loc + ".direction", Msg: "direction is ASC or DESC"}
		case strings.EqualFold(d.value, "DESC"):
			k.desc = true
		case !strings.EqualFold(d.value, "ASC"):
			return sortKey{}, &Error{Location: loc + ".direction", Expr: d.value, Msg: "direction is ASC or DESC"}
		}
	}
	return k, nil
}

// terms are the ORDER BY terms of one key. A missing or null value sorts
// last in either direction; values of different JSON types order as
// numbers, strings, booleans, then lists and objects.
func (k sortKey) terms() []frag {
	dir := " ASC"
	if k.desc {
		dir = " DESC"
	}
	if k.col.kind == colFile {
		return []frag{lit(fileOrderSQL(k.col.name) + dir)}
	}
	from := ` FROM json_each(notes.props) j WHERE j.key = ?`
	key := k.col.name
	return []frag{
		lit(`NOT EXISTS (SELECT 1`+from+` AND j.type != 'null') ASC`, key),
		// A null has no rank and no value, as a missing key has none.
		lit(`(SELECT CASE j.type WHEN 'null' THEN NULL WHEN 'integer' THEN 0 WHEN 'real' THEN 0 WHEN 'text' THEN 1
			WHEN 'true' THEN 2 WHEN 'false' THEN 2 ELSE 3 END`+from+`)`+dir, key),
		// The JSON text of a list or object orders equal values side by side.
		lit(`(SELECT j.value`+from+`)`+dir, key),
	}
}

func fileOrderSQL(name string) string {
	if name == "mtime" {
		return "files.mtime"
	}
	return fileProps[name].sql
}
