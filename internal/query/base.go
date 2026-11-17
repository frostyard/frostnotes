package query

import (
	"fmt"
	"slices"
	"strconv"
	"strings"

	"go.yaml.in/yaml/v3"

	"github.com/frostyard/frostnotes/api"
	"github.com/frostyard/frostnotes/internal/store"
)

// maxLimit is the most rows a query returns.
const maxLimit = 1000

// colKind says where a column's value comes from.
type colKind int

const (
	colNone colKind = iota // a value notesd cannot show: a formula, an unknown file property, this.*
	colNote
	colFile
)

// column is a property a view shows, sorts or groups by.
type column struct {
	kind colKind
	name string // the note key, or the file property's name
}

// Plan is a compiled view: the query to run, and what the result is made
// of.
type Plan struct {
	Query   store.ViewQuery
	View    api.QueryView
	Views   []api.QueryView
	Notice  string // set when the view is outside the subset: nothing is queried
	Columns []api.QueryColumn
	Unshown []string
	Image   string // a cards view's image key as written; empty without one

	cols  []column
	group *column
	image *column
}

type viewDef struct {
	name, typ string
	n         *node // nil for the default view
}

// Compile reads a base, checks the view it names against the subset and
// compiles it. It returns an *Error for a base that is not valid YAML or
// is outside the subset.
func Compile(source []byte, o Options) (*Plan, error) {
	root, perr := parseYAML(source)
	if perr != nil {
		return nil, perr
	}
	views, err := readViews(root)
	if err != nil {
		return nil, err
	}
	p := &Plan{Views: make([]api.QueryView, len(views))}
	for i, v := range views {
		p.Views[i] = api.QueryView{Name: v.name, Type: v.typ}
	}
	idx := 0
	if o.View != nil {
		idx = -1
		for i, v := range views {
			if v.name == *o.View {
				idx = i
				break
			}
		}
		if idx < 0 {
			return nil, &Error{Location: "view", Msg: fmt.Sprintf("no view is named %q", *o.View)}
		}
	}
	v := views[idx]
	loc := "views[" + strconv.Itoa(idx) + "]"
	p.View = p.Views[idx]
	p.Columns, p.Unshown = []api.QueryColumn{}, []string{}
	switch {
	case v.typ == "":
		p.Notice = loc + ": a view with no type is outside the subset"
		return p, nil
	case v.typ != "table" && v.typ != "cards":
		p.Notice = fmt.Sprintf("%s: view type %s is outside the subset", loc, v.typ)
		return p, nil
	case !v.n.get("groupOrder").isNull():
		p.Notice = loc + ": groupOrder is outside the subset"
		return p, nil
	}

	c := newCompiler(o)
	var conds []frag
	for _, f := range []struct {
		n   *node
		loc string
	}{{root.get("filters"), "filters"}, {v.n.get("filters"), loc + ".filters"}} {
		if f.n.isNull() {
			continue
		}
		cond, err := c.filter(f.n, f.loc, 0)
		if err != nil {
			return nil, err
		}
		conds = append(conds, cond)
	}
	if len(conds) > 0 {
		w := and(conds...)
		p.Query.Where, p.Query.Args = w.sql, w.args
	}

	names, err := displayNames(root.get("properties"))
	if err != nil {
		return nil, err
	}
	if err := p.readColumns(v.n, names, loc); err != nil {
		return nil, err
	}
	order, err := p.readOrder(c, v.n, loc)
	if err != nil {
		return nil, err
	}
	p.Query.OrderBy, p.Query.OrderArgs = order.sql, order.args
	if len(p.Query.Args)+len(p.Query.OrderArgs) > maxArgs {
		return nil, &Error{Msg: fmt.Sprintf("the base needs more than %d SQL arguments", maxArgs)}
	}
	p.Query.Limit = o.Limit
	if p.Query.Limit < 1 || p.Query.Limit > maxLimit {
		p.Query.Limit = maxLimit
	}
	if l := v.n.get("limit"); !l.isNull() {
		n, err := positiveInt(l)
		if err != nil {
			return nil, &Error{Location: loc + ".limit", Msg: err.Error()}
		}
		p.Query.Limit = min(p.Query.Limit, n)
	}
	shown := slices.Clone(p.cols)
	for _, c := range []*column{p.group, p.image} {
		if c != nil {
			shown = append(shown, *c)
		}
	}
	for _, c := range shown {
		if c.kind == colFile {
			p.Query.Tags = p.Query.Tags || c.name == "tags"
			p.Query.Links = p.Query.Links || c.name == "links"
		}
	}
	return p, nil
}

