package synctest

import (
	"maps"
	"strings"
	"testing"
)

func TestScenarioJoinWithAnOlderCopy(t *testing.T) {
	w := NewWorld(t, Options{Seed: 5})
	defer w.Close()
	d0 := w.Devices[0]
	old := w.Snapshot(d0)
	tok := w.Token()
	if err := w.Write(d0, "Inbox.md", "# Inbox\n\n- start\n"+tok+"\n"); err != nil {
		t.Fatal(err)
	}
	if err := w.Delete(d0, "Journal/day.md"); err != nil {
		t.Fatal(err)
	}
	if err := w.Move(d0, "Projects/Plan.md", "Plan.md"); err != nil {
		t.Fatal(err)
	}
	if err := w.Settle(); err != nil {
		t.Fatal(err)
	}
	d3, err := w.AddDevice("d3", old)
	if err != nil {
		t.Fatal(err)
	}
	if err := w.Settle(); err != nil {
		t.Fatal(err)
	}
	if err := w.Check(); err != nil {
		t.Fatal(err)
	}
	heads := w.Heads()
	if got := Files(d3); !maps.Equal(got, heads) {
		t.Fatalf("new device's files = %q, want hub's heads %q", got, heads)
	}
	if !strings.Contains(heads["Inbox.md"], tok) {
		t.Fatalf("hub's Inbox.md = %q, want token %s", heads["Inbox.md"], tok)
	}
	for _, p := range []string{"Journal/day.md", "Projects/Plan.md"} {
		if _, ok := heads[p]; ok {
			t.Fatalf("hub still has a head at %s", p)
		}
	}
	if _, ok := heads["Plan.md"]; !ok {
		t.Fatal("hub has no head at Plan.md")
	}
	if len(heads) != 3 {
		t.Fatalf("hub has %d files, want 3: %q", len(heads), heads)
	}
}

func TestScenarioJoinWithAnEditOfItsOwn(t *testing.T) {
	w := NewWorld(t, Options{Seed: 5})
	defer w.Close()
	d1 := w.Devices[1]
	old := w.Snapshot(d1)
	before := w.Heads()
	tok := w.Token()
	old["Inbox.md"] += tok + "\n"
	if _, err := w.AddDevice("d3", old); err != nil {
		t.Fatal(err)
	}
	if err := w.Settle(); err != nil {
		t.Fatal(err)
	}
	if err := w.Check(); err != nil {
		t.Fatal(err)
	}
	heads := w.Heads()
	if !strings.Contains(heads["Inbox.md"], tok) {
		t.Fatalf("hub's Inbox.md = %q, want token %s", heads["Inbox.md"], tok)
	}
	if f, ok := w.HubFile("Inbox.md"); !ok || f.Conflicted {
		t.Fatalf("hub's Inbox.md: found %v, conflicted %v; want true, false", ok, f.Conflicted)
	}
	if len(heads) != 4 {
		t.Fatalf("hub has %d files, want 4: %q", len(heads), heads)
	}
	for p := range before {
		if _, ok := heads[p]; !ok {
			t.Fatalf("hub lost the path %s", p)
		}
	}
}

func TestScenarioResetKeepsAChange(t *testing.T) {
	w := NewWorld(t, Options{Seed: 5})
	defer w.Close()
	d1 := w.Devices[1]
	w.Offline(d1)
	tok := w.Token()
	if err := w.Write(d1, "Inbox.md", Files(d1)["Inbox.md"]+tok+"\n"); err != nil {
		t.Fatal(err)
	}
	w.Online(d1)
	if err := w.Reset(d1); err != nil {
		t.Fatal(err)
	}
	if err := w.Settle(); err != nil {
		t.Fatal(err)
	}
	if err := w.Check(); err != nil {
		t.Fatal(err)
	}
	if got := w.Heads()["Inbox.md"]; !strings.Contains(got, tok) {
		t.Fatalf("hub's Inbox.md = %q, want token %s", got, tok)
	}
}

func TestScenarioRejoinProfile(t *testing.T) {
	profile := Profile{
		Weights: map[Op]int{Edit: 6, Create: 3, Delete: 2, Move: 2, StaleEdit: 2,
			Snapshot: 2, Rejoin: 1, Reset: 1, Offline: 2, Online: 3},
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
