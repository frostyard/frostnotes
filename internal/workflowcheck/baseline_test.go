package workflowcheck

import (
	"encoding/json/v2"
	"fmt"
	"io/fs"
	"net/url"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"runtime"
	"slices"
	"strconv"
	"strings"
	"testing"
	"time"
	"unicode/utf8"

	"go.yaml.in/yaml/v3"
)

// skipped reports whether the repository walk leaves directory name out:
// version control, dependencies, build output, and each name in extra.
func skipped(name string, extra map[string]bool) bool {
	switch name {
	case ".git", "node_modules", "build", "dist", "target":
		return true
	}
	return extra[name]
}

// repoFiles returns every file of the repository as a path from its root with
// forward slashes. The directories skipped, and each of extra, are left out.
func repoFiles(t *testing.T, extra ...string) []string {
	t.Helper()
	root := filepath.Join("..", "..")
	skip := make(map[string]bool, len(extra))
	for _, name := range extra {
		skip[name] = true
	}
	var out []string
	err := filepath.WalkDir(root, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			if skipped(d.Name(), skip) {
				return fs.SkipDir
			}
			return nil
		}
		rel, err := filepath.Rel(root, p)
		if err != nil {
			return err
		}
		out = append(out, filepath.ToSlash(rel))
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	return out
}

// TestInstructionAliases guards row B1 of frostyard/core
// docs/specs/repository-baseline.md: AGENTS.md is the one instruction file,
// and every other agent entry point is a relative symlink to it.
func TestInstructionAliases(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("a Windows checkout has no symlinks")
	}
	agents := filepath.Join("..", "..", "AGENTS.md")
	fi, err := os.Lstat(agents)
	if err != nil {
		t.Fatal(err)
	}
	if !fi.Mode().IsRegular() {
		t.Errorf("AGENTS.md is not a regular file (mode %s); it is the one instruction file", fi.Mode())
	}
	want, err := filepath.EvalSymlinks(agents)
	if err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{
		"CLAUDE.md",
		"GEMINI.md",
		".cursorrules",
		filepath.Join(".github", "copilot-instructions.md"),
	} {
		if got, ok := relativeSymlink(t, name); ok && got != want {
			t.Errorf("%s resolves to %q, want %q", name, got, want)
		}
	}

	name := filepath.Join(".claude", "skills")
	got, ok := relativeSymlink(t, name)
	wantSkills, err := filepath.EvalSymlinks(filepath.Join("..", "..", ".agents", "skills"))
	if err != nil {
		t.Fatalf(".agents/skills: %v", err)
	}
	if ok && got != wantSkills {
		t.Errorf("%s resolves to %q, want %q", name, got, wantSkills)
	}
	if fi, err := os.Stat(filepath.Join("..", "..", name)); err != nil {
		t.Errorf("%s: %v", name, err)
	} else if !fi.IsDir() {
		t.Errorf("%s resolves to a file, want the directory .agents/skills", name)
	}
}

// relativeSymlink reports whether name is a symlink whose target is relative,
// and returns the path it resolves to.
func relativeSymlink(t *testing.T, name string) (string, bool) {
	t.Helper()
	link := filepath.Join("..", "..", name)
	fi, err := os.Lstat(link)
	if err != nil {
		t.Errorf("%s: %v", name, err)
		return "", false
	}
	if fi.Mode()&os.ModeSymlink == 0 {
		t.Errorf("%s is not a symlink", name)
		return "", false
	}
	target, err := os.Readlink(link)
	if err != nil {
		t.Errorf("%s: %v", name, err)
		return "", false
	}
	if filepath.IsAbs(target) {
		t.Errorf("%s links to the absolute path %q; it must be relative", name, target)
		return "", false
	}
	got, err := filepath.EvalSymlinks(link)
	if err != nil {
		t.Errorf("%s: %v", name, err)
		return "", false
	}
	return got, true
}

// correctionsMember names the 5 members a line of .memory/corrections.jsonl
// carries (row B2).
var correctionsMember = []string{"date", "scope", "correction", "evidence", "promoted_to"}

// datePattern is the form of a correction's date.
var datePattern = regexp.MustCompile(`^\d{4}-\d{2}-\d{2}$`)

// correctionProblems returns the problems of 1 line of
// .memory/corrections.jsonl, after frostyard/core scripts/check-corrections.mjs
// (row B2); no problem means a valid line. promoted_to may be empty, the way an
// unpromoted correction has it.
func correctionProblems(line string) []string {
	if strings.TrimSpace(line) == "" {
		return []string{"blank line"}
	}
	var value any
	if err := json.Unmarshal([]byte(line), &value); err != nil {
		return []string{fmt.Sprintf("invalid JSON: %v", err)}
	}
	obj, ok := value.(map[string]any)
	if !ok {
		return []string{"not a plain object"}
	}

	var problems, missing, unknown []string
	for _, name := range correctionsMember {
		if _, ok := obj[name]; !ok {
			missing = append(missing, name)
		}
	}
	for name := range obj {
		if !slices.Contains(correctionsMember, name) {
			unknown = append(unknown, name)
		}
	}
	slices.Sort(unknown)
	if len(missing) > 0 {
		problems = append(problems, "missing key(s): "+strings.Join(missing, ", "))
	}
	if len(unknown) > 0 {
		problems = append(problems, "unknown key(s): "+strings.Join(unknown, ", "))
	}
	for _, name := range correctionsMember {
		v, ok := obj[name]
		if !ok {
			continue
		}
		if _, ok := v.(string); !ok {
			problems = append(problems, "value for "+name+" is not a string")
		}
	}
	if s, ok := obj["date"].(string); ok {
		if !datePattern.MatchString(s) {
			problems = append(problems, "invalid date: "+s)
		} else if _, err := time.Parse("2006-01-02", s); err != nil {
			problems = append(problems, "invalid date: "+s)
		}
	}
	for _, name := range []string{"scope", "correction", "evidence"} {
		if s, ok := obj[name].(string); ok && strings.TrimSpace(s) == "" {
			problems = append(problems, name+" is empty")
		}
	}
	return problems
}

