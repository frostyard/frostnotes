package workflowcheck

import (
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"go.yaml.in/yaml/v3"
)

// The race gate runs in parts (docs/adr/0048): make race names them, and
// CI's Race detector job runs each on a runner of its own. These tests
// hold the Makefile and ci.yml to each other, so that a part cannot leave
// the gate, and the required check cannot pass, without a failure here.

// makeRule is a Makefile target: the words after its colon, up to a
// comment, and its recipe lines.
type makeRule struct {
	prereqs []string
	recipe  []string
}

// makeRules reads the rules and the simple variables (NAME := value) of a
// Makefile. It knows only the forms the race gate uses.
func makeRules(text string) (map[string]makeRule, map[string]string) {
	rules, vars := map[string]makeRule{}, map[string]string{}
	target := ""
	for _, line := range strings.Split(text, "\n") {
		if strings.HasPrefix(line, "\t") {
			if target != "" {
				r := rules[target]
				r.recipe = append(r.recipe, strings.TrimSpace(line))
				rules[target] = r
			}
			continue
		}
		target = ""
		if name, value, ok := strings.Cut(line, ":="); ok && !strings.ContainsAny(strings.TrimSpace(name), " \t") {
			vars[strings.TrimSpace(name)] = strings.TrimSpace(value)
			continue
		}
		name, rest, ok := strings.Cut(line, ":")
		if !ok || name == "" || strings.ContainsAny(name, " \t#.$") {
			continue
		}
		rest, _, _ = strings.Cut(rest, "#")
		target = name
		rules[name] = makeRule{prereqs: strings.Fields(rest)}
	}
	return rules, vars
}

// raceParts are the parts of the race gate this check knows, and what each
// one's go test must and must not hold. A new part needs its line here.
var raceParts = map[string]struct{ must, mustNot []string }{
	// Every package but RACE_SIM, with the flags of the old single run.
	"rest": {
		must:    []string{`go list ./... | grep -v -x -F "$$(go list $(RACE_SIM))"`, `test -n "$$pkgs"`, "go test -race ", " $$pkgs"},
		mustNot: []string{"-gcflags", "checkptr"},
	},
	// RACE_SIM under the race detector, with checkptr out of modernc.org's
	// packages and nowhere else.
	"sim": {
		must:    []string{"go test -race ", " '-gcflags=modernc.org/...=-d=checkptr=0' ", " $(RACE_SIM)"},
		mustNot: []string{"all="},
	},
	// RACE_SIM with checkptr in every package: the checks the sim part
	// leaves out.
	"sim-checkptr": {
		must:    []string{"go test ", " -gcflags=all=-d=checkptr ", " $(RACE_SIM)"},
		mustNot: []string{"checkptr=0"},
	},
}

