package query_test

import (
	"errors"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/frostyard/frostnotes/internal/query"
)

// marker is text a base carries into its literals and keys; no compiled SQL
// may hold it (ADR-0037, check 4: no text of a base reaches SQL text).
const marker = "ZQX"

var (
	sqlWord  = regexp.MustCompile(`[A-Za-z_][A-Za-z_0-9]*`)
	sqlQuote = regexp.MustCompile(`'[^']*'`)
)

// sqlWords is every word of the SQL that Compile writes, and sqlLiterals
// every string literal in it. A word or literal from a base is in neither.
var sqlWords = setOf(`select from where exists not and or in case when then else end is null json_each notes props files path key type value
	instr substr length rtrim replace coalesce frostnotes_time tags links file_id id folded name target dest_id vault_id mtime size
	asc desc as j i f`)

var sqlLiterals = setOf(`integer real text true false array object null / . [] ` + "``")

func setOf(words string) map[string]bool {
	set := map[string]bool{"": true}
	for _, w := range strings.Fields(words) {
		set[w] = true
	}
	return set
}

// checkSQL fails when a compiled query holds text of the base.
func checkSQL(t *testing.T, src string, plan *query.Plan) {
	t.Helper()
	for _, part := range []struct {
		name, sql string
		args      int
	}{{"WHERE", plan.Query.Where, len(plan.Query.Args)}, {"ORDER BY", plan.Query.OrderBy, len(plan.Query.OrderArgs)}} {
		if strings.Contains(part.sql, marker) {
			t.Errorf("%s of %q holds the marker: %s", part.name, src, part.sql)
		}
		if n := strings.Count(part.sql, "?"); n != part.args {
			t.Errorf("%s of %q has %d placeholders for %d arguments", part.name, src, n, part.args)
		}
		for _, q := range sqlQuote.FindAllString(part.sql, -1) {
			if !sqlLiterals[strings.Trim(q, "'")] {
				t.Errorf("%s of %q holds the literal %s: %s", part.name, src, q, part.sql)
			}
		}
		rest := sqlQuote.ReplaceAllString(part.sql, "")
		for _, w := range sqlWord.FindAllString(rest, -1) {
			word := strings.ToLower(w)
			if len(word) > 1 && (word[0] == 'j' || word[0] == 'i') && strings.Trim(word[1:], "0123456789") == "" {
				continue // an alias
			}
			if !sqlWords[word] {
				t.Errorf("%s of %q holds the word %q: %s", part.name, src, w, part.sql)
			}
		}
	}
}

func fuzzOptions() query.Options {
	loc, err := time.LoadLocation("America/New_York")
	if err != nil {
		panic(err)
	}
	return query.Options{
		Types: map[string]string{"due": "date", "rating": "number", "done": "checkbox", "tags": "tags", "owner": "text"},
		This:  &query.This{ID: 7, Path: "A/This.md", Size: 5, MTime: time.Unix(1_700_000_000, 0), Tags: []string{"x"}, Links: []string{"y"}},
		ResolveLink: func(from, target string) (string, error) {
			return "", nil
		},
		Now: time.Date(2026, 3, 5, 15, 0, 0, 0, loc), Loc: loc, Limit: 1000,
	}
}

// compileChecked compiles src, which must give an *query.Error or a plan
// with no text of the base in its SQL.
func compileChecked(t *testing.T, src string) {
	t.Helper()
	plan, err := query.Compile([]byte(src), fuzzOptions())
	if err != nil {
		var qerr *query.Error
		if !errors.As(err, &qerr) {
			t.Fatalf("%q: error %T %v is not a *query.Error", src, err, err)
		}
		return
	}
	checkSQL(t, src, plan)
}

