package synctest

import (
	"testing"

	"github.com/frostyard/frostnotes/api"
)

func TestScenarioHubRestart(t *testing.T) {
	w := NewWorld(t, Options{Seed: 6})
	defer w.Close()
	d0, d1 := w.Devices[0], w.Devices[1]
	a, b := w.Token(), w.Token()
	if err := w.Write(d0, "a.md", a+"\n"); err != nil {
		t.Fatal(err)
	}
	w.RestartHub()
	if err := w.Write(d1, "b.md", b+"\n"); err != nil {
		t.Fatal(err)
	}
	if err := w.Settle(); err != nil {
		t.Fatal(err)
	}
	if err := w.Check(); err != nil {
		t.Fatal(err)
	}
	heads := w.Heads()
	for p, want := range map[string]string{"a.md": a + "\n", "b.md": b + "\n"} {
		if got := heads[p]; got != want {
			t.Fatalf("hub's %s = %q, want %q", p, got, want)
		}
		for _, d := range w.Devices {
			if got := Files(d)[p]; got != want {
				t.Fatalf("%s's %s = %q, want %q", d.Name, p, got, want)
			}
		}
	}
}

func TestScenarioRevocationDuringARun(t *testing.T) {
	w := NewWorld(t, Options{Seed: 6})
	defer w.Close()
	d0, d2 := w.Devices[0], w.Devices[2]
	a, c := w.Token(), w.Token()
	if err := w.Revoke(d2); err != nil {
		t.Fatal(err)
	}
	st, err := w.Status(d2)
	if err != nil {
		t.Fatal(err)
	}
	if st.State != api.SyncStateRevoked {
		t.Fatalf("d2's sync state = %s, want revoked", st.State)
	}
	if err := w.Write(d2, "c.md", c+"\n"); err != nil {
		t.Fatal(err)
	}
	w.Offline(d2)
	if err := w.Write(d0, "a.md", a+"\n"); err != nil {
		t.Fatal(err)
	}
	if err := w.SettleOnline(); err != nil {
		t.Fatal(err)
	}
	heads := w.Heads()
	if got := heads["a.md"]; got != a+"\n" {
		t.Fatalf("hub's a.md = %q, want %q", got, a+"\n")
	}
	if _, ok := heads["c.md"]; ok {
		t.Fatal("revoked d2's c.md reached the hub")
	}
	w.Online(d2)
	if err := w.PairAgain(d2); err != nil {
		t.Fatal(err)
	}
	if err := w.Settle(); err != nil {
		t.Fatal(err)
	}
	if err := w.Check(); err != nil {
		t.Fatal(err)
	}
	if got := w.Heads()["c.md"]; got != c+"\n" {
		t.Fatalf("hub's c.md after pairing again = %q, want %q", got, c+"\n")
	}
	if got := Files(d2)["a.md"]; got != a+"\n" {
		t.Fatalf("d2's a.md after pairing again = %q, want %q", got, a+"\n")
	}
}

func TestScenarioFaultsProfile(t *testing.T) {
	profile := Profile{
		Weights: map[Op]int{Edit: 6, Create: 3, HubRestart: 2, HubCrash: 1, DropAnswer: 2,
			RevokeAndPair: 2, Crash: 2, Offline: 2, Online: 3},
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
