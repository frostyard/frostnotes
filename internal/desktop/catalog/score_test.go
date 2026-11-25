package catalog

import "testing"

// scoreCases are queries and texts with the scores frostui.Match (frostui
// v0.10.1) gives them: its own cases, and the paths and aliases the quick
// switcher and [[ rank. While the MyGo client has frostui, a test of
// cmd/frostnotes also compares Score with frostui.Match on many more.
var scoreCases = []struct {
	query, text string
	score       int
	ok          bool
}{
	{"ab", "a b", 15, true},
	{"ab", "ab", 13, true},
	{"ab", "x ab", 10, true},
	{"ab", "axb", 10, true},
	{"ab", "xab", 5, true},
	{"ab", "zz", 0, false},
	{"upev", "Update everything", 14, true},
	{"xyz", "Update everything", 0, false},
	{"", "anything", 0, true},
	{"  ", "anything", 0, true},
	{"upd", "Update everything", 19, true},
	{"upd", "Stage the system update", 16, true},
	{"plan", "Projects/Plan", 21, true},
	{"plan", "Plan", 30, true},
	{"pp", "Projects/Plan", 15, true},
	{"note 1", "garden/Note 10", 41, true},
	{"PLAN", "plan", 30, true},
	{" plan ", "Plan", 30, true},
	{"ćaf", "Ćafé notes", 20, true},
	{"2026", "Daily/2026-11-25", 26, true},
	{"gf", "garden/fruit/apple tree in the far orchard of the old farm", 12, true},
	{"tree", "garden/fruit/apple tree in the far orchard of the old farm", 10, true},
}

func TestScoreGivesFrostuisScores(t *testing.T) {
	for _, c := range scoreCases {
		if s, ok := Score(c.query, c.text); s != c.score || ok != c.ok {
			t.Errorf("Score(%q, %q) = %d, %v; want %d, %v", c.query, c.text, s, ok, c.score, c.ok)
		}
	}
}