// raceGateProblems checks the race gate of a Makefile and of ci.yml's
// jobs: make ci runs make race; make race is exactly the known parts; each
// part's go test covers what it must, with no flag that drops tests; the
// race-shards job runs one shard for each part; and the job named Race
// detector passes only when every shard passed.
func raceGateProblems(makefile string, jobs *yaml.Node) []string {
	var out []string
	bad := func(format string, args ...any) { out = append(out, fmt.Sprintf(format, args...)) }
	rules, vars := makeRules(makefile)

	if !slices.Contains(rules["ci"].prereqs, "race") {
		bad("Makefile: ci does not depend on race: make ci would run no race gate")
	}
	if sim := vars["RACE_SIM"]; !strings.HasPrefix(sim, "./") || strings.ContainsAny(sim, " \t") || strings.Contains(sim, "...") {
		bad("Makefile: RACE_SIM = %q, want one package path", sim)
	}
	if len(rules["race"].recipe) != 0 {
		bad("Makefile: race has a recipe of its own; its parts are its prerequisites, which CI runs as shards")
	}
	var parts []string
	for _, p := range rules["race"].prereqs {
		name, ok := strings.CutPrefix(p, "race-")
		if _, known := raceParts[name]; !ok || !known {
			bad("Makefile: race depends on %s, a part this check does not know", p)
			continue
		}
		parts = append(parts, name)
	}
	for name, want := range raceParts {
		if !slices.Contains(parts, name) {
			bad("Makefile: race does not depend on race-%s", name)
		}
		recipe := rules["race-"+name].recipe
		if len(recipe) != 1 {
			bad("Makefile: race-%s has %d recipe lines, want 1 go test line", name, len(recipe))
			continue
		}
		line := recipe[0] + " "
		for _, s := range want.must {
			if !strings.Contains(line, s) {
				bad("Makefile: race-%s does not hold %q", name, s)
			}
		}
		for _, s := range append([]string{"-short", "-run", "-skip", "-tags", "||"}, want.mustNot...) {
			if strings.Contains(line, s) {
				bad("Makefile: race-%s holds %q", name, s)
			}
		}
	}

	shards := get(jobs, "race-shards")
	if shards == nil {
		return append(out, "ci.yml: no race-shards job")
	}
	matrix := values(get(get(get(shards, "strategy"), "matrix"), "shard"))
	want := slices.Sorted(slices.Values(parts))
	if got := slices.Sorted(slices.Values(matrix)); !slices.Equal(got, want) {
		bad("ci.yml: the race-shards matrix runs %v, and make race runs %v", got, want)
	}
	if env := get(get(shards, "env"), "SHARD"); env == nil || env.Value != "${{ matrix.shard }}" {
		bad("ci.yml: race-shards does not set SHARD to ${{ matrix.shard }}")
	}
	runs := 0
	walk(shards, func(m *yaml.Node) {
		if get(m, "continue-on-error") != nil {
			bad("ci.yml: race-shards sets continue-on-error: a failed shard would count as passed")
		}
		if r := get(m, "run"); r != nil && strings.TrimSpace(r.Value) == `make "race-$SHARD"` {
			runs++
			if get(m, "if") != nil {
				bad("ci.yml:%d: the step that runs the shard has a condition", r.Line)
			}
		}
	})
	if runs != 1 {
		bad("ci.yml: race-shards has %d steps that run `make \"race-$SHARD\"`, want 1", runs)
	}

	race := get(jobs, "race")
	if race == nil {
		return append(out, "ci.yml: no race job")
	}
	if n := get(race, "name"); n == nil || n.Value != "Race detector" {
		bad("ci.yml: the race job is not named \"Race detector\", the name the rulesets require")
	}
	needs := get(race, "needs")
	if needs == nil || (needs.Value != "race-shards" && !slices.Contains(values(needs), "race-shards")) {
		bad("ci.yml: the race job does not need race-shards")
	}
	if cond := get(race, "if"); cond == nil || cond.Value != "always()" {
		bad("ci.yml: the race job does not run if: always(): after a failed or cancelled shard it would be skipped, and a skipped required check counts as passed")
	}
	passes := 0
	walk(race, func(m *yaml.Node) {
		if get(m, "continue-on-error") != nil {
			bad("ci.yml: the race job sets continue-on-error")
		}
		r := get(m, "run")
		if r == nil {
			return
		}
		if get(m, "if") != nil {
			bad("ci.yml:%d: a step of the race job has a condition", r.Line)
		}
		result := get(get(m, "env"), "SHARDS")
		if result != nil && result.Value == "${{ needs.race-shards.result }}" && slices.Contains(strings.Split(r.Value, "\n"), `test "$SHARDS" = success`) {
			passes++
		}
	})
	if passes != 1 {
		bad("ci.yml: the race job has %d steps that pass only when needs.race-shards.result is success, want 1", passes)
	}
	return out
}

func readRaceGate(t *testing.T) (string, string) {
	t.Helper()
	root := filepath.Join("..", "..")
	makefile, err := os.ReadFile(filepath.Join(root, "Makefile"))
	if err != nil {
		t.Fatal(err)
	}
	ci, err := os.ReadFile(filepath.Join(root, ".github", "workflows", "ci.yml"))
	if err != nil {
		t.Fatal(err)
	}
	return string(makefile), string(ci)
}

func ciJobs(t *testing.T, ci string) *yaml.Node {
	t.Helper()
	var doc yaml.Node
	if err := yaml.Unmarshal([]byte(ci), &doc); err != nil || len(doc.Content) == 0 {
		t.Fatalf("parse ci.yml: %v", err)
	}
	return get(doc.Content[0], "jobs")
}

