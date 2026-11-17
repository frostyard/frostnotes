package query_test

import (
	"errors"
	"strings"
	"testing"

	"github.com/frostyard/frostnotes/internal/query"
)

// TestRefusals: each feature outside the subset, and each malformed base,
// is an *query.Error that names its location and, for a statement, the
// statement. No partial result.
func TestRefusals(t *testing.T) {
	e := fixtureEnv(t)
	stmt := func(s string) string { return filterBase(s) }
	viewFilter := func(s string) string { return "views:\n  - type: table\n    filters: " + quote(s) + "\n" }
	sortBy := func(s string) string { return view("sort:\n  - property: " + s + "\n") }
	tests := []struct {
		name string
		base string
		want string // the start of the message; "" for the whole text in full
		has  string // a fragment the message holds
	}{
		// The document.
		{"malformed YAML", "filters: [\n", "base: not valid YAML", ""},
		{"a tab in YAML", "filters:\n\t- x\n", "base: not valid YAML", ""},
		{"a list is not a base", "- a\n- b\n", "base: a base is a YAML mapping", ""},
		{"a scalar is not a base", "hello\n", "base: a base is a YAML mapping", ""},
		{"a bad second document", "filters: 'true'\n---\nfilters: [\n", "base: not valid YAML after the first document", ""},
		{"two documents", "views: []\n---\nviews: []\n", "base: more than one YAML document", ""},
		{"an alias", "x: &a status\nfilters: *a\n", "filters: YAML anchors and aliases are outside the subset", ""},
		{"a repeated key", "filters: a\nfilters: b\n", "filters: the key is given twice", ""},
		{"views is not a list", "views: x\n", "views: views is a list of mappings", ""},
		{"a view is not a mapping", "views:\n  - x\n", "views[0]: a view is a mapping", ""},
		{"properties is not a mapping", "properties: x\n", "properties: properties is a mapping", ""},
		{"a property is not a mapping", "properties:\n  a: x\n", "properties.a: a property is a mapping", ""},
		{"displayName is not a string", "properties:\n  a:\n    displayName: [x]\n", "properties.a.displayName: displayName is a string", ""},

		// Filters.
		{"two keys in a filter mapping", "filters:\n  and: []\n  or: []\n", "filters: a filter mapping has exactly one key (and, or or not); this one has 2", ""},
		{"an empty filter mapping", "filters: {}\n", "filters: a filter mapping has exactly one key", ""},
		{"an unknown filter key", "filters:\n  xor: []\n", `filters: the filter key "xor" is not and, or or not`, ""},
		{"a list where a filter belongs", "filters: [a, b]\n", "filters: a filter is a statement string, or a mapping", ""},
		{"a number as a filter", "filters:\n  and:\n    - 5\n", "filters.and[0]: a filter is a statement string", ""},
		{"a boolean as a filter", "filters:\n  or:\n    - true\n", "filters.or[0]: a filter is a statement string", ""},
		{"and holds a scalar", "filters:\n  and: x\n", "filters.and: and holds a list of filters", ""},
		{"a nested location", "filters:\n  and:\n    - or:\n        - 'rating > 1'\n        - and:\n            - formula.x > 1\n", "filters.and[0].or[1].and[0]", `"formula.x > 1": formula.x is outside the subset`},
		{"too deep", "filters: " + strings.Repeat(`{"and": [`, 40) + `"status"` + strings.Repeat(`]}`, 40) + "\n", "filters.and[0].and[0]", "filters nest more than 32 levels"},

		// The spec's example.
		{"file.backlinks", "views:\n  - type: table\n    filters:\n      and:\n        - 'rating > 1'\n        - file.backlinks.contains(this)\n",
			`views[0].filters.and[1]: "file.backlinks.contains(this)": file.backlinks is outside the subset`, ""},

		// Outside the subset.
		{"formula in a filter", stmt("formula.x > 1"), `filters: "formula.x > 1": formula.x is outside the subset`, ""},
		{"formula alone", stmt("formula"), `filters: "formula": formula is outside the subset`, ""},
		{"file.embeds", stmt("file.embeds.isEmpty()"), "filters", "file.embeds is outside the subset"},
		{"file.properties", stmt("file.properties"), "filters", "file.properties is outside the subset"},
		{"file.ctime", stmt("file.ctime > today()"), "filters", "file.ctime is outside the subset"},
		{"file.file", stmt("file.file"), "filters", "file.file is outside the subset"},
		{"an unknown file property", stmt("file.colour == 1"), "filters", "file.colour is outside the subset"},
		{"this.note", stmt("this.note.status == 1"), "filters", "this.note.status is outside the subset"},
		{"this.KEY", stmt("this.status == 1"), "filters", "this.status is outside the subset"},
		{"this.file.backlinks", stmt("this.file.backlinks.isEmpty()"), "filters", "this.file.backlinks is outside the subset"},
		{"this.file.ctime", stmt("file.mtime > this.file.ctime"), "filters", "this.file.ctime is outside the subset"},
		{"null", stmt("status == null"), "filters", "null is outside the subset"},
		{"an unknown function", stmt("foo(1)"), `filters: "foo(1)"`, "the function foo is outside the subset"},
		{"link()", stmt(`link("x")`), "filters", "the function link is outside the subset"},
		{"an unknown file function", stmt(`file.nonsense("x")`), "filters", "file.nonsense(…) is outside the subset"},
		{"an unknown method", stmt(`status.lower() == "x"`), "filters", "the method .lower(…) is outside the subset"},
		{"matches", stmt(`status.matches("x")`), "filters", "the method .matches(…) is outside the subset"},
		{"filter()", stmt(`category.filter(value > 1).isEmpty()`), "filters", "outside the subset"},
		{"a regular expression", stmt(`status == /x/`), "filters", `unexpected "/"`},
		{"arithmetic on a property", stmt("rating + 1 > 3"), "filters", "arithmetic is outside the subset"},
		{"arithmetic on numbers", stmt("1 + 2 == 3"), "filters", "arithmetic is outside the subset"},
		{"arithmetic on text", stmt(`"a" + "b" == "ab"`), "filters", "arithmetic is outside the subset"},
		{"a date minus a date", stmt("now() - today() > 1"), "filters", "arithmetic is outside the subset"},
		{"a duration on a property", stmt(`due + "1 d" > today()`), "filters", "arithmetic is outside the subset"},
		{"a duration first", stmt(`"1 d" + now() > today()`), "filters", "arithmetic is outside the subset"},
		{"a comparison of two properties", stmt("rating > mood"), "filters", "a comparison of two properties is outside the subset"},
		{"two file properties", stmt("file.name == file.basename"), "filters", "a comparison of two properties is outside the subset"},
		{"a property and a file property", stmt("status == file.name"), "filters", "a comparison of two properties is outside the subset"},
		{"a method of a constant", stmt(`"abc".contains("a")`), "filters", "applies to a property"},
		{"a method of a condition", stmt(`(rating > 1).contains(1)`), "filters", "applies to a property"},
		{"a method of a date", stmt(`today().contains(1)`), "filters", "applies to a property"},
		{"a property as an argument", stmt(`status.contains(rating)`), "filters", "a property or a condition is outside the subset"},
		{"a condition as an argument", stmt(`status.contains(rating > 1)`), "filters", "a property or a condition is outside the subset"},
		{"a boolean as an argument", stmt(`status.contains(true)`), "filters", "strings or numbers"},
		{"a date as an argument", stmt(`status.contains(today())`), "filters", "strings or numbers"},

		// Dates and durations.
		{"a bad duration unit", stmt(`due < now() - "1 fortnight"`), "filters", `unknown unit "fortnight"`},
		{"a bad duration number", stmt(`due < now() - "x"`), "filters", "want a whole number"},
		{"a duration that is a number", stmt(`due < now() - 5`), "filters", "arithmetic is outside the subset"},
		{"a fractional duration", stmt(`due < now() - "1.5 d"`), "filters", "unknown unit"},
		{"a negative duration", stmt(`due < now() - "-1 d"`), "filters", "want a whole number"},
		{"a duration without a number", stmt(`due < now() - "week"`), "filters", "want a whole number"},
		{"a huge duration", stmt(`due < now() - "9999999 d"`), "filters", "want a whole number"},
		{"date(): not a date", stmt(`due < date("tomorrow")`), "filters", `date("tomorrow") is not an ISO 8601 date`},
		{"date(): a number", stmt(`due < date(5)`), "filters", "date() takes a string"},
		{"date(): no argument", stmt(`due < date()`), "filters", "date() takes 1 string"},
		{"date(): two arguments", stmt(`due < date("2026-01-01", "x")`), "filters", "date() takes 1 string"},
		{"date(): a month 13", stmt(`due < date("2026-13-01")`), "filters", "is not an ISO 8601 date"},
		{"date(): February 30", stmt(`due < date("2026-02-30")`), "filters", "is not an ISO 8601 date"},
		{"date(): an hour 24", stmt(`due < date("2026-01-01T24:00")`), "filters", "is not an ISO 8601 date"},
		{"date(): a zone on a date", stmt(`due < date("2026-01-01Z")`), "filters", "is not an ISO 8601 date"},
		{"date(): a slash", stmt(`due < date("2026/01/01")`), "filters", "is not an ISO 8601 date"},
		{"date(): a short year", stmt(`due < date("26-01-01")`), "filters", "is not an ISO 8601 date"},
		{"date(): a time with the hour only", stmt(`due < date("2026-01-01T09")`), "filters", "is not an ISO 8601 date"},
		{"date(): ten fraction digits", stmt(`due < date("2026-01-01T09:00:00.0123456789")`), "filters", "is not an ISO 8601 date"},
		{"date(): a bad offset", stmt(`due < date("2026-01-01T09:00+25:00")`), "filters", "is not an ISO 8601 date"},
		{"date(): a function as the argument", stmt(`due < date(now())`), "filters", "date() takes a string"},
		{"today() with an argument", stmt(`due < today(1)`), "filters", "today() takes no argument"},
		{"now() with an argument", stmt(`due < now("x")`), "filters", "now() takes no argument"},
		{"today alone", stmt(`due < today`), "filters", "today is a function"},
		{"a typed date against a text that is no date", stmt(`due == "tomorrow"`), "filters", `"tomorrow" is not an ISO 8601 date`},
		{"a typed date against a number", stmt(`due == 5`), "filters", "has the type date"},
		{"file.mtime against a number", stmt(`file.mtime > 5`), "filters", "file.mtime is a date"},
		{"file.mtime against a bad text", stmt(`file.mtime > "soon"`), "filters", `"soon" is not an ISO 8601 date`},
		{"a date is not a condition", stmt(`today()`), "filters", "a date is not a condition"},
		{"a date sum is not a condition", stmt(`today() + "1 d"`), "filters", "a date is not a condition"},
		{"file.mtime is not a condition", stmt(`file.mtime`), "filters", "file.mtime is a date"},

		// Wrong kinds.
		{"file.size against text", stmt(`file.size == "big"`), "filters", "file.size is a number"},
		{"file.name against a number", stmt(`file.name == 5`), "filters", "file.name is text"},
		{"file.path against a boolean", stmt(`file.path == true`), "filters", "file.path is text"},
		{"file.tags against text", stmt(`file.tags == "x"`), "filters", "file.tags is a list"},
		{"file.links against text", stmt(`file.links != "x"`), "filters", "file.links is a list"},
		{"file.links.contains", stmt(`file.links.contains("x")`), "filters", "file.links takes only isEmpty()"},
		{"file.size.contains", stmt(`file.size.contains(1)`), "filters", "file.size has no .contains"},
		{"file.mtime.isEmpty", stmt(`file.mtime.isEmpty()`), "filters", "file.mtime has no .isEmpty"},
		{"typed number against text", stmt(`priority == "x"`), "filters", "priority has the type number: it cannot compare with a string"},
		{"typed number against a boolean", stmt(`priority == true`), "filters", "has the type number"},
		{"typed checkbox against a number", stmt(`done == 1`), "filters", "done has the type checkbox"},
		{"typed text against a number", stmt(`owner == 1`), "filters", "owner has the type text"},
		{"typed checkbox ordering", stmt(`done > true`), "filters", "a boolean compares only with == and !="},
		{"untyped boolean ordering", stmt(`flag < false`), "filters", "a boolean compares only with == and !="},
		{"boolean constants ordering", stmt(`true < false`), "filters", "a boolean compares only with == and !="},
		{"a condition ordering", stmt(`(rating > 1) > true`), "filters", "a boolean compares only with == and !="},
		{"a condition against a number", stmt(`(rating > 1) == 1`), "filters", "this comparison is outside the subset"},
		{"a list-typed property: ==", stmt(`tags == "x"`), `filters: "tags == \"x\""`, "tags has the type tags: a list cannot be compared: use contains, containsAny or isEmpty"},
		{"a list-typed property: !=", stmt(`aliases != "x"`), "filters", "aliases has the type aliases: a list cannot be compared: use contains, containsAny or isEmpty"},
		{"a list-typed property: <", stmt(`category < "x"`), "filters", "category has the type multitext: a list cannot be compared"},
		{"a list-typed property: >=", stmt(`labels >= 1`), "filters", "labels has the type multitext: a list cannot be compared"},
		{"a list-typed property: a literal on the left", stmt(`"x" <= tags`), "filters", "tags has the type tags: a list cannot be compared"},
		{"a list-typed property: a boolean", stmt(`tags == true`), "filters", "a list cannot be compared"},
		{"a list-typed property: a date", stmt(`tags > today()`), "filters", "a list cannot be compared"},
		{"a list-typed property: a boolean item", stmt(`tags.contains(true)`), "filters", "strings or numbers"},
		{"typed number contains", stmt(`priority.contains(1)`), "filters", "priority has the type number: .contains(…) applies to text and lists"},
		{"typed date startsWith", stmt(`due.startsWith("2026")`), "filters", "has the type date"},
		{"contains with no argument", stmt(`status.contains()`), "filters", "contains takes 1 string or number"},
		{"contains with two", stmt(`status.contains("a", "b")`), "filters", "contains takes 1 string or number"},
		{"startsWith with a number", stmt(`status.startsWith(1)`), "filters", "startsWith takes 1 string"},
		{"containsAny with none", stmt(`category.containsAny()`), "filters", "containsAny takes at least 1"},
		{"isEmpty with an argument", stmt(`status.isEmpty(1)`), "filters", "isEmpty() takes no argument"},
		{"hasTag with none", stmt(`file.hasTag()`), "filters", "file.hasTag takes at least 1 tag"},
		{"hasTag with an empty tag", stmt(`file.hasTag("")`), "filters", "not an empty string"},
		{"hasTag with only #", stmt(`file.hasTag("#")`), "filters", "not an empty string"},
		{"hasTag with a number", stmt(`file.hasTag(1)`), "filters", "file.hasTag takes strings"},
		{"inFolder with a number", stmt(`file.inFolder(1)`), "filters", "file.inFolder takes 1 string"},
		{"inFolder with none", stmt(`file.inFolder()`), "filters", "file.inFolder takes 1 string"},
		{"hasLink with a number", stmt(`file.hasLink(5)`), "filters", "file.hasLink takes a string, this or this.file"},
		{"hasLink with none", stmt(`file.hasLink()`), "filters", "file.hasLink takes 1 argument"},
		{"hasLink with a property", stmt(`file.hasLink(status)`), "filters", "file.hasLink takes a string, this or this.file"},
		{"hasProperty with a property", stmt(`file.hasProperty(rating)`), "filters", "a property or a condition is outside the subset"},
		{"this as a condition", stmt(`this`), "filters", "this is not set"},

		// Syntax.
		{"a single =", stmt(`status = "a"`), "filters", `unexpected "="`},
		{"a single &", stmt(`a & b`), "filters", `unexpected "&"`},
		{"a single |", stmt(`a | b`), "filters", `unexpected "|"`},
		{"chained comparisons", stmt(`1 < status < 3`), "filters", `unexpected "<"`},
		{"two statements", stmt(`status status`), "filters", "unexpected"},
		{"an unclosed string", stmt(`status == "a`), "filters", "the string is not closed"},
		{"a bad escape", stmt(`status == "a\q"`), "filters", `unknown escape \q`},
		{"an unclosed parenthesis", stmt(`(status == "a"`), "filters", "unexpected the end of the statement"},
		{"an empty statement", stmt(``), "filters", "unexpected the end of the statement"},
		{"a lone operator", stmt(`&&`), "filters", `unexpected "&&"`},
		{"a dangling operator", stmt(`status ==`), "filters", "unexpected the end of the statement"},
		{"a property of a property", stmt(`status.length > 1`), "filters", "is not a method call"},
		{"note alone", stmt(`note`), "filters", "note must be followed by .NAME"},
		{"file alone", stmt(`file`), "filters", "file must be followed by .NAME"},
		{"note.fn(", stmt(`note.status(1)`), "filters", "note.status(…): a function is outside the subset"},
		{"an unclosed bracket", stmt(`note["a"`), "filters", `expected "]"`},
		{"a bracket with a number", stmt(`note[1] == 1`), "filters", "unexpected the number 1"},
		{"a number out of range", stmt(`rating == 1e999`), "filters", "out of range"},
		{"a comma", stmt(`a, b`), "filters", `unexpected ","`},
		{"a trailing comma", stmt(`status.contains("a",)`), "filters", "unexpected"},
		{"a statement too long", stmt("status == \"" + strings.Repeat("a", 5000) + "\""), "filters", "the most is 4096"},
		{"parentheses too deep", stmt(strings.Repeat("(", 100) + "1" + strings.Repeat(")", 100)), "filters", "nests more than 64 levels"},
		{"! too deep", stmt(strings.Repeat("!", 100) + "a"), "filters", "nests more than 64 levels"},
		{"an error in a view's filter", viewFilter(`file.colour == 1`), `views[0].filters: "file.colour == 1"`, "file.colour is outside the subset"},

		// Sort, group, columns, limit.
		{"sort on a formula", sortBy("formula.x"), `views[0].sort[0]: "formula.x"`, "formula.x is outside the subset"},
		{"sort on file.tags", sortBy("file.tags"), `views[0].sort[0]: "file.tags"`, "file.tags is a list"},
		{"sort on file.links", sortBy("file.links"), `views[0].sort[0]: "file.links"`, "file.links is a list"},
		{"sort on file.ctime", sortBy("file.ctime"), `views[0].sort[0]: "file.ctime"`, "outside the subset"},
		{"sort on this", sortBy("this.file.name"), `views[0].sort[0]: "this.file.name"`, "outside the subset"},
		{"sort on a note property with no name", sortBy("note."), `views[0].sort[0]`, "the property has no name"},
		{"the second sort key", view("sort:\n  - property: rating\n  - property: formula.y\n"), `views[0].sort[1]: "formula.y"`, "outside the subset"},
		{"sort without a property", view("sort:\n  - direction: ASC\n"), "views[0].sort[0].property: property is a property key", ""},
		{"sort is not a list", view("sort: rating\n"), "views[0].sort: sort is a list of mappings", ""},
		{"a sort key is not a mapping", view("sort:\n  - rating\n"), "views[0].sort[0]: a sort key is a mapping", ""},
		{"a bad direction", view("sort:\n  - property: rating\n    direction: SIDEWAYS\n"), `views[0].sort[0].direction: "SIDEWAYS": direction is ASC or DESC`, ""},
		{"a direction that is not a string", view("sort:\n  - property: rating\n    direction: [a]\n"), "views[0].sort[0].direction: direction is ASC or DESC", ""},
		{"groupBy a formula", view("groupBy:\n  property: formula.g\n"), `views[0].groupBy: "formula.g"`, "outside the subset"},
		{"groupBy file.tags", view("groupBy:\n  property: file.tags\n"), `views[0].groupBy: "file.tags"`, "is a list"},
		{"groupBy is not a mapping", view("groupBy: status\n"), "views[0].groupBy: a sort key is a mapping", ""},
		{"limit 0", view("limit: 0\n"), "views[0].limit: the limit is a whole number of at least 1", ""},
		{"a negative limit", view("limit: -3\n"), "views[0].limit:", ""},
		{"a limit that is text", view("limit: many\n"), "views[0].limit:", ""},
		{"a fractional limit", view("limit: 1.5\n"), "views[0].limit:", ""},
		{"order is not a list", "views:\n  - type: table\n    order: file.name\n", "views[0].order: order is a list of property keys", ""},
		{"a column that is not a string", "views:\n  - type: table\n    order: [1]\n", "views[0].order[0]: a column is a property key", ""},
		{"image is not a string", "views:\n  - type: cards\n    image: [x]\n", "views[0].image: image is a property key", ""},
		{"a global filter is checked for a cards view too", "filters: 'x.y.z'\nviews:\n  - type: cards\n", "filters", "is not a method call"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			res, err := e.run(tc.base)
			if err == nil {
				t.Fatalf("a base outside the subset gave %+v", res)
			}
			var qerr *query.Error
			if !errors.As(err, &qerr) {
				t.Fatalf("error %T %v is not a *query.Error", err, err)
			}
			if tc.want == "" && tc.has == "" {
				return
			}
			if !strings.HasPrefix(err.Error(), tc.want) || !strings.Contains(err.Error(), tc.has) {
				t.Errorf("error %q, want it to start with %q and hold %q", err, tc.want, tc.has)
			}
			if !strings.Contains(err.Error(), ":") {
				t.Errorf("error %q names no location", err)
			}
		})
	}
}