// TestCorrectionsInbox guards row B2 of frostyard/core
// docs/specs/repository-baseline.md: .memory/corrections.jsonl holds one JSON
// object per line, with exactly the 5 fields of a correction.
func TestCorrectionsInbox(t *testing.T) {
	data, err := os.ReadFile(filepath.Join("..", "..", ".memory", "corrections.jsonl"))
	if err != nil {
		t.Fatalf(".memory/corrections.jsonl: %v", err)
	}
	if len(data) == 0 {
		t.Fatal(".memory/corrections.jsonl is empty")
	}
	lines := strings.Split(string(data), "\n")
	if n := len(lines); lines[n-1] == "" {
		lines = lines[:n-1]
	}
	if len(lines) == 0 {
		t.Fatal(".memory/corrections.jsonl holds no line")
	}
	for i, line := range lines {
		for _, problem := range correctionProblems(line) {
			t.Errorf("line %d: %s", i+1, problem)
		}
	}
	t.Logf("%d corrections lines", len(lines))
}

// TestCorrectionProblems checks the line rules of row B2 on the cases
// frostyard/core scripts/check-corrections.mjs names.
func TestCorrectionProblems(t *testing.T) {
	const valid = `{"date": "2026-11-05", "scope": "socket", "correction": "c", "evidence": "e", "promoted_to": ""}`
	for _, tc := range []struct {
		name  string
		line  string
		valid bool
	}{
		{"unpromoted", valid, true},
		{"promoted", `{"date": "2026-11-05", "scope": "s", "correction": "c", "evidence": "e", "promoted_to": "a_test.go"}`, true},
		{"blank scope", `{"date": "2026-11-05", "scope": "  ", "correction": "c", "evidence": "e", "promoted_to": ""}`, false},
		{"month 13", `{"date": "2026-13-99", "scope": "s", "correction": "c", "evidence": "e", "promoted_to": ""}`, false},
		{"february 30", `{"date": "2026-02-30", "scope": "s", "correction": "c", "evidence": "e", "promoted_to": ""}`, false},
		{"number value", `{"date": "2026-11-05", "scope": 1, "correction": "c", "evidence": "e", "promoted_to": ""}`, false},
		{"missing member", `{"date": "2026-11-05", "scope": "s", "correction": "c", "evidence": "e"}`, false},
		{"unknown member", `{"date": "2026-11-05", "scope": "s", "correction": "c", "evidence": "e", "promoted_to": "", "extra": "x"}`, false},
		{"duplicate member", `{"date": "2026-11-05", "date": "2026-11-05", "scope": "s", "correction": "c", "evidence": "e", "promoted_to": ""}`, false},
		{"not an object", `["2026-11-05"]`, false},
		{"blank line", "  ", false},
	} {
		problems := correctionProblems(tc.line)
		switch {
		case tc.valid && len(problems) > 0:
			t.Errorf("%s: correctionProblems(%s) = %v, want none", tc.name, tc.line, problems)
		case !tc.valid && len(problems) == 0:
			t.Errorf("%s: correctionProblems(%s) = none, want at least 1", tc.name, tc.line)
		}
	}
}

// riskPath is 1 entry of .github/risk-paths.json.
type riskPath struct {
	Glob  string `json:"glob"`
	Level string `json:"level"`
	Why   string `json:"why"`
}

// riskPathFile is the whole of .github/risk-paths.json.
type riskPathFile struct {
	SchemaVersion int        `json:"schema_version"`
	DefaultLevel  string     `json:"default_level"`
	Paths         []riskPath `json:"paths"`
}

// riskSchemaVersion is the version of the list this repository holds
// (frostyard/core docs/specs/risk-paths.md, ADR-0078).
const riskSchemaVersion = 2

// riskListPath is the list itself, which rule C of frostyard/core
// docs/specs/risk-paths.md requires a glob for.
const riskListPath = ".github/risk-paths.json"

// riskCaller is the caller workflow, which rule C requires a glob for.
const riskCaller = ".github/workflows/risk-paths.yml"

// riskLevels are the levels of frostyard/core docs/specs/risk-paths.md, from
// the strictest to the loosest (ADR-0078).
var riskLevels = []string{"L1", "L2", "L3", "L4"}

// levelRank returns the rank of level, 1 for the strictest L1, or 0 when level
// is not 1 of riskLevels.
func levelRank(level string) int {
	for i, l := range riskLevels {
		if l == level {
			return i + 1
		}
	}
	return 0
}

// strictLevel reports whether level is 1 of the 2 strict levels, L1 or L2: the
// levels at which a why names a category (rule B) and a governed path is
// listed.
func strictLevel(level string) bool {
	rank := levelRank(level)
	return rank == 1 || rank == 2
}

// strictestLevel returns the strictest level of the globs of entries that
// match p, and whether 1 glob matched. A level that riskLevels does not hold
// counts as the strictest, so a bad entry fails closed.
func strictestLevel(entries []riskPath, p string) (string, bool) {
	best, bestRank, found := "", 0, false
	for _, e := range entries {
		if !matchGlob(e.Glob, p) {
			continue
		}
		rank := levelRank(e.Level)
		if rank == 0 {
			rank = 1
		}
		if !found || rank < bestRank {
			best, bestRank, found = e.Level, rank, true
		}
	}
	return best, found
}

// riskCategories are the categories of frostyard/core docs/specs/risk-paths.md
// (section B); each why starts with 1 of them and a colon.
var riskCategories = []string{
	"boot chain",
	"signing and keys",
	"update verification",
	"auth",
	"publication",
	"secrets and credentials",
	"rulesets and settings",
	"destructive operations",
	"governance",
}

// whyCategory reports whether why starts with 1 of riskCategories and a colon.
func whyCategory(why string) bool {
	category, _, ok := strings.Cut(why, ":")
	return ok && slices.Contains(riskCategories, category)
}

