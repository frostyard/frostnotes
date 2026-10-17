package synctest

import (
	"fmt"
	"regexp"
	"strings"
	"testing"
)

const scenarioDailyPath = "daily/2026-10-17.md"

func TestScenarioDailyNoteOneAtATime(t *testing.T) {
	w := NewWorld(t, Options{Seed: 1})
	defer w.Close()
	want := writeScenarioDailyNotes(t, w)
	for _, d := range w.Devices {
		w.Online(d)
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
	checkScenarioDailyNote(t, w, want)
}

func TestScenarioDailyNoteAllAtOnce(t *testing.T) {
	w := NewWorld(t, Options{Seed: 2})
	defer w.Close()
	want := writeScenarioDailyNotes(t, w)
	for _, d := range w.Devices {
		w.Online(d)
	}
	if err := w.Settle(); err != nil {
		t.Fatal(err)
	}
	if err := w.Check(); err != nil {
		t.Fatal(err)
	}
	checkScenarioDailyNote(t, w, want)
}

func writeScenarioDailyNotes(t *testing.T, w *World) string {
	t.Helper()
	for _, d := range w.Devices {
		w.Offline(d)
	}
	a, b, c := w.Token(), w.Token(), w.Token()
	contents := []string{
		"# 2026-10-17\n## Tasks\n- [ ] " + a + "\n## Notes\n",
		"# 2026-10-17\n## Tasks\n## Notes\n" + b + "\n",
		"# 2026-10-17\n" + c + "\n## Tasks\n## Notes\n",
	}
	for i, content := range contents {
		if err := w.Write(w.Devices[i], scenarioDailyPath, content); err != nil {
			t.Fatal(err)
		}
	}
	return "# 2026-10-17\n" + c + "\n## Tasks\n- [ ] " + a + "\n## Notes\n" + b + "\n"
}

func checkScenarioDailyNote(t *testing.T, w *World, want string) {
	t.Helper()
	heads := w.Heads()
	if got := heads[scenarioDailyPath]; got != want {
		t.Fatalf("hub's head at %s = %q, want %q", scenarioDailyPath, got, want)
	}
	for p := range heads {
		if strings.HasPrefix(p, "daily/") && p != scenarioDailyPath {
			t.Fatalf("hub has an extra daily file at %s", p)
		}
	}
	f, ok := w.HubFile(scenarioDailyPath)
	if !ok {
		t.Fatalf("hub has no live file at %s", scenarioDailyPath)
	}
	if f.Conflicted {
		t.Fatalf("hub's daily note at %s is conflicted", scenarioDailyPath)
	}
}

func TestScenarioDailyNotesProfile(t *testing.T) {
	numberedDaily := regexp.MustCompile(`^daily/.* \d+\.md$`)
	profile := Profile{
		Weights: map[Op]int{Daily: 10, Offline: 4, Online: 5, Edit: 2, Append: 2},
		Check: func(w *World) error {
			for p := range w.Heads() {
				if numberedDaily.MatchString(p) {
					return fmt.Errorf("hub has a numbered daily note at %s", p)
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