func positiveInt(n *node) (int, error) {
	i, err := strconv.Atoi(n.value)
	if n.kind != yaml.ScalarNode || n.tag != "!!int" || err != nil || i < 1 {
		return 0, fmt.Errorf("the limit is a whole number of at least 1")
	}
	return i, nil
}

// readViews lists the base's views: their names and types, as written.
func readViews(root *node) ([]viewDef, *Error) {
	vs := root.get("views")
	if vs.isNull() {
		return []viewDef{{typ: "table"}}, nil
	}
	if !vs.isSequence() {
		return nil, &Error{Location: "views", Msg: "views is a list of mappings"}
	}
	if len(vs.items) == 0 {
		return []viewDef{{typ: "table"}}, nil
	}
	var out []viewDef
	for i, item := range vs.items {
		if !item.isMapping() {
			return nil, &Error{Location: "views[" + strconv.Itoa(i) + "]", Msg: "a view is a mapping"}
		}
		def := viewDef{n: item}
		if t := item.get("type"); t.isString() {
			def.typ = t.value
		}
		if t := item.get("name"); t.isString() {
			def.name = t.value
		}
		out = append(out, def)
	}
	return out, nil
}

// displayNames reads each property's displayName, by the key as the base
// writes it with a bare KEY read as note.KEY.
func displayNames(props *node) (map[string]string, *Error) {
	names := map[string]string{}
	if props.isNull() {
		return names, nil
	}
	if !props.isMapping() {
		return nil, &Error{Location: "properties", Msg: "properties is a mapping of keys"}
	}
	for _, pr := range props.pairs {
		if pr.val.isNull() {
			continue
		}
		if !pr.val.isMapping() {
			return nil, &Error{Location: "properties." + pr.key, Msg: "a property is a mapping"}
		}
		d := pr.val.get("displayName")
		if d.isNull() {
			continue
		}
		if !d.isString() {
			return nil, &Error{Location: "properties." + pr.key + ".displayName", Msg: "displayName is a string"}
		}
		names[propID(pr.key)] = d.value
	}
	return names, nil
}

// propID is a property key as properties names it: a bare KEY is note.KEY.
func propID(key string) string {
	for _, prefix := range []string{"note.", "file.", "formula.", "this."} {
		if strings.HasPrefix(key, prefix) {
			return key
		}
	}
	return "note." + key
}

// classify says where a property key's value comes from.
func classify(key string) column {
	id := propID(key)
	switch {
	case strings.HasPrefix(id, "note."):
		return column{colNote, strings.TrimPrefix(id, "note.")}
	case strings.HasPrefix(id, "file."):
		if _, ok := fileProps[strings.TrimPrefix(id, "file.")]; ok {
			return column{colFile, strings.TrimPrefix(id, "file.")}
		}
	}
	return column{kind: colNone}
}

// readColumns reads order, and a cards view's image.
func (p *Plan) readColumns(v *node, names map[string]string, loc string) *Error {
	keys := []string{"file.name"}
	if o := v.get("order"); !o.isNull() {
		if !o.isSequence() {
			return &Error{Location: loc + ".order", Msg: "order is a list of property keys"}
		}
		keys = keys[:0]
		for i, item := range o.items {
			if !item.isString() {
				return &Error{Location: fmt.Sprintf("%s.order[%d]", loc, i), Msg: "a column is a property key"}
			}
			keys = append(keys, item.value)
		}
	}
	seen := map[string]bool{}
	for _, key := range keys {
		col := classify(key)
		p.cols = append(p.cols, col)
		display, ok := names[propID(key)]
		if !ok {
			display = strings.TrimPrefix(key, "note.")
		}
		p.Columns = append(p.Columns, api.QueryColumn{Key: key, DisplayName: display})
		if col.kind == colNone && !seen[key] {
			seen[key] = true
			p.Unshown = append(p.Unshown, key)
		}
	}
	if p.View.Type == "cards" {
		if img := v.get("image"); !img.isNull() {
			if !img.isString() {
				return &Error{Location: loc + ".image", Msg: "image is a property key"}
			}
			p.Image = img.value
			col := classify(img.value)
			p.image = &col
		}
	}
	return nil
}