// governedPaths are the release, secrets, auth and governance paths of this
// repository; .github/risk-paths.json must list each of them.
var governedPaths = []string{
	".github/risk-paths.json",
	".github/workflows/risk-paths.yml",
	"Makefile",
	"mise.toml",
	"mise.lock",
	".svu.yml",
	"internal/workflowcheck/baseline_test.go",
	"internal/repocheck/testfilter_test.go",
	"packaging/flatpak/org.frostyard.Frostnotes.json",
	"packaging/flatpak/frostnotes.sh",
	"packaging/nfpm.yaml",
	"packaging/systemd/frostnotes.service",
	"packaging/sysusers.d/frostnotes.conf",
	"packaging/lodge/app.json",
	"scripts/move-hub-behind-lodge.sh",
	"dev/nsl/provision.sh",
	"internal/secrets/secretservice.go",
	"internal/hub/hub.go",
	"internal/hub/pair.go",
	"internal/hub/withdraw.go",
	"internal/hub/relocate.go",
	"internal/syncer/relocate.go",
	"api/api.go",
	"api/conn.go",
	"internal/mcpserver/http.go",
	"internal/devgw/devgw.go",
	"internal/devgw/blobs.go",
	"internal/blobhttp/blobhttp.go",
	"app/src-tauri/src/open.rs",
	"internal/vault/path.go",
	"internal/vault/guard.go",
	"api/scope.go",
	"internal/lodgeapp/lodgeapp.go",
	"internal/render/sanitize.go",
	"app/src-tauri/tauri.conf.json",
	"app/src/reading/sanitize.ts",
	"internal/store/migrations/0001_init.sql",
}

// riskRuleProblems returns each problem of rule C of frostyard/core
// docs/specs/risk-paths.md: a glob must cover the list and the caller
// workflow, and each glob that matches 1 of the 2 is at L1 or L2.
func riskRuleProblems(entries []riskPath) []string {
	var problems []string
	for _, required := range []string{riskListPath, riskCaller} {
		if _, ok := strictestLevel(entries, required); !ok {
			problems = append(problems, fmt.Sprintf("no glob matches %s, which rule C requires", required))
		}
		for i, p := range entries {
			if matchGlob(p.Glob, required) && !strictLevel(p.Level) {
				problems = append(problems, fmt.Sprintf("entry %d: glob %q matches %s, so its level must be L1 or L2, not %q (rule C)", i, p.Glob, required, p.Level))
			}
		}
	}
	return problems
}

// riskPathProblems returns each problem of the risk list cfg, and no problem
// for a valid version 2 list (frostyard/core docs/specs/risk-paths.md): the
// schema version, the default level, every entry's glob, level and why, and
// rule C. The paths of the repository are not read here:
// TestRiskPaths checks those.
func riskPathProblems(cfg riskPathFile) []string {
	var problems []string
	add := func(format string, args ...any) {
		problems = append(problems, fmt.Sprintf(format, args...))
	}
	if cfg.SchemaVersion != riskSchemaVersion {
		add("schema_version = %d, want %d", cfg.SchemaVersion, riskSchemaVersion)
	}
	if levelRank(cfg.DefaultLevel) == 0 {
		add("default_level = %q, want 1 of %q", cfg.DefaultLevel, riskLevels)
	}
	if len(cfg.Paths) == 0 {
		add("the list holds no path")
	}
	seen := make(map[string]bool, len(cfg.Paths))
	for i, p := range cfg.Paths {
		for _, problem := range validGlob(p.Glob) {
			add("entry %d: glob %q: %s", i, p.Glob, problem)
		}
		if seen[p.Glob] {
			add("entry %d: glob %q appears twice", i, p.Glob)
		}
		seen[p.Glob] = true
		if strings.TrimSpace(p.Why) == "" {
			add("entry %d: glob %q has no why", i, p.Glob)
		}
		if n := utf8.RuneCountInString(p.Why); n > 200 {
			add("entry %d: glob %q has a why of %d characters, want 200 at most", i, p.Glob, n)
		}
		switch {
		case levelRank(p.Level) == 0:
			add("entry %d: glob %q has the level %q, want 1 of %q", i, p.Glob, p.Level, riskLevels)
		case strictLevel(p.Level) && !whyCategory(p.Why):
			add("entry %d: glob %q is at %s, so its why %q must start with 1 of the categories %q and a colon", i, p.Glob, p.Level, p.Why, riskCategories)
		}
	}
	return append(problems, riskRuleProblems(cfg.Paths)...)
}

// TestRiskPaths guards row B6 of frostyard/core
// docs/specs/repository-baseline.md: .github/risk-paths.json is a valid
// version 2 list that holds every governed path of the repository at L1 or
// L2, with a reason, and no stale glob.
func TestRiskPaths(t *testing.T) {
	data, err := os.ReadFile(filepath.Join("..", "..", ".github", "risk-paths.json"))
	if err != nil {
		t.Fatalf(".github/risk-paths.json: %v", err)
	}
	var cfg riskPathFile
	if err := json.Unmarshal(data, &cfg, json.RejectUnknownMembers(true)); err != nil {
		t.Fatalf(".github/risk-paths.json: %v", err)
	}
	for _, problem := range riskPathProblems(cfg) {
		t.Errorf(".github/risk-paths.json: %s", problem)
	}

	globs := make([]string, 0, len(cfg.Paths))
	for _, p := range cfg.Paths {
		globs = append(globs, p.Glob)
	}
	want := append([]string{}, governedPaths...)
	workflows, err := filepath.Glob(filepath.Join("..", "..", ".github", "workflows", "*"))
	if err != nil {
		t.Fatal(err)
	}
	for _, p := range workflows {
		want = append(want, path.Join(".github/workflows", filepath.Base(p)))
	}
	for _, p := range want {
		level, ok := strictestLevel(cfg.Paths, p)
		switch {
		case !ok:
			t.Errorf("%s is a release, secrets, auth or governance path; .github/risk-paths.json must list it", p)
		case !strictLevel(level):
			t.Errorf("%s is a release, secrets, auth or governance path; its globs put it at %s, want L1 or L2", p, level)
		}
	}

	files := repoFiles(t)
	for _, g := range globs {
		if !anyMatch(g, files) {
			t.Errorf("glob %q matches no file in the repository", g)
		}
	}
	t.Logf("%d globs; %d required paths listed; %d files searched", len(globs), len(want), len(files))
}

