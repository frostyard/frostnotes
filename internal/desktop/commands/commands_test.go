package commands

import (
	"os"
	"regexp"
	"slices"
	"strings"
	"testing"

	"github.com/egoist/mygo/ui"
)

var keyNames = map[string]ui.Key{
	"Tab": ui.KeyTab, "Page Down": ui.KeyPageDown, "Page Up": ui.KeyPageUp, "←": ui.KeyLeft, "→": ui.KeyRight,
	",": ui.KeyComma, ";": ui.KeySemicolon, "/": ui.KeySlash, "=": ui.KeyEqual, "-": ui.KeyMinus,
	"Enter": ui.KeyEnter, "F1": ui.KeyF1, "F2": ui.KeyF2, "0": ui.Key0, "9": ui.Key9,
}

// parse reads a key of the spec's table, as "Ctrl + Shift + N".
func parse(t *testing.T, s string) (ui.Modifiers, ui.Key) {
	t.Helper()
	var mods ui.Modifiers
	parts := strings.Split(strings.TrimSpace(s), " + ")
	for _, p := range parts[:len(parts)-1] {
		switch p {
		case "Ctrl":
			mods |= ui.Cmd
		case "Alt":
			mods |= ui.Alt
		case "Shift":
			mods |= ui.Shift
		default:
			t.Fatalf("modifier %q in %q", p, s)
		}
	}
	last := parts[len(parts)-1]
	if k, ok := keyNames[last]; ok {
		return mods, k
	}
	if len(last) == 1 && last[0] >= 'A' && last[0] <= 'Z' {
		return mods, ui.KeyA + ui.Key(last[0]-'A')
	}
	t.Fatalf("key %q in %q", last, s)
	return 0, 0
}

var rowRe = regexp.MustCompile("^\\| (.+?) \\| `([a-z-]+:[a-z0-9-]+)`")

// The table holds every row of the spec's keyboard map, with its keys.
func TestTheTableMatchesTheKeyboardMap(t *testing.T) {
	data, err := os.ReadFile("../../../docs/specs/ui.md")
	if err != nil {
		t.Fatal(err)
	}
	byID := map[string]Spec{}
	for _, s := range Table() {
		if _, dup := byID[s.ID]; dup {
			t.Errorf("%s twice", s.ID)
		}
		byID[s.ID] = s
	}
	rows := 0
	for line := range strings.SplitSeq(string(data), "\n") {
		m := rowRe.FindStringSubmatch(line)
		if m == nil {
			continue
		}
		rows++
		if strings.Contains(m[1], "…") { // Ctrl + 1 … Ctrl + 8
			for _, n := range "12345678" {
				if s, ok := byID["workspace:goto-tab-"+string(n)]; !ok || len(s.Keys) != 1 {
					t.Errorf("goto tab %c", n)
				}
			}
			continue
		}
		s, ok := byID[m[2]]
		if !ok {
			t.Errorf("%s is not in the table", m[2])
			continue
		}
		for k := range strings.SplitSeq(m[1], ", ") {
			mods, key := parse(t, k)
			if !slices.ContainsFunc(s.Keys, func(x Key) bool { return x.Mods == mods && x.Key == key }) {
				t.Errorf("%s lacks %s: %+v", m[2], k, s.Keys)
			}
		}
	}
	if rows < 35 {
		t.Fatalf("read only %d rows of the keyboard map", rows)
	}
}

// The rules Frostnotes and Frostmail share: Ctrl K steps aside in an
// editor, and Ctrl+Shift+P works everywhere.
func TestThePalettesKeys(t *testing.T) {
	for _, s := range Table() {
		if s.ID != "command-palette:open" {
			continue
		}
		found := 0
		for _, k := range s.Keys {
			if k.Key == ui.KeyK && k.Scope == OutsideEditors {
				found++
			}
			if k.Key == ui.KeyP && k.Mods == ui.Cmd|ui.Shift && k.Scope == Everywhere {
				found++
			}
		}
		if found != 2 {
			t.Errorf("%+v", s.Keys)
		}
	}
}
