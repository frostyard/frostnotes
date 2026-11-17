package query_test

import (
	"context"
	"encoding/json/v2"
	"io/fs"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/frostyard/frostnotes/api"
	"github.com/frostyard/frostnotes/internal/markdown"
	"github.com/frostyard/frostnotes/internal/query"
	"github.com/frostyard/frostnotes/internal/rpctest"
	"github.com/frostyard/frostnotes/internal/vault"
)

// env is a vault indexed by a real notesd: the fixture vault, or a
// generated one.
type env struct {
	t     testing.TB
	srv   *rpctest.Server
	id    int64
	dir   string
	types map[string]string
	loc   *time.Location
	now   time.Time // the clock of every run
}

// mtimes are the modification times that the fixture's tests name.
var mtimes = map[string]time.Time{
	"Projects/Alpha.md":       time.Date(2026, 3, 4, 12, 0, 0, 123_000_000, time.UTC),
	"Home.md":                 time.Date(2026, 3, 2, 0, 0, 0, 0, time.UTC),
	"Projects/Archive/Old.md": time.Date(2025, 6, 1, 0, 0, 0, 0, time.UTC),
}

// fixtureEnv copies testdata/vault into a temporary directory, gives each
// file a modification time that its path decides (day 1 of March 2026 plus
// its place in the listing, in minutes), and indexes it.
func fixtureEnv(t testing.TB) *env {
	t.Helper()
	dir := filepath.Join(t.TempDir(), "vault")
	var paths []string
	err := filepath.WalkDir("testdata/vault", func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, _ := filepath.Rel("testdata/vault", p)
		if d.IsDir() {
			return os.MkdirAll(filepath.Join(dir, rel), 0o755)
		}
		data, err := os.ReadFile(p)
		if err != nil {
			return err
		}
		paths = append(paths, rel)
		return os.WriteFile(filepath.Join(dir, rel), data, 0o644)
	})
	if err != nil {
		t.Fatal(err)
	}
	for i, rel := range paths {
		at := time.Date(2026, 3, 1, 0, 0, 0, 0, time.UTC).Add(time.Duration(i) * time.Minute)
		if o, ok := mtimes[filepath.ToSlash(rel)]; ok {
			at = o
		}
		if err := os.Chtimes(filepath.Join(dir, rel), at, at); err != nil {
			t.Fatal(err)
		}
	}
	return indexedEnv(t, dir)
}

// indexedEnv registers dir with a test notesd and waits for its scan.
func indexedEnv(t testing.TB, dir string) *env {
	t.Helper()
	srv := rpctest.Start(t)
	c := srv.Dial(t)
	v, err := c.Vault().Add(t.Context(), &api.VaultAddParams{Path: dir})
	if err != nil {
		t.Fatal(err)
	}
	vv, err := srv.Vaults.Vault(v.ID)
	if err != nil {
		t.Fatal(err)
	}
	if err := vv.Scanned(t.Context()); err != nil {
		t.Fatal(err)
	}
	types, err := vault.LoadTypes(dir)
	if err != nil {
		t.Fatal(err)
	}
	loc, err := time.LoadLocation("America/New_York")
	if err != nil {
		t.Fatal(err)
	}
	// 15:00 on 2026-03-05, New York (UTC-5): before the 8 March clock change.
	now := time.Date(2026, 3, 5, 15, 0, 0, 0, loc)
	return &env{t: t, srv: srv, id: v.ID, dir: dir, types: types, loc: loc, now: now}
}

// options are the compile options of a run.
func (e *env) options() query.Options {
	ctx := context.Background()
	return query.Options{
		Types: e.types, Now: e.now, Loc: e.loc, Limit: 1000,
		ResolveLink: func(from, target string) (string, error) {
			var lookupErr error
			got := markdown.Resolve(from, target, func(name string) []string {
				paths, err := e.srv.DB.FilesNamed(ctx, e.id, name)
				lookupErr = err
				return paths
			})
			return got, lookupErr
		},
	}
}

// this is the this file for the file at path: what note.query gives it.
func (e *env) this(path string) *query.This {
	e.t.Helper()
	ctx := context.Background()
	f, err := e.srv.DB.FileByPath(ctx, e.id, path)
	if err != nil {
		e.t.Fatal(err)
	}
	this := &query.This{ID: f.ID, Path: f.Path, Size: f.Size, MTime: f.ModTime, Tags: []string{}, Links: []string{}}
	if f.Kind() == "note" {
		info, err := e.srv.DB.NoteInfo(ctx, f.ID)
		if err != nil {
			e.t.Fatal(err)
		}
		this.Tags = info.Tags
		links, err := e.srv.DB.Links(ctx, f.ID)
		if err != nil {
			e.t.Fatal(err)
		}
		seen := map[string]bool{}
		for _, l := range links {
			if l.Target != "" && !seen[l.Target] {
				seen[l.Target] = true
				this.Links = append(this.Links, l.Target)
			}
		}
	}
	return this
}

// run compiles and runs a base's view. mod adjusts the options.
func (e *env) run(source string, mod ...func(*query.Options)) (*api.QueryResult, error) {
	e.t.Helper()
	o := e.options()
	for _, m := range mod {
		m(&o)
	}
	plan, err := query.Compile([]byte(source), o)
	if err != nil {
		return nil, err
	}
	if plan.Notice != "" {
		return plan.Result(nil, 0), nil
	}
	rows, total, err := e.srv.DB.RunQuery(context.Background(), e.id, plan.Query)
	if err != nil {
		e.t.Fatalf("run %q: %v\nSQL: %s", source, err, plan.Query.Where)
	}
	return plan.Result(rows, total), nil
}

// mustRun is run for a base that compiles.
func (e *env) mustRun(source string, mod ...func(*query.Options)) *api.QueryResult {
	e.t.Helper()
	res, err := e.run(source, mod...)
	if err != nil {
		e.t.Fatalf("base %q: %v", source, err)
	}
	return res
}

func withThis(path string, e *env) func(*query.Options) {
	return func(o *query.Options) { o.This = e.this(path) }
}

func withView(name string) func(*query.Options) {
	return func(o *query.Options) { o.View = &name }
}

func withLimit(n int) func(*query.Options) { return func(o *query.Options) { o.Limit = n } }

func paths(res *api.QueryResult) []string {
	out := make([]string, 0, len(res.Rows))
	for _, r := range res.Rows {
		out = append(out, r.Path)
	}
	return out
}

// quote writes s as a YAML double-quoted string (every JSON string is one).
func quote(s string) string {
	b, err := json.Marshal(s)
	if err != nil {
		panic(err)
	}
	return string(b)
}

// filterBase is a base whose one filter is the statement.
func filterBase(statement string) string { return "filters: " + quote(statement) + "\n" }
