package query

// method compiles x.NAME(args) on a property
// (docs/specs/bases.md#functions).
func (c *compiler) method(p prop, e callExpr) (frag, *Error) {
	switch e.name {
	case "contains", "startsWith", "endsWith", "containsAny", "containsAll", "isEmpty":
	default:
		return frag{}, outside("the method ." + e.name + "(…)")
	}
	if e.name == "isEmpty" && len(e.args) != 0 {
		return frag{}, errf("column %d: isEmpty() takes no argument", e.col)
	}
	if p.file != "" {
		return c.fileMethod(p, e)
	}
	return c.noteMethod(p, e)
}

// isListType is true for the property types that hold lists.
func isListType(t string) bool { return t == "multitext" || t == "tags" || t == "aliases" }

func (c *compiler) noteMethod(p prop, e callExpr) (frag, *Error) {
	j := c.json()
	each := `EXISTS (SELECT 1 FROM json_each(notes.props) ` + j + ` WHERE ` + j + `.key = ? AND (`
	wrap := func(branches ...frag) frag {
		if len(branches) == 0 {
			return fFalse
		}
		w := or(branches...)
		return frag{each + w.sql + `))`, append([]any{p.key}, w.args...)}
	}
	textOK := !isListType(p.typ) // a list-typed property is not read as text
	listOK := p.typ != "text"    // a text-typed property is not read as a list
	switch p.typ {
	case "number", "checkbox", "date", "datetime":
		if e.name != "isEmpty" {
			return frag{}, errf("column %d: %s has the type %s: .%s(…) applies to text and lists", e.col, p.text, p.typ, e.name)
		}
	}
	// In a list-typed property, a text value is a list of 1 item, that text
	// (`tags: work`); an empty text is no item, and a number or a boolean
	// is not an item.
	textItem := isListType(p.typ)
	item := func(k constant) frag {
		i := c.json()
		if k.k == kNum {
			return lit(`CASE WHEN `+j+`.type = 'array' THEN EXISTS (SELECT 1 FROM json_each(`+arrayOf(j)+`) `+i+` WHERE `+i+`.type IN ('integer', 'real') AND `+i+`.value = ?) ELSE 0 END`, k.numArg)
		}
		f := lit(`CASE WHEN `+j+`.type = 'array' THEN EXISTS (SELECT 1 FROM json_each(`+arrayOf(j)+`) `+i+` WHERE `+i+`.type = 'text' AND `+i+`.value = ?) ELSE 0 END`, k.str)
		if textItem {
			f = lit(`(`+f.sql+` OR (`+j+`.type = 'text' AND `+j+`.value != '' AND `+j+`.value = ?))`, k.str, k.str)
		}
		return f
	}
	switch e.name {
	case "isEmpty":
		return lit(`NOT EXISTS (SELECT 1 FROM json_each(notes.props) `+j+` WHERE `+j+`.key = ? AND NOT (`+j+`.type = 'null'
			OR (`+j+`.type = 'text' AND `+j+`.value = '')
			OR (`+j+`.type IN ('array', 'object') AND NOT EXISTS (SELECT 1 FROM json_each(CASE WHEN `+j+`.type IN ('array', 'object') THEN `+j+`.value ELSE '[]' END)))))`, p.key), nil
	case "contains":
		if len(e.args) != 1 {
			return frag{}, errf("column %d: contains takes 1 string or number", e.col)
		}
		args, err := c.constArgs(e)
		if err != nil {
			return frag{}, err
		}
		if len(args) != 1 {
			return frag{}, errf("column %d: contains takes 1 string or number", e.col)
		}
		k := args[0]
		var branches []frag
		if k.k == kStr && textOK {
			if k.str == "" {
				branches = append(branches, lit(j+`.type = 'text'`))
			} else {
				branches = append(branches, lit(j+`.type = 'text' AND instr(`+j+`.value, ?) > 0`, k.str))
			}
		}
		if listOK {
			branches = append(branches, item(k))
		}
		return wrap(branches...), nil
	case "startsWith", "endsWith":
		s, err := c.oneString(e)
		if err != nil {
			return frag{}, err
		}
		if !textOK {
			return fFalse, nil
		}
		switch {
		case s == "":
			return wrap(lit(j + `.type = 'text'`)), nil
		case e.name == "startsWith":
			return wrap(lit(j+`.type = 'text' AND instr(`+j+`.value, ?) = 1`, s)), nil
		}
		return wrap(lit(j+`.type = 'text' AND substr(`+j+`.value, -length(?)) = ?`, s, s)), nil
	}
	// containsAny, containsAll: lists only.
	if len(e.args) == 0 {
		return frag{}, errf("column %d: %s takes at least 1 string or number", e.col, e.name)
	}
	args, err := c.constArgs(e)
	if err != nil {
		return frag{}, err
	}
	if !listOK {
		return fFalse, nil
	}
	var items []frag
	for _, k := range args {
		items = append(items, item(k))
	}
	if e.name == "containsAny" {
		if len(items) == 0 {
			return fFalse, nil
		}
		return wrap(or(items...)), nil
	}
	holders := j + `.type = 'array'`
	if textItem {
		holders = j + `.type IN ('array', 'text')`
	}
	return wrap(and(append([]frag{lit(holders)}, items...)...)), nil
}