// anyMatch reports whether glob matches 1 of paths.
func anyMatch(glob string, paths []string) bool {
	for _, p := range paths {
		if matchGlob(glob, p) {
			return true
		}
	}
	return false
}

// maxBraceExpansions is the most globs 1 brace group set may give
// (frostyard/core scripts/lib/risk-paths.mjs).
const maxBraceExpansions = 256

// matchGlob reports whether glob matches the whole slash-separated path
// (frostyard/core docs/specs/risk-paths.md): {a,b} is 1 of the alternatives,
// ** as a whole segment is 0 or more whole segments, * and ? never cross a
// slash, and every other character is literal. An empty path matches nothing,
// and a glob that cannot be expanded matches nothing.
func matchGlob(glob, p string) bool {
	if p == "" {
		return false
	}
	expansions, errs := expandBraces(glob)
	if len(errs) > 0 {
		return false
	}
	pathSegments := strings.Split(p, "/")
	for _, expansion := range expansions {
		if matchSegments(strings.Split(expansion, "/"), pathSegments) {
			return true
		}
	}
	return false
}

// expandBraces returns 1 expansion of glob for each combination of its brace
// groups, and the reasons glob cannot be expanded; a glob with a reason expands
// to nothing. Groups do not nest.
func expandBraces(glob string) (expansions []string, errs []string) {
	var parts [][]string
	literal := ""
	for i := 0; i < len(glob); {
		switch glob[i] {
		case '}':
			errs = append(errs, "unmatched '}'")
			i++
		case '{':
			rest := glob[i+1:]
			close, nextOpen := strings.IndexByte(rest, '}'), strings.IndexByte(rest, '{')
			if close < 0 {
				return nil, append(errs, "unmatched '{'")
			}
			if nextOpen >= 0 && nextOpen < close {
				return nil, append(errs, "nested braces are not supported")
			}
			alternatives := strings.Split(rest[:close], ",")
			if len(alternatives) < 2 {
				errs = append(errs, "a brace group needs at least 2 alternatives")
			}
			if slices.Contains(alternatives, "") {
				errs = append(errs, "a brace group has an empty alternative")
			}
			parts = append(parts, []string{literal}, alternatives)
			literal = ""
			i += close + 2
		default:
			literal += glob[i : i+1]
			i++
		}
	}
	if len(errs) > 0 {
		return nil, errs
	}
	parts = append(parts, []string{literal})

	expansions = []string{""}
	for _, alternatives := range parts {
		next := make([]string, 0, len(expansions)*len(alternatives))
		for _, prefix := range expansions {
			for _, alternative := range alternatives {
				next = append(next, prefix+alternative)
			}
		}
		if len(next) > maxBraceExpansions {
			return nil, []string{fmt.Sprintf("brace expansion gives more than %d globs", maxBraceExpansions)}
		}
		expansions = next
	}
	return expansions, nil
}

// matchSegments reports whether the path segments match the glob segments,
// where a ** segment is 0 or more whole segments.
func matchSegments(globSegments, pathSegments []string) bool {
	patterns := make([]*regexp.Regexp, len(globSegments))
	for i, segment := range globSegments {
		if segment != "**" {
			patterns[i] = segmentPattern(segment)
		}
	}
	memo := make(map[[2]int]bool, len(globSegments)*len(pathSegments)+1)
	var visit func(g, p int) bool
	visit = func(g, p int) bool {
		if match, ok := memo[[2]int{g, p}]; ok {
			return match
		}
		var match bool
		switch {
		case g == len(patterns):
			match = p == len(pathSegments)
		case patterns[g] == nil:
			match = visit(g+1, p) || (p < len(pathSegments) && visit(g, p+1))
		default:
			match = p < len(pathSegments) && patterns[g].MatchString(pathSegments[p]) && visit(g+1, p+1)
		}
		memo[[2]int{g, p}] = match
		return match
	}
	return visit(0, 0)
}

// segmentPattern compiles 1 glob segment: * is any run of characters within the
// segment, ? is 1 such character, and every other character is literal.
func segmentPattern(segment string) *regexp.Regexp {
	var b strings.Builder
	b.WriteString("^")
	for _, c := range segment {
		switch c {
		case '*':
			b.WriteString(`[^/]*`)
		case '?':
			b.WriteString(`[^/]`)
		default:
			b.WriteString(regexp.QuoteMeta(string(c)))
		}
	}
	b.WriteString("$")
	return regexp.MustCompile(b.String())
}

