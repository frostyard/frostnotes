package query

import (
	"bytes"
	"encoding/json/jsontext"
	"encoding/json/v2"
	"io"
	"math/big"
	"path"
	"strconv"
	"strings"

	"github.com/frostyard/frostnotes/api"
	"github.com/frostyard/frostnotes/internal/store"
)

var jsonNull = jsontext.Value("null")

// Result makes the QueryResult of the rows the plan's query returned. For a
// notice, nothing was queried: rows is nil and total 0.
func (p *Plan) Result(rows []store.QueryRow, total int) *api.QueryResult {
	res := &api.QueryResult{
		View: p.View, Views: p.Views, Columns: p.Columns, Rows: []api.QueryRow{},
		Total: int64(total), Unshown: p.Unshown,
	}
	if p.Notice != "" {
		res.Notice = &p.Notice
		return res
	}
	if p.Image != "" {
		res.Image = &p.Image
	}
	if p.group != nil {
		res.Groups = []api.QueryGroup{}
	}
	var last jsontext.Value
	for _, r := range rows {
		props := decodeProps(r)
		out := api.QueryRow{Path: r.Path, Kind: api.FileKind(r.Kind), Title: r.Title,
			Values: make([]jsontext.Value, len(p.cols))}
		if r.Kind != store.FileNote {
			out.Title = baseName(r.Path, true)
		}
		for i, c := range p.cols {
			out.Values[i] = c.value(r, props)
		}
		if p.image != nil {
			out.Image = p.image.value(r, props)
		}
		res.Rows = append(res.Rows, out)
		if p.group != nil {
			g := p.group.value(r, props)
			if n := len(res.Groups); n > 0 && sameJSON(last, g) {
				res.Groups[n-1].Count++
			} else {
				res.Groups = append(res.Groups, api.QueryGroup{Value: g, Count: 1})
				last = g
			}
		}
	}
	return res
}

// decodeProps reads a note's frontmatter; an attachment has none.
func decodeProps(r store.QueryRow) map[string]jsontext.Value {
	if r.Kind != store.FileNote {
		return nil
	}
	var props map[string]jsontext.Value
	if err := json.Unmarshal(r.Props, &props); err != nil {
		return nil
	}
	return props
}

// baseName is the last segment of p; with trimExt, without its last
// extension.
func baseName(p string, trimExt bool) string {
	name := path.Base(p)
	if trimExt {
		name = strings.TrimSuffix(name, path.Ext(name))
	}
	return name
}

// value is the column's JSON value for a row: null when it is missing.
func (c column) value(r store.QueryRow, props map[string]jsontext.Value) jsontext.Value {
	switch c.kind {
	case colNote:
		if v, ok := props[c.name]; ok && len(v) > 0 {
			return v
		}
	case colFile:
		var v any
		switch c.name {
		case "path":
			v = r.Path
		case "name":
			v = path.Base(r.Path)
		case "basename":
			v = baseName(r.Path, true)
		case "ext":
			v = strings.TrimPrefix(path.Ext(r.Path), ".")
		case "folder":
			v = strings.TrimSuffix(strings.TrimSuffix(r.Path, path.Base(r.Path)), "/")
		case "size":
			v = r.Size
		case "mtime":
			v = r.MTime.UTC().Format("2006-01-02T15:04:05.000Z07:00")
		case "tags":
			v = nonNil(r.Tags)
		case "links":
			v = nonNil(r.Links)
		}
		if out, err := json.Marshal(v); err == nil {
			return out
		}
	}
	return jsonNull
}

func nonNil(s []string) []string {
	if s == nil {
		return []string{}
	}
	return s
}

// sameJSON compares two JSON values as values: 1 and 1.0 are the same,
// and null is null. Numbers compare exactly (as rationals), never as
// float64, so 2 distinct large integers stay 2 values.
func sameJSON(a, b jsontext.Value) bool {
	return canonicalJSON(a) == canonicalJSON(b)
}

// canonicalJSON writes v as text that is equal for equal values.
func canonicalJSON(v jsontext.Value) string {
	dec := jsontext.NewDecoder(bytes.NewReader(v))
	var b strings.Builder
	for {
		tok, err := dec.ReadToken()
		if err != nil {
			if err != io.EOF {
				return string(v) // not JSON: compare the text
			}
			return b.String()
		}
		switch tok.Kind() {
		case '0':
			text := tok.String()
			if r, ok := new(big.Rat).SetString(text); ok {
				text = r.RatString()
			}
			b.WriteString("n" + text + ";")
		case '"':
			b.WriteString(strconv.Quote(tok.String()) + ";")
		default:
			b.WriteString(tok.String() + ";") // null, true, false, [ ] { }
		}
	}
}
