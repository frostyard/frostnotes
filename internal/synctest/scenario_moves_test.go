package synctest

import (
	"fmt"
	"strings"
	"testing"
)

func TestScenarioMoveChain(t *testing.T) {
	w := NewWorld(t, Options{Seed: 1})
	defer w.Close()
	d0, d1 := w.Devices[0], w.Devices[1]
	if err := w.Write(d0, "n.md", "# n\n"); err != nil {
		t.Fatal(err)
	}
	if err := w.Settle(); err != nil {
		t.Fatal(err)
	}
	w.Offline(d1)
	for _, mv := range [][2]string{{"n.md", "a/n.md"}, {"a/n.md", "b/n.md"}} {
		if err := w.Move(d0, mv[0], mv[1]); err != nil {
			t.Fatal(err)
		}
		if err := w.SettleOnline(); err != nil {
			t.Fatal(err)
		}
	}
	want := "# n\n" + w.Token() + "\n"
	if err := w.Write(d1, "n.md", want); err != nil {
		t.Fatal(err)
	}
	if err := w.Settle(); err != nil {
		t.Fatal(err)
	}
	checkScenarioMoveHeads(t, w, map[string]string{"b/n.md": want}, "n.md", "a/n.md")
	if err := w.Check(); err != nil {
		t.Fatal(err)
	}
}

func TestScenarioSwapWhileAnotherEdits(t *testing.T) {
	w := NewWorld(t, Options{Seed: 2})
	defer w.Close()
	scenarioSwap(t, w, false)
	if err := w.Check(); err != nil {
		t.Fatal(err)
	}
}

func TestScenarioSwapInOnePush(t *testing.T) {
	w := NewWorld(t, Options{Seed: 3})
	defer w.Close()
	scenarioSwap(t, w, true)
	if err := w.Check(); err != nil {
		t.Fatal(err)
	}
}

func scenarioSwap(t *testing.T, w *World, onePush bool) {
	t.Helper()
	d0, d1 := w.Devices[0], w.Devices[1]
	if err := w.Write(d0, "x.md", "X\n"); err != nil {
		t.Fatal(err)
	}
	if err := w.Write(d0, "y.md", "Y\n"); err != nil {
		t.Fatal(err)
	}
	if err := w.Settle(); err != nil {
		t.Fatal(err)
	}
	w.Offline(d1)
	if onePush {
		w.Offline(d0)
	}
	for _, mv := range [][2]string{{"x.md", "t.md"}, {"y.md", "x.md"}, {"t.md", "y.md"}} {
		if err := w.Move(d0, mv[0], mv[1]); err != nil {
			t.Fatal(err)
		}
		if !onePush {
			if err := w.SettleOnline(); err != nil {
				t.Fatal(err)
			}
		}
	}
	x, y := "X\n"+w.Token()+"\n", "Y\n"+w.Token()+"\n"
	if err := w.Write(d1, "x.md", x); err != nil {
		t.Fatal(err)
	}
	if err := w.Write(d1, "y.md", y); err != nil {
		t.Fatal(err)
	}
	if onePush {
		w.Online(d0)
		w.Online(d1)
	}
	if err := w.Settle(); err != nil {
		t.Fatal(err)
	}
	checkScenarioMoveHeads(t, w, map[string]string{"x.md": y, "y.md": x}, "t.md")
	for p := range w.Heads() {
		if strings.HasSuffix(p, " 1.md") {
			t.Fatalf("swap left a numbered path on the hub: %s", p)
		}
	}
}

func TestScenarioCaseOnlyRename(t *testing.T) {
	w := NewWorld(t, Options{Seed: 4})
	defer w.Close()
	d0, d1 := w.Devices[0], w.Devices[1]
	if err := w.Write(d0, "plan.md", "plan\n"); err != nil {
		t.Fatal(err)
	}
	if err := w.Settle(); err != nil {
		t.Fatal(err)
	}
	w.Offline(d1)
	if err := w.Move(d0, "plan.md", "Plan.md"); err != nil {
		t.Fatal(err)
	}
	want := "plan\n" + w.Token() + "\n"
	if err := w.Write(d1, "plan.md", want); err != nil {
		t.Fatal(err)
	}
	if err := w.Settle(); err != nil {
		t.Fatal(err)
	}
	checkScenarioMoveHeads(t, w, map[string]string{"Plan.md": want}, "plan.md")
	if err := w.Check(); err != nil {
		t.Fatal(err)
	}
}

func TestScenarioCaseOnlyCollision(t *testing.T) {
	w := NewWorld(t, Options{Seed: 5})
	defer w.Close()
	d0, d1 := w.Devices[0], w.Devices[1]
	if err := w.Write(d0, "notes.md", "notes\n"); err != nil {
		t.Fatal(err)
	}
	if err := w.Settle(); err != nil {
		t.Fatal(err)
	}
	w.Offline(d1)
	want := "Notes\n" + w.Token() + "\n"
	if err := w.Write(d1, "Notes.md", want); err != nil {
		t.Fatal(err)
	}
	if err := w.Settle(); err != nil {
		t.Fatal(err)
	}
	checkScenarioMoveHeads(t, w, map[string]string{"notes.md": "notes\n", "Notes 1.md": want}, "Notes.md")
	if err := w.Check(); err != nil {
		t.Fatal(err)
	}
}

func checkScenarioMoveHeads(t *testing.T, w *World, want map[string]string, absent ...string) {
	t.Helper()
	heads := w.Heads()
	for p, content := range want {
		if got, ok := heads[p]; !ok || got != content {
			t.Fatalf("hub's head at %s = %q (present %v), want %q", p, got, ok, content)
		}
	}
	for _, p := range absent {
		if _, ok := heads[p]; ok {
			t.Fatalf("hub still has a head at %s", p)
		}
	}
}

func TestScenarioMovesProfile(t *testing.T) {
	profile := Profile{
		Weights: map[Op]int{Edit: 5, Move: 5, Swap: 5, CaseRename: 5, DiskMove: 2, Create: 2, Offline: 3, Online: 4},
		Check: func(w *World) error {
			for _, n := range append([]*Node{w.Hub}, w.Devices...) {
				for p := range Files(n) {
					if strings.Contains(p, ".sync-") {
						return fmt.Errorf("%s still has a parked file at %s", n.Name, p)
					}
				}
			}
			return nil
		},
	}
	for _, tc := range []struct {
		name       string
		seed       uint64
		concurrent bool
	}{
		{name: "stepwise", seed: 1},
		{name: "concurrent", seed: 2, concurrent: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			const steps = 200
			opts := Options{Seed: tc.seed, Concurrent: tc.concurrent, Profile: &profile}
			if err := Run(t, opts, PlanWith(tc.seed, 3, steps, profile.Weights)); err != nil {
				t.Fatal(err)
			}
		})
	}
}