// validGlob returns the reasons glob is not a path glob of frostyard/core
// docs/specs/risk-paths.md; no reason means a valid glob.
func validGlob(glob string) []string {
	if glob == "" {
		return []string{"glob is empty"}
	}
	var errs []string
	for _, c := range []string{"[", "]", "!", `\`} {
		if strings.Contains(glob, c) {
			errs = append(errs, fmt.Sprintf("%q is not supported", c))
		}
	}
	expansions, braceErrs := expandBraces(glob)
	errs = append(errs, braceErrs...)
	for _, expansion := range expansions {
		if strings.HasPrefix(expansion, "/") {
			errs = append(errs, "a glob must not start with '/'")
		}
		if strings.HasPrefix(expansion, "./") {
			errs = append(errs, "a glob must not start with './'")
		}
		for _, segment := range strings.Split(expansion, "/") {
			switch {
			case segment == "":
				errs = append(errs, "a glob must not have an empty segment")
			case segment == "..":
				errs = append(errs, "a glob must not have a '..' segment")
			case strings.Contains(segment, "**") && segment != "**":
				errs = append(errs, "'**' must be a whole segment")
			}
		}
	}
	return unique(errs)
}

// unique returns list without a repeated entry, in the order of 1st appearance.
func unique(list []string) []string {
	seen := make(map[string]bool, len(list))
	out := make([]string, 0, len(list))
	for _, s := range list {
		if !seen[s] {
			seen[s] = true
			out = append(out, s)
		}
	}
	return out
}

// TestValidGlob checks the glob validation of frostyard/core
// docs/specs/risk-paths.md on the cases a repository's list can hold.
func TestValidGlob(t *testing.T) {
	for _, glob := range []string{"a/**", "{a,b}/c", "**"} {
		if problems := validGlob(glob); len(problems) > 0 {
			t.Errorf("validGlob(%q) = %v, want no problem", glob, problems)
		}
	}
	for _, glob := range []string{
		"internal/secrets/{sealed.go,}",
		"{a}",
		"{a,{b,c}}",
		"a}",
		"{a,b",
		"[ab]",
		"!a",
		`a\b`,
		"a**",
		"a/**b",
		"/a",
		"./a",
		"a//b",
		"a/../b",
		strings.Repeat("{a,b}", 9),
	} {
		if problems := validGlob(glob); len(problems) == 0 {
			t.Errorf("validGlob(%q) = no problem, want at least 1", glob)
		}
	}
}

// TestStrictestLevel checks the strictest level of the globs that match a
// path.
func TestStrictestLevel(t *testing.T) {
	for _, tc := range []struct {
		name    string
		entries []riskPath
		path    string
		want    string
		found   bool
	}{
		{"no match", []riskPath{{Glob: "auth/**", Level: "L1"}}, "src/a.go", "", false},
		{"1 match", []riskPath{{Glob: "auth/**", Level: "L2"}}, "auth/a.go", "L2", true},
		{"2 matches, the strictest wins", []riskPath{{Glob: "auth/**", Level: "L2"}, {Glob: "**", Level: "L1"}}, "auth/a.go", "L1", true},
		{"a looser glob listed first", []riskPath{{Glob: "**", Level: "L3"}, {Glob: "auth/**", Level: "L2"}}, "auth/a.go", "L2", true},
		{"an unknown level counts as the strictest", []riskPath{{Glob: "auth/**", Level: "L2"}, {Glob: "auth/*.go", Level: "L5"}}, "auth/a.go", "L5", true},
	} {
		got, found := strictestLevel(tc.entries, tc.path)
		if got != tc.want || found != tc.found {
			t.Errorf("%s: strictestLevel(%v, %q) = %q, %v, want %q, %v", tc.name, tc.entries, tc.path, got, found, tc.want, tc.found)
		}
	}
}

// TestLevelRank checks the rank of a level, from the strictest.
func TestLevelRank(t *testing.T) {
	for level, want := range map[string]int{"L1": 1, "L2": 2, "L3": 3, "L4": 4, "L5": 0, "": 0, "l1": 0} {
		if got := levelRank(level); got != want {
			t.Errorf("levelRank(%q) = %d, want %d", level, got, want)
		}
	}
	for level, want := range map[string]bool{"L1": true, "L2": true, "L3": false, "L4": false, "": false} {
		if got := strictLevel(level); got != want {
			t.Errorf("strictLevel(%q) = %v, want %v", level, got, want)
		}
	}
}

// TestRiskPathProblems checks the validation of the risk list, and rule C, on
// lists held in memory. Each case has 1 cause: a valid list gives no problem,
// and each other case gives exactly 1 problem, which holds want.
func TestRiskPathProblems(t *testing.T) {
	governance := []riskPath{
		{Glob: ".github/risk-paths.json", Level: "L2", Why: "governance: the high-risk path list"},
		{Glob: ".github/workflows/**", Level: "L2", Why: "governance: CI and the caller"},
	}
	with := func(extra ...riskPath) []riskPath {
		return append(slices.Clone(governance), extra...)
	}
	valid := riskPathFile{SchemaVersion: 2, DefaultLevel: "L3", Paths: governance}
	for _, tc := range []struct {
		name string
		cfg  riskPathFile
		want string
	}{
		{"valid v2", valid, ""},
		{"a level outside the 4", riskPathFile{SchemaVersion: 2, DefaultLevel: "L3", Paths: with(riskPath{Glob: "auth/**", Level: "L5", Why: "auth: a token check"})}, `has the level "L5"`},
		{"no default_level", riskPathFile{SchemaVersion: 2, Paths: governance}, `default_level = ""`},
		{"a bad default_level", riskPathFile{SchemaVersion: 2, DefaultLevel: "L0", Paths: governance}, `default_level = "L0"`},
		{"schema_version 1", riskPathFile{SchemaVersion: 1, DefaultLevel: "L3", Paths: governance}, "schema_version = 1, want 2"},
		{"an L3 entry with free text", riskPathFile{SchemaVersion: 2, DefaultLevel: "L3", Paths: with(riskPath{Glob: "docs/how-to/**", Level: "L3", Why: "how-to documentation"})}, ""},
		{"an L1 entry with free text", riskPathFile{SchemaVersion: 2, DefaultLevel: "L3", Paths: with(riskPath{Glob: "signing/**", Level: "L1", Why: "the signing keys"})}, "must start with 1 of the categories"},
		{"an L3 glob for the list itself", riskPathFile{SchemaVersion: 2, DefaultLevel: "L3", Paths: []riskPath{{Glob: ".github/risk-paths.json", Level: "L3", Why: "governance: the list"}, {Glob: ".github/workflows/**", Level: "L2", Why: "governance: CI"}}}, "must be L1 or L2"},
		{"no glob for the caller", riskPathFile{SchemaVersion: 2, DefaultLevel: "L3", Paths: []riskPath{{Glob: ".github/risk-paths.json", Level: "L2", Why: "governance: the list"}}}, "no glob matches .github/workflows/risk-paths.yml"},
		{"a why of 201 characters", riskPathFile{SchemaVersion: 2, DefaultLevel: "L3", Paths: with(riskPath{Glob: "auth/**", Level: "L1", Why: "auth: " + strings.Repeat("x", 195)})}, "has a why of 201 characters"},
	} {
		problems := riskPathProblems(tc.cfg)
		switch {
		case tc.want == "" && len(problems) > 0:
			t.Errorf("%s: riskPathProblems = %v, want none", tc.name, problems)
		case tc.want != "" && len(problems) != 1:
			t.Errorf("%s: riskPathProblems = %v, want exactly 1 problem", tc.name, problems)
		case tc.want != "" && !slices.ContainsFunc(problems, func(problem string) bool { return strings.Contains(problem, tc.want) }):
			t.Errorf("%s: riskPathProblems = %v, want 1 problem that holds %q", tc.name, problems, tc.want)
		}
	}
}

// TestWhyCategory checks that a why names 1 category of frostyard/core
// docs/specs/risk-paths.md, before a colon.
func TestWhyCategory(t *testing.T) {
	for why, want := range map[string]bool{
		"auth: the hub's token check":    true,
		"destructive operations: a wipe": true,
		"security: the sanitizer":        false,
		"auth the hub's token check":     false,
		"Auth: the hub's token check":    false,
		"governance":                     false,
		" governance: the list":          false,
	} {
		if got := whyCategory(why); got != want {
			t.Errorf("whyCategory(%q) = %v, want %v", why, got, want)
		}
	}
}

// TestMatchGlob checks the glob rules of frostyard/core
// docs/specs/risk-paths.md on its own cases and the ones this repository's
// list holds.
func TestMatchGlob(t *testing.T) {
	for _, tc := range []struct {
		glob string
		path string
		want bool
	}{
		{"**/x", "x", true},
		{"**/x", "a/x", true},
		{"**/x", "a/b/x", true},
		{"**/x", "a/x/y", false},
		{"**", "x", true},
		{"**", "a/b", true},
		{"**", ".github/x.yml", true},
		{"x*", "x", true},
		{"x*", "xy", true},
		{"x*", "a/x", false},
		{"x*", "a/xy", false},
		{"docs/*.md", "docs/a.md", true},
		{"docs/*.md", "docs/.md", true},
		{"docs/*.md", "docs/a/b.md", false},
		{"docs/*.md", "src/a.md", false},
		{"shared/**", "shared/a", true},
		{"shared/**", "shared/a/b", true},
		{"shared/**", "other/a", false},
		{".github/{risk-paths.json,settings.yml}", ".github/risk-paths.json", true},
		{".github/{risk-paths.json,settings.yml}", ".github/settings.yml", true},
		{".github/{risk-paths.json,settings.yml}", ".github/workflows/ci.yml", false},
		{".github/workflows/**", ".github/workflows/ci.yml", true},
		{".github/workflows/**", ".github/risk-paths.json", false},
		{"packaging/lodge/**", "packaging/lodge/a/b.sh", true},
		{"packaging/flatpak/{org.frostyard.Frostnotes.json,frostnotes.sh}", "packaging/flatpak/frostnotes.sh", true},
		{"packaging/flatpak/{org.frostyard.Frostnotes.json,frostnotes.sh}", "packaging/flatpak/smoke-test.sh", false},
		{"internal/hub/{hub,pair,withdraw,relocate}.go", "internal/hub/relocate.go", true},
		{"internal/hub/{hub,pair,withdraw,relocate}.go", "internal/hub/relocate_test.go", false},
		{"a/**/x", "a/x", true},
		{"a/**/x", "a/b/x", true},
		{"{app/src-tauri/tauri.conf.json,app/src-tauri/capabilities/**}", "app/src-tauri/capabilities/default.json", true},
		{"{app/src-tauri/tauri.conf.json,app/src-tauri/capabilities/**}", "app/src-tauri/tauri.conf.json", true},
		{"{app/src-tauri/tauri.conf.json,app/src-tauri/capabilities/**}", "app/src-tauri/Cargo.toml", false},
		{"{mise.toml,mise.lock}", "mise.toml", true},
		{"{mise.toml,mise.lock}", "mise.lock", true},
		{"{mise.toml,mise.lock}", "mise.tom", false},
		{"scripts/{a,b}.sh", "scripts/a.sh", true},
		{"scripts/{a,b}.sh", "scripts/c.sh", false},
		{"Makefile", "Makefile", true},
		{"Makefile", "app/Makefile", false},
		{"a/*", "a/b", true},
		{"a/*", "a/b/c", false},
		{"**", "", false},
		{"*.md", "", false},
	} {
		if got := matchGlob(tc.glob, tc.path); got != tc.want {
			t.Errorf("matchGlob(%q, %q) = %v, want %v", tc.glob, tc.path, got, tc.want)
		}
	}
}

// dependabotUpdate is 1 entry of the updates list of
// .github/dependabot.yml.
type dependabotUpdate struct {
	Ecosystem string             `yaml:"package-ecosystem"`
	Directory string             `yaml:"directory"`
	Ignore    []dependabotIgnore `yaml:"ignore"`
}

// dependabotIgnore is 1 entry of an update's ignore list; an ignore with
// versions or update types leaves the other updates of the dependency on.
type dependabotIgnore struct {
	Name        string   `yaml:"dependency-name"`
	Versions    []string `yaml:"versions"`
	UpdateTypes []string `yaml:"update-types"`
}

// dependabotFile is the whole of .github/dependabot.yml.
type dependabotFile struct {
	Version int                `yaml:"version"`
	Updates []dependabotUpdate `yaml:"updates"`
}

// ecosystem names the Dependabot package-ecosystem of the manifest file
// base, if base is a manifest.
func ecosystem(base string) (string, bool) {
	switch base {
	case "go.mod":
		return "gomod", true
	case "package.json":
		return "npm", true
	case "Cargo.toml":
		return "cargo", true
	case "Dockerfile", "Containerfile":
		return "docker", true
	}
	return "", false
}

// normalizeDir drops the trailing slash of a Dependabot directory, and keeps
// the root as "/".
func normalizeDir(dir string) string {
	if dir == "/" {
		return dir
	}
	return strings.TrimSuffix(dir, "/")
}

// TestDependabotCoversManifests guards row B13 of frostyard/core
// docs/specs/repository-baseline.md: .github/dependabot.yml has 1 entry for
// each manifest of the repository, and the github-actions entry at "/".
func TestDependabotCoversManifests(t *testing.T) {
	cfg := readDependabot(t)
	if cfg.Version != 2 {
		t.Errorf("version = %d, want 2", cfg.Version)
	}
	covered := make(map[[2]string]bool, len(cfg.Updates))
	actions := false
	for _, u := range cfg.Updates {
		dir := normalizeDir(u.Directory)
		covered[[2]string{u.Ecosystem, dir}] = true
		if u.Ecosystem == "github-actions" && dir == "/" {
			actions = true
		}
	}
	if !actions {
		t.Error("no github-actions entry with directory /")
	}

	manifests := 0
	for _, f := range repoFiles(t, "third_party", "docs") {
		eco, ok := ecosystem(path.Base(f))
		if !ok {
			continue
		}
		manifests++
		dir := path.Dir(f)
		want := "/"
		if dir != "." {
			want = "/" + dir
		}
		if !covered[[2]string{eco, want}] {
			t.Errorf("%s has no Dependabot entry (package-ecosystem %s, directory %s)", f, eco, want)
		}
	}
	if manifests == 0 {
		t.Fatal("no dependency manifest found")
	}
	t.Logf("%d manifests, %d Dependabot entries", manifests, len(cfg.Updates))
}

// readDependabot reads and parses .github/dependabot.yml.
func readDependabot(t *testing.T) dependabotFile {
	t.Helper()
	data, err := os.ReadFile(filepath.Join("..", "..", ".github", "dependabot.yml"))
	if err != nil {
		t.Fatalf(".github/dependabot.yml: %v", err)
	}
	var cfg dependabotFile
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		t.Fatalf(".github/dependabot.yml: %v", err)
	}
	return cfg
}

// internalGitCrates returns the name of each package of the Cargo.lock
// text data whose source is a git repository of the frostyard organization
// on GitHub, in the order of the file. The lock file, not Cargo.toml, is
// read: cargo writes it in 1 fixed form, it names each git package once by
// its package name (a renamed dependency too), whatever TOML form or section
// Cargo.toml declares it in, and its source is a URL. Each [[package]] has
// its name before its source; a source with no name, or a line of a
// [[package]] that is no key = value outside its dependencies array, is an
// error.
func internalGitCrates(data string) ([]string, error) {
	var crates []string
	inPackage, inArray, name := false, false, ""
	n := 0
	for line := range strings.Lines(data) {
		n++
		line = strings.TrimSpace(line)
		if inArray {
			// The members of a multi-line array, such as dependencies:
			// quoted package names, 1 for each line, up to its ].
			inArray = line != "]"
			continue
		}
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		if strings.HasPrefix(line, "[") {
			inPackage, name = line == "[[package]]", ""
			continue
		}
		if !inPackage {
			continue
		}
		key, value, ok := strings.Cut(line, " = ")
		if !ok {
			return nil, fmt.Errorf("line %d: %q is no key = value", n, line)
		}
		switch key {
		case "dependencies":
			inArray = value == "["
		case "name":
			str, err := strconv.Unquote(value)
			if err != nil {
				return nil, fmt.Errorf("line %d: name %s: %w", n, value, err)
			}
			name = str
		case "source":
			if name == "" {
				return nil, fmt.Errorf("line %d: a source before the package's name", n)
			}
			src, err := strconv.Unquote(value)
			if err != nil {
				return nil, fmt.Errorf("line %d: source %s: %w", n, value, err)
			}
			frostyard, err := frostyardGitSource(src)
			if err != nil {
				return nil, fmt.Errorf("line %d: %w", n, err)
			}
			if frostyard {
				crates = append(crates, name)
			}
		}
	}
	return crates, nil
}

// frostyardGitSource reports whether a Cargo.lock source is a git
// repository whose owner on github.com is frostyard: the host and the first
// path segment of the URL decide, not a substring. A registry or path
// source is not one.
func frostyardGitSource(src string) (bool, error) {
	rest, ok := strings.CutPrefix(src, "git+")
	if !ok {
		return false, nil
	}
	u, err := url.Parse(rest)
	if err != nil {
		return false, fmt.Errorf("source %q: %w", src, err)
	}
	owner, _, _ := strings.Cut(strings.TrimPrefix(u.Path, "/"), "/")
	return strings.EqualFold(u.Hostname(), "github.com") && strings.EqualFold(owner, "frostyard"), nil
}

// cargoIgnores returns the (directory, crate) pairs that the cargo entries
// of cfg ignore completely: an ignore with versions or update types does
// not count.
func cargoIgnores(cfg dependabotFile) map[[2]string]bool {
	ignored := make(map[[2]string]bool)
	for _, u := range cfg.Updates {
		if u.Ecosystem != "cargo" {
			continue
		}
		for _, ig := range u.Ignore {
			if len(ig.Versions) == 0 && len(ig.UpdateTypes) == 0 {
				ignored[[2]string{normalizeDir(u.Directory), ig.Name}] = true
			}
		}
	}
	return ignored
}

// TestDependabotLeavesInternalGitCratesAlone: Dependabot cannot resolve a
// cargo git dependency on a frostyard repository whose crate is not at the
// root of every tag (frostyard-problems, from frostyard/std: its tag v0.3.0
// holds no Cargo.toml, so the update job fails). Such a crate is bumped by
// hand, so the cargo entry of its directory ignores it. The crates come from
// the Cargo.lock beside each Cargo.toml (internalGitCrates).
func TestDependabotLeavesInternalGitCratesAlone(t *testing.T) {
	ignored := cargoIgnores(readDependabot(t))
	crates := 0
	for _, f := range repoFiles(t, "third_party", "docs") {
		if path.Base(f) != "Cargo.toml" {
			continue
		}
		lock := path.Join(path.Dir(f), "Cargo.lock")
		data, err := os.ReadFile(filepath.Join("..", "..", filepath.FromSlash(lock)))
		if err != nil {
			t.Fatalf("%s has no Cargo.lock beside it: %v", f, err)
		}
		dir := "/" + path.Dir(f)
		if path.Dir(f) == "." {
			dir = "/"
		}
		names, err := internalGitCrates(string(data))
		if err != nil {
			t.Fatalf("%s: %v", lock, err)
		}
		for _, name := range names {
			crates++
			if !ignored[[2]string{dir, name}] {
				t.Errorf("%s: the frostyard git crate %s has no ignore entry in the cargo entry for %s", f, name, dir)
			}
		}
	}
	if crates == 0 {
		t.Fatal("no frostyard git crate found; app/src-tauri/Cargo.toml has frostyard-problems")
	}
	t.Logf("%d frostyard git crates checked", crates)
}

// TestInternalGitCrates checks which Cargo.lock packages are frostyard git
// crates, and the lock files it refuses.
func TestInternalGitCrates(t *testing.T) {
	pkg := func(name, source string) string {
		out := "[[package]]\nname = \"" + name + "\"\nversion = \"0.1.0\"\n"
		if source != "" {
			out += "source = \"" + source + "\"\n"
		}
		return out + "\n"
	}
	const head = "# This file is automatically @generated by Cargo.\nversion = 4\n\n"
	for _, tc := range []struct {
		name string
		data string
		want []string
	}{
		{"tag", head + pkg("frostyard-problems", "git+https://github.com/frostyard/std?tag=v0.5.0#f3858d0"), []string{"frostyard-problems"}},
		{"branch, .git, ssh", head + pkg("a", "git+https://github.com/frostyard/a.git?branch=main#0") + pkg("b", "git+ssh://git@github.com/frostyard/b#0"), []string{"a", "b"}},
		{"owner case", head + pkg("c", "git+https://GitHub.com/Frostyard/c#0"), []string{"c"}},
		{"registry and path", head + pkg("serde", "registry+https://github.com/rust-lang/crates.io-index") + pkg("frostnotes", ""), nil},
		{"another owner", head + pkg("d", "git+https://github.com/example/frostyard#0"), nil},
		{"another host with the path", head + pkg("e", "git+https://example.org/github.com/frostyard/e#0"), nil},
		{"the owner in the query", head + pkg("f", "git+https://github.com/example/f?branch=github.com/frostyard/f#0"), nil},
		{"a user-info host", head + pkg("g", "git+https://github.com@example.org/frostyard/g#0"), nil},
		{"metadata section", head + pkg("h", "git+https://github.com/frostyard/h#0") + "[metadata]\nsource = \"git+https://github.com/frostyard/x\"\n", []string{"h"}},
		{"the real lock's form", head + "[[package]]\nname = \"frostnotes\"\nversion = \"0.0.0\"\ndependencies = [\n \"frostyard-problems\",\n \"serde\",\n]\n\n" + pkg("frostyard-problems", "git+https://github.com/frostyard/std?tag=v0.5.0#f3858d0"), []string{"frostyard-problems"}},
	} {
		got, err := internalGitCrates(tc.data)
		if err != nil {
			t.Errorf("%s: %v", tc.name, err)
			continue
		}
		if !slices.Equal(got, tc.want) {
			t.Errorf("%s: internalGitCrates = %q, want %q", tc.name, got, tc.want)
		}
	}
	for _, tc := range []struct{ name, data string }{
		{"a source before the name", "[[package]]\nsource = \"git+https://github.com/frostyard/std#0\"\nname = \"x\"\n"},
		{"an unquoted name", "[[package]]\nname = x\n"},
		{"a line with no value", "[[package]]\nname\n"},
		{"a bad URL", "[[package]]\nname = \"x\"\nsource = \"git+https://github.com/%zz\"\n"},
	} {
		if _, err := internalGitCrates(tc.data); err == nil {
			t.Errorf("%s: internalGitCrates gave no error", tc.name)
		}
	}
}

// TestCargoIgnores checks what counts as a full ignore of a crate.
func TestCargoIgnores(t *testing.T) {
	const crate = "frostyard-problems"
	for _, tc := range []struct {
		name string
		u    dependabotUpdate
		want bool
	}{
		{"full ignore", dependabotUpdate{Ecosystem: "cargo", Directory: "/app/src-tauri", Ignore: []dependabotIgnore{{Name: crate}}}, true},
		{"full ignore, trailing slash", dependabotUpdate{Ecosystem: "cargo", Directory: "/app/src-tauri/", Ignore: []dependabotIgnore{{Name: crate}}}, true},
		{"versions only", dependabotUpdate{Ecosystem: "cargo", Directory: "/app/src-tauri", Ignore: []dependabotIgnore{{Name: crate, Versions: []string{">= 1"}}}}, false},
		{"update types only", dependabotUpdate{Ecosystem: "cargo", Directory: "/app/src-tauri", Ignore: []dependabotIgnore{{Name: crate, UpdateTypes: []string{"version-update:semver-major"}}}}, false},
		{"another ecosystem", dependabotUpdate{Ecosystem: "npm", Directory: "/app/src-tauri", Ignore: []dependabotIgnore{{Name: crate}}}, false},
		{"another directory", dependabotUpdate{Ecosystem: "cargo", Directory: "/web", Ignore: []dependabotIgnore{{Name: crate}}}, false},
	} {
		got := cargoIgnores(dependabotFile{Updates: []dependabotUpdate{tc.u}})[[2]string{"/app/src-tauri", crate}]
		if got != tc.want {
			t.Errorf("%s: ignored = %v, want %v", tc.name, got, tc.want)
		}
	}
}
