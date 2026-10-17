package synctest

import (
	"fmt"
	"maps"
	"testing"
)

func TestScenarioEditDeleteMove(t *testing.T) {
	for _, tc := range []struct {
		order [3]int
		path  string
	}{
		{order: [3]int{0, 1, 2}, path: "g.md"},
		{order: [3]int{0, 2, 1}, path: "g.md"},
		{order: [3]int{1, 0, 2}, path: "g.md"},
		{order: [3]int{1, 2, 0}, path: "g.md"},
		{order: [3]int{2, 0, 1}, path: "g.md"},
		{order: [3]int{2, 1, 0}, path: "f.md"},
	} {
		t.Run(fmt.Sprintf("%d,%d,%d", tc.order[0], tc.order[1], tc.order[2]), func(t *testing.T) {
			w := NewWorld(t, Options{Seed: 3})
			defer w.Close()
			content := scenarioEditDeleteMove(t, w, tc.order)
			want := maps.Clone(seedFiles)
			want[tc.path] = content
			if got := w.Heads(); !maps.Equal(got, want) {
				t.Fatalf("hub's heads = %q, want %q", got, want)
			}
		})
	}
}

func scenarioEditDeleteMove(t *testing.T, w *World, order [3]int) string {
	t.Helper()
	const base = "one\ntwo\n"
	if err := w.Write(w.Devices[0], "f.md", base); err != nil {
		t.Fatal(err)
	}
	if err := w.Settle(); err != nil {
		t.Fatal(err)
	}
	for _, d := range w.Devices {
		w.Offline(d)
	}
	content := base + w.Token() + "\n"
	if err := w.Write(w.Devices[0], "f.md", content); err != nil {
		t.Fatal(err)
	}
	if err := w.Delete(w.Devices[1], "f.md"); err != nil {
		t.Fatal(err)
	}
	if err := w.Move(w.Devices[2], "f.md", "g.md"); err != nil {
		t.Fatal(err)
	}
	for _, i := range order {
		w.Online(w.Devices[i])
		if err := w.SettleOnline(); err != nil {
			t.Fatal(err)
		}
	}
	if err := w.Settle(); err != nil {
		t.Fatal(err)
	}
	if err := w.Check(); err != nil {
		t.Fatal(err)
	}
	return content
}

func TestScenarioContentionProfile(t *testing.T) {
	profile := Profile{
		Weights: map[Op]int{Edit: 6, Delete: 3, Move: 3, StaleEdit: 3, Offline: 4, Online: 5, Create: 1},
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