// FuzzCompile: whatever the base says, Compile never panics, and a base it
// accepts gives SQL that holds no text of the base. Statements are built
// from the fuzz text in each place a literal or a key can be, with the
// marker in front; and the whole statement, and the whole base, are fuzz text.
func FuzzCompile(f *testing.F) {
	for _, seed := range []string{
		"", "a", "x y", `"`, `'`, `\`, `\"`, "'; DROP TABLE files; --", `" OR 1=1 --`, "%", "_", "\x00", "日本", "a\nb", "$.a", "$[0]", "?", "??", "$1",
		"2026-03-05", "1 week", "#tag", "../x", "/", "Projects/", "a'b", `a"b`, "{{x}}", "*/", "/*",
	} {
		f.Add(seed)
	}
	f.Fuzz(func(t *testing.T, raw string) {
		s := strings.ToValidUTF8(raw, "?")
		lit := quote(marker + s) // a double-quoted string for a statement
		key := marker + s
		stmts := []string{
			`status == ` + lit, `status != ` + lit, `status < ` + lit, `rating > 1 && owner == ` + lit,
			`note[` + lit + `] == 1`, `note[` + lit + `] == ` + lit, `note[` + lit + `].contains(` + lit + `)`,
			`note[` + lit + `].startsWith(` + lit + `)`, `note[` + lit + `].endsWith(` + lit + `)`,
			`note[` + lit + `].containsAny(` + lit + `, 1)`, `note[` + lit + `].containsAll(` + lit + `)`,
			`note[` + lit + `].isEmpty()`, `note[` + lit + `]`, `!note[` + lit + `]`, `due == ` + lit, `due < date(` + lit + `)`,
			`file.hasTag(` + lit + `)`, `file.inFolder(` + lit + `)`, `file.hasLink(` + lit + `)`, `file.hasProperty(` + lit + `)`,
			`file.name == ` + lit, `file.path.contains(` + lit + `)`, `file.tags.contains(` + lit + `)`,
			`file.hasLink(this) || file.name == ` + lit, `file.folder == this.file.folder && status == ` + lit,
			`due > now() - ` + lit, `due > today() + ` + lit,
			s, "(" + s + ")", "!" + s, `status == ` + s, `file.hasTag(` + s + `)`, s + ` == 1`,
		}
		for _, stmt := range stmts {
			compileChecked(t, filterBase(stmt))
			compileChecked(t, "filters:\n  not:\n    - "+quote(stmt)+"\n")
		}
		// Keys in sort, groupBy, order, image and properties.
		compileChecked(t, "views:\n  - type: cards\n    image: "+quote("note."+key)+"\n    order: ["+quote(key)+", "+quote("note."+key)+"]\n"+
			"    sort:\n      - property: "+quote("note."+key)+"\n        direction: DESC\n      - property: "+quote(key)+"\n"+
			"    groupBy:\n      property: "+quote(key)+"\nproperties:\n  "+quote("note."+key)+":\n    displayName: "+lit+"\n")
		compileChecked(t, "views:\n  - type: table\n    sort:\n      - property: "+quote("file."+s)+"\n")
		compileChecked(t, "views:\n  - type: table\n    limit: "+quote(s)+"\n    name: "+quote(s)+"\n")
		// The whole base is fuzz text.
		compileChecked(t, raw)
		compileChecked(t, s)
		compileChecked(t, "filters: "+s)
		compileChecked(t, "filters:\n  - "+s)
	})
}

// TestCompileChecksValidBases runs statement forms the other tests rely on
// through the same SQL check as FuzzCompile, so `go test` (no -fuzz) checks
// them too.
func TestCompileChecksValidBases(t *testing.T) {
	for _, stmt := range []string{
		`status == "x" && rating > 3 || !done`, `file.hasTag("a/b") && file.inFolder("P") && file.hasLink("Z")`,
		`due < today() - "1 week"`, `category.containsAny("a", "b") && owner.startsWith("A")`, `file.name.endsWith("md") && file.tags.isEmpty()`,
		`file.hasLink(this) && file.mtime > this.file.mtime`, `note["a b"].contains("x") || file.size > 10`,
	} {
		compileChecked(t, filterBase(stmt))
	}
	compileChecked(t, "views:\n  - type: table\n    sort:\n      - property: rating\n      - property: file.size\n        direction: DESC\n    groupBy:\n      property: status\n")
}
