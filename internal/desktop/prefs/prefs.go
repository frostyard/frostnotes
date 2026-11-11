// Package prefs holds the desktop app's preferences and small state
// (ADR-0021; docs/specs/ui.md#settings-batch-2): one versioned JSON file per
// device, whose defaults are Obsidian's. A stored value that is not one a
// preference takes falls back to its default, so an older or newer file
// never breaks the window. It ports app/src/data/prefs.ts.
package prefs

import (
	"encoding/json/jsontext"
	"encoding/json/v2"
	"os"
	"reflect"
	"slices"
	"strings"

	"github.com/frostyard/frostnotes/internal/desktop/system"
)

// Prefs are the app's preferences.
type Prefs struct {
	DefaultView        string  `json:"defaultView"` // edit, read
	EditingMode        string  `json:"editingMode"` // live, source
	InlineTitle        bool    `json:"inlineTitle"`
	ViewHeader         bool    `json:"viewHeader"`
	ReadableLineLength bool    `json:"readableLineLength"`
	Spellcheck         bool    `json:"spellcheck"`
	Properties         string  `json:"properties"` // visible, hidden, source
	LineNumbers        bool    `json:"lineNumbers"`
	IndentGuides       bool    `json:"indentGuides"`
	FoldHeadings       bool    `json:"foldHeadings"`
	FoldIndent         bool    `json:"foldIndent"`
	AutoPairBrackets   bool    `json:"autoPairBrackets"`
	AutoPairMarkdown   bool    `json:"autoPairMarkdown"`
	SmartLists         bool    `json:"smartLists"`
	IndentWithTabs     bool    `json:"indentWithTabs"`
	TabSize            int     `json:"tabSize"`
	ConfirmDelete      bool    `json:"confirmDelete"`
	LoadRemoteImages   bool    `json:"loadRemoteImages"`
	ShowUnsupported    bool    `json:"showUnsupported"`
	Theme              string  `json:"theme"` // system, light, dark
	FontSize           int     `json:"fontSize"`
	Zoom               float64 `json:"zoom"`
	LastVault          string  `json:"lastVault"`
}

// Default are Obsidian's defaults.
func Default() Prefs {
	return Prefs{
		DefaultView: "edit", EditingMode: "live", InlineTitle: true, ViewHeader: true,
		ReadableLineLength: true, Spellcheck: true, Properties: "visible", IndentGuides: true,
		FoldHeadings: true, FoldIndent: true, AutoPairBrackets: true, AutoPairMarkdown: true,
		SmartLists: true, IndentWithTabs: true, TabSize: 4, ConfirmDelete: true,
		LoadRemoteImages: true, Theme: "system", FontSize: 16, Zoom: 1,
	}
}

// Zooms are the window's zoom steps, as a browser's.
var zooms = []float64{0.5, 0.67, 0.75, 0.8, 0.9, 1, 1.1, 1.25, 1.5, 1.75, 2, 2.5, 3}

// ZoomStep is the next zoom step from zoom in a direction: +1 in, -1 out;
// it stays at either end.
func ZoomStep(zoom float64, direction int) float64 {
	if direction > 0 {
		for _, z := range zooms {
			if z > zoom+1e-9 {
				return z
			}
		}
		return zoom
	}
	for _, z := range slices.Backward(zooms) {
		if z < zoom-1e-9 {
			return z
		}
	}
	return zoom
}

// clean puts each value a preference does not take back to its default.
func (p *Prefs) clean() {
	d := Default()
	oneOf := func(v *string, def string, values ...string) {
		if !slices.Contains(values, *v) {
			*v = def
		}
	}
	oneOf(&p.DefaultView, d.DefaultView, "edit", "read")
	oneOf(&p.EditingMode, d.EditingMode, "live", "source")
	oneOf(&p.Properties, d.Properties, "visible", "hidden", "source")
	oneOf(&p.Theme, d.Theme, "system", "light", "dark")
	if p.TabSize < 1 || p.TabSize > 8 {
		p.TabSize = d.TabSize
	}
	if p.FontSize < 10 || p.FontSize > 30 {
		p.FontSize = d.FontSize
	}
	if !slices.ContainsFunc(zooms, func(z float64) bool { return z-p.Zoom < 1e-9 && p.Zoom-z < 1e-9 }) {
		p.Zoom = d.Zoom
	}
}

// decode reads a record field by field: a value of the wrong type keeps
// its field's default, and unknown keys are dropped.
func decode(data []byte, into any) bool {
	var raw map[string]jsontext.Value
	if json.Unmarshal(data, &raw) != nil {
		return false
	}
	v := reflect.ValueOf(into).Elem()
	for i := range v.NumField() {
		name, _, _ := strings.Cut(v.Type().Field(i).Tag.Get("json"), ",")
		if value, ok := raw[name]; ok && value.Kind() != 'n' { // null keeps the default
			_ = json.Unmarshal(value, v.Field(i).Addr().Interface())
		}
	}
	return true
}

// Parse reads a stored record.
func Parse(data []byte) Prefs {
	p := Default()
	if !decode(data, &p) {
		return Default()
	}
	p.clean()
	return p
}

// Load reads the preferences at path; a missing or broken file is the
// defaults.
func Load(path string) Prefs {
	data, err := os.ReadFile(path)
	if err != nil {
		return Default()
	}
	return Parse(data)
}

// Save writes the preferences to path.
func Save(path string, p Prefs) error {
	data, err := json.Marshal(p, jsontext.WithIndent("  "))
	if err != nil {
		return err
	}
	return system.WriteFile(path, data)
}

// State is what the app keeps between runs that is no preference: the
// event resume point and the commands run lately.
type State struct {
	Pending        []WorkspaceEvent `json:"pending,omitzero"`
	PendingSeq     int64            `json:"pendingSeq,omitzero"`
	Seq            int64            `json:"seq"`
	RecentCommands []string         `json:"recentCommands"`
}

// MaxRecentCommands is how many commands the palette remembers.
const MaxRecentCommands = 10

// LoadState reads the state at path.
func LoadState(path string) State {
	var s State
	if data, err := os.ReadFile(path); err == nil {
		decode(data, &s)
	}
	return s
}

// SaveState writes the state to path.
func SaveState(path string, s State) error {
	data, err := json.Marshal(s)
	if err != nil {
		return err
	}
	return system.WriteFile(path, data)
}

// RanCommand puts id first among the recent commands.
func (s *State) RanCommand(id string) {
	recent := []string{id}
	for _, r := range s.RecentCommands {
		if r != id && len(recent) < MaxRecentCommands {
			recent = append(recent, r)
		}
	}
	s.RecentCommands = recent
}

// WorkspaceEvent is a move or removal held until the vault list is available.
// To is empty for a removal; paths remain relative to VaultID.
type WorkspaceEvent struct {
	VaultID int64  `json:"vaultId"`
	From    string `json:"from"`
	To      string `json:"to,omitzero"`
}