func TestRefusalInAnotherView(t *testing.T) {
	e := fixtureEnv(t)
	base := "views:\n  - type: table\n  - type: table\n    name: B\n    filters:\n      not:\n        - 'x ** 2'\n"
	if _, err := e.run(base); err != nil {
		t.Errorf("the first view does not read the second's filters: %v", err)
	}
	_, err := e.run(base, withView("B"))
	if err == nil || !strings.HasPrefix(err.Error(), `views[1].filters.not[0]: "x ** 2": column 3: unexpected "*"`) {
		t.Errorf("error = %v", err)
	}
}

func TestRefusalsWithThis(t *testing.T) {
	e := fixtureEnv(t)
	// A list constant stands for its items only in hasTag, containsAny and
	// containsAll. Alpha has 4 tags; Beta has 1, which an item count cannot catch.
	for _, this := range []string{"Projects/Alpha.md", "Projects/Beta.md"} {
		for _, stmt := range []string{
			`file.tags.contains(this.file.tags)`, `labels.contains(this.file.tags)`, `status.contains(this.file.links)`,
			`file.name.startsWith(this.file.tags)`, `file.name.endsWith(this.file.tags)`, `status.startsWith(this.file.tags)`,
			`file.inFolder(this.file.tags)`, `file.hasProperty(this.file.tags)`, `file.path.contains(this.file.links)`,
		} {
			_, err := e.run(filterBase(stmt), withThis(this, e))
			if err == nil || !strings.Contains(err.Error(), "not a list") || !strings.HasPrefix(err.Error(), "filters: ") {
				t.Errorf("%s with this = %s: %v, want an error: not a list", stmt, this, err)
			}
		}
		for _, stmt := range []string{`file.hasTag(this.file.tags)`, `file.tags.containsAny(this.file.tags)`, `file.tags.containsAll(this.file.tags)`, `labels.containsAny(this.file.tags)`} {
			if _, err := e.run(filterBase(stmt), withThis(this, e)); err != nil {
				t.Errorf("%s with this = %s: %v", stmt, this, err)
			}
		}
	}
	for _, tc := range []struct{ stmt, has string }{
		{`file.tags == this.file.tags`, "a list cannot be compared"},
		{`this.file.tags == 1`, "a list cannot be compared"},
		{`this.file.links == this.file.tags`, "a list cannot be compared"},
		{`this.file.tags.contains("x")`, "applies to a property"},
		{`file.hasLink(this.file.tags)`, "file.hasLink takes a string, this or this.file"},
		{`this == 1`, "this cannot be compared"},
		{`file.name == this`, "this cannot be compared"},
		{`status.contains(this)`, "a property or a condition is outside the subset"},
		{`this.file.size.contains(1)`, "applies to a property"},
		{`this.file`, "this is not a condition"},
	} {
		_, err := e.run(filterBase(tc.stmt), withThis("Projects/Alpha.md", e))
		if err == nil || !strings.Contains(err.Error(), tc.has) || !strings.HasPrefix(err.Error(), "filters: ") {
			t.Errorf("%s: %v, want an error that holds %q", tc.stmt, err, tc.has)
		}
	}
}