// fileMethod compiles a method on a file property.
func (c *compiler) fileMethod(p prop, e callExpr) (frag, *Error) {
	fp := fileProps[p.file]
	switch {
	case p.file == "links":
		if e.name != "isEmpty" {
			return frag{}, errf("column %d: file.links takes only isEmpty(): use file.hasLink to test a link", e.col)
		}
		return lit(`NOT EXISTS (SELECT 1 FROM links WHERE links.file_id = files.id AND links.target != '')`), nil
	case p.file == "tags":
		return c.tagsMethod(e)
	case fp.kind != pText:
		return frag{}, errf("column %d: %s has no .%s(…): it is a %s", e.col, p.text, e.name, map[propKind]string{pNum: "number", pDate: "date"}[fp.kind])
	}
	switch e.name {
	case "isEmpty":
		return lit(`(` + fp.sql + ` = '')`), nil
	case "containsAny", "containsAll":
		// Lists only: false on text.
		if len(e.args) == 0 {
			return frag{}, errf("column %d: %s takes at least 1 string or number", e.col, e.name)
		}
		if _, err := c.constArgs(e); err != nil {
			return frag{}, err
		}
		return fFalse, nil
	}
	s, err := c.oneString(e)
	if err != nil {
		return frag{}, err
	}
	switch {
	case s == "":
		return fTrue, nil
	case e.name == "contains":
		return lit(`(instr(`+fp.sql+`, ?) > 0)`, s), nil
	case e.name == "startsWith":
		return lit(`(instr(`+fp.sql+`, ?) = 1)`, s), nil
	}
	return lit(`(substr(`+fp.sql+`, -length(?)) = ?)`, s, s), nil
}

// tagsMethod compiles a method on file.tags: a list of text items.
func (c *compiler) tagsMethod(e callExpr) (frag, *Error) {
	has := func(name string) frag {
		return lit(`EXISTS (SELECT 1 FROM tags WHERE tags.file_id = files.id AND tags.name = ?)`, name)
	}
	switch e.name {
	case "isEmpty":
		return lit(`NOT EXISTS (SELECT 1 FROM tags WHERE tags.file_id = files.id)`), nil
	case "startsWith", "endsWith":
		if _, err := c.oneString(e); err != nil {
			return frag{}, err
		}
		return fFalse, nil
	case "contains":
		if len(e.args) != 1 {
			return frag{}, errf("column %d: contains takes 1 string or number", e.col)
		}
	default:
		if len(e.args) == 0 {
			return frag{}, errf("column %d: %s takes at least 1 string or number", e.col, e.name)
		}
	}
	args, err := c.constArgs(e)
	if err != nil {
		return frag{}, err
	}
	var items []frag
	for _, k := range args {
		if k.k == kStr {
			items = append(items, has(k.str))
		} else {
			items = append(items, fFalse) // a tag is text: it never equals a number
		}
	}
	switch e.name {
	case "containsAll":
		return and(items...), nil
	}
	return or(items...), nil
}
