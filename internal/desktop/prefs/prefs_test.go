package prefs

import (
	"path/filepath"
	"slices"
	"testing"
)

func TestDefaultsAndDroppedValues(t *testing.T) {
	p := Parse([]byte(`{"defaultView":"read","editingMode":"nonsense","tabSize":99,"fontSize":"big","lineNumbers":true,"theme":"dark","unknown":1,"zoom":1.25}`))
	if p.DefaultView != "read" || p.EditingMode != "live" || p.TabSize != 4 || p.FontSize != 16 || !p.LineNumbers || p.Theme != "dark" || p.Zoom != 1.25 {
		t.Errorf("%+v", p)
	}
	if got := Parse([]byte("not json")); got != Default() {
		t.Errorf("%+v", got)
	}
	if got := Parse([]byte(`{"zoom":1.3}`)); got.Zoom != 1 {
		t.Errorf("a zoom off the steps: %v", got.Zoom)
	}
}

func TestSaveAndLoad(t *testing.T) {
	path := filepath.Join(t.TempDir(), "sub", "prefs.json")
	if got := Load(path); got != Default() {
		t.Errorf("missing file: %+v", got)
	}
	p := Default()
	p.LastVault, p.SmartLists = "/home/x/Vault", false
	if err := Save(path, p); err != nil {
		t.Fatal(err)
	}
	if got := Load(path); got != p {
		t.Errorf("%+v", got)
	}
}

func TestZoomStep(t *testing.T) {
	if ZoomStep(1, 1) != 1.1 || ZoomStep(1, -1) != 0.9 || ZoomStep(3, 1) != 3 || ZoomStep(0.5, -1) != 0.5 {
		t.Error("zoom steps")
	}
}

func TestState(t *testing.T) {
	path := filepath.Join(t.TempDir(), "state.json")
	var s State
	for i := range MaxRecentCommands + 3 {
		s.RanCommand(string(rune('a' + i)))
	}
	s.RanCommand("c")
	s.Seq = 42
	if s.RecentCommands[0] != "c" || len(s.RecentCommands) != MaxRecentCommands || slices.Index(s.RecentCommands[1:], "c") >= 0 {
		t.Errorf("%q", s.RecentCommands)
	}
	if err := SaveState(path, s); err != nil {
		t.Fatal(err)
	}
	if got := LoadState(path); got.Seq != 42 || !slices.Equal(got.RecentCommands, s.RecentCommands) {
		t.Errorf("%+v", got)
	}
}
