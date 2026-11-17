package query_test

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/frostyard/frostnotes/internal/query"
	"github.com/frostyard/frostnotes/internal/vaultgen"
)

// BenchmarkQuery runs a typical base over a generated vault of about 1000
// notes: compile, SQL and the result, as note.query does. A budget is the
// plan's (docs/plans/0010-m5-queries.md).
func BenchmarkQuery(b *testing.B) {
	dir := filepath.Join(b.TempDir(), "vault")
	if _, err := vaultgen.Generate(dir, vaultgen.Options{Seed: 7, Notes: 1000}); err != nil {
		b.Fatal(err)
	}
	e := indexedEnv(b, dir)
	for _, bench := range []struct{ name, base string }{
		{"filter and sort", `filters:
  and:
    - file.inFolder("Orchard")
    - file.hasTag("orchard")
    - 'created >= date("2026-09-01")'
    - '!file.name.contains("Plan")'
views:
  - type: table
    order: [file.name, created, tags]
    limit: 50
    sort:
      - property: created
        direction: DESC
`},
		{"group by", `views:
  - type: table
    order: [file.name, file.folder]
    groupBy:
      property: file.folder
    sort:
      - property: file.mtime
        direction: DESC
`},
		{"text search", `filters:
  or:
    - file.name.contains("harvest")
    - 'title.startsWith("Pear")'
    - file.hasLink("Overview")
`},
	} {
		b.Run(bench.name, func(b *testing.B) {
			o := e.options()
			for b.Loop() {
				plan, err := query.Compile([]byte(bench.base), o)
				if err != nil {
					b.Fatal(err)
				}
				rows, total, err := e.srv.DB.RunQuery(context.Background(), e.id, plan.Query)
				if err != nil {
					b.Fatal(err)
				}
				if res := plan.Result(rows, total); len(res.Rows) == 0 {
					b.Fatal("no rows")
				}
			}
		})
	}
}