// The Makefile's race gate and CI's Race detector job run the same parts,
// and the required check passes only when all of them passed.
func TestRaceGateRunsEveryPartAsAShard(t *testing.T) {
	makefile, ci := readRaceGate(t)
	for _, p := range raceGateProblems(makefile, ciJobs(t, ci)) {
		t.Error(p)
	}
}

// TestRaceGateProblemsCatchALostPart feeds the check the repository's own
// files with one change each that drops a part of the gate or lets the
// required check pass without it: every one must give a problem.
func TestRaceGateProblemsCatchALostPart(t *testing.T) {
	makefile, ci := readRaceGate(t)
	for _, tc := range []struct {
		name     string
		file     *string
		old, new string
	}{
		{"make ci without the race gate", &makefile, "ci: verify ui-check desktop-build race ", "ci: verify ui-check desktop-build "},
		{"make race without a part", &makefile, "race: race-rest race-sim race-sim-checkptr", "race: race-rest race-sim"},
		{"make race with a part the check does not know", &makefile, "race: race-rest race-sim race-sim-checkptr", "race: race-rest race-sim race-sim-checkptr race-more"},
		{"the rest part without the race detector", &makefile, "go test -race -timeout 25m $$pkgs", "go test -timeout 25m $$pkgs"},
		{"the rest part leaves out more packages", &makefile, `grep -v -x -F "$$(go list $(RACE_SIM))"`, `grep -v -F "$$(go list $(RACE_SIM))"`},
		{"the rest part without checkptr in a package", &makefile, "go test -race -timeout 25m $$pkgs", "go test -race -timeout 25m -gcflags=all=-d=checkptr=0 $$pkgs"},
		{"the sim part without the race detector", &makefile, "go test -race -timeout 25m '-gcflags=modernc.org", "go test -timeout 25m '-gcflags=modernc.org"},
		{"the sim part without checkptr in every package", &makefile, "'-gcflags=modernc.org/...=-d=checkptr=0'", "'-gcflags=all=-d=checkptr=0'"},
		{"the sim part with short tests", &makefile, "go test -race -timeout 25m '-gcflags=modernc.org", "go test -race -short -timeout 25m '-gcflags=modernc.org"},
		{"the checkptr part without checkptr", &makefile, "go test -timeout 25m -gcflags=all=-d=checkptr $(RACE_SIM)", "go test -timeout 25m $(RACE_SIM)"},
		{"the checkptr part on another package", &makefile, "-gcflags=all=-d=checkptr $(RACE_SIM)", "-gcflags=all=-d=checkptr ./internal/merge"},
		{"a part that ignores its failure", &makefile, "-gcflags=all=-d=checkptr $(RACE_SIM)", "-gcflags=all=-d=checkptr $(RACE_SIM) || true"},
		{"RACE_SIM names many packages", &makefile, "RACE_SIM := ./internal/synctest", "RACE_SIM := ./internal/..."},
		{"a shard missing from the matrix", &ci, "shard: [rest, sim, sim-checkptr]", "shard: [rest, sim]"},
		{"a shard that runs another target", &ci, `run: make "race-$SHARD"`, `run: make "race-rest"`},
		{"a shard whose failure is ignored", &ci, "    strategy:\n", "    continue-on-error: true\n    strategy:\n"},
		{"the required check renamed", &ci, "    name: Race detector\n", "    name: Race detector (all)\n"},
		{"the required check without its shards", &ci, "    needs: race-shards\n", "    needs: verify\n"},
		{"the required check skipped after a failure", &ci, "    needs: race-shards\n    if: always()\n", "    needs: race-shards\n"},
		{"the required check passes after a failure", &ci, `test "$SHARDS" = success`, `test "$SHARDS" != cancelled`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			m, c := makefile, ci
			text := &m
			if tc.file == &ci {
				text = &c
			}
			if strings.Count(*text, tc.old) != 1 {
				t.Fatalf("%q is in the file %d times, want 1: the case no longer tests a change", tc.old, strings.Count(*text, tc.old))
			}
			*text = strings.Replace(*text, tc.old, tc.new, 1)
			if p := raceGateProblems(m, ciJobs(t, c)); len(p) == 0 {
				t.Error("no problem found")
			}
		})
	}
}
