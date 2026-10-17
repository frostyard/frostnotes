//go:build integration

package synctest

import (
	"fmt"
	"log/slog"
	"os"
	"strconv"
	"testing"
	"time"
)

// TestISimulation is make sync-sim (ADR-0020, plan 0004's exit evidence
// 1): SIM_SEEDS seeds (default 10) of SIM_STEPS steps (default 10,000)
// over a hub and three devices, stepwise and concurrent (SIM_MODE picks
// one). A failure is reduced, and its plan printed for testdata.
func TestISimulation(t *testing.T) {
	seeds, steps := 10, 10000
	if s := os.Getenv("SIM_SEEDS"); s != "" {
		seeds, _ = strconv.Atoi(s)
	}
	if s := os.Getenv("SIM_STEPS"); s != "" {
		steps, _ = strconv.Atoi(s)
	}
	first := uint64(1)
	if s := os.Getenv("SIM_FIRST"); s != "" {
		first, _ = strconv.ParseUint(s, 10, 64)
	}
	modes := []bool{false, true}
	switch os.Getenv("SIM_MODE") {
	case "stepwise":
		modes = []bool{false}
	case "concurrent":
		modes = []bool{true}
	}
	for _, concurrent := range modes {
		for seed := first; seed < first+uint64(seeds); seed++ {
			name := fmt.Sprintf("%s/seed %d", map[bool]string{false: "stepwise", true: "concurrent"}[concurrent], seed)
			t.Run(name, func(t *testing.T) {
				o := Options{Seed: seed, Steps: steps, Concurrent: concurrent, Watch: os.Getenv("SIM_WATCH")}
				if os.Getenv("SIM_LOG") != "" {
					o.Log = slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelDebug}))
				}
				plan := Plan(seed, 3, steps)
				begin := time.Now()
				err := Run(t, o, plan)
				t.Logf("%v", time.Since(begin).Round(time.Millisecond))
				if err == nil {
					return
				}
				t.Errorf("seed %d: %v", seed, err)
				if os.Getenv("SIM_REDUCE") != "" {
					small, why := Reduce(t, o, plan, 60)
					t.Errorf("reduced to %d steps: %v\n%s", len(small), why, PlanText(small))
				}
			})
		}
	}
}
