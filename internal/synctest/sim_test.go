package synctest

import (
	"log/slog"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

// simOptions reads SIM_SEED, SIM_STEPS and SIM_DEBUG, for running one
// seed by hand.
func simOptions(seed uint64, steps int) Options {
	if s := os.Getenv("SIM_SEED"); s != "" {
		seed, _ = strconv.ParseUint(s, 10, 64)
	}
	if s := os.Getenv("SIM_STEPS"); s != "" {
		steps, _ = strconv.Atoi(s)
	}
	o := Options{Seed: seed, Steps: steps, Watch: os.Getenv("SIM_WATCH")}
	if os.Getenv("SIM_DEBUG") != "" {
		o.Log = slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelDebug}))
	}
	return o
}

// A few hundred stepwise steps on fixed seeds, in make test (ADR-0020);
// make sync-sim runs the long ones.
func TestShortSimulation(t *testing.T) {
	for _, seed := range []uint64{1, 2, 3} {
		o := simOptions(seed, 120)
		if err := Run(t, o, Plan(o.Seed, 3, o.Steps)); err != nil {
			t.Fatalf("seed %d: %v", o.Seed, err)
		}
	}
}

// The reduced plans of failures the simulation found, replayed stepwise
// (ADR-0020): testdata/regressions/*.plan.
func TestRegressions(t *testing.T) {
	paths, err := filepath.Glob(filepath.Join("testdata", "regressions", "*.plan"))
	if err != nil || len(paths) == 0 {
		t.Fatalf("no regression plans: %v", err)
	}
	for _, p := range paths {
		t.Run(strings.TrimSuffix(filepath.Base(p), ".plan"), func(t *testing.T) {
			data, err := os.ReadFile(p)
			if err != nil {
				t.Fatal(err)
			}
			plan, err := ParsePlan(string(data))
			if err != nil {
				t.Fatal(err)
			}
			if err := Run(t, simOptions(0, len(plan)), plan); err != nil {
				t.Fatal(err)
			}
		})
	}
}
